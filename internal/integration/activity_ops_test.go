//go:build integration

// The v2 operation surface against demoapi: Dispatch / Done / BeginRead /
// ApplyRead.
//
// activity_claim_test.go shows the v1 rule failing without the guard a screen
// had to write. These are the same orderings with no guard anywhere in the
// screen — the table owns it now — plus the two shapes v1 could not express:
// a blocking refresh, and work that finishes between two polls.
package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/demoapi"
	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

type opsApp struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Sync  string `json:"sync"`
	Phase string `json:"phase"`
}

// opsScreen is claimScreen rewritten on v2. It keeps no counters: every piece
// of ordering it used to own is a call on the table.
type opsScreen struct {
	api   demoapi.Target
	table table.Model

	applied    int
	unobserved []activity.UnobservedMsg
}

type opsFetched struct {
	read activity.Read
	apps []opsApp
	err  error
}

type opsAnswered struct {
	op  activity.Op
	err error
}

func newOpsScreen(t *testing.T, settle int) *opsScreen {
	t.Helper()
	o := theme.Dark().Table()
	o.Columns = []table.Column{{Title: "Name", Width: 20}, {Title: "Sync", Width: 14}}
	o.ActivityColumn = "Sync"
	o.BusyWhen = func(r table.KeyedRow) (string, bool) {
		p := r.Data.(opsApp).Phase
		return p, p == demoapi.PhaseRunning
	}
	o.Activity.Settle = settle
	return &opsScreen{
		api:   demoapi.From(demoapi.Options{Seed: 3, Apps: 4, Schedule: time.Hour}),
		table: table.New(o),
	}
}

func (s *opsScreen) Title() string         { return "ops" }
func (s *opsScreen) IsCapturingKeys() bool { return false }
func (s *opsScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *opsScreen) SetTheme(theme.Theme)  {}
func (s *opsScreen) Layout() layout.Node   { return layout.Sized(&s.table) }
func (s *opsScreen) Init() tea.Cmd         { return s.fetch("") }
func (s *opsScreen) Help() []key.Binding   { return nil }

func (s *opsScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case opsFetched:
		rows := make([]table.KeyedRow, len(m.apps))
		for i, a := range m.apps {
			rows[i] = table.KeyedRow{Key: a.ID, Cells: []string{a.Name, a.Sync}, Data: a}
		}
		if s.table.ApplyRead(m.read, rows, m.err) {
			s.applied++
		}
	case opsAnswered:
		s.table.Done(m.op, m.err)
	case activity.UnobservedMsg:
		s.unobserved = append(s.unobserved, m)
	}
	var cmd tea.Cmd
	s.table, cmd = s.table.Update(msg)
	return s, cmd
}

// sync is the Observed shape: claim, POST, answer.
func (s *opsScreen) sync(keys []string, query string) tea.Cmd {
	op, claim := s.table.Dispatch(keys, "Syncing", activity.Observed)
	api := s.api
	return tea.Batch(claim, func() tea.Msg {
		var err error
		for _, k := range keys {
			resp, e := api.Client.Post(api.URL("/apps/"+k+"/sync"+query), "application/json", nil)
			if e != nil {
				err = e
				continue
			}
			resp.Body.Close()
		}
		return opsAnswered{op: op, err: err}
	})
}

// refresh is the Held shape: the GET does not answer until it is done.
func (s *opsScreen) refresh(key, takes string) tea.Cmd {
	op, claim := s.table.Dispatch([]string{key}, "Refreshing", activity.Held)
	api := s.api
	return tea.Batch(claim, func() tea.Msg {
		resp, err := api.Client.Get(api.URL("/apps/" + key + "?refresh=normal&takes=" + takes))
		if err == nil {
			resp.Body.Close()
		}
		return opsAnswered{op: op, err: err}
	})
}

func (s *opsScreen) fetch(query string) tea.Cmd {
	api, rd := s.api, s.table.BeginRead()
	return func() tea.Msg {
		resp, err := api.Client.Get(api.URL("/apps?limit=10" + query))
		if err != nil {
			return opsFetched{read: rd, err: err}
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return opsFetched{read: rd, err: errOutage}
		}
		var body struct {
			Rows []opsApp `json:"rows"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return opsFetched{read: rd, err: err}
		}
		return opsFetched{read: rd, apps: body.Rows}
	}
}

func (s *opsScreen) working(key string) bool {
	_, ok := s.table.ActivityState().State(key)
	return ok
}

func startOps(t *testing.T, settle int) (*opsScreen, *harness, string) {
	t.Helper()
	s := newOpsScreen(t, settle)
	h := newHarness(t, s)
	if !h.pumpUntil(2*time.Second, func() bool { return s.applied > 0 }) {
		t.Fatal("the first page never arrived")
	}
	k, ok := s.table.SelectedKey()
	if !ok {
		t.Fatal("no rows")
	}
	return s, h, k
}

// v1's F1 case, with nothing in the screen: a poll issued inside the write's
// latency carries the pre-write phase and must not end the claim.
func TestV2AReadTakenDuringTheWriteDoesNotClearTheClaim(t *testing.T) {
	s, h, k := startOps(t, 0)

	h.exec(s.sync([]string{k}, "?latency=400ms"))
	time.Sleep(50 * time.Millisecond)
	h.exec(s.fetch(""))
	h.pumpFor(250 * time.Millisecond)

	if !s.working(k) {
		t.Error("a read taken before the write answered cleared the claim")
	}
	if s.applied < 2 {
		t.Error("the read was dropped; v2 applies it and only withholds its say over the claim")
	}
}

// A read already in flight at the keypress, delayed past it.
func TestV2AReadInFlightAtTheKeypressDoesNotClearTheClaim(t *testing.T) {
	s, h, k := startOps(t, 0)

	h.exec(s.fetch("&latency=400ms"))
	time.Sleep(30 * time.Millisecond)
	h.exec(s.sync([]string{k}, "?latency=500ms"))
	h.pumpFor(450 * time.Millisecond)

	if !s.working(k) {
		t.Error("a reply that predates the keypress cleared the claim")
	}
}

// An outage withdraws what was waiting on a read, with no RetractAll in the
// screen.
func TestV2AFailedReadWithdrawsTheClaim(t *testing.T) {
	s, h, k := startOps(t, 0)

	h.exec(s.sync([]string{k}, "?blackhole=1"))
	h.pumpFor(150 * time.Millisecond)
	if !s.working(k) {
		t.Fatal("the claim never went on the row")
	}
	resp, err := s.api.Client.Post(s.api.URL("/chaos/down?for=1h"), "application/json", nil)
	if err != nil {
		t.Fatalf("injecting the outage: %v", err)
	}
	resp.Body.Close()

	h.exec(s.fetch(""))
	h.pumpFor(200 * time.Millisecond)
	if s.working(k) {
		t.Error("the claim survived the reads stopping")
	}
}

// Argo's refresh: the GET holds, the app shows nothing, and polls during it
// read exactly what they read before. The row must spin for the whole hold
// and stop when the request answers — neither of which v1 could say.
func TestV2ARefreshSpinsForExactlyAsLongAsTheRequest(t *testing.T) {
	s, h, k := startOps(t, 0)

	h.exec(s.refresh(k, "600ms"))
	for i := 0; i < 3; i++ {
		time.Sleep(120 * time.Millisecond)
		h.exec(s.fetch(""))
		h.pumpFor(30 * time.Millisecond)
		if !s.working(k) {
			t.Fatalf("poll %d during the refresh ended the claim", i)
		}
	}
	if !h.pumpUntil(time.Second, func() bool { return !s.working(k) }) {
		t.Error("the row kept spinning after the refresh answered")
	}
	if len(s.unobserved) != 0 {
		t.Errorf("a Held operation was reported unobserved: %+v", s.unobserved)
	}
}

// The case the developer hit: a sync that finishes between polls. The row
// cannot show it, so the screen is told instead of left guessing.
func TestV2ASyncBetweenPollsIsReported(t *testing.T) {
	s, h, k := startOps(t, 1)

	h.exec(s.sync([]string{k}, "?takes=100ms"))
	h.pumpFor(300 * time.Millisecond) // answered, and the work is already over
	for i := 0; i < 2; i++ {
		h.exec(s.fetch(""))
		h.pumpFor(100 * time.Millisecond)
	}

	if s.working(k) {
		t.Error("the claim outlived its allowance")
	}
	if len(s.unobserved) != 1 || s.unobserved[0].Keys[0] != k {
		t.Fatalf("unobserved = %+v, want one report for %s", s.unobserved, k)
	}
}

// And through the example and the shell: Refresh names itself on the row for
// the length of the request, and Quick sync ends in a statusbar line rather
// than silence.
func TestTheExampleShowsARefreshAndReportsAQuickSync(t *testing.T) {
	h, srv := newActionApp(t)
	defer srv.Close()

	if !h.pumpUntil(5*time.Second, func() bool {
		return strings.Contains(h.render(), demoapi.SyncSynced) ||
			strings.Contains(h.render(), demoapi.SyncOutOfSync)
	}) {
		t.Fatalf("no rows arrived:\n%s", h.render())
	}

	h.pick("Refresh")
	if !h.pumpUntil(time.Second, func() bool { return strings.Contains(h.render(), "Refreshing") }) {
		t.Fatalf("the refresh never showed:\n%s", h.render())
	}
	h.pumpFor(1500 * time.Millisecond) // most of demoapi's 2s hold, across a poll
	if !strings.Contains(h.render(), "Refreshing") {
		t.Errorf("the row stopped saying Refreshing while the request was still held:\n%s", h.render())
	}
	if !h.pumpUntil(3*time.Second, func() bool { return !strings.Contains(h.render(), "Refreshing") }) {
		t.Errorf("the row kept saying Refreshing after the request answered:\n%s", h.render())
	}

	h.pick("Quick sync")
	if !h.pumpUntil(8*time.Second, func() bool {
		v := h.render()
		return strings.Contains(v, "between polls") || strings.Contains(v, "no change observed")
	}) {
		t.Errorf("a sync nobody saw ended in silence:\n%s", h.render())
	}
}

var errOutage = errServiceUnavailable{}

type errServiceUnavailable struct{}

func (errServiceUnavailable) Error() string { return "503 injected outage" }

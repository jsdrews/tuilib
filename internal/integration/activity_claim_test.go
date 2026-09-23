// The rule with no compiler behind it.
//
// A screen that calls Expect has to drop reads taken across its own write, and
// nothing in the library can make it: pkg/activity has no idea a fetch exists,
// and SetKeyedRows says when rows *arrived*, never when they were *requested*
// — which is precisely the gap in question.
//
// So the test is the enforcement, and it is written as a pair: the same screen
// with the guard and without it, against the same server, so the failure the
// rule prevents is visible rather than described. The asymmetry matters too —
// a screen bumping only at the keypress passes the easy version of this and
// fails the one with a slow write, which is why both are here.
package integration

import (
	"encoding/json"
	"net/http"
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

// guard is how much of decision 15 a claimScreen implements.
type guard int

const (
	guardNone   guard = iota // no protection at all
	guardChosen              // bump at the keypress only — the first draft
	guardFull                // and drop reads taken while the write is outstanding
)

// claimScreen is the smallest screen that can hold a claim: a keyed table, a
// fetch, a dispatch, and nothing else. The example does all of this too, with
// an action menu and a console on top; this strips those away so a failure
// here can only be about ordering.
type claimScreen struct {
	api   demoapi.Target
	table table.Model
	mode  guard

	gen, seen, writing int
	applied            int
}

type claimFetched struct {
	gen  int
	apps []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Sync string `json:"sync"`
	}
	err error
}

type claimWritten struct{ keys []string }

func newClaimScreen(t *testing.T, mode guard) *claimScreen {
	t.Helper()
	o := theme.Dark().Table()
	o.Columns = []table.Column{{Title: "Name", Width: 20}, {Title: "Sync", Width: 14}}
	o.ActivityColumn = "Sync"
	settled := activity.Settled(demoapi.SyncSynced, demoapi.SyncOutOfSync)
	o.ActivityWhen = func(c table.Row) (string, bool) { return settled(c[1]) }

	return &claimScreen{
		api:   demoapi.From(demoapi.Options{Seed: 3, Apps: 4, Schedule: time.Hour}),
		table: table.New(o),
		mode:  mode,
	}
}

func (s *claimScreen) Title() string         { return "claims" }
func (s *claimScreen) IsCapturingKeys() bool { return false }
func (s *claimScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *claimScreen) SetTheme(theme.Theme)  {}
func (s *claimScreen) Layout() layout.Node   { return layout.Sized(&s.table) }
func (s *claimScreen) Init() tea.Cmd         { return s.fetch() }
func (s *claimScreen) Help() []key.Binding   { return nil }

func (s *claimScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmds []tea.Cmd

	switch m := msg.(type) {
	case claimFetched:
		stale := m.gen < s.seen
		if s.mode == guardFull && s.writing > 0 {
			stale = true
		}
		if stale {
			break
		}
		s.seen = m.gen
		s.applied++
		if m.err != nil {
			if s.mode == guardFull {
				// Required, not polite: nothing else is coming to retire them.
				s.table.RetractAll()
			}
			break
		}
		rows := make([]table.KeyedRow, len(m.apps))
		for i, a := range m.apps {
			rows[i] = table.KeyedRow{Key: a.ID, Cells: []string{a.Name, a.Sync}}
		}
		s.table.SetKeyedRows(rows)

	case claimWritten:
		s.writing--
		if s.mode == guardFull {
			s.gen++
		}
	}

	var cmd tea.Cmd
	s.table, cmd = s.table.Update(msg)
	cmds = append(cmds, cmd)
	return s, tea.Batch(cmds...)
}

// dispatch is the keypress: claim the rows, then POST.
func (s *claimScreen) dispatch(keys []string, query string) tea.Cmd {
	if s.mode != guardNone {
		s.gen++
	}
	s.writing++
	claim := s.table.Expect(keys, demoapi.SyncSyncing)

	api := s.api
	post := func() tea.Msg {
		for _, k := range keys {
			resp, err := api.Client.Post(api.URL("/apps/"+k+"/sync"+query), "application/json", nil)
			if err == nil {
				resp.Body.Close()
			}
		}
		return claimWritten{keys: keys}
	}
	return tea.Batch(claim, post)
}

func (s *claimScreen) fetch() tea.Cmd {
	s.gen++
	api, gen := s.api, s.gen
	return func() tea.Msg {
		resp, err := api.Client.Get(api.URL("/apps?limit=10"))
		if err != nil {
			return claimFetched{gen: gen, err: err}
		}
		defer resp.Body.Close()
		var body claimFetched
		if err := json.NewDecoder(resp.Body).Decode(&struct {
			Rows *[]struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Sync string `json:"sync"`
			} `json:"rows"`
		}{Rows: &body.apps}); err != nil {
			return claimFetched{gen: gen, err: err}
		}
		body.gen = gen
		return body
	}
}

func (s *claimScreen) working(key string) bool {
	_, ok := s.table.ActivityState().State(key)
	return ok
}

func (s *claimScreen) firstKey(t *testing.T) string {
	t.Helper()
	k, ok := s.table.SelectedKey()
	if !ok || k == "" {
		t.Fatal("no rows arrived, so there is nothing to dispatch against")
	}
	return k
}

// A write is never instantaneous, and a read issued inside its latency carries
// a newer generation and an older truth. Only the outstanding-write check
// catches that one — which is the whole of F1.
func TestAReadTakenDuringTheWriteDoesNotClearTheClaim(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode guard
		kept bool
	}{
		{"no guard at all", guardNone, false},
		{"bumping at the keypress only", guardChosen, false},
		{"and dropping reads taken across the write", guardFull, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newClaimScreen(t, tc.mode)
			h := newHarness(t, s)
			if !h.pumpUntil(2*time.Second, func() bool { return s.applied > 0 }) {
				t.Fatal("the first page never arrived")
			}
			key := s.firstKey(t)

			// The write takes 400ms to be accepted. Everything the server
			// reports inside that window predates it.
			h.exec(s.dispatch([]string{key}, "?latency=400ms"))
			if !s.working(key) {
				t.Fatal("the claim never went on the row")
			}

			// A poll fires 50ms in — long after the keypress, well before the
			// server has heard about it.
			time.Sleep(50 * time.Millisecond)
			h.exec(s.fetch())
			h.pumpFor(250 * time.Millisecond)

			if got := s.working(key); got != tc.kept {
				if tc.kept {
					t.Error("the row stopped spinning: a pre-write read cleared the claim")
				} else {
					t.Error("the row kept spinning without the guard that makes it safe")
				}
			}
		})
	}
}

// The easy version, for contrast: a read already in flight at the keypress.
// The keypress-only bump is enough here, which is exactly why it looked
// sufficient.
func TestAReadInFlightAtTheKeypressDoesNotClearTheClaim(t *testing.T) {
	s := newClaimScreen(t, guardChosen)
	h := newHarness(t, s)
	if !h.pumpUntil(2*time.Second, func() bool { return s.applied > 0 }) {
		t.Fatal("the first page never arrived")
	}
	key := s.firstKey(t)

	// Issued first, and slow, so it is still in flight when the user acts.
	h.exec(s.fetchSlow("400ms"))
	time.Sleep(30 * time.Millisecond)
	h.exec(s.dispatch([]string{key}, ""))
	h.pumpFor(600 * time.Millisecond)

	if !s.working(key) {
		t.Error("a reply that predates the keypress was applied and cleared the claim")
	}
}

// fetchInto reads and reports the failure, which is what a screen polling into
// a 503 window actually sees.
func (s *claimScreen) fetchInto() tea.Cmd {
	s.gen++
	api, gen := s.api, s.gen
	return func() tea.Msg {
		r, err := api.Client.Get(api.URL("/apps?limit=10"))
		if err == nil {
			defer r.Body.Close()
			if r.StatusCode != http.StatusOK {
				err = errOutage
			}
		}
		return claimFetched{gen: gen, err: err}
	}
}

// fetchSlow is fetch with the server holding the reply.
func (s *claimScreen) fetchSlow(d string) tea.Cmd {
	s.gen++
	api, gen := s.api, s.gen
	return func() tea.Msg {
		resp, err := api.Client.Get(api.URL("/apps?limit=10&latency=" + d))
		if err != nil {
			return claimFetched{gen: gen, err: err}
		}
		defer resp.Body.Close()
		var body claimFetched
		if err := json.NewDecoder(resp.Body).Decode(&struct {
			Rows *[]struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Sync string `json:"sync"`
			} `json:"rows"`
		}{Rows: &body.apps}); err != nil {
			return claimFetched{gen: gen, err: err}
		}
		body.gen = gen
		return body
	}
}

// An outage is a healthy screen that has simply stopped being told anything.
// Nothing retires a claim then, so the screen has to.
func TestAFailedReadRetractsTheClaim(t *testing.T) {
	s := newClaimScreen(t, guardFull)
	h := newHarness(t, s)
	if !h.pumpUntil(2*time.Second, func() bool { return s.applied > 0 }) {
		t.Fatal("the first page never arrived")
	}
	key := s.firstKey(t)

	h.exec(s.dispatch([]string{key}, ""))
	h.pumpFor(150 * time.Millisecond)
	if !s.working(key) {
		t.Fatal("the claim never went on the row")
	}

	resp, err := s.api.Client.Post(s.api.URL("/chaos/down?for=1h"), "application/json", nil)
	if err != nil {
		t.Fatalf("injecting the outage: %v", err)
	}
	resp.Body.Close()

	// The read fails, so the screen takes its claims back rather than holding
	// them for the length of the outage.
	h.exec(s.fetchInto())
	h.pumpFor(200 * time.Millisecond)

	if s.working(key) {
		t.Error("the claim survived the reads stopping, so it spins for the length of the outage")
	}
}

var errOutage = errServiceUnavailable{}

type errServiceUnavailable struct{}

func (errServiceUnavailable) Error() string { return "503 injected outage" }

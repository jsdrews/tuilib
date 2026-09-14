package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/demoapi"
	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// activityScreen is examples/patterns/activity reduced to what these tests
// drive: a keyed table with ActivityWhen and ActivityRevision, fetching on
// demand rather than on a timer so the test decides when an observation
// happens — which is the variable every assertion below turns on.
type activityScreen struct {
	tab table.Model
	api *http.Client

	// nextLatency is applied to the next fetch only. One slow poll and one
	// fast click is the shape of the race in decision 18.
	nextLatency time.Duration

	// revs is the last revision seen per key, so a test can show that a
	// revision really did move — otherwise "no flash" is indistinguishable
	// from "nothing happened".
	revs map[string]string
}

const (
	acColSync = 1
	acColRev  = 3
)

func newActivityScreen(t *testing.T) *activityScreen {
	t.Helper()
	s := &activityScreen{
		api:  demoapi.Client(demoapi.New(demoapi.Options{Seed: 21, Apps: 5})),
		revs: map[string]string{},
	}
	o := theme.Dark().Table()
	o.Title = "apps"
	o.Columns = []table.Column{
		{Title: "Name", Width: 24},
		{Title: "Sync", Width: 14},
		{Title: "Health", Width: 13},
		{Title: "Rev", Hidden: true},
	}
	o.ActivityColumn = "Sync"
	o.ActivityWhen = func(c table.Row) (string, bool) {
		return activity.Busy(demoapi.SyncSyncing, demoapi.SyncRefresh)(c[acColSync])
	}
	o.ActivityRevision = func(c table.Row) string { return c[acColRev] }
	s.tab = table.New(o)
	return s
}

func (s *activityScreen) Title() string          { return "activity" }
func (s *activityScreen) Init() tea.Cmd          { return s.fetch() }
func (s *activityScreen) OnEnter(any) tea.Cmd    { return nil }
func (s *activityScreen) Layout() layout.Node    { return layout.Sized(&s.tab) }
func (s *activityScreen) IsCapturingKeys() bool  { return s.tab.IsCapturingKeys() }
func (s *activityScreen) Help() []key.Binding    { return s.tab.Help() }
func (s *activityScreen) SetTheme(t theme.Theme) {}

type rowsMsg struct{ rows []table.KeyedRow }

func (s *activityScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if m, ok := msg.(rowsMsg); ok {
		for _, r := range m.rows {
			if acColRev < len(r.Cells) {
				s.revs[r.Key] = r.Cells[acColRev]
			}
		}
		s.tab.SetKeyedRows(m.rows)
	}
	var cmd tea.Cmd
	s.tab, cmd = s.tab.Update(msg)
	return s, cmd
}

// fetch is the poll, made explicit so a test can start one and let it be
// overtaken.
func (s *activityScreen) fetch() tea.Cmd {
	client := s.api
	url := "http://demoapi/apps?limit=5"
	if s.nextLatency > 0 {
		url += "&latency=" + s.nextLatency.String()
		s.nextLatency = 0
	}
	return func() tea.Msg {
		resp, err := client.Get(url)
		if err != nil {
			return rowsMsg{}
		}
		defer resp.Body.Close()
		var body struct {
			Rows []struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Sync   string `json:"sync"`
				Health string `json:"health"`
				Rev    int64  `json:"rev"`
			} `json:"rows"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		rows := make([]table.KeyedRow, len(body.Rows))
		for i, a := range body.Rows {
			rows[i] = table.KeyedRow{
				Key:   a.ID,
				Cells: []string{a.Name, a.Sync, a.Health, strconv.FormatInt(a.Rev, 10)},
			}
		}
		return rowsMsg{rows: rows}
	}
}

// launch POSTs, the way an action.Func would.
func (s *activityScreen) launch(t *testing.T, id, kind string) {
	t.Helper()
	resp, err := s.api.Post("http://demoapi/apps/"+id+"/"+kind, "", nil)
	if err != nil {
		t.Fatalf("POST %s: %v", kind, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST %s: status %d", kind, resp.StatusCode)
	}
}

func (s *activityScreen) firstKey(t *testing.T) string {
	t.Helper()
	k, ok := s.tab.SelectedKey()
	if !ok || k == "" {
		t.Fatal("no rows to act on")
	}
	return k
}

// Decision 18's ordering, produced rather than simulated. A poll is already in
// flight when the user clicks, so its reply carries the pre-click value — and
// a derived "nothing is busy" must not retire the local indicator the click
// created.
//
// The failure this guards is nasty in the field and invisible in a unit test:
// the spinner blinks off and on once, at a moment set by the poll phase.
func TestStalePollDoesNotWipeALocalIndicator(t *testing.T) {
	s := newActivityScreen(t)
	h := newHarness(t, s)
	h.pumpFor(150 * time.Millisecond)

	key := s.firstKey(t)

	// A poll leaves, and will take most of a second to come back.
	s.nextLatency = 600 * time.Millisecond
	h.exec(s.fetch())
	time.Sleep(30 * time.Millisecond)

	// The user clicks. This is what the shell's broadcast does to the row.
	h.send(activity.StartMsg{Keys: []string{key}, Label: "syncing", RunID: 1})
	if !strings.Contains(h.render(), "syncing") {
		t.Fatal("the click did not start an indicator")
	}

	// The stale reply lands, reporting the row as resting.
	h.pumpFor(900 * time.Millisecond)

	if st, ok := s.tab.ActivityState().State(key); !ok || st.Done {
		t.Fatalf("state = %+v, %v; a stale observation retired the local entry", st, ok)
	}
	if !strings.Contains(h.render(), "syncing") {
		t.Errorf("the indicator was wiped by a pre-click page:\n%s", h.render())
	}
}

// Decision 19, end to end. Refresh is instant server-side, so the work is over
// long before the next observation; the row must keep moving until that
// observation, then clear.
//
// Clearing on the action's return instead would show a tick and then a cell
// reading exactly as it did before the user acted.
func TestLocalIndicatorWaitsForTheNextObservation(t *testing.T) {
	s := newActivityScreen(t)
	h := newHarness(t, s)
	h.pumpFor(150 * time.Millisecond)

	key := s.firstKey(t)

	// The shell's broadcast, the POST, and the run finishing — all of which
	// happen well inside one poll interval for a fire-and-forget verb.
	h.send(activity.StartMsg{Keys: []string{key}, Label: "refreshing", RunID: 7})
	s.launch(t, key, "refresh")
	h.send(activity.EndMsg{RunID: 7})

	st, ok := s.tab.ActivityState().State(key)
	if !ok {
		t.Fatal("the entry vanished when the action returned")
	}
	if st.Done {
		t.Fatalf("state = %+v; the row stopped moving before anything was observed", st)
	}
	if !strings.Contains(h.render(), "refreshing") {
		t.Errorf("the row went quiet between the action and the poll:\n%s", h.render())
	}

	// The poll lands. The server finished the refresh before it was asked, so
	// this observation reports the row resting — which is what ends the wait.
	h.exec(s.fetch())
	h.pumpFor(300 * time.Millisecond)

	if _, ok := s.tab.ActivityState().State(key); ok {
		t.Error("the observation did not end the handoff")
	}
	if strings.Contains(h.render(), "refreshing") {
		t.Errorf("the indicator outlived its confirmation:\n%s", h.render())
	}
}

// The other half of decision 19: when the server is still working, the
// observation hands over to the derived entry rather than clearing, so the row
// never stops moving.
func TestLocalIndicatorHandsOverToTheServersOwnStatus(t *testing.T) {
	s := newActivityScreen(t)
	h := newHarness(t, s)
	h.pumpFor(150 * time.Millisecond)

	key := s.firstKey(t)

	h.send(activity.StartMsg{Keys: []string{key}, Label: "syncing", RunID: 9})
	s.launch(t, key, "sync") // four seconds server-side
	h.send(activity.EndMsg{RunID: 9})

	h.exec(s.fetch())
	h.pumpFor(300 * time.Millisecond)

	st, ok := s.tab.ActivityState().State(key)
	if !ok {
		t.Fatal("the row went idle while the server was still working")
	}
	if st.Done {
		t.Fatalf("state = %+v, want the derived entry running", st)
	}
	// The server's own word, not the action's lowercase Busy text.
	if st.Label != demoapi.SyncSyncing {
		t.Errorf("label = %q, want the derived %q", st.Label, demoapi.SyncSyncing)
	}
}

// Decision 20: work that begins and ends between two observations leaves no
// trace but a revision, and must flash rather than pass unnoticed.
func TestUnobservedChangeFlashes(t *testing.T) {
	s := newActivityScreen(t)
	h := newHarness(t, s)
	h.pumpFor(150 * time.Millisecond)

	key := s.firstKey(t)

	// Nobody in this session started it, and it is over before the next
	// observation — so no indicator was ever broadcast and no poll will ever
	// see it running.
	s.launch(t, key, "refresh")

	h.exec(s.fetch())
	h.pumpFor(300 * time.Millisecond)

	st, ok := s.tab.ActivityState().State(key)
	if !ok {
		t.Fatalf("nothing marked the row:\n%s", h.render())
	}
	if !st.Changed {
		t.Errorf("state = %+v, want a revision flash", st)
	}
	if g := glyph.Default().ActivityChanged; !strings.Contains(h.render(), g) {
		t.Errorf("the %q glyph is not on screen:\n%s", g, h.render())
	}
}

// And the counterpart: a completion the user watched all the way through must
// not flash on top of the spinner it already saw. Two reports of one piece of
// news is what the settled set exists to prevent.
func TestWatchedCompletionDoesNotAlsoFlash(t *testing.T) {
	s := newActivityScreen(t)
	h := newHarness(t, s)
	h.pumpFor(150 * time.Millisecond)

	key := s.firstKey(t)
	revBefore := s.revOf(t, key)
	s.launch(t, key, "sync") // slow: observable as running

	// Observe it running…
	h.exec(s.fetch())
	h.pumpFor(300 * time.Millisecond)
	if st, ok := s.tab.ActivityState().State(key); !ok || st.Done {
		t.Fatalf("state = %+v, %v; the running job was never observed", st, ok)
	}

	// …wait it out, then observe it finished.
	done := h.pumpUntil(6*time.Second, func() bool {
		h.exec(s.fetch())
		h.pumpFor(250 * time.Millisecond)
		st, live := s.tab.ActivityState().State(key)
		return !live || st.Done
	})
	if !done {
		t.Fatal("the job never finished")
	}

	// The revision did move, so a flash was genuinely available to fire —
	// without this the test would pass just as well against a row nothing
	// happened to.
	if rev := s.revOf(t, key); rev == revBefore {
		t.Fatalf("revision is still %q; the completion never reached the rows", rev)
	}

	if st, live := s.tab.ActivityState().State(key); live && st.Changed {
		t.Errorf("a watched completion flashed as well: %+v", st)
	}
	if g := glyph.Default().ActivityChanged; strings.Contains(h.render(), g) {
		t.Errorf("the %q glyph appeared after a completion the user watched:\n%s", g, h.render())
	}
}

// revOf is the last revision the screen saw for a key — the value
// ActivityRevision watches.
func (s *activityScreen) revOf(t *testing.T, key string) string {
	t.Helper()
	rev, ok := s.revs[key]
	if !ok {
		t.Fatalf("no row for key %q", key)
	}
	return rev
}

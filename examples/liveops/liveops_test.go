package liveops

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/demoapi"
	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/poll"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

const exW, exH = 100, 30

// The applications pane's package comment promises rows that spin because the
// server says so. A showcase that quietly stops showing its subject is worse
// than none — the reader concludes the feature does not work — so the promises
// are asserted rather than trusted, as examples/workbench's are.
//
// What is asserted here is what belongs to the example: that it renders what
// the server sends, and that BusyWhen is wired to the column the data
// actually arrives in. That the server schedules background work and moves a
// revision is demoapi's promise, and demoapi tests it against a pinned clock
// rather than making this suite wait out real seconds.

// fastCmd runs a command but gives up on one that is really a timer.
//
// The screen's command stream mixes an in-process fetch, which answers in
// microseconds, with pkg/poll's re-arm, which sleeps for the poll interval.
// Draining everything would make each test wait out the interval for no
// benefit; taking whatever answers quickly gets the fetch and drops the tick.
func fastCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case m := <-ch:
		return m
	case <-time.After(300 * time.Millisecond):
		return nil
	}
}

// pump feeds cmd's messages back into the screen until it runs dry.
func pump(s screen.Screen, cmd tea.Cmd) screen.Screen {
	for i := 0; cmd != nil && i < 16; i++ {
		msg := fastCmd(cmd)
		if msg == nil {
			return s
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			var next []tea.Cmd
			for _, c := range batch {
				if m := fastCmd(c); m != nil {
					var c2 tea.Cmd
					s, c2 = s.Update(m)
					next = append(next, c2)
				}
			}
			cmd = tea.Batch(next...)
			continue
		}
		s, cmd = s.Update(msg)
	}
	return s
}

func newActivityExample(t *testing.T) screen.Screen {
	t.Helper()
	s := New(theme.New(palette.Dark()))
	s = pump(s, s.Init())
	s.Layout().Render(geom.New(0, 0, exW, exH))
	return s
}

// refreshOnce drives one poll cycle: the tick's message, then whatever fetch
// it produced.
func refreshOnce(s screen.Screen) (screen.Screen, string) {
	s2, cmd := s.Update(poll.RefreshMsg{})
	s2 = pump(s2, cmd)
	return s2, s2.Layout().Render(geom.New(0, 0, exW, exH))
}

func TestActivityExampleRendersWhatTheServerSends(t *testing.T) {
	s := newActivityExample(t)
	v := s.Layout().Render(geom.New(0, 0, exW, exH))
	if !strings.Contains(v, "Synced") && !strings.Contains(v, "OutOfSync") {
		t.Fatalf("no rows arrived from the server:\n%s", v)
	}
}

// Promise 2, and the part that is genuinely the example's: a status the server
// reports spins the row, with no broadcast anywhere — this test is not the
// shell, so nothing but BusyWhen can produce the indicator.
//
// The action is launched through the screen's own Actions(), so the wiring
// under test is the one a user drives: Selection() → a POST → the server
// reporting the work → the poll bringing it back → the column lighting up.
func TestActivityExampleDerivesFromTheServer(t *testing.T) {
	s := newActivityExample(t)

	set, ok := s.(action.Provider)
	if !ok {
		t.Fatal("the example stopped providing actions")
	}
	var sync action.Action
	for _, a := range set.Actions().Actions {
		if a.Label == "Sync" {
			sync = a
		}
	}
	if sync.Run == nil {
		t.Fatal("no Sync action to launch")
	}

	// In the background: the action streams the job's log, which takes longer
	// than this test needs to wait. Only the POST at the front of it matters.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = sync.Run(ctx, io.Discard) }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var v string
		s, v = refreshOnce(s)
		// No shell here, so no broadcast is possible: the only thing that can
		// have produced an indicator is BusyWhen reading the polled data.
		//
		// Named from demoapi's own constant rather than typed as a literal.
		// Two earlier versions of this assertion hardcoded the casing and both
		// broke when the example changed how it labels a busy row — which is a
		// presentation choice this test has no business depending on.
		if strings.Contains(v, demoapi.PhaseRunning) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, v := refreshOnce(s)
	t.Errorf("the server reported work and no row showed it:\n%s", v)
}

// ---- what the showcase adds around the activity --------------------------------

func liveScreen(t *testing.T) *Screen {
	t.Helper()
	return newActivityExample(t).(*Screen)
}

func find(set action.Set, label string) (action.Action, bool) {
	for _, a := range set.Actions {
		if a.Label == label {
			return a, true
		}
	}
	return action.Action{}, false
}

func press(s *Screen, keys ...tea.KeyMsg) {
	for _, k := range keys {
		s.Update(k)
		s.Layout().Render(geom.New(0, 0, exW, exH))
	}
}

var tab = tea.KeyMsg{Type: tea.KeyTab}

// Every pane spins until its own first data lands, and Reload shows it again.
func TestEachPaneLoadsUntilItsFirstData(t *testing.T) {
	s := New(theme.New(palette.Dark())).(*Screen)
	s.Init()
	if !s.table.Loading() || !s.work.Loading() || !s.events.Loading() {
		t.Fatal("a pane drew no spinner before its first data")
	}
	s = pump(s, s.Init()).(*Screen)
	if s.table.Loading() || s.events.Loading() {
		t.Error("the applications or the events kept spinning after the first read landed")
	}
	s.Update(workloadsMsg{rows: seedWorkloads()})
	if s.work.Loading() {
		t.Fatal("the workloads kept spinning after their data landed")
	}

	press(s, tab)
	reload, ok := find(s.Actions(), "Reload workloads")
	if !ok {
		t.Fatal("no Reload on the workloads pane")
	}
	reload.Do()
	if !s.work.Loading() {
		t.Error("Reload did not show the spinner again")
	}
}

// The poll's verbs live on the panes without a server behind them, and the
// workloads title says what the poll is doing.
func TestPollVerbsPauseAndRetime(t *testing.T) {
	s := liveScreen(t)
	if _, ok := find(s.Actions(), "Sync"); !ok {
		t.Fatal("the applications pane lost its verbs")
	}
	press(s, tab)
	pause, ok := find(s.Actions(), "Pause")
	if !ok {
		t.Fatal("no Pause on the workloads pane")
	}
	pause.Do()
	if !s.poll.Paused() || !strings.Contains(s.work.Title(), "paused") {
		t.Errorf("Pause: paused %v, title %q", s.poll.Paused(), s.work.Title())
	}
	if _, ok := find(s.Actions(), "Resume"); !ok {
		t.Error("a paused poll offers no Resume")
	}
	faster, _ := find(s.Actions(), "Faster")
	faster.Do()
	if s.poll.Interval() != pollInterval/2 {
		t.Errorf("Faster: interval %s, want %s", s.poll.Interval(), pollInterval/2)
	}
}

// The events log says what each read changed.
func TestEventsLogWhatChanged(t *testing.T) {
	prev := []appRow{{ID: "a", Name: "web", Sync: demoapi.SyncSynced, Health: "Healthy"}}
	next := []appRow{{ID: "a", Name: "web", Sync: demoapi.SyncOutOfSync, Health: "Degraded"}}
	got := strings.Join(appChanges(prev, next), "\n")
	if !strings.Contains(got, "Synced → OutOfSync") || !strings.Contains(got, "health Healthy → Degraded") {
		t.Errorf("changes: %q", got)
	}

	s := liveScreen(t)
	s.Update(workloadsMsg{rows: seedWorkloads(), events: []string{"api-gateway: pod lost (5/6)"}})
	if !strings.Contains(s.events.View(), "pod lost") {
		t.Error("a workloads event never reached the log")
	}
}

// Only the applications say Running. The integration tests read that word as
// "a row is working" anywhere on screen, so another pane using it would make
// them pass or fail for the wrong reason.
func TestOnlyTheApplicationsSayRunning(t *testing.T) {
	s := liveScreen(t)
	s.Update(workloadsMsg{rows: seedWorkloads()})
	s.Layout().Render(geom.New(0, 0, exW, exH))
	for name, v := range map[string]string{"workloads": s.work.View(), "events": s.events.View()} {
		if strings.Contains(v, demoapi.PhaseRunning) {
			t.Errorf("the %s pane says %q:\n%s", name, demoapi.PhaseRunning, v)
		}
	}
}

// A theme swap keeps marks, loaded data and the poll's state (rule 4).
func TestThemeSwapKeepsEveryPanesState(t *testing.T) {
	s := liveScreen(t)
	s.Update(workloadsMsg{rows: seedWorkloads()})
	press(s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if s.table.MarkCount() != 1 {
		t.Fatalf("setup: %d marked", s.table.MarkCount())
	}
	s.SetTheme(theme.New(palette.Light()))
	s.Layout().Render(geom.New(0, 0, exW, exH))
	if s.table.MarkCount() != 1 {
		t.Error("the mark was lost across the swap")
	}
	if s.work.Loading() || len(s.work.Rows()) == 0 {
		t.Error("the workloads went back to loading, or lost their rows")
	}
	if !strings.Contains(s.events.View(), "applications loaded") {
		t.Error("the events log lost its lines")
	}
}

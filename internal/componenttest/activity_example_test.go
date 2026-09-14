package componenttest

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/demoapi"
	exactivity "github.com/jsdrews/tuilib/examples/patterns/activity"
	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/poll"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// The activity example's package comment promises three sources of row state.
// A demo that quietly stops demonstrating its subject is worse than no demo —
// the reader concludes the feature does not work — so the promises are
// asserted rather than trusted. Same reasoning as multiselect_example_test.go.
//
// What is asserted here is what belongs to the example: that it renders what
// the server sends, and that ActivityWhen is wired to the column the data
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
	s := exactivity.New(theme.Dark())
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
// shell, so nothing but ActivityWhen can produce the indicator.
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
		// have produced an indicator is ActivityWhen reading the polled data.
		//
		// Named from demoapi's own constant rather than typed as a literal.
		// Two earlier versions of this assertion hardcoded the casing and both
		// broke when the example changed how it labels a busy row — which is a
		// presentation choice this test has no business depending on.
		if strings.Contains(v, demoapi.SyncSyncing) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, v := refreshOnce(s)
	t.Errorf("the server reported work and no row showed it:\n%s", v)
}

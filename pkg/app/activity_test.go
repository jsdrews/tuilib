package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// spyScreen records the activity broadcasts the shell sends down the stack.
// Asserting on the messages rather than on a rendered spinner keeps these
// tests about the shell's contract; whether a component draws what it is told
// is componenttest's job.
type spyScreen struct {
	l    list.Model
	set  action.Set
	ran  recorder
	msgs []tea.Msg
}

func (s *spyScreen) Title() string          { return "Deploys" }
func (s *spyScreen) Init() tea.Cmd          { return nil }
func (s *spyScreen) OnEnter(any) tea.Cmd    { return nil }
func (s *spyScreen) Layout() layout.Node    { return layout.Sized(&s.l) }
func (s *spyScreen) Help() []key.Binding    { return s.l.Help() }
func (s *spyScreen) IsCapturingKeys() bool  { return false }
func (s *spyScreen) Actions() action.Set    { return s.set }
func (s *spyScreen) SetTheme(t theme.Theme) { s.l = list.New(t.List()) }

func (s *spyScreen) Update(m tea.Msg) (screen.Screen, tea.Cmd) {
	switch m.(type) {
	case activity.StartMsg, activity.UpdateMsg, activity.EndMsg:
		s.msgs = append(s.msgs, m)
	}
	var c tea.Cmd
	s.l, c = s.l.Update(m)
	return s, c
}

func (s *spyScreen) starts() []activity.StartMsg {
	var out []activity.StartMsg
	for _, m := range s.msgs {
		if v, ok := m.(activity.StartMsg); ok {
			out = append(out, v)
		}
	}
	return out
}

func (s *spyScreen) updates() []activity.UpdateMsg {
	var out []activity.UpdateMsg
	for _, m := range s.msgs {
		if v, ok := m.(activity.UpdateMsg); ok {
			out = append(out, v)
		}
	}
	return out
}

func (s *spyScreen) ends() []activity.EndMsg {
	var out []activity.EndMsg
	for _, m := range s.msgs {
		if v, ok := m.(activity.EndMsg); ok {
			out = append(out, v)
		}
	}
	return out
}

func targetedSet(s *spyScreen, busy string, run action.Func) action.Set {
	return action.Set{
		Target:  "2 items",
		Targets: []string{"api", "web"},
		Count:   2,
		Actions: []action.Action{{
			Label: "Sync",
			Busy:  busy,
			Multi: true,
			Run:   run,
		}},
	}
}

func TestActionWithTargetsBroadcastsStartAndEnd(t *testing.T) {
	s := &spyScreen{}
	m := newActApp(t, s)
	s.set = targetedSet(s, "syncing", runAct("Sync", &s.ran))

	m = actKey(m, "a")
	m = actKey(m, "enter")
	waitFor(t, func() bool { return s.ran.len() > 0 })
	m = step(m, struct{}{})

	starts := s.starts()
	if len(starts) != 1 {
		t.Fatalf("got %d StartMsg, want 1", len(starts))
	}
	if got := starts[0].Label; got != "syncing" {
		t.Errorf("Label = %q, want the action's Busy text", got)
	}
	if len(starts[0].Keys) != 2 || starts[0].Keys[0] != "api" {
		t.Errorf("Keys = %v, want the Set's Targets", starts[0].Keys)
	}
	if starts[0].RunID == 0 {
		t.Error("StartMsg carries no RunID, so EndMsg can never match it")
	}

	waitFor(t, func() bool { return len(s.ends()) > 0 })
	ends := s.ends()
	if ends[0].RunID != starts[0].RunID {
		t.Errorf("EndMsg RunID = %d, want %d", ends[0].RunID, starts[0].RunID)
	}
	if ends[0].Err != nil {
		t.Errorf("Err = %v, want nil", ends[0].Err)
	}
}

// Busy defaults to the label, so an action that never heard of the field still
// says something sensible on the row.
func TestBusyDefaultsToTheLabel(t *testing.T) {
	s := &spyScreen{}
	m := newActApp(t, s)
	s.set = targetedSet(s, "", runAct("Sync", &s.ran))

	m = actKey(m, "a")
	m = actKey(m, "enter")
	waitFor(t, func() bool { return len(s.starts()) > 0 })

	if got := s.starts()[0].Label; got != "sync" {
		t.Errorf("Label = %q, want the lowercased action label", got)
	}
}

// A Set with no Targets is every screen written before this existed. It must
// behave exactly as it did.
func TestActionWithoutTargetsBroadcastsNothing(t *testing.T) {
	s := &spyScreen{}
	m := newActApp(t, s)
	s.set = action.Set{
		Target: "alpha", Count: 1,
		Actions: []action.Action{{Label: "Sync", Run: runAct("Sync", &s.ran)}},
	}

	m = actKey(m, "a")
	m = actKey(m, "enter")
	waitFor(t, func() bool { return s.ran.len() > 0 })
	m = step(m, struct{}{})

	if n := len(s.msgs); n != 0 {
		t.Errorf("broadcast %d activity messages for a Set with no Targets: %v", n, s.msgs)
	}
}

// The confirm detour is where the keys are dropped if pendingTgts is
// forgotten — and it is the path every destructive verb takes.
func TestConfirmDetourCarriesTargets(t *testing.T) {
	s := &spyScreen{}
	m := newActApp(t, s)
	set := targetedSet(s, "deleting", runAct("Delete", &s.ran))
	set.Actions[0].Label = "Delete"
	set.Actions[0].Confirm = "Delete 2 items?"
	s.set = set

	m = actKey(m, "a")
	m = actKey(m, "enter") // opens the confirm
	m = actKey(m, "y")     // confirms
	waitFor(t, func() bool { return s.ran.len() > 0 })
	m = step(m, struct{}{})

	starts := s.starts()
	if len(starts) != 1 {
		t.Fatalf("got %d StartMsg after a confirm, want 1", len(starts))
	}
	if len(starts[0].Keys) != 2 {
		t.Errorf("Keys = %v, want both targets through the modal", starts[0].Keys)
	}
}

// A status is a UI state change, not news. It relabels the rows and adds
// nothing to a log whose badge counts events.
func TestCaptureStatusRelabelsAndIsNotLogged(t *testing.T) {
	s := &spyScreen{}
	m := newActApp(t, s)
	s.set = targetedSet(s, "syncing", func(_ context.Context, out io.Writer) error {
		activity.Progress(out, "syncing 1/2")
		fmt.Fprintln(out, "one real line")
		s.ran.add("Sync")
		return nil
	})

	m = actKey(m, "a")
	m = actKey(m, "enter")
	waitFor(t, func() bool { return len(s.ends()) > 0 })
	m = step(m, struct{}{})

	ups := s.updates()
	if len(ups) != 1 || ups[0].Label != "syncing 1/2" {
		t.Fatalf("updates = %v, want one relabel", ups)
	}
	if ups[0].RunID == 0 {
		t.Error("UpdateMsg carries no RunID, so no entry can match it")
	}

	for _, r := range m.outBuf.Records() {
		if r.Text == "syncing 1/2" {
			t.Error("a progress report was written into the log")
		}
	}
}

func TestFailedActionEndsWithItsError(t *testing.T) {
	s := &spyScreen{}
	m := newActApp(t, s)
	boom := errors.New("connection refused")
	s.set = targetedSet(s, "syncing", func(context.Context, io.Writer) error {
		s.ran.add("Sync")
		return boom
	})

	m = actKey(m, "a")
	m = actKey(m, "enter")
	waitFor(t, func() bool { return len(s.ends()) > 0 })

	if got := s.ends()[0].Err; got == nil || got.Error() != boom.Error() {
		t.Errorf("EndMsg Err = %v, want %v", got, boom)
	}
}

// An Exclusive verb left held on a row that finished would be permanently
// unavailable there, which is worse than the race the gate exists to stop.
func TestCapturedReleasesEveryPerTargetGate(t *testing.T) {
	s := &spyScreen{}
	m := newActApp(t, s)
	s.set = targetedSet(s, "syncing", runAct("Sync", &s.ran))
	s.set.Actions[0].Exclusive = true

	m = actKey(m, "a")
	m = actKey(m, "enter")
	waitFor(t, func() bool { return len(s.ends()) > 0 })
	m = step(m, struct{}{})

	if n := len(m.running); n != 0 {
		t.Errorf("running holds %d entries after completion", n)
	}
	if n := len(m.busyKeys); n != 0 {
		t.Errorf("busyKeys holds %d gates after completion: %v", n, m.busyKeys)
	}
}

// The per-target gate: two overlapping selections that share a row collide on
// that row, where the old per-selection key could not see it.
//
// Driven against the registry directly rather than through a live action. A
// run that is still going has an undrained stream, and runner.Next blocks
// reading it — so any attempt to step the shell while one is in flight
// deadlocks the test rather than testing the gate.
func TestExclusiveGateIsPerTarget(t *testing.T) {
	m := newActApp(t, &spyScreen{})
	sync := action.Action{Label: "Sync", Multi: true, Exclusive: true}

	m.running["tag1"] = actionRun{keys: []string{"api", "web"}, busy: "syncing"}
	m.busyKeys[action.RunKey(sync, "api")] = "tag1"
	m.busyKeys[action.RunKey(sync, "web")] = "tag1"

	overlapping := action.Set{
		Target:  "2 others",
		Targets: []string{"web", "worker"},
		Count:   2,
		Actions: []action.Action{sync},
	}
	if !m.runningFor(overlapping)[action.RunKey(sync, overlapping.Target)] {
		t.Error("a selection overlapping an in-flight target was not reported as running")
	}

	disjoint := overlapping
	disjoint.Targets = []string{"cache", "worker"}
	if m.runningFor(disjoint)[action.RunKey(sync, disjoint.Target)] {
		t.Error("a disjoint selection was blocked by an unrelated run")
	}

	// A different verb over the same rows is unaffected: the gate is held
	// against an action's identity, not against the row alone.
	other := overlapping
	other.Actions = []action.Action{{Label: "Refresh", Multi: true, Exclusive: true}}
	if m.runningFor(other)[action.RunKey(other.Actions[0], other.Target)] {
		t.Error("an unrelated verb was blocked by a Sync in flight")
	}
}

// tableScreen is the real shape: a keyed table with an activity column,
// forwarding every message to it as rule 6 requires.
type tableScreen struct {
	t   table.Model
	set action.Set
	ran recorder
}

func (s *tableScreen) Title() string         { return "Apps" }
func (s *tableScreen) Init() tea.Cmd         { return nil }
func (s *tableScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *tableScreen) Layout() layout.Node   { return layout.Sized(&s.t) }
func (s *tableScreen) Help() []key.Binding   { return s.t.Help() }
func (s *tableScreen) IsCapturingKeys() bool { return false }
func (s *tableScreen) Actions() action.Set   { return s.set }
func (s *tableScreen) SetTheme(th theme.Theme) {
	o := th.Table()
	o.ActivityColumn = "Status"
	o.Columns = []table.Column{{Title: "Name", Width: 16}, {Title: "Status", Width: 14}}
	s.t = table.New(o)
	s.t.SetKeyedRows([]table.KeyedRow{
		{Key: "api", Cells: []string{"api-server", "Synced"}},
		{Key: "web", Cells: []string{"web-frontend", "Synced"}},
	})
}
func (s *tableScreen) Update(m tea.Msg) (screen.Screen, tea.Cmd) {
	var c tea.Cmd
	s.t, c = s.t.Update(m)
	return s, c
}

// The whole feature, end to end: pick a verb, watch the rows it acts on say
// so. Both halves are covered separately — the broadcast here, the rendering
// in internal/componenttest — but nothing else asserts they meet.
func TestActionPutsASpinnerOnItsTargetRows(t *testing.T) {
	s := &tableScreen{}
	m := newActApp(t, s)
	s.set = action.Set{
		Target:  "api-server",
		Targets: []string{"api"},
		Count:   1,
		Actions: []action.Action{{
			Label: "Sync",
			Busy:  "syncing",
			Run: func(_ context.Context, out io.Writer) error {
				fmt.Fprintln(out, "sync started")
				s.ran.add("Sync")
				return nil
			},
		}},
	}

	m = actKey(m, "a")
	m = actKey(m, "enter")
	waitFor(t, func() bool { return s.ran.len() > 0 })

	if got := s.t.ActivityCount(); got != 1 {
		t.Fatalf("ActivityCount = %d, want the target row working", got)
	}
	if !strings.Contains(m.View(), "syncing") {
		t.Errorf("the target row does not say what is happening to it:\n%s", m.View())
	}
	if strings.Count(m.View(), "syncing") != 1 {
		t.Error("more rows than the target are showing the indicator")
	}
}

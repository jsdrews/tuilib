package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// actScreen is a screen with verbs. ran records what actually executed, so a
// test can tell "the shell dispatched it" from "the shell drew a menu".
type actScreen struct {
	l       list.Model
	set     action.Set
	ran     recorder
	pushed  bool
	blocked bool
}

// recorder is what an action writes into. It is mutex-guarded because an
// action.Func runs on its own goroutine — which is the entire point of the
// feature, and also the reason an action must never touch screen state
// directly. The test models the same discipline it asks of callers.
type recorder struct {
	mu   sync.Mutex
	seen []string
}

func (r *recorder) add(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, s)
}

func (r *recorder) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seen...)
}

func (r *recorder) len() int { return len(r.list()) }

func (s *actScreen) Title() string         { return "Deploys" }
func (s *actScreen) Init() tea.Cmd         { return nil }
func (s *actScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *actScreen) Layout() layout.Node   { return layout.Sized(&s.l) }
func (s *actScreen) Help() []key.Binding   { return s.l.Help() }
func (s *actScreen) IsCapturingKeys() bool { return s.blocked }
func (s *actScreen) Actions() action.Set   { return s.set }
func (s *actScreen) SetTheme(t theme.Theme) {
	o := t.List()
	o.Items = []string{"alpha", "beta", "gamma"}
	s.l = list.New(o)
}
func (s *actScreen) Update(m tea.Msg) (screen.Screen, tea.Cmd) {
	var c tea.Cmd
	s.l, c = s.l.Update(m)
	return s, c
}

// plainScreen has no verbs — it does not implement action.Provider at all,
// which is the state every screen that predates this feature is in.
type plainScreen struct{ l list.Model }

func (s *plainScreen) Title() string         { return "Plain" }
func (s *plainScreen) Init() tea.Cmd         { return nil }
func (s *plainScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *plainScreen) Layout() layout.Node   { return layout.Sized(&s.l) }
func (s *plainScreen) Help() []key.Binding   { return s.l.Help() }
func (s *plainScreen) IsCapturingKeys() bool { return false }
func (s *plainScreen) SetTheme(t theme.Theme) {
	o := t.List()
	o.Items = []string{"one"}
	s.l = list.New(o)
}
func (s *plainScreen) Update(m tea.Msg) (screen.Screen, tea.Cmd) {
	var c tea.Cmd
	s.l, c = s.l.Update(m)
	return s, c
}

func newActApp(t *testing.T, root screen.Screen) Model {
	t.Helper()
	m := New(Options{
		Root:       root,
		Themes:     []theme.Theme{theme.Dark()},
		SkipConfig: true,
		Mouse:      MouseClick,
		ActionsKey: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "actions")),
		OutputKey:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "output")),
	})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(Model)
}

// step applies a message and then whatever its commands produce, the way
// bubbletea's event loop does. Without this the menu's ChosenMsg — which
// arrives as a command, not a return value — never reaches the shell.
// draw renders a frame so every component has a fresh rect. Mouse tests need
// it: Rect.Hit rejects a rect from an earlier generation, and a component that
// has never been laid out has no rect at all.
func draw(m Model) Model {
	_ = m.View()
	return m
}

func step(m Model, msg tea.Msg) Model {
	var mm tea.Model = m
	mm, cmd := mm.Update(msg)
	for i := 0; cmd != nil && i < 32; i++ {
		out := cmd()
		if out == nil {
			break
		}
		if batch, ok := out.(tea.BatchMsg); ok {
			cmd = nil
			for _, c := range batch {
				if c == nil {
					continue
				}
				if sub := c(); sub != nil {
					mm, _ = mm.Update(sub)
				}
			}
			break
		}
		mm, cmd = mm.Update(out)
	}
	return mm.(Model)
}

func actKey(m Model, s string) Model {
	if len(s) == 1 {
		return step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	}
	switch s {
	case "enter":
		return step(m, tea.KeyMsg{Type: tea.KeyEnter})
	case "esc":
		return step(m, tea.KeyMsg{Type: tea.KeyEsc})
	case "down":
		return step(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	panic("unmapped key " + s)
}

func runAct(label string, sink *recorder) action.Func {
	return func(_ context.Context, out io.Writer) error {
		fmt.Fprintf(out, "%s line one\n", label)
		sink.add(label)
		return nil
	}
}

func oneAction(s *actScreen) action.Set {
	return action.Set{
		Target: "alpha", Count: 1,
		Actions: []action.Action{
			{Label: "Restart", Run: runAct("Restart", &s.ran)},
			{Label: "Scale", Run: runAct("Scale", &s.ran)},
		},
	}
}

func TestActionsKeyOpensTheMenu(t *testing.T) {
	s := &actScreen{}
	m := newActApp(t, s)
	s.set = oneAction(s)

	m = actKey(m, "a")
	if !strings.Contains(m.View(), "Restart") {
		t.Fatal("pressing the actions key did not draw the menu")
	}
	if !strings.Contains(m.View(), "alpha") {
		t.Error("menu should be titled with the target")
	}
}

// The key is inert where the menu would open an empty box — the hint is the
// source of truth for whether it does anything, matching how "?" behaves.
func TestActionsKeyIsInertWithoutVerbs(t *testing.T) {
	m := newActApp(t, &plainScreen{})
	before := m.View()
	if got := actKey(m, "a").View(); got != before {
		t.Error("actions key did something on a screen with no verbs")
	}
	if strings.Contains(before, "actions") {
		t.Error("the actions hint is advertised on a screen that has none")
	}
}

func TestActionsKeyIsAdvertisedWhereItWorks(t *testing.T) {
	s := &actScreen{}
	m := newActApp(t, s)
	s.set = oneAction(s)
	m.apply()
	if !hasKey(m.helpBindings(s), "a") {
		t.Error("the shell should advertise its own actions key")
	}
}

func hasKey(bs []key.Binding, want string) bool {
	for _, b := range bs {
		for _, k := range b.Keys() {
			if k == want {
				return true
			}
		}
	}
	return false
}

func TestEnterRunsTheChosenAction(t *testing.T) {
	s := &actScreen{}
	m := newActApp(t, s)
	s.set = oneAction(s)

	m = actKey(m, "a")
	m = actKey(m, "enter")

	waitFor(t, func() bool { return s.ran.len() > 0 })
	if got := s.ran.list(); len(got) != 1 || got[0] != "Restart" {
		t.Errorf("ran = %v, want [Restart]", got)
	}
	if strings.Contains(m.View(), "Scale") {
		t.Error("the menu should close the moment an action is chosen")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition never became true")
}

func TestEscClosesTheMenuWithoutRunning(t *testing.T) {
	s := &actScreen{}
	m := newActApp(t, s)
	s.set = oneAction(s)

	m = actKey(m, "a")
	m = actKey(m, "esc")
	if strings.Contains(m.View(), "Restart") {
		t.Error("esc should close the menu")
	}
	if s.ran.len() != 0 {
		t.Errorf("ran = %v, want nothing", s.ran.list())
	}
}

// While the overlay is up it owns the keyboard completely — the screen must
// not also see the keys, or a filter would open behind the menu.
func TestOverlayOwnsTheKeyboard(t *testing.T) {
	s := &actScreen{}
	m := newActApp(t, s)
	s.set = oneAction(s)

	m = actKey(m, "a")
	before := s.l.Cursor()
	m = actKey(m, "down") // moves the menu cursor, not the list's
	if s.l.Cursor() != before {
		t.Errorf("list cursor moved to %d while the menu was up", s.l.Cursor())
	}
	m = actKey(m, "enter")
	waitFor(t, func() bool { return s.ran.len() > 0 })
	if got := s.ran.list(); got[0] != "Scale" {
		t.Errorf("ran %q, want Scale — the arrow belonged to the menu", got[0])
	}
}

// q must not quit and o must not open the console while the menu is up.
func TestGlobalsAreSuppressedUnderTheMenu(t *testing.T) {
	s := &actScreen{}
	m := newActApp(t, s)
	s.set = oneAction(s)

	m = actKey(m, "a")
	m = actKey(m, "o")
	if strings.Contains(m.View(), "Output") {
		t.Error("the output console opened from under the menu")
	}
	if !strings.Contains(m.View(), "Restart") {
		t.Error("the menu should still be up")
	}
}

func TestConfirmSitsBetweenThePickAndTheRun(t *testing.T) {
	s := &actScreen{}
	m := newActApp(t, s)
	s.set = action.Set{
		Target: "alpha", Count: 1,
		Actions: []action.Action{{
			Label:   "Delete",
			Confirm: "Delete alpha?",
			Run:     runAct("Delete", &s.ran),
		}},
	}

	m = actKey(m, "a")
	m = actKey(m, "enter")
	if !strings.Contains(m.View(), "Delete alpha?") {
		t.Fatal("the confirm modal did not appear")
	}
	if s.ran.len() != 0 {
		t.Fatal("the action ran before it was confirmed")
	}

	m = actKey(m, "esc")
	if s.ran.len() != 0 {
		t.Error("cancelling the confirm still ran the action")
	}

	m = actKey(m, "a")
	m = actKey(m, "enter")
	m = actKey(m, "y")
	waitFor(t, func() bool { return s.ran.len() > 0 })
}

// A Do action is the only one the shell cannot narrate, so it is the only one
// that gets an invocation line. A Run action already opens its own event, and
// a second head record would make every action report twice in a badge that
// counts events (rule 17).
func TestRunActionLogsExactlyOneEvent(t *testing.T) {
	s := &actScreen{}
	m := newActApp(t, s)
	s.set = oneAction(s)

	m = actKey(m, "a")
	m = actKey(m, "enter")
	waitFor(t, func() bool { return s.ran.len() > 0 })

	heads := 0
	for _, r := range m.outBuf.Records() {
		if r.Head {
			heads++
		}
	}
	if heads != 1 {
		t.Errorf("logged %d head records for one action, want 1", heads)
		for _, r := range m.outBuf.Records() {
			t.Logf("head=%v %s: %s", r.Head, r.Source, r.Text)
		}
	}
}

func TestDoActionGetsAnInvocationLine(t *testing.T) {
	s := &actScreen{}
	m := newActApp(t, s)
	s.set = action.Set{
		Target: "alpha", Count: 1,
		Actions: []action.Action{{
			Label: "Open",
			Do:    func() tea.Cmd { s.pushed = true; return nil },
		}},
	}

	m = actKey(m, "a")
	m = actKey(m, "enter")

	if !s.pushed {
		t.Fatal("the Do action did not run")
	}
	var found bool
	for _, r := range m.outBuf.Records() {
		if r.Head && strings.Contains(r.Text, "Open") {
			found = true
		}
	}
	if !found {
		t.Error("a Do action should still leave a line saying it happened")
	}
}

// Right-click points at a row first, then asks about it. Reversed, the menu
// would describe whatever was selected before the pointer moved.
func TestRightClickSelectsThenOpens(t *testing.T) {
	s := &actScreen{}
	m := newActApp(t, s)
	s.set = oneAction(s)

	// Third row of the list body: breadcrumb row 0, pane border row 1.
	m = step(draw(m), tea.MouseMsg{X: 6, Y: 4, Action: tea.MouseActionPress, Button: tea.MouseButtonRight})

	if s.l.Cursor() != 2 {
		t.Errorf("list cursor = %d, want 2 — the press must land before the menu opens", s.l.Cursor())
	}
	if !strings.Contains(m.View(), "Restart") {
		t.Error("right-click did not open the menu")
	}
}

func TestRightClickDoesNothingWithoutVerbs(t *testing.T) {
	m := newActApp(t, &plainScreen{})
	before := m.View()
	got := step(draw(m), tea.MouseMsg{X: 6, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonRight})
	if got.View() != before {
		t.Error("right-click opened something on a screen with no verbs")
	}
}

// The gesture that opened the menu, aimed elsewhere, re-asks about that row
// rather than costing a dismiss and a second click.
func TestRightClickOutsideTheMenuRetargets(t *testing.T) {
	s := &actScreen{}
	m := newActApp(t, s)
	s.set = oneAction(s)

	m = step(draw(m), tea.MouseMsg{X: 6, Y: 2, Action: tea.MouseActionPress, Button: tea.MouseButtonRight})
	if s.l.Cursor() != 0 {
		t.Fatalf("setup: cursor = %d, want 0", s.l.Cursor())
	}
	// Draw again so the open menu has a rect of its own — it can only decline
	// a press it knows it was not drawn under. The second press has to land
	// clear of the box, which is anchored where the first one did.
	m = step(draw(m), tea.MouseMsg{X: 50, Y: 4, Action: tea.MouseActionPress, Button: tea.MouseButtonRight})
	if s.l.Cursor() != 2 {
		t.Errorf("cursor = %d, want 2 — a right press away should retarget", s.l.Cursor())
	}
	if !strings.Contains(m.View(), "Restart") {
		t.Error("the menu should still be open, against the new row")
	}
}

func TestFailedActionReportsAtErrorLevel(t *testing.T) {
	s := &actScreen{}
	m := newActApp(t, s)
	s.set = action.Set{
		Target: "alpha", Count: 1,
		Actions: []action.Action{{
			Label: "Break",
			Run: func(context.Context, io.Writer) error {
				s.ran.add("Break")
				return errors.New("exploded")
			},
		}},
	}

	m = actKey(m, "a")
	m = actKey(m, "enter")
	waitFor(t, func() bool { return s.ran.len() > 0 })

	waitFor(t, func() bool {
		for _, r := range m.outBuf.Records() {
			if r.Level == 1 && strings.Contains(r.Text, "exploded") {
				return true
			}
		}
		return false
	})
}

// Screens that predate the feature keep compiling and simply have no verbs.
func TestScreenWithoutProviderIsUnaffected(t *testing.T) {
	m := newActApp(t, &plainScreen{})
	if !m.currentActions().Empty() {
		t.Error("a screen that never declared verbs reported some")
	}
}

// Mouse still fans out to the screen while no overlay is up.
func TestNormalClicksStillReachTheScreen(t *testing.T) {
	s := &actScreen{}
	m := newActApp(t, s)
	s.set = oneAction(s)

	step(draw(m), tea.MouseMsg{X: 6, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if s.l.Cursor() != 1 {
		t.Errorf("cursor = %d, want 1 — an ordinary click must still select", s.l.Cursor())
	}
}

var _ = mouse.Msg{}

package focus

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// stub is a minimal Focusable that records whether it was focused and
// whether its Focus command was collected.
type stub struct {
	focused   bool
	capturing bool
	cmdFired  bool
}

func (s *stub) Focus() tea.Cmd {
	s.focused = true
	return func() tea.Msg { s.cmdFired = true; return nil }
}
func (s *stub) Blur()                 { s.focused = false }
func (s *stub) Focused() bool         { return s.focused }
func (s *stub) IsCapturingKeys() bool { return s.capturing }

// plain has no Capturer implementation, to check the optional-interface path.
type plain struct{ focused bool }

func (p *plain) Focus() tea.Cmd { p.focused = true; return nil }
func (p *plain) Blur()          { p.focused = false }
func (p *plain) Focused() bool  { return p.focused }

func keyPress(s string) tea.KeyMsg {
	if s == "tab" {
		return tea.KeyMsg{Type: tea.KeyTab}
	}
	return tea.KeyMsg{Type: tea.KeyShiftTab}
}

// assertOnly checks exactly one member is focused, at index want.
func assertOnly(t *testing.T, items []*stub, want int) {
	t.Helper()
	for i, it := range items {
		if got := it.Focused(); got != (i == want) {
			t.Errorf("item %d focused = %v, want %v", i, got, i == want)
		}
	}
}

func TestInitFocusesFirstItem(t *testing.T) {
	a, b, c := &stub{}, &stub{}, &stub{}
	g := NewGroup(a, b, c)
	g.Init()
	assertOnly(t, []*stub{a, b, c}, 0)
}

func TestTabCyclesForwardAndWraps(t *testing.T) {
	a, b, c := &stub{}, &stub{}, &stub{}
	g := NewGroup(a, b, c)
	g.Init()

	for _, want := range []int{1, 2, 0} {
		g, _ = g.Update(keyPress("tab"))
		assertOnly(t, []*stub{a, b, c}, want)
		if g.Index() != want {
			t.Errorf("Index() = %d, want %d", g.Index(), want)
		}
	}
}

func TestShiftTabCyclesBackwardAndWraps(t *testing.T) {
	a, b, c := &stub{}, &stub{}, &stub{}
	g := NewGroup(a, b, c)
	g.Init()

	for _, want := range []int{2, 1, 0} {
		g, _ = g.Update(keyPress("shift+tab"))
		assertOnly(t, []*stub{a, b, c}, want)
	}
}

func TestWithoutWrapStopsAtEnds(t *testing.T) {
	a, b := &stub{}, &stub{}
	g := NewGroup(a, b).WithoutWrap()
	g.Init()

	g, _ = g.Update(keyPress("shift+tab"))
	if g.Index() != 0 {
		t.Errorf("shift+tab at the start moved to %d, want 0", g.Index())
	}
	g, _ = g.Update(keyPress("tab"))
	g, _ = g.Update(keyPress("tab"))
	if g.Index() != 1 {
		t.Errorf("tab past the end moved to %d, want 1", g.Index())
	}
}

// The click-to-focus path: a component asks for focus and the group grants
// it, blurring the previously focused sibling.
func TestRequestGrantsFocusToTarget(t *testing.T) {
	a, b, c := &stub{}, &stub{}, &stub{}
	g := NewGroup(a, b, c)
	g.Init()

	g, _ = g.Update(RequestMsg{Target: c})
	assertOnly(t, []*stub{a, b, c}, 2)
}

// A request naming something this group doesn't hold must be left alone, so
// nested groups don't steal each other's targets.
func TestRequestForUnknownTargetIsIgnored(t *testing.T) {
	a, b := &stub{}, &stub{}
	outsider := &stub{}
	g := NewGroup(a, b)
	g.Init()

	g, cmd := g.Update(RequestMsg{Target: outsider})
	if cmd != nil {
		t.Errorf("unknown target produced a command")
	}
	assertOnly(t, []*stub{a, b}, 0)
	if outsider.Focused() {
		t.Errorf("outsider was focused by a group that doesn't hold it")
	}
}

func TestRequestForAlreadyFocusedIsANoop(t *testing.T) {
	a, b := &stub{}, &stub{}
	g := NewGroup(a, b)
	g.Init()

	_, cmd := g.Update(RequestMsg{Target: a})
	if cmd != nil {
		t.Errorf("re-requesting the focused item produced a command")
	}
}

func TestFocusCommandIsReturned(t *testing.T) {
	a, b := &stub{}, &stub{}
	g := NewGroup(a, b)
	if cmd := g.Init(); cmd == nil {
		t.Fatalf("Init returned no command; the focused item's cmd was dropped")
	} else {
		cmd()
	}
	if !a.cmdFired {
		t.Errorf("focused item's command was not the one returned")
	}
}

func TestSetIndexClamps(t *testing.T) {
	a, b := &stub{}, &stub{}
	g := NewGroup(a, b)
	g.SetIndex(99)
	if g.Index() != 1 {
		t.Errorf("SetIndex(99) = %d, want 1 (clamped)", g.Index())
	}
	g.SetIndex(-5)
	if g.Index() != 0 {
		t.Errorf("SetIndex(-5) = %d, want 0 (clamped)", g.Index())
	}
}

func TestIsCapturingKeysFollowsFocusedItem(t *testing.T) {
	a, b := &stub{}, &stub{capturing: true}
	g := NewGroup(a, b)
	g.Init()

	if g.IsCapturingKeys() {
		t.Errorf("group reports capturing while a non-capturing item is focused")
	}
	g, _ = g.Update(keyPress("tab"))
	if !g.IsCapturingKeys() {
		t.Errorf("group should report capturing when the focused item does")
	}
}

func TestIsCapturingKeysFalseWithoutCapturer(t *testing.T) {
	g := NewGroup(&plain{})
	g.Init()
	if g.IsCapturingKeys() {
		t.Errorf("an item that doesn't implement Capturer must never capture")
	}
}

func TestEmptyGroupIsInert(t *testing.T) {
	g := NewGroup()
	if cmd := g.Init(); cmd != nil {
		t.Errorf("empty group produced a command")
	}
	if g.Focused() != nil {
		t.Errorf("empty group reported a focused item")
	}
	if g.IsCapturingKeys() {
		t.Errorf("empty group reported capturing")
	}
	g, _ = g.Update(keyPress("tab"))
	if g.Index() != 0 {
		t.Errorf("cycling an empty group moved the index to %d", g.Index())
	}
}

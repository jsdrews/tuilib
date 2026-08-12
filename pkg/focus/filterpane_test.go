package focus

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// filterPane stands in for a filterable component: two focusable regions
// (body and filter) behind one Focusable, which is the shape that produces
// the cross-component focus bugs.
type filterPane struct {
	body   bool
	filter bool
}

func (p *filterPane) Focus() tea.Cmd {
	if p.filter {
		return nil // the filter already owns input within this component
	}
	p.body = true
	return nil
}
func (p *filterPane) Blur()                 { p.body, p.filter = false, false }
func (p *filterPane) Focused() bool         { return p.body || p.filter }
func (p *filterPane) IsCapturingKeys() bool { return p.filter }

func (p *filterPane) focusFilter() { p.body, p.filter = false, true }

// Exactly one region across the whole screen may be active.
func assertSingleActive(t *testing.T, panes []*filterPane) {
	t.Helper()
	active := 0
	for _, p := range panes {
		if p.body {
			active++
		}
		if p.filter {
			active++
		}
	}
	if active != 1 {
		t.Errorf("%d active regions across the screen, want exactly 1", active)
	}
}

// Switching panes must clear the filter you left. A filter left focused on a
// blurred component is invisible and still swallows keys.
func TestSwitchingPanesClearsTheFilterYouLeft(t *testing.T) {
	a, b := &filterPane{}, &filterPane{}
	g := NewGroup(a, b)
	g.Init()

	a.focusFilter() // user clicks into the first pane's filter
	assertSingleActive(t, []*filterPane{a, b})

	// Leaving the filter (esc) lets tab cycle again.
	a.filter, a.body = false, true
	g, _ = g.Update(keyPress("tab"))

	if a.filter {
		t.Errorf("the filter on the pane we left is still focused")
	}
	if a.body {
		t.Errorf("the pane we left is still highlighted")
	}
	assertSingleActive(t, []*filterPane{a, b})
}

// A click on one pane's filter must not leave the other pane active.
func TestFocusRequestFromAFilterClearsTheOtherPane(t *testing.T) {
	a, b := &filterPane{}, &filterPane{}
	g := NewGroup(a, b)
	g.Init() // a is focused

	// Clicking b's filter: the component focuses its own filter, then asks
	// the group for focus. The grant must not re-highlight b's body, and
	// must clear a entirely.
	b.focusFilter()
	g, _ = g.Update(RequestMsg{Target: b})

	if a.Focused() {
		t.Errorf("the previously focused pane is still active")
	}
	if b.body {
		t.Errorf("the focus grant re-highlighted the body over the filter")
	}
	if !b.filter {
		t.Errorf("the focus grant blurred the filter it was granted for")
	}
	assertSingleActive(t, []*filterPane{a, b})
}

// tab must not cycle while a filter is taking input: it would strand a
// half-typed query, and pkg/table binds tab to complete a key:value term.
func TestTabDoesNotCycleWhileAFilterHasInput(t *testing.T) {
	a, b := &filterPane{}, &filterPane{}
	g := NewGroup(a, b)
	g.Init()
	a.focusFilter()

	before := g.Index()
	g, cmd := g.Update(keyPress("tab"))

	if g.Index() != before {
		t.Errorf("tab cycled to pane %d while a filter had input", g.Index())
	}
	if cmd != nil {
		t.Errorf("tab produced a focus command while a filter had input")
	}
	if !a.filter {
		t.Errorf("tab blurred the filter instead of being left to it")
	}
}

func TestTabCyclesOnceTheFilterIsDone(t *testing.T) {
	a, b := &filterPane{}, &filterPane{}
	g := NewGroup(a, b)
	g.Init()
	a.focusFilter()

	// esc leaves the filter; the body takes the highlight back.
	a.filter, a.body = false, true

	g, _ = g.Update(keyPress("tab"))

	if g.Index() != 1 {
		t.Errorf("Index() = %d, want 1 — tab should cycle once no filter has input", g.Index())
	}
	if !b.body {
		t.Errorf("the newly focused pane was not highlighted")
	}
}

// The group reports capturing from whichever pane holds focus, so the app
// shell's global keys stay out of a filter that's taking input.
func TestCapturingFollowsTheFocusedPanesFilter(t *testing.T) {
	a, b := &filterPane{}, &filterPane{}
	g := NewGroup(a, b)
	g.Init()

	if g.IsCapturingKeys() {
		t.Errorf("capturing with no filter focused")
	}
	a.focusFilter()
	if !g.IsCapturingKeys() {
		t.Errorf("not capturing while the focused pane's filter has input")
	}

	// A filter on a pane that does NOT hold focus must not report capturing —
	// that would suppress global keys with no visible reason.
	a.Blur()
	b.body = true
	g.SetIndex(1)
	a.focusFilter()
	if g.IsCapturingKeys() {
		t.Errorf("an unfocused pane's filter reported capturing")
	}
}

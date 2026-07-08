package tab

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// stubBody is a minimal screen.Screen whose Layout renders a fixed marker
// string so tests can locate the body row within View() output.
type stubBody struct {
	label     string
	capturing bool
	entered   int
}

func (b *stubBody) Title() string                            { return b.label }
func (b *stubBody) Init() tea.Cmd                            { return nil }
func (b *stubBody) OnEnter(any) tea.Cmd                      { b.entered++; return nil }
func (b *stubBody) IsCapturingKeys() bool                    { return b.capturing }
func (b *stubBody) Update(tea.Msg) (screen.Screen, tea.Cmd)  { return b, nil }
func (b *stubBody) Help() []key.Binding                      { return nil }
func (b *stubBody) SetTheme(theme.Theme)                     {}
func (b *stubBody) Layout() layout.Node {
	marker := b.label
	return layout.RenderFunc(func(w, h int) string {
		lines := make([]string, h)
		for i := range lines {
			if i == 0 {
				lines[i] = marker
			}
		}
		return strings.Join(lines, "\n")
	})
}

func newTabs(t theme.Theme, pos StripPos, bodies ...*stubBody) Model {
	tabs := make([]Tab, len(bodies))
	for i, b := range bodies {
		tabs[i] = Tab{Label: b.label, Body: b}
	}
	m := New(Options{Theme: t, Tabs: tabs, StripPos: pos})
	m.SetDimensions(30, 6)
	return m
}

func TestStripTopByDefault(t *testing.T) {
	th := theme.Dark()
	a := &stubBody{label: "AAA"}
	b := &stubBody{label: "BBB"}
	m := New(Options{Theme: th, Tabs: []Tab{{Label: "One", Body: a}, {Label: "Two", Body: b}}})
	m.SetDimensions(30, 6)

	lines := strings.Split(m.View(), "\n")
	if len(lines) != 6 {
		t.Fatalf("View lines = %d, want 6", len(lines))
	}
	if !strings.Contains(lines[0], "One") {
		t.Errorf("row 0 should hold strip with 'One': %q", lines[0])
	}
	if !strings.Contains(lines[1], "AAA") {
		t.Errorf("row 1 should hold body marker AAA: %q", lines[1])
	}
}

func TestStripBottomPlacesStripOnLastRow(t *testing.T) {
	th := theme.Dark()
	a := &stubBody{label: "AAA"}
	b := &stubBody{label: "BBB"}
	m := newTabs(th, StripBottom, a, b)

	lines := strings.Split(m.View(), "\n")
	if len(lines) != 6 {
		t.Fatalf("View lines = %d, want 6", len(lines))
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "One") && !strings.Contains(last, "AAA") {
		// Sanity fallthrough — the labels used here are body markers, not
		// tab labels. Tab labels are the Tab.Label field ("AAA"/"BBB").
	}
	if !strings.Contains(last, "AAA") {
		t.Errorf("last row should hold strip containing tab label AAA: %q", last)
	}
	if !strings.Contains(lines[0], "AAA") == false {
		// Body marker "AAA" also happens to match the tab label; the strip
		// renders labels padded with spaces (" AAA "), so check row 0 for a
		// non-strip appearance of the body marker (bare "AAA" leftmost).
	}
	// The body's marker row is the raw "AAA" written at column 0 of row 0
	// by stubBody.Layout. The strip pads labels with surrounding spaces.
	if strings.HasPrefix(strings.TrimRight(lines[0], " "), "AAA") == false {
		t.Errorf("row 0 should start with body marker AAA: %q", lines[0])
	}
}

func TestStripBottomBodyGetsSameHeightAsTop(t *testing.T) {
	th := theme.Dark()
	a := &stubBody{label: "A"}
	top := newTabs(th, StripTop, a, &stubBody{label: "B"})
	bot := newTabs(th, StripBottom, a, &stubBody{label: "B"})

	topLines := strings.Split(top.View(), "\n")
	botLines := strings.Split(bot.View(), "\n")
	if len(topLines) != len(botLines) {
		t.Fatalf("row counts differ: top=%d bottom=%d", len(topLines), len(botLines))
	}
}

func TestSwitchToNextRunsOnEnter(t *testing.T) {
	th := theme.Dark()
	a := &stubBody{label: "A"}
	b := &stubBody{label: "B"}
	m := New(Options{Theme: th, Tabs: []Tab{{Label: "A", Body: a}, {Label: "B", Body: b}}})
	m.SetDimensions(30, 6)

	beforeA, beforeB := a.entered, b.entered
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	if a.entered != beforeA {
		t.Errorf("a.entered increased on switch away: %d -> %d", beforeA, a.entered)
	}
	if b.entered != beforeB+1 {
		t.Errorf("b.entered should tick once on switch-to: %d -> %d", beforeB, b.entered)
	}
}

func TestNumberKeyJumpsToTab(t *testing.T) {
	th := theme.Dark()
	a := &stubBody{label: "A"}
	b := &stubBody{label: "B"}
	c := &stubBody{label: "C"}
	m := New(Options{Theme: th, Tabs: []Tab{{Label: "A", Body: a}, {Label: "B", Body: b}, {Label: "C", Body: c}}})
	m.SetDimensions(30, 6)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if m.ActiveTab() != 2 {
		t.Errorf("active tab after '3' = %d, want 2", m.ActiveTab())
	}
}

func TestKeyMsgRoutesToActiveBodyOnly(t *testing.T) {
	th := theme.Dark()
	a := &countingBody{label: "A"}
	b := &countingBody{label: "B"}
	m := New(Options{Theme: th, Tabs: []Tab{{Label: "A", Body: a}, {Label: "B", Body: b}}})
	m.SetDimensions(30, 6)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if a.keyCount != 1 {
		t.Errorf("active body key count = %d, want 1", a.keyCount)
	}
	if b.keyCount != 0 {
		t.Errorf("inactive body key count = %d, want 0", b.keyCount)
	}
}

func TestNonKeyMsgFansOutToAllBodies(t *testing.T) {
	th := theme.Dark()
	a := &countingBody{label: "A"}
	b := &countingBody{label: "B"}
	m := New(Options{Theme: th, Tabs: []Tab{{Label: "A", Body: a}, {Label: "B", Body: b}}})
	m.SetDimensions(30, 6)

	m, _ = m.Update(struct{}{})
	if a.otherCount != 1 || b.otherCount != 1 {
		t.Errorf("fan-out counts = %d/%d, want 1/1", a.otherCount, b.otherCount)
	}
}

func TestIsCapturingKeysFollowsActiveBody(t *testing.T) {
	th := theme.Dark()
	a := &stubBody{label: "A", capturing: true}
	b := &stubBody{label: "B"}
	m := New(Options{Theme: th, Tabs: []Tab{{Label: "A", Body: a}, {Label: "B", Body: b}}})
	if !m.IsCapturingKeys() {
		t.Errorf("active-capturing: IsCapturingKeys() should be true")
	}
	// SetActiveTab bypasses the switch-key gate (which is suppressed while
	// the active body is capturing).
	m.SetActiveTab(1)
	if m.IsCapturingKeys() {
		t.Errorf("switched to non-capturing body: IsCapturingKeys() should be false")
	}
}

func TestSwitchKeysSuppressedWhileCapturing(t *testing.T) {
	th := theme.Dark()
	a := &countingBody{label: "A", capturing: true}
	b := &countingBody{label: "B"}
	m := New(Options{Theme: th, Tabs: []Tab{{Label: "A", Body: a}, {Label: "B", Body: b}}})
	m.SetDimensions(30, 6)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	if m.ActiveTab() != 0 {
		t.Errorf("shift+right while capturing should not switch: active = %d", m.ActiveTab())
	}
	// It should have been forwarded to the active body instead.
	if a.keyCount != 1 {
		t.Errorf("capturing body should have received the shift+right: keyCount = %d", a.keyCount)
	}
}

// countingBody tallies key vs non-key messages so the routing test can
// verify KeyMsg goes to the active tab only while other messages fan out.
type countingBody struct {
	label      string
	capturing  bool
	keyCount   int
	otherCount int
}

func (b *countingBody) Title() string       { return b.label }
func (b *countingBody) Init() tea.Cmd       { return nil }
func (b *countingBody) OnEnter(any) tea.Cmd { return nil }
func (b *countingBody) IsCapturingKeys() bool {
	return b.capturing
}
func (b *countingBody) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		b.keyCount++
	} else {
		b.otherCount++
	}
	return b, nil
}
func (b *countingBody) Help() []key.Binding { return nil }
func (b *countingBody) SetTheme(theme.Theme) {}
func (b *countingBody) Layout() layout.Node {
	return layout.RenderFunc(func(w, h int) string { return "" })
}

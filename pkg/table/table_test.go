package table

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderRowDefaultSep(t *testing.T) {
	cols := []Column{{Title: "A", Width: 4}, {Title: "B", Width: 4}}
	got := renderRow([]string{"hi", "yo"}, cols, []int{4, 4}, " ")
	if got != "hi   yo  " {
		t.Errorf("default sep render = %q", got)
	}
}

func TestRenderRowVerticalSep(t *testing.T) {
	cols := []Column{{Title: "A", Width: 4}, {Title: "B", Width: 4}}
	got := renderRow([]string{"hi", "yo"}, cols, []int{4, 4}, " │ ")
	if !strings.Contains(got, "│") {
		t.Errorf("vertical sep missing in %q", got)
	}
	if w := ansi.StringWidth(got); w != 4+3+4 {
		t.Errorf("vertical sep width = %d, want %d", w, 4+3+4)
	}
}

func TestSplitGlyphStylePlain(t *testing.T) {
	prefix, glyph, suffix, ok := splitGlyphStyle("─")
	if !ok || prefix != "" || glyph != '─' || suffix != "" {
		t.Errorf("splitGlyphStyle plain = (%q, %q, %q, %v)", prefix, string(glyph), suffix, ok)
	}
}

func TestSplitGlyphStyleStyled(t *testing.T) {
	in := "\x1b[38;5;240m─\x1b[39m"
	prefix, glyph, suffix, ok := splitGlyphStyle(in)
	if !ok {
		t.Fatalf("splitGlyphStyle ok=false on %q", in)
	}
	if prefix != "\x1b[38;5;240m" {
		t.Errorf("prefix = %q", prefix)
	}
	if glyph != '─' {
		t.Errorf("glyph = %q", string(glyph))
	}
	if suffix != "\x1b[39m" {
		t.Errorf("suffix = %q", suffix)
	}
}

func TestBuildRuleStyledWidth(t *testing.T) {
	in := "\x1b[38;5;240m─\x1b[39m"
	got := buildRule(in, 12)
	if w := ansi.StringWidth(got); w != 12 {
		t.Errorf("rule visible width = %d, want 12", w)
	}
	if !strings.HasPrefix(got, "\x1b[38;5;240m") {
		t.Errorf("rule missing SGR prefix: %q", got)
	}
	if !strings.HasSuffix(got, "\x1b[39m") {
		t.Errorf("rule missing SGR suffix: %q", got)
	}
	stripped := ansi.Strip(got)
	if stripped != strings.Repeat("─", 12) {
		t.Errorf("rule body = %q, want 12 horizontal-line glyphs", stripped)
	}
}

func TestBuildRuleEmpty(t *testing.T) {
	if got := buildRule("", 10); got != "" {
		t.Errorf("buildRule(\"\") = %q, want empty", got)
	}
	if got := buildRule("─", 0); got != "" {
		t.Errorf("buildRule(width=0) = %q, want empty", got)
	}
}

func TestNewWithBordersThreadsSep(t *testing.T) {
	cols := []Column{{Title: "A", Width: 4}, {Title: "B", Width: 4}}
	m := New(Options{
		Width:         20,
		Height:        5,
		Columns:       cols,
		Rows:          []Row{{"hi", "yo"}},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
		Borders: Borders{
			Vertical:   "│",
			HeaderRule: "─",
		},
	})
	if m.colSep != " │ " {
		t.Errorf("colSep = %q, want %q", m.colSep, " │ ")
	}
	if m.headerRule != "─" {
		t.Errorf("headerRule = %q", m.headerRule)
	}
}

func TestNewWithoutBordersDefaultsToSpace(t *testing.T) {
	cols := []Column{{Title: "A", Width: 4}}
	m := New(Options{
		Width:         10,
		Height:        3,
		Columns:       cols,
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	if m.colSep != " " {
		t.Errorf("default colSep = %q, want single space", m.colSep)
	}
	if m.headerRule != "" {
		t.Errorf("default headerRule = %q, want empty", m.headerRule)
	}
}

func TestContentAutoSizeFromTitleOnly(t *testing.T) {
	m := New(Options{
		Width:         40,
		Height:        5,
		Columns:       []Column{{Title: "Region"}}, // Width=0, no rows
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	if m.widths[0] != ansi.StringWidth("Region") {
		t.Errorf("title-only width = %d, want %d", m.widths[0], ansi.StringWidth("Region"))
	}
}

func TestContentAutoSizeShortTitleFloor(t *testing.T) {
	m := New(Options{
		Width:         40,
		Height:        5,
		Columns:       []Column{{Title: "X"}}, // 1-rune title, no rows
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	if m.widths[0] != 4 {
		t.Errorf("short-title floor = %d, want 4", m.widths[0])
	}
}

func TestContentAutoSizeFromCells(t *testing.T) {
	m := New(Options{
		Width:         40,
		Height:        5,
		Columns:       []Column{{Title: "x"}}, // Width=0
		Rows:          []Row{{"hello world"}, {"hi"}, {"medium"}},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	if m.widths[0] != ansi.StringWidth("hello world") {
		t.Errorf("content-auto width = %d, want %d", m.widths[0], ansi.StringWidth("hello world"))
	}
}

func TestContentAutoSizeStripsANSI(t *testing.T) {
	// Cell with ANSI escapes should size by visible width, not raw bytes.
	colored := "\x1b[31mfoo\x1b[39m"
	m := New(Options{
		Width:         40,
		Height:        5,
		Columns:       []Column{{Title: "x"}},
		Rows:          []Row{{colored}},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	if m.widths[0] != 4 { // floor — visible width is 3, title "x" is 1
		t.Errorf("ANSI-stripped width = %d, want 4 (floor)", m.widths[0])
	}
}

func TestFlexAbsorbsLeftover(t *testing.T) {
	// inner width 30, two cols: fixed 10 + flex(1) — flex should grow to ~19 (30-10-1 sep).
	m := New(Options{
		Width:         32, // outer: 32 - 2 borders - 1 scrollbar = 29 inner
		Height:        5,
		Columns: []Column{
			{Title: "fixed", Width: 10},
			{Title: "flex", Width: 0, Flex: 1},
		},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	inner := m.body.VisibleWidth()
	if inner <= 0 {
		t.Skip("pane has no inner width in this test environment")
	}
	wantFlex := inner - 10 - 1 // separator " " between two cols
	if m.widths[1] != wantFlex {
		t.Errorf("flex width = %d, want %d (inner=%d)", m.widths[1], wantFlex, inner)
	}
	if m.widths[0] != 10 {
		t.Errorf("fixed width changed: %d, want 10", m.widths[0])
	}
}

func TestFlexProportionalSplit(t *testing.T) {
	// Two flex cols, weights 1 + 3 → 25% / 75% of leftover.
	m := New(Options{
		Width:         42, // inner ~ 39
		Height:        5,
		Columns: []Column{
			{Title: "fixed", Width: 5},
			{Title: "a", Flex: 1},
			{Title: "b", Flex: 3},
		},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	inner := m.body.VisibleWidth()
	if inner <= 0 {
		t.Skip("pane has no inner width in this test environment")
	}
	totalSep := 2 * 1 // 2 inter-column gaps, single space
	leftover := inner - 5 - 4 - 4 - totalSep // base widths: fixed=5, two flex floor=4 each
	if leftover <= 0 {
		t.Skip("not enough room to test flex split")
	}
	gotA := m.widths[1] - 4
	gotB := m.widths[2] - 4
	if gotA+gotB != leftover {
		t.Errorf("flex shares = %d + %d = %d, want %d", gotA, gotB, gotA+gotB, leftover)
	}
	// b should be ~3x a (last column absorbs rounding remainder).
	if gotA*3 > gotB+2 || gotA*3 < gotB-2 {
		t.Errorf("flex split off-ratio: a=%d, b=%d (want ~1:3)", gotA, gotB)
	}
}

func TestFlexNoExpansionWhenTooNarrow(t *testing.T) {
	// Bases sum exceeds inner — flex columns should not grow (no negative).
	m := New(Options{
		Width:         10,
		Height:        5,
		Columns: []Column{
			{Title: "long-fixed", Width: 30},
			{Title: "flex", Flex: 1},
		},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	if m.widths[0] != 30 {
		t.Errorf("fixed width = %d, want 30", m.widths[0])
	}
	if m.widths[1] != 4 { // flex stays at floor base
		t.Errorf("flex width = %d, want base 4 (no expansion)", m.widths[1])
	}
}

func TestMaxWidthCapsFlexGrowth(t *testing.T) {
	// City min 10, MaxWidth 20; only flex column → grows to cap, leftover unused.
	m := New(Options{
		Width:         80,
		Height:        5,
		Columns: []Column{
			{Title: "city", Width: 10, Flex: 1, MaxWidth: 20},
			{Title: "fixed", Width: 8},
		},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	if m.widths[0] != 20 {
		t.Errorf("flex column width = %d, want 20 (capped)", m.widths[0])
	}
}

func TestMaxWidthRedistributesToOtherFlex(t *testing.T) {
	// Two flex cols. City caps at 20; remainder should go entirely to Foo.
	m := New(Options{
		Width:         80, // inner ~ 77
		Height:        5,
		Columns: []Column{
			{Title: "city", Width: 10, Flex: 1, MaxWidth: 20},
			{Title: "foo", Width: 4, Flex: 1},
		},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	if m.widths[0] != 20 {
		t.Errorf("city width = %d, want 20 (capped)", m.widths[0])
	}
	inner := m.body.VisibleWidth()
	if inner <= 0 {
		t.Skip("no inner width")
	}
	wantFoo := inner - 20 - 1 // single sep
	if m.widths[1] != wantFoo {
		t.Errorf("foo width = %d, want %d (absorbed cap surplus)", m.widths[1], wantFoo)
	}
}

func TestMaxWidthBothCappedLeavesUnused(t *testing.T) {
	// Both flex cols capped; leftover stays unused.
	m := New(Options{
		Width:         80,
		Height:        5,
		Columns: []Column{
			{Title: "a", Width: 4, Flex: 1, MaxWidth: 10},
			{Title: "b", Width: 4, Flex: 1, MaxWidth: 10},
		},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	if m.widths[0] != 10 || m.widths[1] != 10 {
		t.Errorf("widths = [%d,%d], want [10,10]", m.widths[0], m.widths[1])
	}
}

func TestColumnEdgesDefaultSep(t *testing.T) {
	// Three 10-wide columns separated by a single space → edges at 0, 11, 22.
	m := New(Options{
		Width:         60,
		Height:        5,
		Columns:       []Column{{Title: "a", Width: 10}, {Title: "b", Width: 10}, {Title: "c", Width: 10}},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	got := m.columnEdges()
	want := []int{0, 11, 22}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("columnEdges = %v, want %v", got, want)
	}
}

func TestColumnEdgesVerticalSep(t *testing.T) {
	// Vertical sep is " │ " — visible width 3. Edges at 0, 10+3=13, 13+10+3=26.
	m := New(Options{
		Width:         60,
		Height:        5,
		Columns:       []Column{{Title: "a", Width: 10}, {Title: "b", Width: 10}, {Title: "c", Width: 10}},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
		Borders:       Borders{Vertical: "│"},
	})
	got := m.columnEdges()
	want := []int{0, 13, 26}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("columnEdges = %v, want %v", got, want)
	}
}

func TestNextColumnEdge(t *testing.T) {
	m := New(Options{
		Width:         60,
		Height:        5,
		Columns:       []Column{{Title: "a", Width: 10}, {Title: "b", Width: 10}, {Title: "c", Width: 10}},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	if got := m.nextColumnEdge(0); got != 11 {
		t.Errorf("nextColumnEdge(0) = %d, want 11", got)
	}
	if got := m.nextColumnEdge(11); got != 22 {
		t.Errorf("nextColumnEdge(11) = %d, want 22", got)
	}
	if got := m.nextColumnEdge(22); got != -1 {
		t.Errorf("nextColumnEdge(22) = %d, want -1", got)
	}
	if got := m.nextColumnEdge(5); got != 11 {
		t.Errorf("nextColumnEdge(5) = %d, want 11", got)
	}
}

func TestPrevColumnEdge(t *testing.T) {
	m := New(Options{
		Width:         60,
		Height:        5,
		Columns:       []Column{{Title: "a", Width: 10}, {Title: "b", Width: 10}, {Title: "c", Width: 10}},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	if got := m.prevColumnEdge(22); got != 11 {
		t.Errorf("prevColumnEdge(22) = %d, want 11", got)
	}
	if got := m.prevColumnEdge(11); got != 0 {
		t.Errorf("prevColumnEdge(11) = %d, want 0", got)
	}
	if got := m.prevColumnEdge(0); got != -1 {
		t.Errorf("prevColumnEdge(0) = %d, want -1", got)
	}
	if got := m.prevColumnEdge(15); got != 11 {
		t.Errorf("prevColumnEdge(15) = %d, want 11", got)
	}
}

func TestColumnStepKeysSnapToBoundaries(t *testing.T) {
	// Wide enough to need h-scroll. Three 30-wide columns in a 40-wide outer.
	m := New(Options{
		Width:         40,
		Height:        5,
		Columns:       []Column{{Title: "a", Width: 30}, {Title: "b", Width: 30}, {Title: "c", Width: 30}},
		Rows:          []Row{{"a1", "b1", "c1"}},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	// shift+right should snap to the next column edge (31).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	if got := m.body.XOffset(); got != 31 {
		t.Errorf("after shift+right, XOffset = %d, want 31", got)
	}
	// Another shift+right should snap to 62 — but MaxXOffset will clamp it
	// if the content fits past that.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	want := min2(62, m.body.MaxXOffset())
	if got := m.body.XOffset(); got != want {
		t.Errorf("after second shift+right, XOffset = %d, want %d", got, want)
	}
	// shift+left from here should snap to the previous edge (31).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	if got := m.body.XOffset(); got != 31 {
		t.Errorf("after shift+left, XOffset = %d, want 31", got)
	}
	// shift+left again → 0.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	if got := m.body.XOffset(); got != 0 {
		t.Errorf("after second shift+left, XOffset = %d, want 0", got)
	}
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestSetDimensionsRecomputesFlex(t *testing.T) {
	m := New(Options{
		Width:         32,
		Height:        5,
		Columns: []Column{
			{Title: "fixed", Width: 10},
			{Title: "flex", Flex: 1},
		},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	w0 := m.widths[1]
	m.SetDimensions(60, 5)
	if m.widths[1] <= w0 {
		t.Errorf("flex width after resize = %d, want > %d", m.widths[1], w0)
	}
}

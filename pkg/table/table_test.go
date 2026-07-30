package table

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jsdrews/tuilib/pkg/geom"
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
		Width:  32, // outer: 32 - 2 borders - 1 scrollbar = 29 inner
		Height: 5,
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
		Width:  42, // inner ~ 39
		Height: 5,
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
	totalSep := 2 * 1                        // 2 inter-column gaps, single space
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
		Width:  10,
		Height: 5,
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
		Width:  80,
		Height: 5,
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
		Width:  80, // inner ~ 77
		Height: 5,
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
		Width:  80,
		Height: 5,
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
		Width:  32,
		Height: 5,
		Columns: []Column{
			{Title: "fixed", Width: 10},
			{Title: "flex", Flex: 1},
		},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	w0 := m.widths[1]
	m.SetRect(geom.Rect{W: 60, H: 5})
	if m.widths[1] <= w0 {
		t.Errorf("flex width after resize = %d, want > %d", m.widths[1], w0)
	}
}

// drainViewportMsg runs cmd and returns the first ViewportChangedMsg it
// produces, or nil if none. Handles both single msgs and tea.BatchMsg by
// invoking each sub-cmd in turn.
func drainViewportMsg(cmd tea.Cmd) *ViewportChangedMsg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if vp, ok := msg.(ViewportChangedMsg); ok {
		return &vp
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range batch {
			if vp := drainViewportMsg(sub); vp != nil {
				return vp
			}
		}
	}
	return nil
}

func newViewportTable(rows int, height int) Model {
	rr := make([]Row, rows)
	for i := range rr {
		rr[i] = Row{"x"}
	}
	return New(Options{
		Width:         20,
		Height:        height,
		Columns:       []Column{{Title: "a", Width: 4}},
		Rows:          rr,
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
}

func TestViewportChangedFiresOnInit(t *testing.T) {
	m := newViewportTable(20, 8)
	if m.dataRows() <= 0 {
		t.Skip("no dataRows in test environment")
	}
	if !m.vpPending {
		t.Fatal("expected viewport pending after New")
	}
	// First Update flushes the pending viewport as a msg.
	_, cmd := m.Update(struct{}{})
	vp := drainViewportMsg(cmd)
	if vp == nil {
		t.Fatal("expected ViewportChangedMsg on first Update, got none")
	}
	if vp.FirstVisible != 0 {
		t.Errorf("FirstVisible = %d, want 0", vp.FirstVisible)
	}
	if vp.TotalRows != 20 {
		t.Errorf("TotalRows = %d, want 20", vp.TotalRows)
	}
	if vp.LastVisible < vp.FirstVisible {
		t.Errorf("LastVisible=%d < FirstVisible=%d", vp.LastVisible, vp.FirstVisible)
	}
}

func TestViewportChangedNoRepeatOnUnchangedViewport(t *testing.T) {
	m := newViewportTable(20, 8)
	if m.dataRows() <= 0 {
		t.Skip("no dataRows in test environment")
	}
	m, _ = m.Update(struct{}{}) // drain initial
	// Second Update with no state change should not re-emit.
	_, cmd := m.Update(struct{}{})
	if vp := drainViewportMsg(cmd); vp != nil {
		t.Errorf("unexpected re-emit: %+v", vp)
	}
}

func TestViewportChangedOnScroll(t *testing.T) {
	m := newViewportTable(50, 8)
	if m.dataRows() <= 0 {
		t.Skip("no dataRows in test environment")
	}
	m, _ = m.Update(struct{}{}) // drain initial
	// Jump to bottom guarantees a viewport shift on any pane height.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	vp := drainViewportMsg(cmd)
	if vp == nil {
		t.Fatal("expected ViewportChangedMsg after G, got none")
	}
	if vp.FirstVisible == 0 {
		t.Errorf("FirstVisible unchanged after scroll: %d", vp.FirstVisible)
	}
	if vp.LastVisible != vp.TotalRows-1 {
		t.Errorf("LastVisible=%d, want TotalRows-1=%d", vp.LastVisible, vp.TotalRows-1)
	}
}

func TestViewportChangedOnSetRows(t *testing.T) {
	m := newViewportTable(20, 8)
	if m.dataRows() <= 0 {
		t.Skip("no dataRows in test environment")
	}
	m, _ = m.Update(struct{}{}) // drain initial
	// SetRows changes total; the next Update should carry the msg.
	m.SetRows([]Row{{"a"}, {"b"}, {"c"}})
	_, cmd := m.Update(struct{}{})
	vp := drainViewportMsg(cmd)
	if vp == nil {
		t.Fatal("expected ViewportChangedMsg after SetRows, got none")
	}
	if vp.TotalRows != 3 {
		t.Errorf("TotalRows = %d, want 3", vp.TotalRows)
	}
}

func TestViewportChangedSuppressedUntilDimensionsApplied(t *testing.T) {
	// Height 0 → dataRows == 0 → viewport not real → no emission.
	m := newViewportTable(20, 0)
	if m.vpPending {
		t.Errorf("viewport should not be pending when dataRows==0")
	}
	_, cmd := m.Update(struct{}{})
	if vp := drainViewportMsg(cmd); vp != nil {
		t.Errorf("unexpected emit with zero dataRows: %+v", vp)
	}
	// Applying dimensions makes the viewport real → next Update carries msg.
	m.SetRect(geom.Rect{W: 20, H: 8})
	if m.dataRows() <= 0 {
		t.Skip("no dataRows even after SetDimensions in test environment")
	}
	_, cmd = m.Update(struct{}{})
	if vp := drainViewportMsg(cmd); vp == nil {
		t.Fatal("expected ViewportChangedMsg after SetDimensions, got none")
	}
}

// drainRowFocusedMsg pulls the first RowFocusedMsg out of a Cmd (single
// or batched), mirroring drainViewportMsg.
func drainRowFocusedMsg(cmd tea.Cmd) *RowFocusedMsg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if rf, ok := msg.(RowFocusedMsg); ok {
		return &rf
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range batch {
			if rf := drainRowFocusedMsg(sub); rf != nil {
				return rf
			}
		}
	}
	return nil
}

func newFocusTable(rows []Row) Model {
	return New(Options{
		Width:         20,
		Height:        8,
		Columns:       []Column{{Title: "name", Width: 6}, {Title: "value", Width: 6}},
		Rows:          rows,
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
}

func TestRowFocusedFiresOnInit(t *testing.T) {
	m := newFocusTable([]Row{{"a", "1"}, {"b", "2"}, {"c", "3"}})
	_, cmd := m.Update(struct{}{})
	rf := drainRowFocusedMsg(cmd)
	if rf == nil {
		t.Fatal("expected RowFocusedMsg on first Update, got none")
	}
	if rf.Empty {
		t.Errorf("Empty=true on non-empty init")
	}
	if rf.Row != 0 {
		t.Errorf("Row = %d, want 0", rf.Row)
	}
	if len(rf.Cells) != 2 || rf.Cells[0] != "a" || rf.Cells[1] != "1" {
		t.Errorf("Cells = %v, want [a 1]", rf.Cells)
	}
	if len(rf.Columns) != 2 || rf.Columns[0] != "name" || rf.Columns[1] != "value" {
		t.Errorf("Columns = %v, want [name value]", rf.Columns)
	}
}

func TestRowFocusedNoRepeatOnUnchanged(t *testing.T) {
	m := newFocusTable([]Row{{"a", "1"}, {"b", "2"}})
	m, _ = m.Update(struct{}{}) // drain init
	_, cmd := m.Update(struct{}{})
	if rf := drainRowFocusedMsg(cmd); rf != nil {
		t.Errorf("unexpected re-emit: %+v", rf)
	}
}

func TestRowFocusedOnCursorMove(t *testing.T) {
	m := newFocusTable([]Row{{"a", "1"}, {"b", "2"}, {"c", "3"}})
	m, _ = m.Update(struct{}{}) // drain init
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	rf := drainRowFocusedMsg(cmd)
	if rf == nil {
		t.Fatal("expected RowFocusedMsg after cursor down, got none")
	}
	if rf.Row != 1 || rf.Cells[0] != "b" {
		t.Errorf("focused = row %d %v, want row 1 [b 2]", rf.Row, rf.Cells)
	}
}

func TestRowFocusedOnSetRowsChangesContent(t *testing.T) {
	m := newFocusTable([]Row{{"a", "1"}, {"b", "2"}})
	m, _ = m.Update(struct{}{}) // drain init (row 0 = "a")
	// SetRows with different content at row 0 should re-emit.
	m.SetRows([]Row{{"z", "9"}, {"b", "2"}})
	_, cmd := m.Update(struct{}{})
	rf := drainRowFocusedMsg(cmd)
	if rf == nil {
		t.Fatal("expected RowFocusedMsg after SetRows changing focused row")
	}
	if rf.Cells[0] != "z" {
		t.Errorf("Cells[0] = %q, want z", rf.Cells[0])
	}
}

func TestRowFocusedNoRepeatWhenSetRowsKeepsSameContent(t *testing.T) {
	m := newFocusTable([]Row{{"a", "1"}, {"b", "2"}})
	m, _ = m.Update(struct{}{}) // drain init
	// SetRows with same cells at row 0 should NOT re-emit focus.
	m.SetRows([]Row{{"a", "1"}, {"b", "2"}})
	_, cmd := m.Update(struct{}{})
	if rf := drainRowFocusedMsg(cmd); rf != nil {
		t.Errorf("unexpected re-emit when focused row unchanged: %+v", rf)
	}
}

func TestRowFocusedEmitsEmptyOnTransitionToNoRows(t *testing.T) {
	m := newFocusTable([]Row{{"a", "1"}})
	m, _ = m.Update(struct{}{}) // drain init
	m.SetRows(nil)
	_, cmd := m.Update(struct{}{})
	rf := drainRowFocusedMsg(cmd)
	if rf == nil {
		t.Fatal("expected RowFocusedMsg{Empty:true} after SetRows(nil)")
	}
	if !rf.Empty {
		t.Errorf("Empty = %v, want true; msg = %+v", rf.Empty, rf)
	}
	if rf.Cells != nil || rf.Columns != nil {
		t.Errorf("Cells/Columns should be nil on Empty, got %v / %v", rf.Cells, rf.Columns)
	}
}

func TestRowFocusedSuppressedOnEmptyInit(t *testing.T) {
	// A table constructed with no rows shouldn't emit an initial Empty
	// message — subscribers only care about transitions.
	m := newFocusTable(nil)
	_, cmd := m.Update(struct{}{})
	if rf := drainRowFocusedMsg(cmd); rf != nil {
		t.Errorf("unexpected initial emit for empty table: %+v", rf)
	}
}

func TestHiddenColumnNotRendered(t *testing.T) {
	m := New(Options{
		Width:  40,
		Height: 6,
		Columns: []Column{
			{Title: "Name", Width: 8},
			{Title: "Namespace", Width: 12, Hidden: true},
			{Title: "Status", Width: 8},
		},
		Rows:          []Row{{"nginx", "default", "Running"}},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	view := m.View()
	if strings.Contains(view, "Namespace") {
		t.Errorf("hidden column title 'Namespace' rendered: %q", view)
	}
	if strings.Contains(view, "default") {
		t.Errorf("hidden column cell 'default' rendered: %q", view)
	}
	if !strings.Contains(view, "nginx") {
		t.Errorf("visible cell 'nginx' missing: %q", view)
	}
	if !strings.Contains(view, "Running") {
		t.Errorf("visible cell 'Running' missing: %q", view)
	}
}

func TestHiddenColumnWidthIsZero(t *testing.T) {
	m := New(Options{
		Width:  40,
		Height: 5,
		Columns: []Column{
			{Title: "A", Width: 10},
			{Title: "B", Width: 10, Hidden: true},
			{Title: "C", Width: 10},
		},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	if m.widths[1] != 0 {
		t.Errorf("hidden column width = %d, want 0", m.widths[1])
	}
	if m.widths[0] != 10 || m.widths[2] != 10 {
		t.Errorf("visible widths = [%d, _, %d], want [10, _, 10]", m.widths[0], m.widths[2])
	}
}

func TestHiddenColumnParticipatesInFilter(t *testing.T) {
	// key:value scoped to a hidden column should still match — the
	// canonical downstream use case (filter pods by hidden namespace).
	m := New(Options{
		Width:      40,
		Height:     10,
		Filterable: true,
		Columns: []Column{
			{Title: "Name", Width: 8},
			{Title: "Namespace", Width: 12, Hidden: true},
		},
		Rows: []Row{
			{"nginx", "default"},
			{"redis", "cache"},
			{"api", "default"},
		},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	m.SetValue("namespace:default")
	if len(m.visible) != 2 {
		t.Fatalf("visible count = %d, want 2 (namespace:default should hit 2 rows)", len(m.visible))
	}
	if m.visible[0][0] != "nginx" || m.visible[1][0] != "api" {
		t.Errorf("filter result = %v, want [nginx api]", m.visible)
	}
}

func TestHiddenColumnBareFilterMatchesHiddenCell(t *testing.T) {
	// A bare term (no key:) still scans every cell in the row including
	// hidden ones, so "cache" filters down to the redis row via the
	// hidden Namespace column.
	m := New(Options{
		Width:      40,
		Height:     10,
		Filterable: true,
		Columns: []Column{
			{Title: "Name", Width: 8},
			{Title: "Namespace", Width: 12, Hidden: true},
		},
		Rows: []Row{
			{"nginx", "default"},
			{"redis", "cache"},
		},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	m.SetValue("cache")
	if len(m.visible) != 1 || m.visible[0][0] != "redis" {
		t.Errorf("bare 'cache' filter = %v, want [redis, cache]", m.visible)
	}
}

func TestHiddenColumnAppearsInRowFocusedMsg(t *testing.T) {
	m := New(Options{
		Width:  40,
		Height: 8,
		Columns: []Column{
			{Title: "Name", Width: 8},
			{Title: "Namespace", Width: 12, Hidden: true},
		},
		Rows: []Row{
			{"nginx", "default"},
		},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	_, cmd := m.Update(struct{}{})
	rf := drainRowFocusedMsg(cmd)
	if rf == nil {
		t.Fatal("expected RowFocusedMsg on init")
	}
	if len(rf.Cells) != 2 || rf.Cells[1] != "default" {
		t.Errorf("Cells = %v, want hidden namespace 'default' at [1]", rf.Cells)
	}
	if len(rf.Columns) != 2 || rf.Columns[1] != "Namespace" {
		t.Errorf("Columns = %v, want hidden 'Namespace' at [1]", rf.Columns)
	}
}

func TestHiddenColumnFlexDistributionIgnoresHidden(t *testing.T) {
	// Two visible columns + one hidden flex column. Hidden shouldn't
	// consume any horizontal space; the visible flex should grow into
	// the full leftover.
	m := New(Options{
		Width:  40,
		Height: 5,
		Columns: []Column{
			{Title: "fixed", Width: 10},
			{Title: "hidden-flex", Flex: 5, Hidden: true},
			{Title: "flex", Flex: 1},
		},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	inner := m.body.VisibleWidth()
	if inner <= 0 {
		t.Skip("no inner width in test environment")
	}
	if m.widths[1] != 0 {
		t.Errorf("hidden flex width = %d, want 0", m.widths[1])
	}
	// The visible flex should have absorbed all leftover after the fixed
	// column + one separator (only visible columns count towards seps).
	want := inner - 10 - 1
	if m.widths[2] != want {
		t.Errorf("visible flex width = %d, want %d (inner=%d, hidden ignored)", m.widths[2], want, inner)
	}
}

func TestHiddenColumnEdgesSkipHidden(t *testing.T) {
	// columnEdges drives shift+left/right snapping — hidden columns
	// shouldn't appear in the snap targets.
	m := New(Options{
		Width:  60,
		Height: 5,
		Columns: []Column{
			{Title: "a", Width: 10},
			{Title: "b", Width: 10, Hidden: true},
			{Title: "c", Width: 10},
		},
		HeaderStyle:   lipgloss.NewStyle(),
		SelectedStyle: lipgloss.NewStyle(),
	})
	edges := m.columnEdges()
	// Only two visible columns: edges at 0 and 11 (10 + 1 sep).
	if len(edges) != 2 {
		t.Fatalf("columnEdges = %v, want 2 entries (hidden skipped)", edges)
	}
	if edges[0] != 0 || edges[1] != 11 {
		t.Errorf("edges = %v, want [0 11]", edges)
	}
}

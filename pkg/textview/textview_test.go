package textview

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/geom"
)

// newTV builds a textview for tests. Width/height are pane outer dims.
func newTV(content string, width, height int, searchable, wrap bool) Model {
	return New(Options{
		Width:      width,
		Height:     height,
		Title:      "test",
		Content:    content,
		Wrap:       wrap,
		Searchable: searchable,
		// Non-empty MatchStyle so match rendering emits ANSI escapes we can
		// detect from tests.
		MatchStyle: lipgloss.NewStyle().Reverse(true),
		Filter:     filter.Options{Width: width, Placeholder: "search"},
	})
}

func TestNewSplitsContentIntoLines(t *testing.T) {
	m := newTV("a\nb\nc", 20, 10, false, false)
	if got, want := len(m.lines), 3; got != want {
		t.Errorf("line count = %d, want %d (m.lines=%q)", got, want, m.lines)
	}
}

func TestWrapWrapsLongLinesToInnerWidth(t *testing.T) {
	long := strings.Repeat("word ", 20) // 100 chars
	// pane inner width = width - 3 (2 border + 1 scrollbar).
	m := newTV(strings.TrimRight(long, " "), 30, 10, false, true)
	inner := m.body.VisibleWidth()
	if inner < 5 {
		t.Fatalf("unexpected inner width %d", inner)
	}
	for i, ln := range m.lines {
		if got := runeLen(ln); got > inner {
			t.Errorf("wrapped line %d width %d > inner %d: %q", i, got, inner, ln)
		}
	}
	if len(m.lines) < 2 {
		t.Errorf("expected multiple wrapped lines, got %d", len(m.lines))
	}
}

func TestWrapOffPreservesRawLines(t *testing.T) {
	long := strings.Repeat("word ", 20)
	m := newTV(strings.TrimRight(long, " "), 30, 10, false, false)
	if got, want := len(m.lines), 1; got != want {
		t.Errorf("wrap-off line count = %d, want %d", got, want)
	}
}

func TestSetContentReplacesAndResetsScroll(t *testing.T) {
	m := newTV(strings.Repeat("line\n", 50), 40, 10, false, false)
	m.body.SetYOffset(30)
	if m.body.YOffset() != 30 {
		t.Fatalf("pre-set YOffset = %d, want 30", m.body.YOffset())
	}
	m.SetContent("fresh\ncontent")
	if got := m.body.YOffset(); got != 0 {
		t.Errorf("post-set YOffset = %d, want 0 (reset to top)", got)
	}
	if got, want := len(m.lines), 2; got != want {
		t.Errorf("post-set line count = %d, want %d", got, want)
	}
	if m.raw != "fresh\ncontent" {
		t.Errorf("Content() = %q, want %q", m.Content(), "fresh\ncontent")
	}
}

func TestSetWrapReflowsAndRecomputesMatches(t *testing.T) {
	long := strings.Repeat("word ", 20)
	m := newTV(strings.TrimRight(long, " "), 30, 10, false, true)
	before := len(m.lines)
	m.SetWrap(false)
	if len(m.lines) >= before {
		t.Errorf("wrap-off should collapse lines: before=%d after=%d", before, len(m.lines))
	}
	m.SetWrap(true)
	if len(m.lines) != before {
		t.Errorf("re-wrap should restore line count: before=%d now=%d", before, len(m.lines))
	}
}

func TestWrapToggleKey(t *testing.T) {
	m := newTV(strings.Repeat("word ", 30), 30, 10, false, true)
	wrapped := len(m.lines)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	if m.wrap {
		t.Errorf("w should toggle wrap off; still on")
	}
	if len(m.lines) >= wrapped {
		t.Errorf("wrap-off should collapse lines: wrapped=%d now=%d", wrapped, len(m.lines))
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	if !m.wrap {
		t.Errorf("w should toggle wrap back on")
	}
}

func TestGTopBottomKeys(t *testing.T) {
	// Enough lines that scroll actually moves.
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line " + itoa(i)
	}
	m := newTV(strings.Join(lines, "\n"), 40, 10, false, false)
	// G → bottom.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	// Body should scroll away from top.
	if m.body.YOffset() == 0 {
		t.Errorf("G should scroll away from top, YOffset stayed 0")
	}
	// g → top.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if m.body.YOffset() != 0 {
		t.Errorf("g should return to top, YOffset = %d", m.body.YOffset())
	}
}

func TestSearchFocusesFilterOnSlash(t *testing.T) {
	m := newTV("apple\nbanana\ncherry", 40, 10, true, false)
	if m.filter.Focused() {
		t.Fatal("filter should start unfocused")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.filter.Focused() {
		t.Errorf("/ should focus filter")
	}
	if !m.Searching() {
		t.Errorf("Searching() should be true while filter focused")
	}
}

func TestSearchFindsMatchesAcrossLines(t *testing.T) {
	m := newTV("apple\nbanana\ncherry apple\napricot", 40, 10, true, false)
	m.SetQuery("apple")
	if len(m.matches) != 2 {
		t.Errorf("matches = %d, want 2", len(m.matches))
	}
	if m.matchIdx != 0 {
		t.Errorf("matchIdx after SetQuery = %d, want 0", m.matchIdx)
	}
	// Match offsets should land on the correct byte positions.
	if got, want := m.matches[0].line, 0; got != want {
		t.Errorf("matches[0].line = %d, want %d", got, want)
	}
	if got, want := m.matches[0].start, 0; got != want {
		t.Errorf("matches[0].start = %d, want %d", got, want)
	}
	if got, want := m.matches[1].line, 2; got != want {
		t.Errorf("matches[1].line = %d, want %d", got, want)
	}
	if got, want := m.matches[1].start, 7; got != want {
		t.Errorf("matches[1].start = %d, want %d", got, want)
	}
}

func TestNextPrevMatchWrapAround(t *testing.T) {
	m := newTV("aaa\nbbb\naaa\nccc\naaa", 40, 10, true, false)
	m.SetQuery("aaa")
	if len(m.matches) != 3 {
		t.Fatalf("matches = %d, want 3", len(m.matches))
	}
	// Step forward twice.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.matchIdx != 1 {
		t.Errorf("after n: matchIdx = %d, want 1", m.matchIdx)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.matchIdx != 2 {
		t.Errorf("after n: matchIdx = %d, want 2", m.matchIdx)
	}
	// Wrap around forward.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.matchIdx != 0 {
		t.Errorf("after n wrap: matchIdx = %d, want 0", m.matchIdx)
	}
	// Wrap around backward.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	if m.matchIdx != 2 {
		t.Errorf("after N wrap: matchIdx = %d, want 2", m.matchIdx)
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	m := newTV("Apple\nBANANA\ncherry", 40, 10, true, false)
	m.SetQuery("apple")
	if len(m.matches) != 1 {
		t.Errorf("case-insensitive apple: matches = %d, want 1", len(m.matches))
	}
	m.SetQuery("banana")
	if len(m.matches) != 1 {
		t.Errorf("case-insensitive banana: matches = %d, want 1", len(m.matches))
	}
}

func TestSearchPreservedAcrossSetContent(t *testing.T) {
	m := newTV("apple", 40, 10, true, false)
	m.SetQuery("apple")
	if len(m.matches) != 1 {
		t.Fatalf("initial matches = %d, want 1", len(m.matches))
	}
	// Query stays; matches recompute against new content.
	m.SetContent("apple\napple\napple")
	if len(m.matches) != 3 {
		t.Errorf("post-SetContent matches = %d, want 3", len(m.matches))
	}
	if m.matchIdx != 0 {
		t.Errorf("matchIdx after SetContent = %d, want 0 (reset to first)", m.matchIdx)
	}
}

func TestSetDimensionsRewrapsWhenWrapOn(t *testing.T) {
	msg := strings.Repeat("word ", 30) // 150 chars
	m := newTV(strings.TrimRight(msg, " "), 40, 10, false, true)
	linesWide := len(m.lines)
	m.SetRect(geom.Rect{W: 20, H: 10})
	if len(m.lines) <= linesWide {
		t.Errorf("shrink should produce more wrapped lines: was %d, now %d", linesWide, len(m.lines))
	}
	m.SetRect(geom.Rect{W: 40, H: 10})
	if len(m.lines) != linesWide {
		t.Errorf("grow back should restore line count: was %d, now %d", linesWide, len(m.lines))
	}
}

func TestSearchableFalseIgnoresSlash(t *testing.T) {
	m := newTV("apple", 40, 10, false, false)
	before := m.filter.Focused()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if m.filter.Focused() != before {
		t.Errorf("/ should be inert when Searchable=false")
	}
	if m.Searching() {
		t.Errorf("Searching() should be false when Searchable=false")
	}
	// SetQuery is a no-op.
	m.SetQuery("apple")
	if len(m.matches) != 0 {
		t.Errorf("SetQuery on non-searchable should be no-op; got %d matches", len(m.matches))
	}
}

func TestHelpIncludesSearchBindingsWhenActive(t *testing.T) {
	// Non-searchable: no search bindings.
	m := newTV("hi", 40, 10, false, false)
	nonSearchable := m.Help()
	// Searchable but no query: Search binding shown, no n/N.
	m = newTV("hi", 40, 10, true, false)
	searchable := m.Help()
	if len(searchable) <= len(nonSearchable) {
		t.Errorf("searchable Help() should include Search binding, got %d vs %d", len(searchable), len(nonSearchable))
	}
	// After a query is set, n/N should appear.
	m.SetQuery("h")
	withQuery := m.Help()
	if len(withQuery) <= len(searchable) {
		t.Errorf("Help() with query should include n/N, got %d vs %d", len(withQuery), len(searchable))
	}
}

func TestSetContentEmptyClearsMatches(t *testing.T) {
	m := newTV("apple\napple", 40, 10, true, false)
	m.SetQuery("apple")
	if len(m.matches) == 0 {
		t.Fatal("expected matches before clear")
	}
	m.SetContent("")
	if len(m.matches) != 0 {
		t.Errorf("empty content should clear matches, got %d", len(m.matches))
	}
	if m.matchIdx != -1 {
		t.Errorf("matchIdx after empty content = %d, want -1", m.matchIdx)
	}
}

// runeLen returns the visible character count of s, ignoring ANSI escapes.
func runeLen(s string) int { return len([]rune(s)) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

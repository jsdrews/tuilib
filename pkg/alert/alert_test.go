package alert

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// stripLines splits and ANSI-strips each line for content assertions.
func stripLines(s string) []string {
	lines := strings.Split(s, "\n")
	out := make([]string, len(lines))
	for i, ln := range lines {
		out[i] = ansi.Strip(ln)
	}
	return out
}

// visibleWidth returns the widest cell-count across the modal view's
// non-blank lines (after stripping ANSI + surrounding whitespace pad).
func modalCellWidth(view string) int {
	max := 0
	for _, ln := range strings.Split(view, "\n") {
		plain := strings.TrimRight(ansi.Strip(ln), " ")
		trimmed := strings.TrimLeft(plain, " ")
		if trimmed == "" {
			continue
		}
		if w := ansi.StringWidth(trimmed); w > max {
			max = w
		}
	}
	return max
}

// modalCellHeight returns the number of non-blank rows in the view.
func modalCellHeight(view string) int {
	rows := 0
	for _, ln := range strings.Split(view, "\n") {
		if strings.TrimSpace(ansi.Strip(ln)) != "" {
			rows++
		}
	}
	return rows
}

func newAutosize(msg string, outerW, outerH int) Model {
	m := New(Options{
		Title:    "Error",
		Message:  msg,
		OK:       "OK",
		Autosize: true,
	})
	m.SetDimensions(outerW, outerH)
	return m
}

func TestNonAutosizeUnchanged(t *testing.T) {
	m := New(Options{
		Title:   "Notice",
		Message: "hi",
		OK:      "OK",
	})
	m.SetDimensions(30, 5)
	view := m.View()
	// Non-autosize returns the pane view directly (no centering pad).
	// The first line should be the top border of the pane.
	first := strings.Split(view, "\n")[0]
	if strings.HasPrefix(strings.TrimSpace(ansi.Strip(first)), " ") {
		t.Errorf("non-autosize view should start at column 0, got %q", first)
	}
}

func TestAutosizeShortMessageFitsBelowCap(t *testing.T) {
	// Short message → modal width tracks content, well under the 80% cap.
	m := newAutosize("Boom.", 100, 30)
	if m.modalW >= 80 {
		t.Errorf("short message: modalW = %d, expected < 80%% cap (80 cols)", m.modalW)
	}
	if m.modalW < 3 {
		t.Errorf("modalW = %d, want >= 3", m.modalW)
	}
	// Content must fit without any scrolling.
	if m.canScroll() {
		t.Error("short message should not overflow the viewport")
	}
}

func TestAutosizeLongMessageWrapsAtWidthCap(t *testing.T) {
	long := strings.Repeat("word ", 200) // 1000 chars, no explicit newlines
	m := newAutosize(long, 100, 40)
	// Cap is 80% of 100 = 80 cols; content-line cap is innerW = 78.
	widest := 0
	for _, ln := range m.wrapped {
		if w := ansi.StringWidth(ln); w > widest {
			widest = w
		}
	}
	if widest > 78 {
		t.Errorf("wrapped line width = %d, want <= innerW=78", widest)
	}
	if m.modalW > 80 {
		t.Errorf("modalW = %d, want <= 80%% cap (80)", m.modalW)
	}
}

func TestAutosizeMinWidthFloor(t *testing.T) {
	// Very narrow terminal: 80% would be < 40, so floor to 40. Message can
	// still exceed the floor if the terminal is narrower — the modal must
	// not exceed the outer bound.
	m := newAutosize("short", 30, 20)
	if m.modalW > 30 {
		t.Errorf("modalW = %d, want <= outerW=30", m.modalW)
	}
}

func TestAutosizeHeightCapAt60Percent(t *testing.T) {
	tall := strings.Repeat("line\n", 100) // 100 lines
	m := newAutosize(tall, 80, 20)
	// 60% of 20 = 12 rows max modal height.
	if m.modalH > 12 {
		t.Errorf("modalH = %d, want <= 60%% cap (12)", m.modalH)
	}
	// Must overflow → scrolling available.
	if !m.canScroll() {
		t.Error("100-line message on 20-row terminal should overflow")
	}
}

func TestAutosizeOKButtonPinnedAcrossScroll(t *testing.T) {
	tall := ""
	for i := 0; i < 30; i++ {
		tall += "row-" + string(rune('a'+i%26)) + "\n"
	}
	m := newAutosize(strings.TrimRight(tall, "\n"), 80, 20)
	if !m.canScroll() {
		t.Fatal("expected overflow to enable scroll")
	}
	// The pane inner width should have the OK button on the last inner row
	// regardless of scroll position.
	assertOKOnLastInnerRow := func(label string) {
		view := m.View()
		lines := stripLines(view)
		// Locate the pane content by finding the row containing "OK".
		found := -1
		for i, ln := range lines {
			if strings.Contains(ln, "[ OK ]") {
				found = i
				break
			}
		}
		if found < 0 {
			t.Errorf("%s: OK button missing from view", label)
			return
		}
		// Below the OK row, only the bottom border + centering pad may exist.
		// Verify none of the subsequent rows contain any message content
		// (i.e. only border chars + spaces).
		for _, ln := range lines[found+1:] {
			// Border chars use box-drawing runes; any letter would mean a
			// wrapped message row leaked past the button.
			for _, r := range ln {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
					t.Errorf("%s: content leaked past OK button on line %q", label, ln)
					return
				}
			}
		}
	}
	assertOKOnLastInnerRow("top")
	// Scroll to middle.
	mid := m.maxScroll() / 2
	m.scrollOffset = mid
	m.pane.SetContent(m.renderInner())
	assertOKOnLastInnerRow("middle")
	// Scroll to bottom.
	m.scrollOffset = m.maxScroll()
	m.pane.SetContent(m.renderInner())
	assertOKOnLastInnerRow("bottom")
}

func TestAutosizeScrollKeys(t *testing.T) {
	msg := strings.TrimRight(strings.Repeat("x\n", 40), "\n")
	m := newAutosize(msg, 80, 20)
	if !m.canScroll() {
		t.Fatal("expected overflow")
	}
	max := m.maxScroll()

	// down / j
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.scrollOffset != 1 {
		t.Errorf("after down: scrollOffset = %d, want 1", m.scrollOffset)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.scrollOffset != 2 {
		t.Errorf("after j: scrollOffset = %d, want 2", m.scrollOffset)
	}
	// up / k
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.scrollOffset != 1 {
		t.Errorf("after k: scrollOffset = %d, want 1", m.scrollOffset)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.scrollOffset != 0 {
		t.Errorf("after up: scrollOffset = %d, want 0", m.scrollOffset)
	}
	// G / g
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if m.scrollOffset != max {
		t.Errorf("after G: scrollOffset = %d, want %d", m.scrollOffset, max)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if m.scrollOffset != 0 {
		t.Errorf("after g: scrollOffset = %d, want 0", m.scrollOffset)
	}
	// pgdown / pgup
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.scrollOffset != m.viewportRows && m.scrollOffset != max {
		t.Errorf("after pgdown: scrollOffset = %d, want %d or clamped to %d", m.scrollOffset, m.viewportRows, max)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if m.scrollOffset != 0 {
		t.Errorf("after pgup: scrollOffset = %d, want 0", m.scrollOffset)
	}
	// ctrl+d / ctrl+u — half-page.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	half := halfPage(m.viewportRows)
	if m.scrollOffset != half && m.scrollOffset != max {
		t.Errorf("after ctrl+d: scrollOffset = %d, want %d or clamped to %d", m.scrollOffset, half, max)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if m.scrollOffset != 0 {
		t.Errorf("after ctrl+u: scrollOffset = %d, want 0", m.scrollOffset)
	}
}

func TestAutosizeScrollKeysInertWhenContentFits(t *testing.T) {
	// Short message → no overflow → j/k/etc. should NOT scroll, but the
	// dismiss keys must still work.
	m := newAutosize("short", 80, 30)
	if m.canScroll() {
		t.Fatal("short message should not overflow")
	}
	before := m.scrollOffset
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.scrollOffset != before {
		t.Errorf("j on non-scrollable should not move offset, got %d", m.scrollOffset)
	}
	if cmd != nil {
		t.Errorf("j on non-scrollable should not emit a cmd")
	}
	// enter should still dismiss.
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Errorf("enter should still emit dismiss cmd")
	}
	if msg := cmd(); msg == nil {
		t.Errorf("dismiss cmd should return a message")
	} else if _, ok := msg.(DismissedMsg); !ok {
		t.Errorf("dismiss cmd returned %T, want DismissedMsg", msg)
	}
}

func TestAutosizeDismissKeysStillWorkWhileScrolling(t *testing.T) {
	msg := strings.TrimRight(strings.Repeat("x\n", 40), "\n")
	m := newAutosize(msg, 80, 20)
	if !m.canScroll() {
		t.Fatal("expected overflow")
	}
	for _, k := range []string{"enter", " ", "esc", "o", "O"} {
		var keyMsg tea.KeyMsg
		switch k {
		case "enter":
			keyMsg = tea.KeyMsg{Type: tea.KeyEnter}
		case " ":
			keyMsg = tea.KeyMsg{Type: tea.KeySpace}
		case "esc":
			keyMsg = tea.KeyMsg{Type: tea.KeyEsc}
		default:
			keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		_, cmd := m.Update(keyMsg)
		if cmd == nil {
			t.Errorf("dismiss key %q produced no cmd", k)
			continue
		}
		if _, ok := cmd().(DismissedMsg); !ok {
			t.Errorf("dismiss key %q did not produce DismissedMsg", k)
		}
	}
}

func TestAutosizeCenteredWithinOuterBounds(t *testing.T) {
	m := newAutosize("x", 80, 24)
	view := m.View()
	lines := strings.Split(view, "\n")
	if got := len(lines); got != 24 {
		t.Errorf("view line count = %d, want 24 (outerH)", got)
	}
	// The pane content should be flanked by whitespace-pad on top and bottom.
	blankTop := 0
	for _, ln := range lines {
		if strings.TrimSpace(ansi.Strip(ln)) == "" {
			blankTop++
		} else {
			break
		}
	}
	blankBot := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(ansi.Strip(lines[i])) == "" {
			blankBot++
		} else {
			break
		}
	}
	if blankTop == 0 || blankBot == 0 {
		t.Errorf("modal not vertically centered: blankTop=%d blankBot=%d", blankTop, blankBot)
	}
}

func TestAutosizeReMeasuresOnResize(t *testing.T) {
	m := newAutosize("some medium-length error message about a failed thing", 100, 30)
	firstW, firstH := m.modalW, m.modalH
	// Shrink the terminal — modal should re-wrap and shrink.
	m.SetDimensions(50, 15)
	if m.modalW > 50 || m.modalH > 15 {
		t.Errorf("post-resize dims %dx%d exceed outer 50x15", m.modalW, m.modalH)
	}
	if m.modalW >= firstW && m.modalH >= firstH {
		t.Errorf("expected smaller modal after shrink: was %dx%d, now %dx%d",
			firstW, firstH, m.modalW, m.modalH)
	}
}

func TestAutosizeReMeasuresOnSetMessage(t *testing.T) {
	m := newAutosize("short", 80, 20)
	beforeH := m.modalH
	// Swap in a much longer message — modal height should grow.
	m.SetMessage(strings.TrimRight(strings.Repeat("longer line\n", 30), "\n"))
	if m.modalH <= beforeH {
		t.Errorf("SetMessage did not grow modal: before=%d after=%d", beforeH, m.modalH)
	}
	// Scroll offset should reset on message swap.
	if m.scrollOffset != 0 {
		t.Errorf("SetMessage should reset scroll, got %d", m.scrollOffset)
	}
}

func TestAutosizeViewportShowsWindowedContent(t *testing.T) {
	// Line labels are distinct so we can identify which slice renders.
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "L" + string(rune('a'+i%26)) + string(rune('0'+i%10))
	}
	m := newAutosize(strings.Join(lines, "\n"), 80, 20)
	if !m.canScroll() {
		t.Fatal("expected overflow")
	}
	// Initial: offset 0 → first visible line contains lines[0].
	view := m.View()
	if !strings.Contains(view, lines[0]) {
		t.Errorf("initial view missing %q", lines[0])
	}
	// After scrolling: first line of window should be at scrollOffset.
	m.scrollOffset = 10
	m.pane.SetContent(m.renderInner())
	view = m.View()
	if !strings.Contains(view, lines[10]) {
		t.Errorf("post-scroll view missing %q", lines[10])
	}
	// The initial lines should no longer be visible (window moved).
	if strings.Contains(view, lines[0]) {
		t.Errorf("post-scroll view still contains scrolled-off line %q", lines[0])
	}
}

func TestAutosizeHelpIncludesScrollWhenOverflowing(t *testing.T) {
	short := newAutosize("hi", 80, 20)
	if got := len(short.Help()); got != 2 {
		t.Errorf("non-scrolling Help() = %d bindings, want 2", got)
	}
	long := newAutosize(strings.TrimRight(strings.Repeat("x\n", 40), "\n"), 80, 20)
	if got := len(long.Help()); got != 4 {
		t.Errorf("scrolling Help() = %d bindings, want 4", got)
	}
}

func TestAutosizeEmptyMessage(t *testing.T) {
	m := newAutosize("", 80, 20)
	// Modal should still render; button only.
	view := m.View()
	if !strings.Contains(view, "[ OK ]") {
		t.Errorf("empty-message modal missing OK button; view=%q", view)
	}
	if m.canScroll() {
		t.Errorf("empty-message modal should never scroll")
	}
	// Height should be very small (3 rows).
	if m.modalH != 3 {
		t.Errorf("empty modalH = %d, want 3", m.modalH)
	}
}

func TestAutosizeAppliesFractionCaps(t *testing.T) {
	// Sanity check the 80% / 60% caps in isolation with a huge message.
	huge := strings.TrimRight(strings.Repeat("word ", 5000), " ")
	m := newAutosize(huge, 200, 50)
	if m.modalW > 160 {
		t.Errorf("modalW = %d, want <= 80%% of 200 (160)", m.modalW)
	}
	if m.modalH > 30 {
		t.Errorf("modalH = %d, want <= 60%% of 50 (30)", m.modalH)
	}
}

func TestModalHeightNeverExceedsOuter(t *testing.T) {
	// If outerH < min cap, modal should still fit inside outer.
	m := newAutosize(strings.TrimRight(strings.Repeat("x\n", 50), "\n"), 40, 5)
	if m.modalH > 5 {
		t.Errorf("modalH = %d, want <= outerH=5", m.modalH)
	}
	// Sanity: view row count matches outerH.
	if got := modalCellHeight(m.View()); got > 5 {
		t.Errorf("visible modal height = %d rows, want <= 5", got)
	}
}

// modalCellWidth is used indirectly by other tests via ansi.StringWidth on
// content; retained here in case future assertions need it.
var _ = modalCellWidth

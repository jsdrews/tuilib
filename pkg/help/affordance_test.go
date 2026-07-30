package help

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/x/ansi"
)

func bindings(n int) []key.Binding {
	out := make([]key.Binding, n)
	for i := range out {
		out[i] = key.NewBinding(
			key.WithKeys(string(rune('a'+i))),
			key.WithHelp(string(rune('a'+i)), "action"),
		)
	}
	return out
}

// affordanceGlyphAt returns the cell offset of the "?" in the rendered
// footer line — where the user actually sees the affordance.
func affordanceGlyphAt(t *testing.T, m Model, width int) int {
	t.Helper()
	line, _, _ := m.ShortViewBudget(width)
	plain := ansi.Strip(line)
	i := strings.IndexByte(plain, '?')
	if i < 0 {
		t.Fatalf("no affordance glyph in rendered line %q", plain)
	}
	// IndexByte gives a byte offset; the separator is a multi-byte "•", so
	// measure the prefix in cells to get the column the user clicks.
	return ansi.StringWidth(plain[:i])
}

// The affordance sits in a different place in each of the three footer
// shapes. Whatever AffordanceSpan reports has to bracket the glyph the user
// can actually see, or a click on "? help" lands somewhere else.
func TestAffordanceSpanBracketsRenderedGlyph(t *testing.T) {
	const width = 60

	for _, tc := range []struct {
		name     string
		minimal  bool
		expanded bool
		count    int
	}{
		{"minimal collapsed", true, false, 8},
		{"minimal expanded", true, true, 8},
		{"verbose collapsed overflowing", false, false, 20},
		{"verbose expanded", false, true, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(Options{})
			m.SetMinimal(tc.minimal)
			m.SetExpanded(tc.expanded)
			m.SetBindings(bindings(tc.count))

			start, w, ok := m.AffordanceSpan(width)
			if !ok {
				t.Fatalf("AffordanceSpan reported no affordance")
			}
			at := affordanceGlyphAt(t, m, width)
			if at < start || at >= start+w {
				t.Errorf("span [%d,%d) does not contain the rendered glyph at %d",
					start, start+w, at)
			}
		})
	}
}

// Minimal mode is the default, and it renders the affordance first — the
// case a naive "it's at the end of the line" assumption gets wrong.
func TestAffordanceSpanStartsAtZeroInMinimalMode(t *testing.T) {
	m := New(Options{Minimal: true})
	m.SetBindings(bindings(6))

	start, _, ok := m.AffordanceSpan(60)
	if !ok {
		t.Fatalf("no affordance reported in minimal mode")
	}
	if start != 0 {
		t.Errorf("start = %d, want 0 — minimal renders the affordance first", start)
	}
}

// With no bindings there is nothing to expand, so there is no affordance to
// click and the help key is inert.
func TestAffordanceSpanAbsentWithoutBindings(t *testing.T) {
	m := New(Options{Minimal: true})

	if _, _, ok := m.AffordanceSpan(60); ok {
		t.Errorf("reported an affordance with no bindings")
	}
}

// In verbose mode a binding set that fits inline needs no affordance.
func TestAffordanceSpanAbsentWhenEverythingFits(t *testing.T) {
	m := New(Options{})
	m.SetBindings(bindings(2))

	if _, _, ok := m.AffordanceSpan(200); ok {
		t.Errorf("reported an affordance when every binding fits inline")
	}
}

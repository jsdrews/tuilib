package componenttest

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/examples/data/multiselect"
	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// The multiselect example is the only place marking is demonstrated, and its
// package comment makes three specific promises to whoever reads it. A demo
// that quietly stops demonstrating its subject is worse than no demo — the
// reader concludes the feature does not work — so the promises are asserted
// rather than trusted.

const exW, exH = 100, 30

func newMultiselect(t *testing.T) screen.Screen {
	t.Helper()
	s := multiselect.New(theme.Dark())
	s.Init()
	s.OnEnter(nil)
	render(s)
	return s
}

func render(s screen.Screen) { renderTo(s) }

func renderTo(s screen.Screen) string {
	geom.NextGen()
	return s.Layout().Render(geom.New(0, 0, exW, exH))
}

func markRow(s screen.Screen) screen.Screen {
	next, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	render(next)
	return next
}

func cursorDown(s screen.Screen) screen.Screen {
	next, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	render(next)
	return next
}

func actionsOf(t *testing.T, s screen.Screen) action.Set {
	t.Helper()
	p, ok := s.(action.Provider)
	if !ok {
		t.Fatal("multiselect screen no longer provides actions")
	}
	return p.Actions()
}

// Space marks, and the action.Set reports the marked count — the whole point
// of the screen.
func TestMultiselectExampleMarksAndReportsCount(t *testing.T) {
	s := newMultiselect(t)

	if got := actionsOf(t, s).Count; got != 1 {
		t.Fatalf("unmarked screen: Count = %d, want 1 (the cursor row)", got)
	}

	s = markRow(s)
	s = cursorDown(s)
	s = markRow(s)

	set := actionsOf(t, s)
	if set.Count != 2 {
		t.Errorf("after marking two rows: Count = %d, want 2", set.Count)
	}
	if set.Target != "2 items" {
		t.Errorf("Target = %q, want %q", set.Target, "2 items")
	}
}

// "Describe" carries no Multi, so a second mark must disable it. This is the
// arity gate the launcher blurb points a reader at.
func TestMultiselectExampleGatesNonMultiActions(t *testing.T) {
	s := newMultiselect(t)

	find := func(set action.Set, label string) action.Action {
		t.Helper()
		for _, a := range set.Actions {
			if a.Label == label {
				return a
			}
		}
		t.Fatalf("action %q is gone from the example", label)
		return action.Action{}
	}

	if a := find(actionsOf(t, s), "Describe"); a.Multi {
		t.Fatal("Describe declares Multi — the example no longer shows the arity gate")
	}
	if a := find(actionsOf(t, s), "Restart"); !a.Multi {
		t.Error("Restart must declare Multi to contrast with Describe")
	}
}

// A mark filtered out of sight is still selected. The package comment calls
// this out as the screen's genuine surprise, so it is the thing most likely to
// be "fixed" by someone who thinks it is a bug.
func TestMultiselectExampleMarksSurviveFiltering(t *testing.T) {
	s := newMultiselect(t)
	s = markRow(s) // api-server, an env:prod row

	before := actionsOf(t, s).Count

	// Focus the filter with "/", type a query that excludes the marked row,
	// and commit it. Without the "/" these keystrokes drive the body instead.
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "env:staging" {
		s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := renderTo(s)

	// Guard the guard: if the filter stopped excluding the marked row this
	// test would pass without asserting anything.
	if strings.Contains(view, "prod") {
		t.Fatal("filter env:staging no longer hides the prod rows — " +
			"this test can no longer detect a drifting selection")
	}

	if got := actionsOf(t, s).Count; got != before {
		t.Errorf("selection changed when the marked row was filtered away: "+
			"Count = %d, want %d — marks are keyed and must survive the filter", got, before)
	}
}

// SetTheme must carry the marks across the rebuild (rule 4). Dropping the
// SetMarks line is invisible until a user loses a selection to a theme swap.
func TestMultiselectExampleMarksSurviveThemeSwap(t *testing.T) {
	s := newMultiselect(t)
	s = markRow(s)
	s = cursorDown(s)
	s = markRow(s)

	before := actionsOf(t, s).Count

	th, ok := s.(interface{ SetTheme(theme.Theme) })
	if !ok {
		t.Fatal("multiselect screen no longer implements SetTheme")
	}
	th.SetTheme(theme.Light())
	render(s)

	if got := actionsOf(t, s).Count; got != before {
		t.Errorf("marks lost across SetTheme: Count = %d, want %d", got, before)
	}
}

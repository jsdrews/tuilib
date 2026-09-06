package componenttest

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/examples/data/treeactions"
	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// The tree-actions example's subject is that a marked branch is resolved to a
// subtree by the *screen*, per verb — so the resolution is what gets asserted.

func newTreeActions(t *testing.T) screen.Screen {
	t.Helper()
	s := treeactions.New(theme.Dark())
	s.Init()
	s.OnEnter(nil)
	render(s)
	return s
}

func send(s screen.Screen, m tea.Msg) screen.Screen {
	next, _ := s.Update(m)
	render(next)
	return next
}

func setOf(t *testing.T, s screen.Screen) action.Set {
	t.Helper()
	p, ok := s.(action.Provider)
	if !ok {
		t.Fatal("tree-actions screen no longer provides actions")
	}
	return p.Actions()
}

// Marking a namespace must make the cascading verbs report its pods, not "1".
func TestTreeActionsCascadesAMarkedBranch(t *testing.T) {
	s := newTreeActions(t)

	// Row 0 is the root; row 1 is the "prod" namespace.
	s = send(s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	s = send(s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	set := setOf(t, s)
	if set.Count != 6 {
		t.Errorf("Count = %d, want 6 — prod holds 6 pods; a marked branch must cascade", set.Count)
	}
	if set.Target != "6 pods" {
		t.Errorf("Target = %q, want %q", set.Target, "6 pods")
	}
}

// Describe reads the marked node itself, so it must not declare Multi — one
// marked branch is still one thing to describe.
func TestTreeActionsDescribeDoesNotCascade(t *testing.T) {
	s := newTreeActions(t)
	set := setOf(t, s)

	for _, want := range []struct {
		label string
		multi bool
	}{{"Restart", true}, {"Describe", false}} {
		found := false
		for _, a := range set.Actions {
			if a.Label != want.label {
				continue
			}
			found = true
			if a.Multi != want.multi {
				t.Errorf("%s: Multi = %v, want %v", a.Label, a.Multi, want.multi)
			}
		}
		if !found {
			t.Errorf("action %q is gone from the example", want.label)
		}
	}
}

// Delete is guarded to staging, so a prod selection must arrive disabled with
// a reason rather than simply missing.
func TestTreeActionsDeleteIsGuardedToStaging(t *testing.T) {
	s := newTreeActions(t)
	s = send(s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // prod
	s = send(s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	for _, a := range setOf(t, s).Actions {
		if a.Label == "Delete" {
			if a.Disabled == "" {
				t.Error("Delete is enabled on a prod selection; the guard is gone")
			}
			return
		}
	}
	t.Error("Delete action is gone from the example")
}

package componenttest

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/input"
	"github.com/jsdrews/tuilib/pkg/list"
)

// TestEscReleasesACapturingComponent is the invariant behind a real trap: a
// component that reports IsCapturingKeys blocks focus.Group from cycling, so
// if nothing releases it the keyboard is stuck there for good.
//
// pkg/input had exactly this shape — capturing while focused, intercepting no
// keys — which made any bare input inside a Group a one-way door. It shipped
// that way, and the example demonstrating focus.Group was the thing that hit
// it. Rule 27 tells readers to "leave the field with enter or esc"; this
// asserts that instruction is true rather than aspirational.
func TestEscReleasesACapturingComponent(t *testing.T) {
	esc := tea.KeyMsg{Type: tea.KeyEsc}

	t.Run("input", func(t *testing.T) {
		m := input.New(input.Options{})
		m.Focus()
		m.SetValue("half typed")
		if !m.IsCapturingKeys() {
			t.Fatal("a focused input should capture keys")
		}
		m, _ = m.Update(esc)
		if m.IsCapturingKeys() {
			t.Error("esc did not release the input — a Group can never cycle off it")
		}
		if m.Value() != "half typed" {
			t.Errorf("esc changed the value to %q; it should only release focus", m.Value())
		}
	})

	t.Run("list filter", func(t *testing.T) {
		m := list.New(list.Options{Items: []string{"a", "b"}, Filterable: true})
		m.Focus()
		m.FocusFilter()
		if !m.IsCapturingKeys() {
			t.Fatal("a focused filter should capture keys")
		}
		m, _ = m.Update(esc)
		if m.IsCapturingKeys() {
			t.Error("esc did not release the filter")
		}
	})
}

// TestGroupCyclesAfterEsc drives the whole loop the trap broke: type into a
// field, release it, and tab on to the next pane.
func TestGroupCyclesAfterEsc(t *testing.T) {
	field := input.New(input.Options{})
	rows := list.New(list.Options{Items: []string{"a", "b"}})
	g := focus.NewGroup(&field, &rows)
	g.Init()

	if !g.Is(&field) {
		t.Fatal("the group should start on its first member")
	}
	field, _ = field.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	g, _ = g.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !g.Is(&field) {
		t.Fatal("tab should be declined while the field holds the keyboard")
	}

	field, _ = field.Update(tea.KeyMsg{Type: tea.KeyEsc})
	g, _ = g.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !g.Is(&rows) {
		t.Error("after esc released the field, tab should have cycled on")
	}
}

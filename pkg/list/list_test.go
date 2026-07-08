package list

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/filter"
)

// drainSelectedChangedMsg pulls the first SelectedChangedMsg out of a Cmd
// tree, walking through tea.BatchMsg composites. Returns nil when none.
func drainSelectedChangedMsg(cmd tea.Cmd) *SelectedChangedMsg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if sc, ok := msg.(SelectedChangedMsg); ok {
		return &sc
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range batch {
			if sc := drainSelectedChangedMsg(sub); sc != nil {
				return sc
			}
		}
	}
	return nil
}

func newFocusList(items []string) Model {
	return New(Options{
		Width:         30,
		Height:        8,
		Items:         items,
		SelectedColor: lipgloss.Color("15"),
	})
}

func newFilterableFocusList(items []string) Model {
	return New(Options{
		Width:         30,
		Height:        10,
		Items:         items,
		Filterable:    true,
		SelectedColor: lipgloss.Color("15"),
		Filter:        filter.Options{Width: 30, Placeholder: "filter"},
	})
}

func TestSelectedChangedFiresOnInit(t *testing.T) {
	m := newFocusList([]string{"a", "b", "c"})
	_, cmd := m.Update(struct{}{})
	sc := drainSelectedChangedMsg(cmd)
	if sc == nil {
		t.Fatal("expected SelectedChangedMsg on first Update, got none")
	}
	if sc.Empty {
		t.Errorf("Empty=true on non-empty init")
	}
	if sc.Index != 0 {
		t.Errorf("Index = %d, want 0", sc.Index)
	}
	if sc.Item != "a" {
		t.Errorf("Item = %q, want %q", sc.Item, "a")
	}
}

func TestSelectedChangedNoRepeatOnUnchanged(t *testing.T) {
	m := newFocusList([]string{"a", "b"})
	m, _ = m.Update(struct{}{}) // drain init
	_, cmd := m.Update(struct{}{})
	if sc := drainSelectedChangedMsg(cmd); sc != nil {
		t.Errorf("unexpected re-emit: %+v", sc)
	}
}

func TestSelectedChangedOnCursorMove(t *testing.T) {
	m := newFocusList([]string{"a", "b", "c"})
	m, _ = m.Update(struct{}{}) // drain init
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	sc := drainSelectedChangedMsg(cmd)
	if sc == nil {
		t.Fatal("expected SelectedChangedMsg after cursor down")
	}
	if sc.Index != 1 || sc.Item != "b" {
		t.Errorf("focused = %d %q, want 1 %q", sc.Index, sc.Item, "b")
	}
}

func TestSelectedChangedOnSetItemsChangesContent(t *testing.T) {
	m := newFocusList([]string{"a", "b"})
	m, _ = m.Update(struct{}{}) // drain init (index 0 = "a")
	m.SetItems([]string{"z", "b"})
	_, cmd := m.Update(struct{}{})
	sc := drainSelectedChangedMsg(cmd)
	if sc == nil {
		t.Fatal("expected re-emit after SetItems changing focused item")
	}
	if sc.Item != "z" {
		t.Errorf("Item = %q, want z", sc.Item)
	}
}

func TestSelectedChangedNoRepeatWhenSetItemsKeepsSameContent(t *testing.T) {
	m := newFocusList([]string{"a", "b"})
	m, _ = m.Update(struct{}{}) // drain init
	m.SetItems([]string{"a", "b"})
	_, cmd := m.Update(struct{}{})
	if sc := drainSelectedChangedMsg(cmd); sc != nil {
		t.Errorf("unexpected re-emit when focused item unchanged: %+v", sc)
	}
}

func TestSelectedChangedEmitsEmptyOnTransitionToNoItems(t *testing.T) {
	m := newFocusList([]string{"a"})
	m, _ = m.Update(struct{}{}) // drain init
	m.SetItems(nil)
	_, cmd := m.Update(struct{}{})
	sc := drainSelectedChangedMsg(cmd)
	if sc == nil {
		t.Fatal("expected SelectedChangedMsg{Empty:true} after SetItems(nil)")
	}
	if !sc.Empty {
		t.Errorf("Empty = %v, want true; msg = %+v", sc.Empty, sc)
	}
	if sc.Item != "" || sc.Index != 0 {
		t.Errorf("Item/Index should be zero on Empty, got %q / %d", sc.Item, sc.Index)
	}
}

func TestSelectedChangedSuppressedOnEmptyInit(t *testing.T) {
	m := newFocusList(nil)
	_, cmd := m.Update(struct{}{})
	if sc := drainSelectedChangedMsg(cmd); sc != nil {
		t.Errorf("unexpected initial emit for empty list: %+v", sc)
	}
}

func TestSelectedChangedOnFilterNarrowsToDifferentItem(t *testing.T) {
	m := newFilterableFocusList([]string{"apple", "apricot", "banana"})
	m, _ = m.Update(struct{}{}) // drain init: index 0 = "apple"
	// Focus filter, then type "b" — filter narrows to just "banana" and the
	// focused item at cursor=0 changes from "apple" to "banana", triggering
	// the emit on the same tick.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	sc := drainSelectedChangedMsg(cmd)
	if sc == nil {
		t.Fatal("expected SelectedChangedMsg after filter narrows to different item")
	}
	if sc.Item != "banana" {
		t.Errorf("Item = %q, want banana", sc.Item)
	}
}

func TestSelectedChangedOnSetCursor(t *testing.T) {
	m := newFocusList([]string{"a", "b", "c"})
	m, _ = m.Update(struct{}{}) // drain init
	m.SetCursor(2)
	_, cmd := m.Update(struct{}{})
	sc := drainSelectedChangedMsg(cmd)
	if sc == nil {
		t.Fatal("expected SelectedChangedMsg after SetCursor")
	}
	if sc.Index != 2 || sc.Item != "c" {
		t.Errorf("focused = %d %q, want 2 %q", sc.Index, sc.Item, "c")
	}
}

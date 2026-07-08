package tree

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestSelectedChangedFiresOnInit(t *testing.T) {
	m := newTree(n("root", n("a"), n("b")))
	_, cmd := m.Update(struct{}{})
	sc := drainSelectedChangedMsg(cmd)
	if sc == nil {
		t.Fatal("expected SelectedChangedMsg on first Update, got none")
	}
	if sc.Empty {
		t.Errorf("Empty=true on non-empty init")
	}
	if len(sc.Path) != 1 || sc.Path[0] != "root" {
		t.Errorf("Path = %v, want [root]", sc.Path)
	}
	if sc.Label != "root" {
		t.Errorf("Label = %q, want root", sc.Label)
	}
	if sc.Depth != 0 {
		t.Errorf("Depth = %d, want 0", sc.Depth)
	}
}

func TestSelectedChangedNoRepeatOnUnchanged(t *testing.T) {
	m := newTree(n("root", n("a"), n("b")))
	m, _ = m.Update(struct{}{}) // drain init
	_, cmd := m.Update(struct{}{})
	if sc := drainSelectedChangedMsg(cmd); sc != nil {
		t.Errorf("unexpected re-emit: %+v", sc)
	}
}

func TestSelectedChangedOnCursorMove(t *testing.T) {
	m := newTree(n("root", n("a"), n("b")))
	m, _ = m.Update(struct{}{}) // drain init: cursor on "root"
	// down: cursor to "root/a"
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	sc := drainSelectedChangedMsg(cmd)
	if sc == nil {
		t.Fatal("expected SelectedChangedMsg after cursor down")
	}
	if len(sc.Path) != 2 || sc.Path[0] != "root" || sc.Path[1] != "a" {
		t.Errorf("Path = %v, want [root a]", sc.Path)
	}
	if sc.Label != "a" {
		t.Errorf("Label = %q, want a", sc.Label)
	}
	if sc.Depth != 1 {
		t.Errorf("Depth = %d, want 1", sc.Depth)
	}
}

func TestSelectedChangedOnExpand(t *testing.T) {
	// InitialDepth=1 expands root, so "a" is a visible collapsed child.
	m := newTree(n("root", n("a", n("leaf"))))
	m, _ = m.Update(struct{}{}) // drain init
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // cursor→ "root/a"
	_, cmd := m.Update(struct{}{})                                     // drain focus emit from move
	if drainSelectedChangedMsg(cmd) != nil {
		// Expected: the focus msg from the cursor-down was already flushed
		// in that Update's cmd. A subsequent tick shouldn't re-emit.
	}
	// Expand "a" via space — the cursor stays on "a", so no re-emit.
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if sc := drainSelectedChangedMsg(cmd); sc != nil {
		t.Errorf("expanding cursor node shouldn't re-emit focus: %+v", sc)
	}
}

func TestSelectedChangedOnSetRootChangesFocus(t *testing.T) {
	m := newTree(n("root", n("a"), n("b")))
	m, _ = m.Update(struct{}{}) // drain init: focus = ["root"]
	// Swap the tree — new root has a different label; the SetRoot logic
	// pins the cursor to path "root" which no longer exists, so it walks
	// up ancestors and lands on the new root, whose label is "newroot".
	m.SetRoot(n("newroot", n("x")))
	_, cmd := m.Update(struct{}{})
	sc := drainSelectedChangedMsg(cmd)
	if sc == nil {
		t.Fatal("expected re-emit after SetRoot changes focused node")
	}
	if len(sc.Path) != 1 || sc.Path[0] != "newroot" {
		t.Errorf("Path = %v, want [newroot]", sc.Path)
	}
}

func TestSelectedChangedNoRepeatWhenSetRootPreservesPath(t *testing.T) {
	m := newTree(n("root", n("a"), n("b")))
	m, _ = m.Update(struct{}{}) // drain init: focus = ["root"]
	// Same-structure SetRoot — cursor stays on "root".
	m.SetRoot(n("root", n("a"), n("b")))
	_, cmd := m.Update(struct{}{})
	if sc := drainSelectedChangedMsg(cmd); sc != nil {
		t.Errorf("unexpected re-emit when focused path unchanged: %+v", sc)
	}
}

func TestSelectedChangedEmitsEmptyOnTransitionToEmpty(t *testing.T) {
	m := newTree(n("root", n("a")))
	m, _ = m.Update(struct{}{}) // drain init
	m.SetRoot(nil)
	_, cmd := m.Update(struct{}{})
	sc := drainSelectedChangedMsg(cmd)
	if sc == nil {
		t.Fatal("expected SelectedChangedMsg{Empty:true} after SetRoot(nil)")
	}
	if !sc.Empty {
		t.Errorf("Empty = %v, want true; msg = %+v", sc.Empty, sc)
	}
	if sc.Path != nil || sc.Label != "" || sc.Depth != 0 {
		t.Errorf("Path/Label/Depth should be zero on Empty, got %v / %q / %d", sc.Path, sc.Label, sc.Depth)
	}
}

func TestSelectedChangedSuppressedOnEmptyInit(t *testing.T) {
	m := newTree(nil)
	_, cmd := m.Update(struct{}{})
	if sc := drainSelectedChangedMsg(cmd); sc != nil {
		t.Errorf("unexpected initial emit for empty tree: %+v", sc)
	}
}

func TestSelectedChangedIncludesDuplicateSiblingSuffix(t *testing.T) {
	// Two children with the label "dup" — second uses "dup:2" in the path.
	m := newTree(n("root", n("dup"), n("dup")))
	m, _ = m.Update(struct{}{}) // drain init (cursor on "root")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // → "root/dup"
	// Move to the second "dup"; drain the focus that fires here.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // → "root/dup:2"
	sc := drainSelectedChangedMsg(cmd)
	if sc == nil {
		t.Fatal("expected SelectedChangedMsg on duplicate-sibling move")
	}
	if len(sc.Path) != 2 || sc.Path[0] != "root" || sc.Path[1] != "dup:2" {
		t.Errorf("Path = %v, want [root dup:2]", sc.Path)
	}
	if sc.Label != "dup:2" {
		t.Errorf("Label = %q, want dup:2 (path segment, matches issue spec)", sc.Label)
	}
}

func TestSelectedChangedOnSetCursor(t *testing.T) {
	m := newTree(n("root", n("a"), n("b")))
	m, _ = m.Update(struct{}{}) // drain init
	m.SetCursor(2)              // "root/b"
	_, cmd := m.Update(struct{}{})
	sc := drainSelectedChangedMsg(cmd)
	if sc == nil {
		t.Fatal("expected SelectedChangedMsg after SetCursor")
	}
	if len(sc.Path) != 2 || sc.Path[1] != "b" {
		t.Errorf("Path = %v, want [root b]", sc.Path)
	}
}

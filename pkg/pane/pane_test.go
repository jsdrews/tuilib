package pane

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Build a pane wide enough that content overflows so xOffset is meaningful.
func newOverflowPane(t *testing.T) Pane {
	t.Helper()
	p := New(Options{Width: 20, Height: 5})
	p.SetContent(strings.Repeat("x", 200))
	return p
}

func TestHomeJumpsToLeftEdge(t *testing.T) {
	p := newOverflowPane(t)
	p.SetXOffset(50)
	if p.XOffset() != 50 {
		t.Fatalf("setup: XOffset = %d, want 50", p.XOffset())
	}
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	if p.XOffset() != 0 {
		t.Errorf("after '0', XOffset = %d, want 0", p.XOffset())
	}
}

func TestEndJumpsToRightEdge(t *testing.T) {
	p := newOverflowPane(t)
	want := p.MaxXOffset()
	if want <= 0 {
		t.Fatalf("setup: MaxXOffset = %d, expected positive", want)
	}
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'$'}})
	if p.XOffset() != want {
		t.Errorf("after '$', XOffset = %d, want %d", p.XOffset(), want)
	}
}

func TestHomeKeyAlias(t *testing.T) {
	p := newOverflowPane(t)
	p.SetXOffset(40)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyHome})
	if p.XOffset() != 0 {
		t.Errorf("after Home, XOffset = %d, want 0", p.XOffset())
	}
}

func TestEndKeyAlias(t *testing.T) {
	p := newOverflowPane(t)
	want := p.MaxXOffset()
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if p.XOffset() != want {
		t.Errorf("after End, XOffset = %d, want %d", p.XOffset(), want)
	}
}

func TestEdgeJumpClampsWhenContentFits(t *testing.T) {
	p := New(Options{Width: 40, Height: 5})
	p.SetContent("short")
	// MaxXOffset should be 0; $ should be a no-op, not panic.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'$'}})
	if p.XOffset() != 0 {
		t.Errorf("after '$' with fitting content, XOffset = %d, want 0", p.XOffset())
	}
}

package form

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/geom"
)

func titled(t *testing.T) Model {
	t.Helper()
	m := New(Options{
		Title:          "Create user",
		ActiveBorder:   lipgloss.NormalBorder(),
		InactiveBorder: lipgloss.NormalBorder(),
		Fields: []Field{
			Text(TextOptions{Key: "name", Label: "Name"}),
			Text(TextOptions{Key: "email", Label: "Email"}),
		},
	})
	m.SetRect(geom.New(0, 0, 40, 16))
	m.View()
	return m
}

func TestTitleFramesTheForm(t *testing.T) {
	m := titled(t)
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if len(lines) != 16 {
		t.Fatalf("%d lines, want the frame to fill its 16-row rect", len(lines))
	}
	if !strings.Contains(lines[0], "Create user") {
		t.Errorf("title not on the frame's top border: %q", lines[0])
	}
	if !strings.Contains(lines[1], "Name") {
		t.Errorf("first field not inside the frame: %q", lines[1])
	}
	if m.Title() != "Create user" || m.Rect().W != 40 {
		t.Errorf("Title %q, Rect %v", m.Title(), m.Rect())
	}
}

// The fields sit inside the frame's edge, so their rects — what a click is
// tested against — must be offset by it.
func TestFramedFieldsHitTestInsideTheFrame(t *testing.T) {
	m := titled(t)
	first := m.fields[0].Rect()
	if first.X != 1 || first.Y != 1 {
		t.Fatalf("first field at (%d,%d), want (1,1) inside the border", first.X, first.Y)
	}
	second := m.fields[1].Rect()
	m, _ = m.Update(press(second.X+2, second.Y+1))
	if m.focus != 1 {
		t.Errorf("click on the second field focused %d", m.focus)
	}
}

func TestNoTitleKeepsTheBareStack(t *testing.T) {
	m := placed(t)
	if first := m.fields[0].Rect(); first.X != 0 || first.Y != 0 {
		t.Errorf("an untitled form moved its fields to (%d,%d)", first.X, first.Y)
	}
	if m.Title() != "" {
		t.Errorf("untitled form reports title %q", m.Title())
	}
}

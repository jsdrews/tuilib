package componenttest

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/mouse"
)

// trackMidY is the middle of the scrollbar track (content rows 3..12).
// Dragging to the middle rather than to an end is what makes this work for
// every component: a list starts at the top and a logview starts at the
// bottom, so either end is a no-op for one of them.
const trackMidY = 8

func mouseAt(x, y int, a tea.MouseAction, b tea.MouseButton) mouse.Msg {
	m := mouse.Msg{MouseMsg: tea.MouseMsg{X: x, Y: y, Action: a, Button: b}}
	if a == tea.MouseActionPress && b == tea.MouseButtonLeft {
		m.Clicks = 1
	}
	return m
}

// Dragging the scrollbar has to survive the next frame.
//
// layout.Sized calls SetRect on every render, and for cursor-owning
// components SetRect refreshes, which re-asserts "the cursor is visible".
// A scrollbar that moves the viewport alone is therefore undone one frame
// later: the list jumps back to wherever the cursor still is, which for an
// untouched list is the top. The scroll has to move the cursor too, exactly
// as the wheel does.
func TestScrollbarDragSurvivesTheNextFrame(t *testing.T) {
	for _, h := range harnesses() {
		t.Run(h.name, func(t *testing.T) {
			before := h.view()

			h.update(mouseAt(scrollbarX, deepRowY, tea.MouseActionPress, tea.MouseButtonLeft))
			h.update(mouseAt(scrollbarX, trackMidY, tea.MouseActionMotion, tea.MouseButtonLeft))
			dragged := h.view()

			if dragged == before {
				t.Fatalf("dragging the scrollbar did not scroll at all")
			}

			h.update(mouseAt(scrollbarX, trackMidY, tea.MouseActionRelease, tea.MouseButtonLeft))
			h.setRect() // the SetRect every render performs
			settled := h.view()

			if settled != dragged {
				t.Errorf("the view moved after the drag ended — the scroll was " +
					"undone by the next frame's cursor-visible pass")
			}
			if settled == before {
				t.Errorf("the view snapped back to where it started")
			}
		})
	}
}

// Repeated frames must not drift either: a scroll that survives one render
// but creeps on the next is just a slower version of the same bug.
func TestScrollbarDragIsStableAcrossFrames(t *testing.T) {
	for _, h := range harnesses() {
		t.Run(h.name, func(t *testing.T) {
			h.update(mouseAt(scrollbarX, deepRowY, tea.MouseActionPress, tea.MouseButtonLeft))
			h.update(mouseAt(scrollbarX, trackMidY, tea.MouseActionMotion, tea.MouseButtonLeft))
			h.update(mouseAt(scrollbarX, trackMidY, tea.MouseActionRelease, tea.MouseButtonLeft))

			h.setRect()
			first := h.view()
			for range 3 {
				h.setRect()
			}

			if h.view() != first {
				t.Errorf("the view kept moving over successive frames after the drag")
			}
		})
	}
}

package pane

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
)

var paneRect = geom.Rect{X: 5, Y: 2, W: 30, H: 10}

func newScrollPane(t *testing.T, lines int) Pane {
	t.Helper()
	p := New(Options{})
	p.SetRect(geom.New(paneRect.X, paneRect.Y, paneRect.W, paneRect.H))
	body := make([]string, lines)
	for i := range body {
		body[i] = fmt.Sprintf("line %d", i)
	}
	p.SetContent(strings.Join(body, "\n"))
	return p
}

func mouseAt(x, y int, action tea.MouseAction, button tea.MouseButton) mouse.Msg {
	m := mouse.Msg{MouseMsg: tea.MouseMsg{X: x, Y: y, Action: action, Button: button}}
	if action == tea.MouseActionPress && button == tea.MouseButtonLeft {
		m.Clicks = 1
	}
	return m
}

func pressAt(x, y int) mouse.Msg {
	return mouseAt(x, y, tea.MouseActionPress, tea.MouseButtonLeft)
}
func dragTo(x, y int) mouse.Msg {
	return mouseAt(x, y, tea.MouseActionMotion, tea.MouseButtonLeft)
}
func releaseAt(x, y int) mouse.Msg {
	return mouseAt(x, y, tea.MouseActionRelease, tea.MouseButtonLeft)
}

func TestClickScrollbarTrackJumps(t *testing.T) {
	p := newScrollPane(t, 200)
	bar := p.VScrollbarRect()

	// Click near the bottom of the track.
	_, ok := p.HandleScrollbar(pressAt(bar.X, bar.Y+bar.H-1))

	if !ok {
		t.Fatalf("a click on the scrollbar was not consumed")
	}
	if p.YOffset() == 0 {
		t.Errorf("clicking near the bottom of the track did not scroll")
	}
}

func TestClickScrollbarTopGoesToTop(t *testing.T) {
	p := newScrollPane(t, 200)
	bar := p.VScrollbarRect()
	p.SetYOffset(100)

	p.HandleScrollbar(pressAt(bar.X, bar.Y))

	if p.YOffset() != 0 {
		t.Errorf("clicking the top of the track left offset at %d, want 0", p.YOffset())
	}
}

func TestDragScrollbarTracksMotion(t *testing.T) {
	p := newScrollPane(t, 200)
	bar := p.VScrollbarRect()

	p.HandleScrollbar(pressAt(bar.X, bar.Y))
	if !p.ScrollbarDrag() {
		t.Fatalf("pressing the scrollbar did not start a drag")
	}

	// Motion while held keeps scrolling, even off the bar's column.
	p.HandleScrollbar(dragTo(bar.X-4, bar.Y+bar.H-1))

	if p.YOffset() == 0 {
		t.Errorf("dragging down the track did not scroll")
	}
}

// The release can land anywhere — outside the bar, outside the pane — and
// must still end the drag.
func TestReleaseAnywhereEndsDrag(t *testing.T) {
	p := newScrollPane(t, 200)
	bar := p.VScrollbarRect()
	p.HandleScrollbar(pressAt(bar.X, bar.Y))

	_, ok := p.HandleScrollbar(releaseAt(999, 999))

	if !ok {
		t.Errorf("the release that ended a drag was not consumed")
	}
	if p.ScrollbarDrag() {
		t.Errorf("drag still in progress after release")
	}
}

// Once the drag is over, motion is no longer ours.
func TestMotionAfterReleaseIsNotConsumed(t *testing.T) {
	p := newScrollPane(t, 200)
	bar := p.VScrollbarRect()
	p.HandleScrollbar(pressAt(bar.X, bar.Y))
	p.HandleScrollbar(releaseAt(bar.X, bar.Y))

	if _, ok := p.HandleScrollbar(dragTo(bar.X, bar.Y+3)); ok {
		t.Errorf("motion after the drag ended was still consumed")
	}
}

func TestClickInBodyIsNotAScrollbarEvent(t *testing.T) {
	p := newScrollPane(t, 200)

	if _, ok := p.HandleScrollbar(pressAt(paneRect.X+3, paneRect.Y+3)); ok {
		t.Errorf("a click in the body was treated as a scrollbar event")
	}
}

func TestScrollbarInertWhenContentFits(t *testing.T) {
	p := newScrollPane(t, 2)
	bar := p.VScrollbarRect()

	p.HandleScrollbar(pressAt(bar.X, bar.Y+bar.H-1))

	if p.YOffset() != 0 {
		t.Errorf("clicking the track scrolled a pane whose content fits")
	}
}

// A pane whose owner windows its own rows must not have its viewport moved —
// the owner would undo it on the next render. The target row is reported so
// the owner can move its cursor instead.
func TestVirtualScrollReportsTargetWithoutMovingViewport(t *testing.T) {
	p := newScrollPane(t, 20)
	p.SetVirtualScroll(500, 8, 0)
	bar := p.VScrollbarRect()
	before := p.YOffset()

	row, ok := p.HandleScrollbar(pressAt(bar.X, bar.Y+bar.H-1))

	if !ok {
		t.Fatalf("the scrollbar click was not consumed")
	}
	if p.YOffset() != before {
		t.Errorf("viewport moved under a virtual-scroll owner: %d → %d", before, p.YOffset())
	}
	if row == 0 {
		t.Errorf("no target row reported for the owner to scroll to")
	}
	if row > 500-8 {
		t.Errorf("target row %d exceeds the last valid window start", row)
	}
}

func TestHorizontalScrollbarJumps(t *testing.T) {
	p := New(Options{HScrollbar: true})
	p.SetRect(geom.New(paneRect.X, paneRect.Y, paneRect.W, paneRect.H))
	p.SetContent(strings.Repeat("x", 400))

	bar := p.HScrollbarRect()
	if bar.W == 0 {
		t.Fatalf("setup: no horizontal scrollbar rect")
	}

	_, ok := p.HandleScrollbar(pressAt(bar.X+bar.W-1, bar.Y))

	if !ok {
		t.Fatalf("a click on the horizontal scrollbar was not consumed")
	}
	if p.XOffset() == 0 {
		t.Errorf("clicking the right of the h-track did not scroll horizontally")
	}
}

func TestStaleRectDeclinesScrollbarClicks(t *testing.T) {
	p := newScrollPane(t, 200)
	bar := p.VScrollbarRect()
	geom.NextGen()

	if _, ok := p.HandleScrollbar(pressAt(bar.X, bar.Y+bar.H-1)); ok {
		t.Errorf("a pane with a stale rect consumed a scrollbar click")
	}
}

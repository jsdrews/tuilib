package confirm

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
)

var modalRect = geom.Rect{X: 10, Y: 4, W: 40, H: 7}

// newModal builds a confirm with a one-line message. Inner content is
// message, blank, buttons — so the buttons land on the third content line.
func newModal(t *testing.T) Model {
	t.Helper()
	m := New(Options{Message: "Delete file?", Confirm: "Delete", Cancel: "Keep"})
	m.SetRect(geom.New(modalRect.X, modalRect.Y, modalRect.W, modalRect.H))
	return m
}

const buttonsY = 4 + 1 + 2 // rect.Y + border + two content lines above

func press(x, y int) mouse.Msg {
	return mouse.Msg{
		MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		Clicks:   1,
	}
}

func result(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// "[ Keep ]" is 8 cells and sits first; "[ Delete ]" follows after two spaces.
const (
	cancelX  = 10 + 1 + 1  // rect.X + border + one cell into "[ Keep ]"
	confirmX = 10 + 1 + 12 // into "[ Delete ]", past "[ Keep ]" + gap
)

// A click on a button commits it — a mouse user pressing "Delete" has
// already chosen, and requiring a second click to confirm would not make the
// dialog safer.
func TestClickConfirmButtonCommits(t *testing.T) {
	m := newModal(t)

	_, cmd := m.Update(press(confirmX, buttonsY))

	if _, ok := result(cmd).(ConfirmedMsg); !ok {
		t.Errorf("clicking the confirm button gave %T, want ConfirmedMsg", result(cmd))
	}
}

func TestClickCancelButtonCommits(t *testing.T) {
	m := newModal(t)

	_, cmd := m.Update(press(cancelX, buttonsY))

	if _, ok := result(cmd).(CancelledMsg); !ok {
		t.Errorf("clicking the cancel button gave %T, want CancelledMsg", result(cmd))
	}
}

// The gap between the buttons is not a button.
func TestClickBetweenButtonsDoesNothing(t *testing.T) {
	m := newModal(t)

	// "[ Keep ]" is 8 wide, so cells 8 and 9 of the content are the gap.
	_, cmd := m.Update(press(modalRect.X+1+8, buttonsY))

	if cmd != nil {
		t.Errorf("clicking between the buttons produced %T", result(cmd))
	}
}

func TestClickOnMessageDoesNothing(t *testing.T) {
	m := newModal(t)

	_, cmd := m.Update(press(modalRect.X+3, modalRect.Y+1))

	if cmd != nil {
		t.Errorf("clicking the message body produced %T", result(cmd))
	}
}

// Clicks outside the modal are ignored: click-outside-to-cancel is too easy
// to trigger by accident on a destructive action.
func TestClickOutsideModalIsIgnored(t *testing.T) {
	m := newModal(t)

	_, cmd := m.Update(press(modalRect.X+200, modalRect.Y+2))

	if cmd != nil {
		t.Errorf("clicking outside the modal produced %T", result(cmd))
	}
}

func TestClickUpdatesSelectionToMatchCommit(t *testing.T) {
	m := newModal(t)
	if m.Value() {
		t.Fatalf("setup: Initial=false should start on cancel")
	}

	m, _ = m.Update(press(confirmX, buttonsY))

	if !m.Value() {
		t.Errorf("committing via the confirm button left the selection on cancel")
	}
}

func TestStaleRectDeclinesClicks(t *testing.T) {
	m := newModal(t)
	geom.NextGen()

	_, cmd := m.Update(press(confirmX, buttonsY))

	if cmd != nil {
		t.Errorf("a modal with a stale rect responded to a click")
	}
}

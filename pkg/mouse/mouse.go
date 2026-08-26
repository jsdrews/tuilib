// Package mouse carries the mouse event tuilib components actually handle,
// and the click-counting that turns raw presses into single and double
// clicks.
//
// Components never see bubbletea's tea.MouseMsg. The app shell translates it
// into a mouse.Msg first, adding the one thing a component can't work out on
// its own: whether this press is the second of a double click. Detection
// needs a timestamp, a previous position, and a configurable threshold, and
// keeping all three in the shell means no component holds mouse state and no
// component needs a threshold plumbed into its Options.
//
// A component handles mouse input by matching mouse.Msg and testing the rect
// it was last given:
//
//	case mouse.Msg:
//	    if !m.rect.Hit(msg.X, msg.Y) {
//	        return m, nil          // not ours — let a sibling claim it
//	    }
//	    if msg.IsDoubleClick() {
//	        return m, m.activate()
//	    }
//	    if msg.IsPress() {
//	        return m, focus.Request(m)
//	    }
//
// Rect.Hit already rejects events aimed at a component that wasn't drawn in
// the current frame, so a hidden pane declines everything without needing to
// know it's hidden.
package mouse

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// DefaultDoubleClickInterval is the window in which a second press in the
// same cell counts as a double click. 500ms is the common desktop default
// and forgiving enough for terminals over a slow link.
const DefaultDoubleClickInterval = 500 * time.Millisecond

// Msg is a mouse event with its click count resolved. It embeds bubbletea's
// event, so X, Y, Action, Button and the modifier flags read through
// directly.
type Msg struct {
	tea.MouseMsg

	// Clicks is 1 for a single press, 2 for the second press of a double
	// click, and 0 for anything that isn't a button press (wheel, motion,
	// release). It does not climb past 2 — a triple click reads as a fresh
	// single click, since nothing in tuilib binds one.
	Clicks int
}

// IsPress reports whether this is a left-button press. This is the event
// components act on: acting on press rather than release keeps clicks feeling
// immediate, and matches how lazygit and htop behave.
func (m Msg) IsPress() bool {
	return m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft
}

// IsDoubleClick reports whether this press completes a double click. Rule 14
// maps it to the same verb as enter — open whatever is under the cursor.
func (m Msg) IsDoubleClick() bool { return m.IsPress() && m.Clicks >= 2 }

// IsRightPress reports whether this is a right-button press — the gesture
// that asks "what can I do to this?" and opens the action menu.
//
// It carries no click count. Nothing in tuilib binds a right double click, and
// the Tracker only counts left presses, so a second right press is simply
// another right press.
//
// Worth knowing before you rely on it: whether this ever arrives is the
// terminal's decision. Most emulators forward right presses once mouse
// reporting is on, but some (macOS Terminal.app) always show their own context
// menu instead. Right-click is therefore an accelerator on top of a keyboard
// path, never the only way to reach something.
func (m Msg) IsRightPress() bool {
	return m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonRight
}

// IsWheelUp / IsWheelDown report vertical wheel movement. Rule 23 makes the
// wheel behave exactly as the up and down arrows do for the component under
// the pointer.
func (m Msg) IsWheelUp() bool   { return m.Button == tea.MouseButtonWheelUp }
func (m Msg) IsWheelDown() bool { return m.Button == tea.MouseButtonWheelDown }

// IsPointPress reports whether this press is pointing at something — either
// button.
//
// Components use it where the verb is "focus this and put the cursor here",
// which both buttons mean: a right-click opens a menu about the row under the
// pointer, so that row has to become the selection first. Only IsPress may
// *activate* something, which is what keeps a right-click from committing a
// modal button or flipping a toggle.
func (m Msg) IsPointPress() bool { return m.IsPress() || m.IsRightPress() }

// Tracker converts tea.MouseMsg into Msg, counting rapid repeat presses in
// the same cell as double clicks. The app shell owns one; callers running
// without the shell can use one directly.
//
// The zero Tracker uses DefaultDoubleClickInterval.
type Tracker struct {
	interval time.Duration

	lastX, lastY int
	lastPress    time.Time
	clicks       int
}

// NewTracker returns a Tracker using the given double-click window. A
// non-positive interval falls back to DefaultDoubleClickInterval.
func NewTracker(interval time.Duration) Tracker {
	if interval <= 0 {
		interval = DefaultDoubleClickInterval
	}
	return Tracker{interval: interval}
}

// SetInterval updates the double-click window.
func (t *Tracker) SetInterval(d time.Duration) {
	if d > 0 {
		t.interval = d
	}
}

// Track resolves e into a Msg. now is passed in rather than read from the
// clock so the behaviour is testable.
//
// A press counts as a double click when it lands in the same cell as the
// previous press and within the interval. Requiring the same cell — rather
// than merely a nearby one — means a fast click on one list row followed by a
// fast click on another reads as two separate selections, which is what the
// user meant.
func (t *Tracker) Track(e tea.MouseMsg, now time.Time) Msg {
	interval := t.interval
	if interval <= 0 {
		interval = DefaultDoubleClickInterval
	}

	m := Msg{MouseMsg: e}
	if e.Action != tea.MouseActionPress || e.Button != tea.MouseButtonLeft {
		return m
	}

	sameCell := e.X == t.lastX && e.Y == t.lastY
	inTime := !t.lastPress.IsZero() && now.Sub(t.lastPress) <= interval
	if sameCell && inTime && t.clicks == 1 {
		t.clicks = 2
	} else {
		t.clicks = 1
	}

	t.lastX, t.lastY, t.lastPress = e.X, e.Y, now
	m.Clicks = t.clicks
	return m
}

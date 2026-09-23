// Package poll is a thin interval-driven ticker for screens that need to
// auto-refresh remote state — k8s deployment status, Prefect runs, REST
// endpoints — without re-implementing the same tea.Tick + generation-
// counter dance every time.
//
// The shape: construct a Model with an Interval, batch Init() into your
// screen's Init() (returns the first tick cmd), and forward every tea.Msg
// to its Update. When the interval elapses, Update returns RefreshMsg
// alongside the next tick — your screen matches RefreshMsg in its own
// Update, kicks off the fetch, and (when the fetch completes) calls
// MarkRefreshed so LastRefresh() reflects the success.
//
// Pause/Resume are first-class: Pause stops emitting RefreshMsg until
// Resume returns the cmd that re-arms the next tick. SetInterval changes
// the cadence and reschedules. An already-scheduled tick from the prior
// cadence is dropped on arrival instead of firing under the new state.
//
// Every scheduled tick carries a serial number and a Model honours only
// the one it most recently issued, so a tick delivered more than once
// advances the schedule once. That matters because Update re-arms on
// every tick it accepts: without the check, a duplicate delivery would
// return two RefreshMsg and arm two successors, and the chain would
// double every interval until the screen was fetching continuously.
// Two ways to deliver one tick twice are easy to write — calling Update
// twice in a single pass, and one screen instance sitting in the message
// path twice, which the screen stack's fan-out (CLAUDE.md rule 6) makes
// reachable — and neither looks wrong at the call site.
//
// Polling is opt-in to the parent: this package never touches its data,
// only signals "now is a good time to refetch." Pair with the keyed-row
// APIs on pkg/list and pkg/table (SetKeyedItems / SetKeyedRows + the
// matching SelectedKey accessors) so cursor position survives the swap
// when the underlying set has reordered or partially changed.
package poll

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Options configures a new Model. Interval is required; everything else
// has a sane zero-value default.
type Options struct {
	// Interval is the time between refreshes. Must be > 0.
	Interval time.Duration
	// Paused starts the model in the paused state — Init returns nil and
	// no ticks fire until Resume() is called.
	Paused bool
}

// Model owns the ticker state. Embed as a value; mutate via the methods.
type Model struct {
	interval time.Duration
	paused   bool
	last     time.Time

	// tag is the serial number of the tick this Model is waiting for. It
	// is bumped by every reschedule, which is what makes a superseded
	// tick — a duplicate delivery, or one from before a Resume or
	// SetInterval — identifiable as stale on arrival.
	tag int
}

// RefreshMsg is emitted from Update when the interval elapses. Match it
// in your screen's Update to kick off whatever async fetch backs the
// view, then call MarkRefreshed when the fetch resolves.
type RefreshMsg struct{}

// tickMsg is internal — carries the tag it was scheduled under so a stale
// tick can be dropped rather than advancing the schedule.
type tickMsg struct {
	tag int
}

// New constructs a Model. Panics if Interval is not positive — polling
// at a zero interval is meaningless and almost always a bug.
func New(opts Options) Model {
	if opts.Interval <= 0 {
		panic("poll: Interval must be > 0")
	}
	return Model{
		interval: opts.Interval,
		paused:   opts.Paused,
	}
}

// Init returns the cmd that schedules the first tick. Returns nil when
// the model was constructed paused.
//
// It arms the tick this Model is already waiting for rather than issuing
// a new tag, which is what keeps the value receiver honest: a screen
// whose own Init has bubbletea's value receiver calls this on a copy,
// and a tag bumped there would be discarded while the tick carried it —
// leaving every tick rejected and the screen silently never polling.
// Calling Init twice therefore arms two ticks with the same tag, of
// which Update accepts the first and drops the second.
func (m Model) Init() tea.Cmd {
	if m.paused {
		return nil
	}
	return m.armTick()
}

// Update consumes tickMsg and emits RefreshMsg + the next tick cmd when
// the elapsed tick is the one this Model is waiting for. Other messages
// pass through untouched.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	t, ok := msg.(tickMsg)
	if !ok {
		return nil
	}
	if t.tag != m.tag || m.paused {
		return nil
	}
	return tea.Batch(
		func() tea.Msg { return RefreshMsg{} },
		m.scheduleTick(),
	)
}

// MarkRefreshed stamps the last-refresh time with the current wall clock.
// Call from your screen's Update when the fetch backing a RefreshMsg
// resolves so LastRefresh() reflects only successful refreshes.
func (m *Model) MarkRefreshed() {
	m.last = time.Now()
}

// MarkRefreshedAt stamps the last-refresh time with the given instant.
// Useful in tests and when the data carries its own timestamp.
func (m *Model) MarkRefreshedAt(t time.Time) {
	m.last = t
}

// LastRefresh returns the last instant MarkRefreshed was called. Returns
// the zero time before the first refresh — callers that render a "last
// refreshed Xs ago" indicator should special-case time.IsZero() for a
// "never" or "loading…" label.
func (m Model) LastRefresh() time.Time { return m.last }

// Paused reports whether ticks are currently suppressed.
func (m Model) Paused() bool { return m.paused }

// Interval returns the current interval.
func (m Model) Interval() time.Duration { return m.interval }

// Pause stops emitting RefreshMsg. The currently-scheduled tick (if any)
// arrives and is ignored, and nothing re-arms it; a later Resume issues a
// new tag, so that tick stays stale even if it is still in flight then.
func (m *Model) Pause() {
	m.paused = true
}

// Resume re-arms the ticker and returns the cmd that schedules the next
// tick. No-op cmd (nil) if already running. The next tick will fire one
// Interval from now — Resume does not refresh immediately. To force an
// immediate refresh, your screen can dispatch RefreshMsg{} itself
// alongside the Resume() cmd.
func (m *Model) Resume() tea.Cmd {
	if !m.paused {
		return nil
	}
	m.paused = false
	return m.scheduleTick()
}

// SetInterval changes the cadence and reschedules the next tick. The
// previously-scheduled tick is dropped on arrival, because rescheduling
// issues a new tag.
//
// When the model is paused the new interval is recorded and nothing is
// scheduled, and nothing needs retiring either: a tick arriving while
// paused is ignored, and the Resume that ends the pause issues a new tag
// of its own — so a tick from the old cadence cannot outlive it.
func (m *Model) SetInterval(d time.Duration) tea.Cmd {
	if d <= 0 {
		return nil
	}
	m.interval = d
	if m.paused {
		return nil
	}
	return m.scheduleTick()
}

// Refresh returns a cmd that emits a single RefreshMsg. Use to trigger
// an immediate refresh outside the normal cadence — e.g. on user-pressed
// "r" — without disturbing the tick schedule.
func (m Model) Refresh() tea.Cmd {
	return func() tea.Msg { return RefreshMsg{} }
}

// scheduleTick issues a new tag and arms the tick carrying it, which
// retires every tick already in flight.
func (m *Model) scheduleTick() tea.Cmd {
	m.tag++
	return m.armTick()
}

// armTick arms a tick stamped with the tag this Model is waiting for now.
func (m Model) armTick() tea.Cmd {
	tag := m.tag
	return tea.Tick(m.interval, func(time.Time) tea.Msg {
		return tickMsg{tag: tag}
	})
}

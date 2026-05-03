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
// the cadence and reschedules. Both bump an internal generation so any
// already-scheduled tick from the prior cadence is dropped on arrival
// instead of firing under the new state.
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
	gen      int
	last     time.Time
}

// RefreshMsg is emitted from Update when the interval elapses. Match it
// in your screen's Update to kick off whatever async fetch backs the
// view, then call MarkRefreshed when the fetch resolves.
type RefreshMsg struct{}

// tickMsg is internal — carries the generation it was scheduled under so
// stale ticks (from before a Pause/Resume/SetInterval) can be dropped.
type tickMsg struct {
	gen int
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
func (m Model) Init() tea.Cmd {
	if m.paused {
		return nil
	}
	return m.scheduleTick()
}

// Update consumes tickMsg and emits RefreshMsg + the next tick cmd when
// the elapsed tick matches the current generation. Other messages pass
// through untouched.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	t, ok := msg.(tickMsg)
	if !ok {
		return nil
	}
	if t.gen != m.gen || m.paused {
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
// will arrive but be ignored because Pause bumps the generation.
func (m *Model) Pause() {
	if m.paused {
		return
	}
	m.paused = true
	m.gen++
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
	m.gen++
	return m.scheduleTick()
}

// SetInterval changes the cadence and reschedules the next tick. The
// previously-scheduled tick is dropped on arrival via the generation
// bump. When the model is paused, the new interval is recorded but no
// tick is scheduled until Resume.
func (m *Model) SetInterval(d time.Duration) tea.Cmd {
	if d <= 0 {
		return nil
	}
	m.interval = d
	m.gen++
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

func (m Model) scheduleTick() tea.Cmd {
	gen := m.gen
	return tea.Tick(m.interval, func(time.Time) tea.Msg {
		return tickMsg{gen: gen}
	})
}

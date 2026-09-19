// Package activity is per-row in-flight state: the spinner and status label a
// table cell, list row or tree node shows while work against that specific row
// is running.
//
// It is the row-scale counterpart to pane.Pane's loading state. SetLoading
// means "this whole component has no data yet" and replaces the body; activity
// means "two of these forty rows are busy and the other thirty-eight are still
// true", which the pane has no way to say.
//
// # The data is the only source
//
// A component observes its own rows on every keyed swap, a predicate says
// which of them are working, and those rows spin. Nothing else participates:
// no action, no app shell, no second endpoint, no state this package is told
// about out of band. The remote system is the source of truth and this renders
// what it says.
//
// That has one consequence worth stating plainly, because it is a design
// choice and not an oversight: work that begins and ends between two
// observations is never seen, and an action dispatched from the TUI shows
// nothing until a later poll reports it. Covering those needs state this
// package would have to be told about separately.
//
// # Held by key
//
// Every entry is keyed by the key from SetKeyedRows / SetKeyedItems, or a tree
// node's path — the same key marking uses, and for a sharper reason. The whole
// premise of showing a spinner is that something is changing the data
// underneath, so a polled refresh reordering the rows mid-flight is the
// expected case rather than the unlucky one. An indicator held by index would
// drift onto a neighbour exactly when the user is watching it.
//
// # Never pre-styled
//
// Render returns text the caller colors itself, through Options.Style. This
// package emits no escapes, because a table cell needs a foreground-only one
// or it punches a hole in the selected row's background (CLAUDE.md rule 19),
// while a list row can use lipgloss freely. Only the component knows which.
//
// # Ownership
//
// A Set holds its own spinner and drives its own tick chain, so a component
// embedding one gets the animation without a screen re-pushing rows every
// frame. Derive returns a tea.Cmd the caller must propagate, exactly as
// pane.SetLoading does (rule 17). The chain runs while any key is busy and
// stops when the last one settles, so an idle component schedules nothing.
package activity

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"
)

// State is one key's in-flight state.
type State struct {
	// Label is what the row says — "running", "Syncing". It is the value the
	// predicate matched, so it is the server's own word rather than one
	// invented here.
	Label string

	// Since is when this key was first observed busy, not when it was last
	// observed. Elapsed time therefore measures the work rather than the poll
	// that last saw it.
	Since time.Time
}

// Options configures a Set.
type Options struct {
	// Spinner is the frame set. Nil means spinner.Dot, matching pane.
	Spinner *spinner.Spinner

	// Style colors the rendered indicator.
	//
	// A function rather than a lipgloss.Style because a table cell needs a
	// foreground-only escape (rule 19) and a list row does not — and only the
	// component knows which it is. The theme builders supply the right form
	// per component. Nil leaves the text plain.
	Style func(st State, text string) string
}

// Set is the keyed collection of busy rows plus the spinner that animates
// them. Components embed one.
//
// It is a value, and every mutating method takes a pointer: Derive replaces
// the collection outright rather than editing it, so a copy taken beforehand
// keeps the observation it was made with. A component therefore has to keep
// the Set it mutated — which is what bubbletea's value-receiver Update already
// does by returning the model.
type Set struct {
	// busy is what the last observation said. There is no second layer: this
	// map is replaced wholesale by every Derive, so it is never anything but
	// the most recent read.
	busy map[string]State

	spin  spinner.Model
	style func(State, string) string

	// ticking is a claim that a tick chain is in flight, and lastTick is when
	// that claim was last true. Both, because the claim can become false
	// without this Set hearing about it — see revive.
	ticking  bool
	lastTick time.Time
}

// New builds a Set from opts.
func New(opts Options) Set {
	frames := spinner.Dot
	if opts.Spinner != nil {
		frames = *opts.Spinner
	}
	return Set{
		busy:  map[string]State{},
		spin:  spinner.New(spinner.WithSpinner(frames)),
		style: opts.Style,
	}
}

// Derive replaces the whole collection from one observation of the data.
//
// busy maps the keys that are working to what they should say; every key
// absent from it is not working. Wholesale rather than incremental because an
// observation *is* the whole truth as of that moment — an incremental API
// would make "this row stopped being busy" something the caller has to notice
// and report, which is the bookkeeping this exists to remove.
//
// A key's Since survives across observations, so a row that stays busy keeps
// measuring from when the work was first seen.
//
// The returned command is the animation's first tick; batch it into your
// screen's command stream the way SetLoading's is batched.
func (s *Set) Derive(busy map[string]string) tea.Cmd {
	if s.busy == nil {
		s.busy = map[string]State{}
	}
	next := make(map[string]State, len(busy))
	now := time.Now()
	for k, label := range busy {
		st := State{Label: label, Since: now}
		if prev, ok := s.busy[k]; ok {
			st.Since = prev.Since
		}
		next[k] = st
	}
	s.busy = next
	return s.armTick()
}

// Handle advances the animation. Components call it from Update and return the
// command; every message that is not a spinner tick is a no-op beyond the
// chance to notice the chain has stalled.
func (s *Set) Handle(msg tea.Msg) tea.Cmd {
	if m, ok := msg.(spinner.TickMsg); ok {
		return s.tick(m)
	}
	return s.revive()
}

// Adopt takes over another Set's entries, keeping this Set's own options.
//
// This is the rule-4 primitive: a theme swap rebuilds the component, so the
// new Set carries the new palette and the old observation. Without it a swap
// would blank every indicator until the next poll.
func (s *Set) Adopt(other Set) tea.Cmd {
	// Copied, not aliased: the two Sets are independent afterwards, so the
	// discarded one cannot write into the live one through a shared map.
	next := make(map[string]State, len(other.busy))
	for k, st := range other.busy {
		next[k] = st
	}
	s.busy = next
	return s.armTick()
}

// State reports one key's state.
func (s Set) State(key string) (State, bool) {
	st, ok := s.busy[key]
	return st, ok
}

// Active reports whether any key is busy.
func (s Set) Active() bool { return len(s.busy) > 0 }

// Count is how many keys are busy.
func (s Set) Count() int { return len(s.busy) }

// Render is the indicator for one key, fitted to width and styled.
//
// Below the width the label needs, the glyph alone is drawn: a cell of eight
// showing "⣾ syncin" is worse than one showing "⣾". Reports false when the key
// is not busy, so a caller can fall through to whatever the row normally
// shows — which is the whole of how an indicator ends. There is no outcome
// glyph, because when "running" becomes "failed" the cell goes back to
// rendering the row's own value, which already says "failed" in the app's own
// colours; a ✗ held over the top restates it less precisely.
func (s Set) Render(key string, width int) (string, bool) {
	st, ok := s.busy[key]
	if !ok || width <= 0 {
		return "", false
	}
	// spinner.Dot's frames carry a trailing space, which would put two between
	// the glyph and the label.
	text := strings.TrimRight(s.spin.View(), " ")
	if st.Label != "" && xansi.StringWidth(text)+1+xansi.StringWidth(st.Label) <= width {
		text += " " + st.Label
	}
	if xansi.StringWidth(text) > width {
		text = xansi.Cut(text, 0, width)
	}
	if s.style != nil {
		text = s.style(st, text)
	}
	return text, true
}

// Badge right-aligns key's indicator at the end of a row width cells wide.
//
// The row is what gives way when the two cannot both fit: the badge is the
// news, and a row label the user can already read most of loses less by being
// cut than the indicator does by disappearing. The indicator is capped at half
// the width so a long label cannot swallow the row entirely.
//
// Returns row untouched when the key is not busy, so a caller can compose
// unconditionally. Row may already carry escapes — widths are measured and
// cuts made ANSI-aware.
func (s Set) Badge(key, row string, width int) string {
	if width <= 0 {
		return row
	}
	text, ok := s.Render(key, width/2)
	if !ok {
		return row
	}
	bw := xansi.StringWidth(text)
	if bw >= width {
		return text
	}
	avail := width - bw - 1
	rw := xansi.StringWidth(row)
	if rw > avail {
		row = xansi.Cut(row, 0, avail)
		rw = avail
	}
	return row + strings.Repeat(" ", avail-rw+1) + text
}

func (s *Set) armTick() tea.Cmd {
	if s.ticking || !s.Active() {
		return nil
	}
	s.ticking = true
	s.lastTick = time.Now()
	return s.spin.Tick
}

func (s *Set) tick(m spinner.TickMsg) tea.Cmd {
	if !s.Active() {
		s.ticking = false
		return nil
	}
	s.lastTick = time.Now()
	var cmd tea.Cmd
	s.spin, cmd = s.spin.Update(m)
	return cmd
}

// revive restarts an animation whose chain was broken from outside.
//
// The chain lives in tea.Cmds, and a component keeps it alive only by
// receiving the ticks it asked for. Anything that stops delivering messages to
// a component — a screen stack that routes to the top screen only, a tab that
// hides a body — ends the chain, while ticking stays true because nothing told
// this Set otherwise, and armTick then refuses to start another: the spinner
// is frozen for good on a row that is still working.
//
// Stopping while hidden is correct; staying stopped is not. So instead of
// trusting the flag, check whether a tick has actually arrived recently and
// re-arm if not. A duplicate chain is harmless: bubbles tags each tick and a
// spinner rejects one from a superseded chain, so two chains collapse back
// into one on the next frame.
func (s *Set) revive() tea.Cmd {
	if !s.Active() {
		return nil
	}
	if !s.ticking {
		return s.armTick()
	}
	// Generous against the frame interval: normal jitter must not look like a
	// dead chain, and being slightly late to revive costs nothing.
	if fps := s.spin.Spinner.FPS; fps > 0 && time.Since(s.lastTick) < 4*fps {
		return nil
	}
	s.lastTick = time.Now()
	return s.spin.Tick
}

// Busy builds the ordinary predicate: a case-insensitive match against the
// values that mean "in progress", labelled with the value as it appeared.
//
//	activity.Busy("running", "pending", "waiting")
//
// Values are compared with surrounding space and any ANSI styling stripped, so
// a cell coloured by the screen still matches.
func Busy(values ...string) func(value string) (label string, busy bool) {
	set := lowered(values)
	return func(value string) (string, bool) {
		plain := strings.TrimSpace(xansi.Strip(value))
		if set[strings.ToLower(plain)] {
			return plain, true
		}
		return "", false
	}
}

// Settled is Busy inverted: it names the values that mean nothing is
// happening, and reports everything else as in flight, labelled with the value
// as it appeared.
//
//	activity.Settled("successful", "failed", "canceled")
//
// Prefer it when the API documents a set of *terminal* states, which is how
// most of them are written — and because it keeps working when the server
// learns a new in-progress status, where a list of busy values would quietly
// treat that new one as settled and stop spinning. The trade is the mirror
// image: a new *settled* status this does not know about spins forever. Pick
// the list the server is less likely to extend.
//
// An empty value is settled: a blank cell is not work in progress.
func Settled(values ...string) func(value string) (label string, busy bool) {
	set := lowered(values)
	return func(value string) (string, bool) {
		plain := strings.TrimSpace(xansi.Strip(value))
		if plain == "" || set[strings.ToLower(plain)] {
			return "", false
		}
		return plain, true
	}
}

func lowered(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[strings.ToLower(strings.TrimSpace(v))] = true
	}
	return set
}

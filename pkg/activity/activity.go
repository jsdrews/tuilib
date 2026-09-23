// Package activity is per-row in-flight state: the spinner and status label a
// table cell, list row or tree node shows while work against that specific row
// is running.
//
// It is the row-scale counterpart to pane.Pane's loading state. SetLoading
// means "this whole component has no data yet" and replaces the body; activity
// means "two of these forty rows are busy and the other thirty-eight are still
// true", which the pane has no way to say.
//
// # The data is the source, and there is one map
//
// A component observes its own rows on every keyed swap, a predicate says
// which of them are working, and those rows spin. When the busy-ness is not a
// field on the row — an operations API, a job status resource — the screen
// hands the same map over with SetBusy instead. Two entrances, one map, one
// writer: a component uses the predicate or it is told, never both, which is
// what makes this feature unable to contradict itself. No action broadcast, no
// app shell involvement, and nothing accumulates: every observation replaces
// the collection outright.
//
// # Work the user just started
//
// One thing is not in the data yet. A POST returns in milliseconds and the
// next poll is seconds away, so between the keypress and the observation that
// reports it the row reads exactly as it did before — the thing the verb was
// about says nothing. Expect covers that window with a claim, and every
// observation retires claims (Options.Settle). A claim is not a second opinion
// about the same question: nothing writes back into it, and it survives only
// until the data can speak to it.
//
// Two rules live in the screen rather than here, because this package has no
// idea a fetch exists. It must not apply a read taken across its own write, or
// a reply that predates the keypress clears the claim. And it must retract
// when a read fails, because an outage is exactly the case where no
// observation is coming to retire anything.
//
// What remains outside all of this, as a design choice rather than an
// oversight: work that somebody else began and ended between two observations
// is never seen.
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

// claim is one expected entry: what the screen said it asked for, the value
// the key showed when it said so, and how many uninformative observations the
// entry has left.
type claim struct {
	st    State
	at    string
	spare int
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

	// Settle is how many observations that say nothing new an unconfirmed
	// expected claim survives — see Expect.
	//
	// Counted in observations rather than seconds, because no duration the
	// client can measure is the right length of that wait: it is set by the
	// server's reconcile lag, which is neither published nor stable. An
	// observation is spent only when it repeats the value the key had when the
	// claim was made; one that reports the key busy confirms the claim
	// instead, and one that reports a different settled value retires it
	// outright, whichever way the allowance stands.
	//
	// Zero — the default, and correct for any API whose handler sets the
	// status inline — retires a claim on the next observation. A reconciler
	// wants one or two, not a number tuned to its control loop.
	Settle int
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

	// expected is what the screen has asked the server to do and the server
	// has not yet been observed doing. Every observation retires entries from
	// it; nothing else ever reads back into it. It is deliberately not a
	// second opinion about the same question — an entry survives only until
	// the data can speak to it.
	expected map[string]claim

	// scope is the keys the component currently holds, and held is whether it
	// has ever been told. Both, because a component that holds no rows is not
	// the same as one that has never said which rows it holds, and the first
	// must scope everything away while the second scopes nothing.
	//
	// Entries outside it are kept but not rendered, counted or animated: a map
	// from an operations endpoint can legitimately name a row on another page
	// of a paged table, and discarding it on arrival would leave that row
	// inert when it scrolls into view.
	scope map[string]bool
	held  bool

	settle int

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
		busy:     map[string]State{},
		expected: map[string]claim{},
		settle:   opts.Settle,
		spin:     spinner.New(spinner.WithSpinner(frames)),
		style:    opts.Style,
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
func (s *Set) Derive(busy map[string]string) tea.Cmd { return s.Observe(busy, nil) }

// Observe is Derive with the values an expected claim is judged against.
//
// values carries the current value of each key the caller is watching — for a
// table the activity column's cell, for a list the item's text. Only keys with
// a claim against them are ever consulted, so a caller builds it from
// Expecting and it is empty in the ordinary case. Passing nil makes every
// observation an uninformative one, which is the right reading when the values
// are not available: a screen on the SetBusy entrance holds them itself and
// hands over busy keys alone.
//
// The three ways a claim ends are documented on Options.Settle. They are all
// here because they are all the same decision — how much this observation is
// entitled to say about a row the user just acted on.
func (s *Set) Observe(busy, values map[string]string) tea.Cmd {
	if s.busy == nil {
		s.busy = map[string]State{}
	}
	next := make(map[string]State, len(busy))
	now := time.Now()
	for k, label := range busy {
		st := State{Label: label, Since: now}
		switch prev, ok := s.busy[k]; {
		case ok:
			st.Since = prev.Since
		default:
			// A key arriving busy for the first time inherits the moment the
			// screen claimed it, when it claimed it — so elapsed time measures
			// the work from the keypress rather than from the poll that first
			// caught up with it.
			if c, claimed := s.expected[k]; claimed {
				st.Since = c.st.Since
			}
		}
		next[k] = st
	}
	s.busy = next
	s.retire(busy, values)
	return s.armTick()
}

// retire applies one observation to the expected map.
func (s *Set) retire(busy, values map[string]string) {
	for k, c := range s.expected {
		if _, confirmed := busy[k]; confirmed {
			delete(s.expected, k)
			continue
		}
		if v, seen := values[k]; seen && v != c.at {
			// The server acted: the key is settled at a value it did not hold
			// when the claim was made, so the work is over rather than pending.
			delete(s.expected, k)
			continue
		}
		if c.spare <= 0 {
			delete(s.expected, k)
			continue
		}
		c.spare--
		s.expected[k] = c
	}
}

// Expect records that the screen has asked the server to work on keys, before
// any observation can say so.
//
// It is a claim, not a second source of truth: every observation retires
// entries from it (see Options.Settle) and nothing ever writes back into it, so
// it cannot outlive the data's ability to speak to it. at carries each key's
// current value, which is what lets a later observation notice the server
// acted; a caller without values passes nil.
//
// label is the guess — "Syncing" — and it is what the row says until an
// observation replaces it with the server's own word.
//
// The returned command is the animation's first tick, exactly as Derive's is.
func (s *Set) Expect(keys []string, label string, at map[string]string) tea.Cmd {
	if len(keys) == 0 {
		return nil
	}
	if s.expected == nil {
		s.expected = map[string]claim{}
	}
	now := time.Now()
	for _, k := range keys {
		if k == "" {
			continue
		}
		s.expected[k] = claim{
			st:    State{Label: label, Since: now},
			at:    at[k],
			spare: s.settle,
		}
	}
	return s.armTick()
}

// Expecting is the keys currently carrying a claim.
//
// A component calls it to find out which values are worth collecting for the
// next Observe. It is empty whenever nobody has dispatched anything, which is
// almost always, so the collection costs nothing in the ordinary case.
func (s Set) Expecting() []string {
	if len(s.expected) == 0 {
		return nil
	}
	keys := make([]string, 0, len(s.expected))
	for k := range s.expected {
		keys = append(keys, k)
	}
	return keys
}

// Retract drops the claims on keys, for a request that failed.
//
// Politeness on a write that was refused — the next observation would retire
// the claim anyway — and the honest thing on a key the screen has stopped
// acting on.
func (s *Set) Retract(keys ...string) {
	for _, k := range keys {
		delete(s.expected, k)
	}
}

// RetractAll drops every claim, for a read that failed.
//
// This one is not politeness. A claim is a promise that the next observation
// will explain it, and when observations have stopped arriving — an outage, a
// fetch that errored, a poll the user paused — there is nothing left to keep
// that promise. Holding the claim then asserts something the screen has no way
// to support, for as long as the interruption lasts rather than for one poll.
func (s *Set) RetractAll() { clear(s.expected) }

// Scope tells the Set which keys the component currently holds.
//
// Entries outside it are kept but neither rendered, counted nor animated. That
// matters only on the SetBusy entrance, where the map comes from somewhere
// other than the rows and can name one the component does not hold — on another
// page of a paged table, or gone since the last read. A predicate's map is a
// subset of the rows by construction, so scoping is the identity there.
//
// Keys are kept rather than discarded because rows and busy-ness arrive on
// separate cadences: a key the component does not hold yet is the ordinary case
// on a paged table, and dropping it on arrival would leave that row inert when
// it scrolls into view.
func (s *Set) Scope(keys []string) {
	scope := make(map[string]bool, len(keys))
	for _, k := range keys {
		if k != "" {
			scope[k] = true
		}
	}
	s.scope, s.held = scope, true
}

// visible reports whether key is one the component currently holds.
func (s Set) visible(key string) bool { return !s.held || s.scope[key] }

// stateOf is the union the renderers read: what was observed, else what was
// claimed, and nothing for a key outside the component's scope.
func (s Set) stateOf(key string) (State, bool) {
	if !s.visible(key) {
		return State{}, false
	}
	// Observed beats claimed. The label is then the server's own word rather
	// than the screen's guess, which is the whole reason the claim is allowed
	// to be superseded silently.
	if st, ok := s.busy[key]; ok {
		return st, true
	}
	c, ok := s.expected[key]
	return c.st, ok
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

	// The claims and the scope come too. A theme swap that dropped them would
	// blank the row the user just acted on and re-run the tick chain against an
	// unscoped map, both of which are visible within one frame.
	claims := make(map[string]claim, len(other.expected))
	for k, c := range other.expected {
		claims[k] = c
	}
	s.expected = claims
	if other.held {
		scope := make(map[string]bool, len(other.scope))
		for k, v := range other.scope {
			scope[k] = v
		}
		s.scope, s.held = scope, true
	}
	return s.armTick()
}

// State reports one key's state — observed if the last read said so, claimed
// if the screen has asked for work the read has not caught up with yet.
func (s Set) State(key string) (State, bool) { return s.stateOf(key) }

// Active reports whether any key the component holds is working.
func (s Set) Active() bool { return s.Count() > 0 }

// Count is how many of the component's keys are working — observed or claimed.
//
// Scoped, so it answers "how many of my rows" rather than "how many entries do
// I hold": a SetBusy map naming rows on another page would otherwise animate a
// spinner nothing can see and inflate a counter a screen might put in a title.
func (s Set) Count() int {
	n := 0
	for k := range s.busy {
		if s.visible(k) {
			n++
		}
	}
	for k := range s.expected {
		if _, both := s.busy[k]; both {
			continue
		}
		if s.visible(k) {
			n++
		}
	}
	return n
}

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
	st, ok := s.stateOf(key)
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

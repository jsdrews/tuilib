// Package activity is per-row in-flight state: the spinner and status label a
// table cell, list row or tree node shows while work against that specific row
// is running.
//
// It is the row-scale counterpart to pane.Pane's loading state. SetLoading
// means "this whole component has no data yet" and replaces the body; activity
// means "two of these forty rows are busy and the other thirty-eight are still
// true", which the pane has no way to say.
//
// # The model
//
// The server is the source of truth. When the user starts work locally, the
// row is told at once, and that claim lasts until the truth can take over.
//
// A component observes its own rows on every keyed swap, a predicate says
// which of them are working, and those rows spin. Every observation replaces
// the collection outright; nothing accumulates.
//
// Work the user starts is an operation (op.go): Dispatch opens a claim, Done
// reports that the request answered, and the Mode says who knows when the
// work ends — the server's status (Observed) or the request itself (Held).
// BeginRead and Accept order reads against those answers, so a read taken
// across a write cannot end its claim and a failed read withdraws what was
// waiting on one. Screens reach all of this through their component —
// table.Dispatch, table.ApplyRead, and the same on list and tree — and rarely
// touch a Set.
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
	"sort"
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

	// Unknown is true while reads are failing and this key is busy only
	// because the last successful read said so. The work may have finished
	// since; nobody can see. Render draws it static, with a "?" where the
	// spinner would be — neither cleared, which would invent a read nobody
	// took, nor animated, which would claim someone is still watching.
	Unknown bool
}

// claim is one expected entry: what the screen said it asked for, the value
// the key showed when it said so, and how many uninformative observations the
// entry has left.
type claim struct {
	st    State
	at    string
	spare int

	// The v2 fields. op is the Dispatch that made this claim, zero for a v1
	// Expect. mode decides what ends it. acked is whether the dispatching
	// request has answered, and ackAt is the read clock when it did: an
	// observation taken before that cannot speak to the claim, because it
	// cannot know about the write.
	op    uint64
	mode  Mode
	acked bool
	ackAt uint64
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
	// claim survives — an Observed Dispatch, or an Expect.
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
// It is a value, and every mutating method takes a pointer: ObserveRows replaces
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

	// clock orders reads against acknowledgements; see BeginRead. applied is
	// the clock value of the newest read accepted, and obsAt is the one the
	// next observation was taken at — set by Accept, and otherwise "now",
	// which is right for a stream whose events arrive in order.
	clock   uint64
	applied uint64
	obsAt   uint64
	fromRd  bool

	// failing is whether the newest read settled was a failure. Set by
	// Accept, cleared by any observation.
	failing bool

	// statuses is each key's status as of the last observation — label plus
	// revision — which is what Dispatch records a claim against.
	statuses map[string]string

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

// ObserveRows is one observation of the rows a component holds: it runs eval
// over each and replaces the whole collection with what it reports.
//
// keys are the rows' keys in order; eval(i) reports row i's label, whether it
// is busy, and an optional revision. The label and revision together are the
// row's status — what a dispatched claim compares against to notice the server
// acted — and the label alone is what a busy row shows. A nil eval means the
// component has no predicate: nothing is observed busy, but the read still
// counts down Observed claims, or a screen that dispatches without one would
// spin forever.
//
// Wholesale rather than incremental because an observation *is* the whole
// truth as of that moment. A key's Since survives across observations, so a
// row that stays busy keeps measuring from when the work was first seen.
//
// The returned command is the animation's first tick; a component batches it
// into its own command stream.
func (s *Set) ObserveRows(keys []string, eval func(i int) (label string, busy bool, rev string)) tea.Cmd {
	s.setScope(keys)
	if eval == nil {
		if len(s.expected) == 0 {
			return nil
		}
		return s.observe(nil, nil)
	}
	busy := map[string]string{}
	statuses := make(map[string]string, len(keys))
	for i, k := range keys {
		if k == "" {
			continue
		}
		label, b, rev := eval(i)
		if b {
			busy[k] = label
		}
		if rev != "" {
			label += "\x1f" + rev
		}
		statuses[k] = label
	}
	s.statuses = statuses
	// Unconditionally, including when nothing matches: an empty observation
	// is a real one, and the only thing that can stop the last spinner.
	return s.observe(busy, statuses)
}

// observe applies one observation: busy is every working key and its label,
// values each key's status for judging claims.
//
// Nil values makes every observation an uninformative one for claims.
//
// The three ways a claim ends are documented on Options.Settle. They are all
// here because they are all the same decision — how much this observation is
// entitled to say about a row the user just acted on.
func (s *Set) observe(busy, values map[string]string) tea.Cmd {
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
	s.failing = false
	at := s.clock
	if s.fromRd {
		at, s.fromRd = s.obsAt, false
	}
	report := s.retire(busy, values, at)
	return tea.Batch(s.armTick(), report)
}

// retire applies one observation, taken at read clock at, to the claims.
//
// A key reported busy confirms its claim at any time: that is the server's own
// word, whenever the read was taken. Everything else depends on the claim's
// mode. A Held claim is ended by Done and nothing an observation says reaches
// it. An Observed claim is judged only by reads taken after its request
// answered — a read from before then cannot know about the write — and those
// end it by a changed status, or by running out of allowance.
//
// An Observed claim from Dispatch that ends without ever being confirmed is
// reported, grouped by operation, so the screen can say something rather
// than let the row look ignored.
func (s *Set) retire(busy, values map[string]string, at uint64) tea.Cmd {
	type ended struct {
		msg UnobservedMsg
	}
	var reports map[uint64]*ended
	unobserved := func(k string, c claim, changed bool) {
		if c.op == 0 {
			return
		}
		if reports == nil {
			reports = map[uint64]*ended{}
		}
		e, ok := reports[c.op]
		if !ok {
			e = &ended{msg: UnobservedMsg{Op: c.op, Label: c.st.Label}}
			reports[c.op] = e
		}
		e.msg.Keys = append(e.msg.Keys, k)
		e.msg.Changed = e.msg.Changed || changed
	}

	for k, c := range s.expected {
		if _, confirmed := busy[k]; confirmed {
			if c.mode == Held {
				continue // the observed label shows; Done still ends it
			}
			delete(s.expected, k)
			continue
		}
		if c.mode == Held {
			continue
		}
		if c.op != 0 && (!c.acked || at < c.ackAt) {
			continue // taken across the write: says nothing about it
		}
		if v, seen := values[k]; seen && v != c.at {
			// The server acted: the key is settled at a value it did not hold
			// when the claim was made, so the work is over rather than pending.
			delete(s.expected, k)
			unobserved(k, c, true)
			continue
		}
		if c.spare <= 0 {
			delete(s.expected, k)
			unobserved(k, c, false)
			continue
		}
		c.spare--
		s.expected[k] = c
	}

	if len(reports) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(reports))
	for _, e := range reports {
		sort.Strings(e.msg.Keys)
		msg := e.msg
		cmds = append(cmds, func() tea.Msg { return msg })
	}
	return tea.Batch(cmds...)
}

// setScope records the keys the component currently holds. Entries outside it
// — a claim on a row a refresh removed, or dispatched before its row arrived —
// are kept but neither rendered, counted nor animated, so the row lights up if
// it comes back.
func (s *Set) setScope(keys []string) {
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
	c, claimed := s.expected[key]
	if st, ok := s.busy[key]; ok {
		// A claim still standing here is a request still open, so the key is
		// live whatever the reads are doing.
		st.Unknown = s.failing && !claimed
		return st, true
	}
	return c.st, claimed
}

// ReadsFailing reports whether the newest read failed and no observation has
// arrived since. Rows busy only on the strength of an earlier read are drawn
// Unknown meanwhile; a screen typically says so on its title as well.
func (s Set) ReadsFailing() bool { return s.failing }

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
	s.clock, s.applied, s.failing = other.clock, other.applied, other.failing
	s.statuses = make(map[string]string, len(other.statuses))
	for k, v := range other.statuses {
		s.statuses[k] = v
	}
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
// I hold": a claim on a row a refresh removed would otherwise animate a spinner
// nothing can see and inflate a counter a screen might put in a title.
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
	if st.Unknown {
		text = "?"
	}
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
//
// The value comes back plain whether or not it matched: under a component's
// BusyWhen it is the row's status, which an operation compares against to
// notice the server acted, so a settled row has to say what it settled at.
func Busy(values ...string) func(value string) (label string, busy bool) {
	set := lowered(values)
	return func(value string) (string, bool) {
		plain := strings.TrimSpace(xansi.Strip(value))
		return plain, set[strings.ToLower(plain)]
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
		return plain, plain != "" && !set[strings.ToLower(plain)]
	}
}

func lowered(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[strings.ToLower(strings.TrimSpace(v))] = true
	}
	return set
}

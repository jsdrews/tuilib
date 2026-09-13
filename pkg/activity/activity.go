// Package activity is per-row in-flight state: the spinner and status label a
// table cell, list row or tree node shows while work against that specific row
// is running.
//
// It is the row-scale counterpart to pane.Pane's loading state. SetLoading
// means "this whole component has no data yet" and replaces the body;
// activity means "two of these forty rows are busy and the other thirty-eight
// are still true", which the pane has no way to say.
//
// # Held by key
//
// Every entry is keyed the way marks are — by the key from SetKeyedRows /
// SetKeyedItems, or a tree node's path — and for the same reason, only more
// so. The whole premise of showing a spinner is that something is changing the
// data underneath, so a polled refresh reordering the rows mid-flight is the
// expected case rather than the unlucky one. An indicator held by index would
// drift onto a neighbour exactly when the user is watching it.
//
// # Never pre-styled
//
// Render returns text a caller colors itself, through Options.Style. This
// package holds no lipgloss import and emits no escapes, for the reason
// pkg/glyph gives: a table cell needs a foreground-only escape or it punches a
// hole in the selected row's background (CLAUDE.md rule 19), while a list row
// can use lipgloss freely. Only the component knows which it is.
//
// # Ownership
//
// A Set holds its own spinner and drives its own tick chain, so a component
// embedding one gets the animation without a screen having to re-push rows
// every frame. Setters return a tea.Cmd the caller must propagate, exactly as
// pane.SetLoading does (rule 17). The chain runs while any key is running and
// stops when the last one finishes, so an idle component schedules nothing.
package activity

import (
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/glyph"
)

// DefaultHold is how long a finished key shows its outcome before clearing.
//
// A spinner that simply vanishes leaves no evidence of what happened, and the
// statusbar receipt is one line for a whole run and wiped by the next keypress
// (rule 20). When six rows are working and one fails, the per-row outcome is
// the only surface that says which.
const DefaultHold = 2 * time.Second

// DefaultConfirm bounds decision 19's handoff: how long an entry whose action
// succeeded keeps the row moving while it waits for the data to catch up.
//
// It is not a guess about how long the work takes — the handoff normally ends
// at the next observation, whenever that is. It is a bound on how long the
// library will wait to be told anything at all, for the screen whose polling
// is paused, whose tab nobody is looking at, or whose endpoint is down.
const DefaultConfirm = 30 * time.Second

// State is one key's in-flight state.
type State struct {
	// Label is what the row says — "syncing", "syncing 3/7".
	Label string

	// RunID is the run that owns this entry, for the broadcasts that address
	// a whole run at once. Zero when the entry was set directly.
	RunID int64

	// Since is when the work started, not when the label last changed.
	Since time.Time

	// Done reports that the work finished and the entry is showing its
	// outcome before it clears.
	Done bool

	// Err is the outcome, non-nil on failure. Meaningless unless Done.
	Err error

	// Changed reports that this entry is a revision flash — the row's work
	// changed somewhere the TUI never saw running (decision 20) — rather than
	// the outcome of something it watched. Meaningless unless Done.
	Changed bool
}

// Running reports whether the work is still in flight.
func (s State) Running() bool { return !s.Done }

// Failed reports whether the work finished unsuccessfully.
func (s State) Failed() bool { return s.Done && s.Err != nil }

// Options configures a Set.
type Options struct {
	// Spinner is the frame set. Nil means spinner.Dot, matching pane.
	Spinner *spinner.Spinner

	// Glyphs supplies ActivityOK / ActivityFail. Empty fields resolve to
	// glyph.Default.
	Glyphs glyph.Set

	// Hold is how long a finished key shows its outcome. Zero means
	// DefaultHold; negative means hold indefinitely, for a component whose
	// data will not refresh on its own and whose owner will call Clear.
	Hold time.Duration

	// Confirm caps how long a locally-started entry waits for the data to
	// confirm it finished, before falling back to reporting its own outcome.
	// Zero means DefaultConfirm. Only consulted once Derive has been called
	// at least once — with no source of truth there is nothing to wait for.
	Confirm time.Duration

	// Style colors the rendered indicator.
	//
	// A function rather than a lipgloss.Style because a table cell needs a
	// foreground-only escape (rule 19) and a list row does not — and only the
	// component knows which it is. The theme builders supply the right form
	// per component. Nil leaves the text plain.
	Style func(st State, text string) string
}

// Set is the keyed collection of in-flight rows plus the spinner that animates
// them. Components embed one.
//
// Copies share their entries, the same way a component's marks map does: a
// value-receiver Update that hands back a copy still sees writes made through
// a pointer elsewhere. The spinner and the ticking flag are value state, so
// they travel with whichever copy the component stores.
type Set struct {
	// local is what this session started: SetActivity, or a shell broadcast.
	// derived is what the data last said (decision 18). A key may appear in
	// both, and local wins — see stateOf.
	local   map[string]*entry
	derived map[string]State

	// revs is the last revision observed per key, for decision 20's flash.
	revs map[string]string

	// settled is the keys whose change the current observation already
	// explains: those the previous one reported busy, and those whose handoff
	// it just retired. A flash on top of a spinner the user watched all the
	// way to its end reports the same news twice — see Revise.
	settled map[string]bool

	// derives records that Derive has been called at least once, which is how
	// the Set knows a source of truth exists to hand off to. A component with
	// no ActivityWhen never calls Derive, so decision 19 switches itself off
	// exactly where it could not work.
	derives bool

	spin    spinner.Model
	glyphs  glyph.Set
	hold    time.Duration
	confirm time.Duration
	style   func(State, string) string
	ticking bool
}

type entry struct {
	State

	// gen invalidates a hold timer whose key has since been restarted or
	// finished again, so a stale tea.Tick cannot clear a live entry.
	gen int64

	// awaiting is decision 19: the action finished successfully and this
	// entry is holding the row until the data is next observed. It still
	// renders as running, because that is what the TUI last knew.
	awaiting bool
}

// New builds a Set from opts.
func New(opts Options) Set {
	frames := spinner.Dot
	if opts.Spinner != nil {
		frames = *opts.Spinner
	}
	hold := opts.Hold
	if hold == 0 {
		hold = DefaultHold
	}
	confirm := opts.Confirm
	if confirm == 0 {
		confirm = DefaultConfirm
	}
	return Set{
		local:   map[string]*entry{},
		derived: map[string]State{},
		revs:    map[string]string{},
		spin:    spinner.New(spinner.WithSpinner(frames)),
		glyphs:  opts.Glyphs.Resolve(),
		hold:    hold,
		confirm: confirm,
		style:   opts.Style,
	}
}

// StartMsg begins activity on Keys. Broadcast by the app shell when a run it
// launched reports itself started; a component applies only the keys it holds.
type StartMsg struct {
	Keys  []string
	Label string
	RunID int64
}

// UpdateMsg relabels every key belonging to RunID — the row-facing half of an
// action's progress reporting. See Progress.
type UpdateMsg struct {
	RunID int64
	Label string
}

// EndMsg finishes every key belonging to RunID, with Err deciding the outcome
// glyph.
type EndMsg struct {
	RunID int64
	Err   error
}

// clearMsg retires one key after its hold elapses.
type clearMsg struct {
	key string
	gen int64
}

// expireMsg ends decision 19's wait for a key the data never confirmed.
type expireMsg struct {
	key string
	gen int64
}

// Handle applies a broadcast or a spinner tick, and returns any command that
// keeps the animation running.
//
// holds reports whether this component owns a key; a nil holds accepts every
// key in a StartMsg. Keys the component does not hold are ignored, which is
// the same decline-what-isn't-yours behaviour components already perform for
// mouse events outside their rect (rule 28).
func (s *Set) Handle(msg tea.Msg, holds func(key string) bool) tea.Cmd {
	switch m := msg.(type) {
	case spinner.TickMsg:
		return s.tick(m)

	case StartMsg:
		keys := m.Keys
		if holds != nil {
			keys = keys[:0:0]
			for _, k := range m.Keys {
				if holds(k) {
					keys = append(keys, k)
				}
			}
		}
		return s.StartRun(keys, m.Label, m.RunID)

	case UpdateMsg:
		s.RelabelRun(m.RunID, m.Label)

	case EndMsg:
		return s.FinishRun(m.RunID, m.Err)

	case clearMsg:
		s.clearGen(m.key, m.gen)

	case expireMsg:
		return s.expire(m.key, m.gen)
	}
	return nil
}

// Start begins activity on one key, replacing any entry already there.
func (s *Set) Start(key, label string) tea.Cmd { return s.start(key, label, 0) }

// StartRun begins activity on every key of a run.
func (s *Set) StartRun(keys []string, label string, runID int64) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		if c := s.start(k, label, runID); c != nil {
			cmd = c
		}
	}
	return cmd
}

func (s *Set) start(key, label string, runID int64) tea.Cmd {
	if key == "" {
		return nil
	}
	if s.local == nil {
		s.local = map[string]*entry{}
	}
	e, ok := s.local[key]
	if !ok {
		e = &entry{}
		s.local[key] = e
	}
	e.gen++
	e.awaiting = false
	e.State = State{Label: label, RunID: runID, Since: time.Now()}
	return s.armTick()
}

// Relabel changes what one key says without restarting it, so Since keeps
// measuring the work rather than the last progress report. Local layer only:
// a derived label is a fact about the data and is replaced by the next
// observation, not edited.
func (s *Set) Relabel(key, label string) {
	if e, ok := s.local[key]; ok && !e.Done {
		e.Label = label
	}
}

// RelabelRun changes what every key of a run says.
func (s *Set) RelabelRun(runID int64, label string) {
	if runID == 0 {
		return
	}
	for _, e := range s.local {
		if e.RunID == runID && !e.Done {
			e.Label = label
		}
	}
}

// Finish marks one key done and starts its hold. A nil err is a success.
func (s *Set) Finish(key string, err error) tea.Cmd {
	e, ok := s.local[key]
	if !ok {
		return nil
	}
	return s.finish(key, e, err)
}

// FinishRun marks every key of a run done.
func (s *Set) FinishRun(runID int64, err error) tea.Cmd {
	if runID == 0 {
		return nil
	}
	var cmds []tea.Cmd
	for k, e := range s.local {
		if e.RunID != runID {
			continue
		}
		if c := s.finish(k, e, err); c != nil {
			cmds = append(cmds, c)
		}
	}
	return tea.Batch(cmds...)
}

func (s *Set) finish(key string, e *entry, err error) tea.Cmd {
	e.gen++
	if err == nil && s.derives {
		// Decision 19. The dispatch worked, so whatever it started is now the
		// source of truth's business. Reporting ✓ here would clear the row
		// before the data has said anything about the work, which for an
		// action that returns in 200ms means the user sees a tick and then no
		// evidence anything happened. Keep the row moving until the next
		// observation instead: the spinner covers the gap between "we asked"
		// and "we have been told", which is the window in which the TUI has
		// nothing true to say.
		e.awaiting = true
		gen := e.gen
		return tea.Tick(s.confirm, func(time.Time) tea.Msg {
			return expireMsg{key: key, gen: gen}
		})
	}
	// A failed dispatch started nothing, so there is nothing for the data to
	// confirm and the error is the outcome. Report it.
	e.awaiting = false
	e.Done, e.Err = true, err
	return s.armHold(key, e)
}

// armHold puts an entry into its outcome hold and returns the timer.
func (s *Set) armHold(key string, e *entry) tea.Cmd {
	if s.hold < 0 {
		return nil
	}
	gen, hold := e.gen, s.hold
	return tea.Tick(hold, func(time.Time) tea.Msg { return clearMsg{key: key, gen: gen} })
}

// expire ends a handoff the data never confirmed, falling back to the outcome
// the action itself reported.
func (s *Set) expire(key string, gen int64) tea.Cmd {
	e, ok := s.local[key]
	if !ok || e.gen != gen || !e.awaiting {
		return nil
	}
	e.awaiting = false
	e.gen++
	e.Done, e.Err = true, nil
	return s.armHold(key, e)
}

// Clear retires one key immediately, outcome or not. Both layers: a derived
// entry left behind would keep the row spinning until the next observation,
// which is not what "clear this" means.
func (s *Set) Clear(key string) {
	delete(s.local, key)
	delete(s.derived, key)
}

// ClearAll retires everything.
func (s *Set) ClearAll() {
	for k := range s.local {
		delete(s.local, k)
	}
	for k := range s.derived {
		delete(s.derived, k)
	}
}

func (s *Set) clearGen(key string, gen int64) {
	if e, ok := s.local[key]; ok && e.gen == gen {
		delete(s.local, key)
	}
}

// Adopt takes over another Set's entries, keeping this Set's own options.
//
// This is the rule-4 primitive: a theme swap rebuilds the component, so the
// new Set carries the new palette and the old work. The returned command
// re-arms the spinner and any hold that was mid-flight, or a swap during those
// two seconds would strand an outcome glyph on the row forever.
func (s *Set) Adopt(other Set) tea.Cmd {
	if s.local == nil {
		s.local = map[string]*entry{}
	}
	if s.derived == nil {
		s.derived = map[string]State{}
	}
	if s.revs == nil {
		s.revs = map[string]string{}
	}
	if s.settled == nil {
		s.settled = map[string]bool{}
	}
	s.derives = s.derives || other.derives
	for k := range other.settled {
		s.settled[k] = true
	}

	var cmds []tea.Cmd
	for k, e := range other.local {
		cp := *e
		s.local[k] = &cp
		key, gen := k, cp.gen
		switch {
		case cp.awaiting:
			cmds = append(cmds, tea.Tick(s.confirm, func(time.Time) tea.Msg {
				return expireMsg{key: key, gen: gen}
			}))
		case cp.Done && s.hold >= 0:
			hold := s.hold
			cmds = append(cmds, tea.Tick(hold, func(time.Time) tea.Msg {
				return clearMsg{key: key, gen: gen}
			}))
		}
	}
	for k, st := range other.derived {
		s.derived[k] = st
	}
	for k, r := range other.revs {
		s.revs[k] = r
	}
	if c := s.armTick(); c != nil {
		cmds = append(cmds, c)
	}
	return tea.Batch(cmds...)
}

// stateOf resolves the two layers. Local wins, which is what makes a stale
// poll unable to wipe a live spinner: a Derive that reports a key as resting
// removes the derived entry and leaves the local one exactly where it was.
func (s Set) stateOf(key string) (State, bool) {
	if e, ok := s.local[key]; ok {
		return e.State, true
	}
	if st, ok := s.derived[key]; ok {
		return st, true
	}
	return State{}, false
}

// State reports one key's state, local layer first.
func (s Set) State(key string) (State, bool) { return s.stateOf(key) }

// Active reports whether any key has an entry — running, holding an outcome,
// or derived from the data.
func (s Set) Active() bool { return len(s.local) > 0 || len(s.derived) > 0 }

// Count is how many keys are still running, held outcomes excluded.
func (s Set) Count() int {
	n := 0
	for k := range s.local {
		if st, _ := s.stateOf(k); !st.Done {
			n++
		}
	}
	for k := range s.derived {
		if _, shadowed := s.local[k]; !shadowed {
			n++
		}
	}
	return n
}

// Render is the indicator for one key, fitted to width and styled.
//
// Below the width the label needs, the glyph alone is drawn: a cell of eight
// showing "⠹ syncin" is worse than one showing "⠹". Reports false when the key
// has no entry, so a caller can fall through to whatever the row normally
// shows.
func (s Set) Render(key string, width int) (string, bool) {
	st, ok := s.stateOf(key)
	if !ok || width <= 0 {
		return "", false
	}
	text := s.glyphFor(st)
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
// Returns row untouched when the key has no entry, so a caller can compose
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

// Width is the visible width Render would produce for key, before styling.
// Callers that lay out around the indicator need it without paying for the
// escapes.
func (s Set) Width(key string, width int) int {
	text, ok := s.Render(key, width)
	if !ok {
		return 0
	}
	return xansi.StringWidth(text)
}

func (s Set) glyphFor(st State) string {
	switch {
	case st.Changed:
		return s.glyphs.ActivityChanged
	case st.Failed():
		return s.glyphs.ActivityFail
	case st.Done:
		return s.glyphs.ActivityOK
	default:
		// spinner.Dot's frames carry a trailing space, which would put two
		// between the glyph and the label and cost a cell the outcome glyphs
		// do not. Trim it so every state composes identically.
		return strings.TrimRight(s.spin.View(), " ")
	}
}

// running reports whether anything still needs the animation.
func (s Set) running() bool {
	for _, e := range s.local {
		if !e.Done {
			return true
		}
	}
	for k := range s.derived {
		// A derived entry under a held outcome is not what the row is showing.
		if e, ok := s.local[k]; ok && e.Done {
			continue
		}
		return true
	}
	return false
}

func (s *Set) armTick() tea.Cmd {
	if s.ticking || !s.running() {
		return nil
	}
	s.ticking = true
	return s.spin.Tick
}

func (s *Set) tick(m spinner.TickMsg) tea.Cmd {
	if !s.running() {
		s.ticking = false
		return nil
	}
	var cmd tea.Cmd
	s.spin, cmd = s.spin.Update(m)
	return cmd
}

// Derive replaces the derived layer from one observation of the data.
//
// busy maps the keys that are in flight to what they should say; every key
// absent from it is not in flight. Wholesale rather than incremental because an
// observation *is* the whole truth as of that moment — an incremental API would
// make "this row stopped being busy" something the caller has to notice and
// report, which is the bookkeeping this is meant to remove.
//
// A key's Since survives across observations, so elapsed time measures the work
// rather than the poll that last saw it.
//
// Calling this is also the observation decision 19 waits for: it retires any
// local entry whose action finished successfully since the last one. That is
// why it must be called on every swap, including one where nothing is busy —
// an empty map is a real observation and the only thing that can end a handoff.
func (s *Set) Derive(busy map[string]string) tea.Cmd {
	s.derives = true
	if s.local == nil {
		s.local = map[string]*entry{}
	}

	s.settled = make(map[string]bool, len(s.derived))
	for k := range s.derived {
		s.settled[k] = true
	}

	next := make(map[string]State, len(busy))
	for k, label := range busy {
		st := State{Label: label, Since: time.Now()}
		if prev, ok := s.derived[k]; ok {
			st.Since = prev.Since
		}
		next[k] = st
	}
	s.derived = next

	for k, e := range s.local {
		if e.awaiting {
			s.settled[k] = true
			delete(s.local, k)
		}
	}
	return s.armTick()
}

// Change is one key's revision plus what a flash should say about it.
type Change struct {
	// Rev is a value that changes whenever the row's underlying work does —
	// finished_at, resourceVersion, an ETag, the id of the latest run. Its
	// content is never interpreted; only whether it differs from last time.
	Rev string

	// Label is what the flash shows. Usually the row's own new value, because
	// in a table the indicator replaces the cell — a bare glyph there would
	// hide the very change it is pointing at.
	Label string
}

// Revise records revisions and flashes the keys whose work changed unseen.
//
// A key whose Rev differs from the last observation, and which is neither busy
// nor locally owned, gets a brief Changed outcome: the row's work moved and
// this session never saw it running (decision 20). A key seen for the first
// time never flashes, or every row would flash on the first load.
//
// Nor does a key the last observation reported busy, or one whose handoff this
// observation just retired. Those changes were on screen as a spinner from
// start to finish, and a flash after them says the same thing twice — the
// flash exists for work that was never visible at all.
//
// Call it after Derive, which is what decides whether a key counts as busy.
func (s *Set) Revise(changes map[string]Change) tea.Cmd {
	if s.revs == nil {
		s.revs = map[string]string{}
	}
	var cmds []tea.Cmd
	for k, c := range changes {
		prev, known := s.revs[k]
		s.revs[k] = c.Rev
		if !known || prev == c.Rev {
			continue
		}
		if _, busy := s.derived[k]; busy {
			continue
		}
		if _, owned := s.local[k]; owned {
			continue
		}
		if s.settled[k] {
			// The user watched this one finish. Flashing it as well would
			// report the same news a second time.
			continue
		}
		if cmd := s.flash(k, c.Label); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	for k := range s.revs {
		if _, ok := changes[k]; !ok {
			delete(s.revs, k)
		}
	}
	return tea.Batch(cmds...)
}

func (s *Set) flash(key, label string) tea.Cmd {
	if s.local == nil {
		s.local = map[string]*entry{}
	}
	e, ok := s.local[key]
	if !ok {
		e = &entry{}
		s.local[key] = e
	}
	e.gen++
	e.awaiting = false
	e.State = State{Label: label, Done: true, Changed: true}
	return s.armHold(key, e)
}

// Busy builds the ordinary ActivityWhen predicate: a case-insensitive match
// against the values that mean "in progress", labelled with the value as it
// actually appeared.
//
//	activity.Busy("running", "pending", "waiting")
//
// Values are compared with surrounding space and any ANSI styling stripped, so
// a cell coloured by the screen still matches.
func Busy(values ...string) func(value string) (label string, busy bool) {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[strings.ToLower(strings.TrimSpace(v))] = true
	}
	return func(value string) (string, bool) {
		plain := strings.TrimSpace(xansi.Strip(value))
		if set[strings.ToLower(plain)] {
			return plain, true
		}
		return "", false
	}
}

// Progresser is implemented by a writer that can report a label change back to
// the rows an action is running against. runner's capture writer satisfies it.
type Progresser interface {
	Progress(text string)
}

// Progress updates the label on the rows the running action is acting on.
//
// It rides the io.Writer an action already holds rather than adding a second
// channel back to the UI, the way http.Flusher extends http.ResponseWriter. A
// no-op when out does not support it, so an action written against a plain
// io.Writer keeps working:
//
//	Run: func(ctx context.Context, out io.Writer) error {
//	    fmt.Fprintln(out, "sync started")      // the console
//	    activity.Progress(out, "syncing 3/7")  // the rows
//	    …
//	}
//
// Progress is deliberately not logged. It is a UI state change, not news: a
// run reporting progress ten times would otherwise post ten records into an
// event the console badge counts as one.
func Progress(out io.Writer, text string) {
	if p, ok := out.(Progresser); ok {
		p.Progress(text)
	}
}

package output

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/confirm"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// Notice asks the app shell to surface text in the statusbar.
//
// The screen can't return app.Info directly — pkg/app imports this package,
// so the dependency only runs one way. The shell treats a Notice exactly as
// it treats StatusInfoMsg / StatusErrorMsg, which means it also lands in this
// buffer: the export path outlives the keypress that would have wiped it.
type Notice struct {
	Text  string
	Level Level
}

func notice(text string, lvl Level) tea.Cmd {
	return func() tea.Msg { return Notice{Text: text, Level: lvl} }
}

// Screen is the console view: a logview over the buffer, plus clear, kill,
// and export.
//
// It is pushed and popped rather than drawn as an overlay, so the stack
// handles key routing, theme propagation and esc-to-close with no new mode
// in the shell. The cost is a breadcrumb crumb for a place the user didn't
// really navigate to.
type Screen struct {
	buf  *Buffer
	opts Options
	lv   logview.Model

	// synced/epoch mirror the buffer into the logview incrementally: append
	// the tail while the epoch holds, rebuild when it moves (see
	// Buffer.Epoch).
	synced int
	epoch  int

	killing      bool
	conf         confirm.Model
	pendingKill  int64
	pendingLabel string

	picking bool
	picker  list.Model
}

// NewScreen builds the console over buf. Pass options from OptionsFrom(t).
func NewScreen(buf *Buffer, opts Options) *Screen {
	opts.fillDefaults()
	opts.Keys.FillDefaults()
	s := &Screen{
		buf:   buf,
		opts:  opts,
		lv:    logview.New(opts.Logview),
		epoch: -1,
	}
	s.sync()
	return s
}

// Title labels the screen in the breadcrumb.
func (s *Screen) Title() string { return "Output" }

// Init satisfies screen.Screen.
func (s *Screen) Init() tea.Cmd { return nil }

// OnEnter re-syncs in case records arrived while the screen was being pushed.
func (s *Screen) OnEnter(any) tea.Cmd {
	s.sync()
	s.lv.SetFollow(true)
	return nil
}

// IsCapturingKeys reports whether something on this screen owns the keyboard
// — a modal, the run picker, or the logview's search filter. The shell reads
// it to keep its globals (including esc-pop) out of the way.
func (s *Screen) IsCapturingKeys() bool {
	return s.killing || s.picking || s.lv.IsCapturingKeys()
}

// Layout is the logview filling the body, with the kill confirmation or run
// picker composited on top when either is up.
func (s *Screen) Layout() layout.Node {
	base := layout.Sized(&s.lv)
	switch {
	case s.killing:
		return layout.ZStack(base, layout.Center(52, 7, layout.Sized(&s.conf)))
	case s.picking:
		return layout.ZStack(base, layout.Center(52, 12, layout.Sized(&s.picker)))
	}
	return base
}

// Help composes the logview's bindings with the screen's own actions, and
// swaps in the modal's while one is up so the hint strip tracks context.
func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

// HelpSections forwards the logview's own groups and adds the console's
// verbs under a heading naming what they act on.
//
// Forwarding is the whole point: without it this screen falls back to one
// group titled "Output" standing over the logview's scroll and search keys,
// which is the failure rule 10 exists to prevent — and it would reach every
// app that sets OutputKey, not just one screen an author wrote.
//
// While a modal is up its keys are the only ones that do anything, so they
// are the only ones listed, the same gating a screen hosting pkg/confirm
// applies (rule 22).
func (s *Screen) HelpSections() []help.Section {
	if s.killing {
		return help.SectionsOf(&s.conf)
	}
	if s.picking {
		return help.SectionsOf(&s.picker, help.Group("Runs",
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"))))
	}
	own := []key.Binding{s.opts.Keys.Clear, s.opts.Keys.Export}
	if s.buf != nil && s.buf.InFlight() > 0 {
		own = append(own, s.opts.Keys.Kill)
	}
	return help.SectionsOf(&s.lv, help.Group("Log", own...))
}

// Update mirrors the buffer, then routes input. While a modal is up it owns
// every message and the logview sees none, per CLAUDE.md rule 20.
func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	s.sync()

	if s.killing {
		return s.updateKill(msg)
	}
	if s.picking {
		return s.updatePicker(msg)
	}

	if k, ok := msg.(tea.KeyMsg); ok && !s.lv.IsCapturingKeys() {
		switch {
		case key.Matches(k, s.opts.Keys.Clear):
			// No confirmation: the log is a convenience, not a record, and a
			// modal on the cheap action is friction.
			s.buf.Clear()
			s.sync()
			return s, nil
		case key.Matches(k, s.opts.Keys.Export):
			return s, s.export()
		case key.Matches(k, s.opts.Keys.Kill):
			return s, s.beginKill()
		}
	}

	var cmd tea.Cmd
	s.lv, cmd = s.lv.Update(msg)
	return s, cmd
}

func (s *Screen) updateKill(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch msg.(type) {
	case confirm.ConfirmedMsg:
		id := s.pendingKill
		s.killing, s.pendingKill, s.pendingLabel = false, 0, ""
		if err := s.buf.Kill(id); err != nil {
			return s, notice("kill failed: "+err.Error(), LevelError)
		}
		return s, nil
	case confirm.CancelledMsg:
		s.killing, s.pendingKill, s.pendingLabel = false, 0, ""
		return s, nil
	}
	var cmd tea.Cmd
	s.conf, cmd = s.conf.Update(msg)
	return s, cmd
}

func (s *Screen) updatePicker(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && !s.picker.IsCapturingKeys() && k.String() == "esc" {
		s.picking = false
		return s, nil
	}
	if s.picker.IsActivate(msg) {
		if k, ok := s.picker.SelectedKey(); ok {
			if id, err := strconv.ParseInt(k, 10, 64); err == nil {
				s.picking = false
				s.armKill(id)
				return s, nil
			}
		}
		return s, nil
	}
	var cmd tea.Cmd
	s.picker, cmd = s.picker.Update(msg)
	return s, cmd
}

// beginKill picks a run to signal. With one in flight it goes straight to the
// confirmation; with several it asks which first, which is rule 8's
// menu-driven case arriving on its own rather than as three letter shortcuts.
func (s *Screen) beginKill() tea.Cmd {
	runs := s.buf.Runs()
	switch len(runs) {
	case 0:
		return notice("nothing running", LevelInfo)
	case 1:
		s.armKill(runs[0].ID)
		return nil
	}

	items := make([]list.KeyedItem, len(runs))
	for i, r := range runs {
		items[i] = list.KeyedItem{
			Key:     strconv.FormatInt(r.ID, 10),
			Display: r.Label + "  " + elapsed(r.Started),
		}
	}
	s.picker = list.New(s.opts.Picker)
	s.picker.SetKeyedItems(items)
	s.picking = true
	return nil
}

func (s *Screen) armKill(id int64) {
	label := strconv.FormatInt(id, 10)
	for _, r := range s.buf.Runs() {
		if r.ID == id {
			label = r.Label
			break
		}
	}
	s.pendingKill = id
	s.pendingLabel = label
	s.conf = confirm.New(s.killOptions())
	s.killing = true
}

func (s *Screen) killOptions() confirm.Options {
	opts := s.opts.Confirm
	opts.Message = "Kill " + s.pendingLabel + "?"
	opts.Confirm = "Kill"
	opts.Cancel = "Keep running"
	return opts
}

// export writes the log to a file and reports where it went.
//
// Auto-named rather than prompted: a prompt would add a text field, a focus
// region and a capture state to an otherwise pure viewer, for a path nobody
// cares about. What they care about is knowing where it landed.
//
// It writes what the pane is showing — filter mode narrows it, a bare search
// query does not — with the query recorded as a header line so a narrowed
// export can't be mistaken for the whole log.
func (s *Screen) export() tea.Cmd {
	dir := s.opts.ExportDir
	if dir == "" {
		dir = os.TempDir()
	}
	path := filepath.Join(dir, "output-"+time.Now().Format("20060102-150405")+".log")

	var b strings.Builder
	if q := s.lv.Query(); q != "" && s.lv.FilterMode() {
		b.WriteString("# filter: " + q + "\n")
	}
	for _, r := range s.exportRecords() {
		b.WriteString(s.opts.RenderPlain(r))
		b.WriteByte('\n')
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return notice("export failed: "+err.Error(), LevelError)
	}
	return notice("wrote "+path, LevelInfo)
}

func (s *Screen) exportRecords() []Record {
	recs := s.buf.Records()
	q := s.lv.Query()
	if q == "" || !s.lv.FilterMode() {
		return recs
	}
	q = strings.ToLower(q)
	out := make([]Record, 0, len(recs))
	for _, r := range recs {
		if strings.Contains(strings.ToLower(s.opts.RenderPlain(r)), q) {
			out = append(out, r)
		}
	}
	return out
}

// SetTheme rebuilds the themed pieces, carrying the logview's query, filter
// mode and follow state across the swap.
//
// Colors come back from the theme by definition, so a hand-set BadgeStyle or
// line color does not survive — the same bargain every other component makes
// under CLAUDE.md rule 4. The non-visual knobs (cap, export dir, source
// width, keys) are preserved.
func (s *Screen) SetTheme(t theme.Theme) {
	q, fm, follow := s.lv.Query(), s.lv.FilterMode(), s.lv.Following()

	next := OptionsFrom(t)
	next.MaxRecords = s.opts.MaxRecords
	next.ExportDir = s.opts.ExportDir
	next.SourceWidth = s.opts.SourceWidth
	next.Keys = s.opts.Keys
	s.opts = next

	s.lv = logview.New(s.opts.Logview)
	s.epoch = -1
	s.sync()
	s.lv.SetQuery(q)
	s.lv.SetFilterMode(fm)
	s.lv.SetFollow(follow)

	if s.killing {
		s.conf = confirm.New(s.killOptions())
	}
	if s.picking {
		cursor := s.picker.Cursor()
		items := s.picker.Items()
		s.picker = list.New(s.opts.Picker)
		s.picker.SetItems(items)
		s.picker.SetCursor(cursor)
	}
}

// Buffer exposes the ring this screen is reading, for callers that built the
// screen and want to keep feeding it.
func (s *Screen) Buffer() *Buffer { return s.buf }

// sync mirrors the buffer into the logview. Appending the tail is the normal
// path; a changed epoch means records were dropped off the front, which
// invalidates the mirror and forces a rebuild.
func (s *Screen) sync() {
	if s.buf == nil {
		return
	}
	if s.epoch != s.buf.Epoch() {
		s.rebuild()
		return
	}
	recs := s.buf.Records()
	if s.synced >= len(recs) {
		return
	}
	s.lv.AppendLines(s.opts.RenderAll(recs[s.synced:]))
	s.synced = len(recs)
}

func (s *Screen) rebuild() {
	follow, q, fm := s.lv.Following(), s.lv.Query(), s.lv.FilterMode()

	recs := s.buf.Records()
	s.lv.Clear()
	s.lv.AppendLines(s.opts.RenderAll(recs))
	s.synced = len(recs)
	s.epoch = s.buf.Epoch()

	if q != "" {
		s.lv.SetQuery(q)
	}
	s.lv.SetFilterMode(fm)
	s.lv.SetFollow(follow)
}

// elapsed renders a coarse age for the run picker.
func elapsed(since time.Time) string {
	d := time.Since(since).Round(time.Second)
	if d < time.Minute {
		return strconv.Itoa(int(d.Seconds())) + "s"
	}
	return strconv.Itoa(int(d.Minutes())) + "m"
}

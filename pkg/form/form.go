// Package form is a form drawn as one card: each field a label over its
// value, a note under it saying what it is for, Submit and Cancel buttons,
// and a line of the keys that work. The focused field carries a bar down its
// left edge, and typed values sit in a well a tone lighter than the card.
//
//	o := t.Form()
//	o.Title = "Sign in"
//	o.Fields = []form.Field{
//	    form.Text(form.TextOptions{Key: "user", Label: "Username", Required: true,
//	        Note: "The name you sign in with."}),
//	    form.Password(form.PasswordOptions{Key: "pass", Label: "Password", Required: true}),
//	    form.Select(form.SelectOptions{Key: "role", Label: "Role",
//	        Options: []string{"admin", "editor", "viewer"}, RequirePick: true}),
//	}
//	s.form = form.New(o)
//
// The form sizes and places itself: it is at most MaxWidth columns wide,
// exactly as tall as its fields, and centred in whatever rect it is given. A
// screen that is only the form hands it the whole body with
// layout.Sized(&s.form); a modal puts the same node in a ZStack over the
// screen and sets Options.Modal. Height reports the rows it needs, for a host
// that wants to know.
//
// Steps make it a wizard: each step is a page of fields with its own title,
// Back and Next move between them, esc goes back a step, and a step can skip
// itself based on what was answered before it. Values from every step arrive
// together in one SubmittedMsg.
//
// The form validates: Required, RequirePick, MultiSelect's Min and any
// Validate run when the user moves past a step or submits. A refused step
// flags the offending fields, moves focus to the first, and posts InvalidMsg;
// a flagged field then re-checks on every keystroke so a fix is acknowledged
// at once. A screen can never receive values the form refused.
//
// Enter moves to the next field and, on the last, submits (or moves to the
// next step). Tab and shift+tab move between fields and buttons. esc cancels,
// or goes back a step. The form always captures keys: mirror IsCapturingKeys
// in the screen's own, and leave on CancelledMsg.
//
// Banner, when Options.Banner has Text, is drawn above the card on an intro
// screen — big block letters with a moving gradient (pkg/banner). It is
// dropped when the rect is too narrow or short for it, and never drawn on a
// modal.
package form

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/banner"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/surface"
)

// SubmittedMsg is posted when the user submits a form whose every reached
// field is valid. Values holds each field's value by Key, from every step the
// user went through; steps skipped by their Skip are left out.
type SubmittedMsg struct{ Values Values }

// CancelledMsg is posted on esc (on the first step) or the Cancel button.
type CancelledMsg struct{}

// InvalidMsg is posted when a step or a submit is refused. Keys names the
// failing fields, in order. The form has already flagged them and moved focus
// to the first, so a screen needs this only to react (a log line, a count).
type InvalidMsg struct{ Keys []string }

// Values are a form's answers by field Key: a string for Text, Password,
// TextArea and Select (empty until a RequirePick select is chosen), a bool
// for Confirm, and a []string for MultiSelect.
type Values map[string]any

// String is v[key] as a string, or "".
func (v Values) String(key string) string { s, _ := v[key].(string); return s }

// Bool is v[key] as a bool, or false.
func (v Values) Bool(key string) bool { b, _ := v[key].(bool); return b }

// Strings is v[key] as a []string, or nil.
func (v Values) Strings(key string) []string { s, _ := v[key].([]string); return s }

// Step is one page of a wizard.
type Step struct {
	// Title names the step; it follows the form's title on the frame.
	Title  string
	Fields []Field
	// Skip leaves the step out when it reports true for the answers so far.
	// It is asked whenever the user moves past a neighbouring step, so a
	// step can come and go as earlier answers change.
	Skip func(Values) bool
}

// DefaultMaxWidth is how wide the card is when Options.MaxWidth is zero.
const DefaultMaxWidth = 60

// Options configures a form. Start from theme.Form().
type Options struct {
	// Title names the form on its frame.
	Title string
	// Fields is a single-page form. Steps, when set, replaces it.
	Fields []Field
	// Steps make the form a wizard: one page each, in order.
	Steps []Step

	// SubmitText labels the button that submits. Defaults to "Submit".
	SubmitText string
	// NextText, BackText and CancelText label the wizard's buttons and
	// Cancel. Default "Next", "Back" and "Cancel".
	NextText, BackText, CancelText string

	// MaxWidth caps the card's width. Defaults to DefaultMaxWidth.
	MaxWidth int
	// FieldSpacing is the number of blank rows between fields. Defaults to
	// 1.
	FieldSpacing *int
	// Modal draws the form as a modal: the help line moves inside the card,
	// since below it would sit on whatever the modal covers, and the banner
	// is never drawn.
	Modal bool
	// Banner is drawn above the card when its Text is set. theme.Form()
	// fills its colours.
	Banner banner.Options

	Styles Styles

	// The frame. theme.Form() fills them from the surface's overlay form.
	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	Glyphs         glyph.Set
	SlotBrackets   pane.SlotBracketStyle
	Surface        surface.Resolved
}

// With returns a copy of opts with Fields replaced, for chaining from a
// theme builder: t.Form().With([]form.Field{…}).
func (opts Options) With(fields []Field) Options {
	opts.Fields = fields
	return opts
}

// Styles are the form's colours. theme.Form() fills them; set a field only
// to deviate from the palette.
type Styles struct {
	// Text styles values and option labels.
	Text lipgloss.Style
	// Label and LabelFocused style a field's label, and LabelFocused the
	// frame's step progress too.
	Label, LabelFocused lipgloss.Style
	// Note styles the note under a field, and the "more" row of a long list.
	Note        lipgloss.Style
	Placeholder lipgloss.Style
	CursorColor lipgloss.TerminalColor
	// Bar styles the focus bar and a MultiSelect's cursor.
	Bar lipgloss.Style
	// Chosen and Unchosen style the radio and check glyphs.
	Chosen, Unchosen lipgloss.Style
	// Well and WellFocused are the background behind a typed value; nil
	// draws none.
	Well, WellFocused lipgloss.TerminalColor
	// Error styles an invalid field's label and its message.
	Error lipgloss.Style
	// Button and ButtonActive style the buttons, idle and focused. Chips
	// draws them as "  label  " filled, in place of "[ label ]".
	Button, ButtonActive lipgloss.Style
	Chips                bool
	// HelpKey and HelpDesc style the help line.
	HelpKey, HelpDesc lipgloss.Style
}

// Model is a form.
type Model struct {
	steps   []Step
	step    int
	focus   int // a field of the current step, then the buttons
	flagged map[*field]bool

	title                          string
	submitT, nextT, backT, cancelT string
	maxW, spacing                  int
	modal                          bool
	// gutter is the gap between a line frame and the focus bar, which would
	// otherwise draw as a second line beside the frame's own.
	gutter int
	styles *Styles
	glyphs glyph.Set

	frame  pane.Pane
	banner banner.Model
	rect   geom.Rect
	// geo is where the last View drew everything. A pointer because View
	// has a value receiver and Update must hit-test against what was drawn.
	geo *geometry
}

// New builds a form, focused on its first field.
func New(opts Options) Model {
	steps := opts.Steps
	if len(steps) == 0 {
		steps = []Step{{Fields: opts.Fields}}
	}
	spacing := 1
	if opts.FieldSpacing != nil {
		spacing = max(0, *opts.FieldSpacing)
	}
	st := opts.Styles
	m := Model{
		steps:   steps,
		flagged: map[*field]bool{},
		title:   opts.Title,
		submitT: or(opts.SubmitText, "Submit"),
		nextT:   or(opts.NextText, "Next"),
		backT:   or(opts.BackText, "Back"),
		cancelT: or(opts.CancelText, "Cancel"),
		maxW:    opts.MaxWidth,
		spacing: spacing,
		modal:   opts.Modal,
		styles:  &st,
		glyphs:  opts.Glyphs.Resolve(),
		banner:  banner.New(opts.Banner),
		geo:     &geometry{},
	}
	if m.maxW <= 0 {
		m.maxW = DefaultMaxWidth
	}
	if _, lined := opts.Surface.Surface().Lined(); lined || opts.Surface.IsZero() {
		m.gutter = 1
	}
	m.frame = pane.New(pane.Options{
		Focused:        true,
		ActiveColor:    opts.ActiveColor,
		InactiveColor:  opts.InactiveColor,
		ActiveBorder:   opts.ActiveBorder,
		InactiveBorder: opts.InactiveBorder,
		Glyphs:         opts.Glyphs,
		SlotBrackets:   opts.SlotBrackets,
		Surface:        opts.Surface,
	})
	m.focusField(0)
	return m
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// Init starts the cursor blink and the banner.
func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.focusField(m.focus), m.banner.Init())
}

func (m Model) fields() []*field {
	out := make([]*field, len(m.steps[m.step].Fields))
	for i, f := range m.steps[m.step].Fields {
		out[i] = f.field()
	}
	return out
}

func (m Model) onButton() bool { return m.focus >= len(m.steps[m.step].Fields) }
func (m Model) stops() int     { return len(m.steps[m.step].Fields) + len(m.buttons()) }

// skipped reports whether step i is left out for the answers so far.
func (m Model) skipped(i int) bool {
	s := m.steps[i]
	return s.Skip != nil && s.Skip(m.Values())
}

// neighbour is the next step that runs in direction d, or -1.
func (m Model) neighbour(d int) int {
	for i := m.step + d; i >= 0 && i < len(m.steps); i += d {
		if !m.skipped(i) {
			return i
		}
	}
	return -1
}

func (m Model) lastStep() bool { return m.neighbour(1) < 0 }

type button int

const (
	primaryButton button = iota
	backButton
	cancelButton
)

func (m Model) buttons() []button {
	if m.neighbour(-1) >= 0 {
		return []button{primaryButton, backButton, cancelButton}
	}
	return []button{primaryButton, cancelButton}
}

func (m Model) label(b button) string {
	switch b {
	case backButton:
		return m.backT
	case cancelButton:
		return m.cancelT
	}
	if m.lastStep() {
		return m.submitT
	}
	return m.nextT
}

// focusField moves focus to stop i, wrapping, and focuses or blurs each field.
func (m *Model) focusField(i int) tea.Cmd {
	n := m.stops()
	m.focus = ((i % n) + n) % n
	var cmd tea.Cmd
	for j, f := range m.fields() {
		if j == m.focus {
			cmd = f.ctl.focus()
		} else {
			f.ctl.blur()
		}
	}
	return cmd
}

// Update handles keys and clicks, advances the banner, and forwards
// everything else to the focused field.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var bcmd tea.Cmd
	m.banner, bcmd = m.banner.Update(msg)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		cmd := m.key(msg)
		return m, tea.Batch(bcmd, cmd)
	case mouse.Msg:
		return m, tea.Batch(bcmd, m.mouse(msg))
	}
	var cmd tea.Cmd
	if fs := m.fields(); !m.onButton() {
		cmd = fs[m.focus].ctl.other(msg)
	}
	return m, tea.Batch(bcmd, cmd)
}

func (m *Model) key(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "tab":
		return m.focusField(m.focus + 1)
	case "shift+tab":
		return m.focusField(m.focus - 1)
	case "esc":
		return m.press(backButton)
	}
	fs := m.fields()
	if m.onButton() {
		bs := m.buttons()
		bi := m.focus - len(fs)
		switch k.String() {
		case "left", "h":
			return m.focusField(len(fs) + max(0, bi-1))
		case "right", "l":
			return m.focusField(len(fs) + min(len(bs)-1, bi+1))
		case "enter", " ":
			return m.press(bs[bi])
		}
		return nil
	}
	f := fs[m.focus]
	if k.String() == "enter" && !f.ctl.takesEnter() {
		if m.focus == len(fs)-1 {
			return m.press(primaryButton)
		}
		return m.focusField(m.focus + 1)
	}
	cmd := f.ctl.key(k)
	m.revalidate()
	return cmd
}

// press acts on a button. esc presses Back, which cancels on the first step.
func (m *Model) press(b button) tea.Cmd {
	switch b {
	case backButton:
		prev := m.neighbour(-1)
		if prev < 0 {
			return func() tea.Msg { return CancelledMsg{} }
		}
		m.step = prev
		return m.focusField(0)
	case cancelButton:
		return func() tea.Msg { return CancelledMsg{} }
	}
	if !m.lastStep() {
		if cmd := m.refuse(m.step); cmd != nil {
			return cmd
		}
		m.step = m.neighbour(1)
		return m.focusField(0)
	}
	for i := range m.steps {
		if m.skipped(i) {
			continue
		}
		if cmd := m.refuse(i); cmd != nil {
			return cmd
		}
	}
	vals := Values{}
	for i, s := range m.steps {
		if m.skipped(i) {
			continue
		}
		for _, f := range s.Fields {
			vals[f.Key()] = f.field().ctl.value()
		}
	}
	return func() tea.Msg { return SubmittedMsg{Values: vals} }
}

// refuse validates step i. When something fails it flags the failures, takes
// the user to the first one, and returns the InvalidMsg; otherwise nil.
func (m *Model) refuse(i int) tea.Cmd {
	var keys []string
	first := -1
	for j, f := range m.steps[i].Fields {
		ff := f.field()
		ff.err = ff.check()
		m.flagged[ff] = ff.err != nil
		if ff.err != nil {
			keys = append(keys, ff.key)
			if first < 0 {
				first = j
			}
		}
	}
	if first < 0 {
		return nil
	}
	m.step = i
	cmd := m.focusField(first)
	return tea.Batch(cmd, func() tea.Msg { return InvalidMsg{Keys: keys} })
}

// revalidate re-checks the fields a refusal flagged, so a correction is
// acknowledged at once without complaining about fields not yet reached.
func (m *Model) revalidate() {
	for f, on := range m.flagged {
		if !on {
			continue
		}
		f.err = f.check()
		if f.err == nil {
			m.flagged[f] = false
		}
	}
}

func (m *Model) mouse(e mouse.Msg) tea.Cmd {
	if !e.IsPress() || !m.geo.card.Hit(e.X, e.Y) {
		return nil
	}
	for _, b := range m.geo.buttons {
		if b.r.Hit(e.X, e.Y) {
			m.focus = len(m.fields()) + b.index
			m.focusField(m.focus)
			return m.press(m.buttons()[b.index])
		}
	}
	row := e.Y - m.geo.content.Y + m.geo.offset
	if row < 0 || row >= len(m.geo.rows) {
		return nil
	}
	ref := m.geo.rows[row]
	if ref.field < 0 || ref.field >= len(m.fields()) {
		return nil
	}
	var cmd tea.Cmd
	if ref.field != m.focus {
		cmd = m.focusField(ref.field)
	}
	if ref.value >= 0 {
		m.fields()[ref.field].ctl.click(ref.value, e.X-m.geo.content.X-2-m.gutter)
		m.revalidate()
	}
	return cmd
}

// SetRect is the area the form may use; it draws itself centred inside.
func (m *Model) SetRect(r geom.Rect) { m.rect = r }

// Rect is where the card was last drawn.
func (m Model) Rect() geom.Rect { return m.geo.card }

// SetTitle changes the form's title.
func (m *Model) SetTitle(s string) { m.title = s }

// Title is the form's title.
func (m Model) Title() string { return m.title }

// Step is the index of the wizard step on screen.
func (m Model) Step() int { return m.step }

// FocusedIndex is the focused stop on the current step: a field's index, or
// len(fields) and up for the buttons.
func (m Model) FocusedIndex() int { return m.focus }

// FieldRect is where field i of the current step was last drawn — its label,
// value and note. Empty before the first View, or for an index out of range.
func (m Model) FieldRect(i int) geom.Rect {
	if i < 0 || i >= len(m.geo.fields) {
		return geom.Rect{}
	}
	return m.geo.fields[i]
}

// SubmitRect is where the submit (or Next) button was last drawn.
func (m Model) SubmitRect() geom.Rect {
	for _, b := range m.geo.buttons {
		if m.buttons()[b.index] == primaryButton {
			return b.r
		}
	}
	return geom.Rect{}
}

// IsCapturingKeys is always true: the form owns the keyboard while it is up.
func (m Model) IsCapturingKeys() bool { return true }

// Values is every field's current value by Key, on every step.
func (m Model) Values() Values {
	out := Values{}
	for _, s := range m.steps {
		for _, f := range s.Fields {
			out[f.Key()] = f.field().ctl.value()
		}
	}
	return out
}

// Value is one field's current value, or nil for an unknown key.
func (m Model) Value(key string) any { return m.Values()[key] }

// String is one field's value as a string, or "".
func (m Model) String(key string) string { return m.Values().String(key) }

// Bool is one field's value as a bool, or false.
func (m Model) Bool(key string) bool { return m.Values().Bool(key) }

// Strings is one field's value as a []string, or nil.
func (m Model) Strings(key string) []string { return m.Values().Strings(key) }

// Help is the flat form of HelpSections.
func (m Model) Help() []key.Binding { return help.Flatten(m.HelpSections()) }

// HelpSections groups moving between fields, committing the form, and the
// focused field's own keys.
func (m Model) HelpSections() []help.Section {
	submit := []key.Binding{m.escKey()}
	if enter, ok := m.enterKey(); ok {
		submit = append([]key.Binding{enter}, submit...)
	}
	return help.Sections(
		help.Group(help.SectionEdit, m.editKeys()...),
		help.Group(help.SectionSubmit, submit...),
		help.Group(help.SectionNavigate,
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "next")),
			key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "prev"))),
	)
}

func (m Model) editKeys() []key.Binding {
	if m.onButton() {
		return []key.Binding{key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←→", "button"))}
	}
	return m.fields()[m.focus].ctl.help()
}

// enterKey is what enter does where focus is, or false on a field that takes
// enter itself (a TextArea, whose own keys say "newline").
func (m Model) enterKey() (key.Binding, bool) {
	desc := "next"
	switch {
	case m.onButton():
		desc = "press"
	case m.fields()[m.focus].ctl.takesEnter():
		return key.Binding{}, false
	case m.focus == len(m.fields())-1 && m.lastStep():
		desc = "submit"
	case m.focus == len(m.fields())-1:
		desc = "next step"
	}
	return key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", desc)), true
}

func (m Model) escKey() key.Binding {
	desc := "cancel"
	if m.neighbour(-1) >= 0 {
		desc = "back"
	}
	return key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", desc))
}

// State is what the user did to a form: the step, focus, every value, and
// which fields are flagged. Take it before a theme rebuild and Restore it
// onto the form built from the new Options.
type State struct {
	built   bool
	step    int
	focus   int
	values  map[string]any
	flagged map[string]bool
}

// State snapshots the form.
func (m Model) State() State {
	if m.geo == nil {
		return State{}
	}
	s := State{built: true, step: m.step, focus: m.focus, values: map[string]any{}, flagged: map[string]bool{}}
	for _, st := range m.steps {
		for _, f := range st.Fields {
			ff := f.field()
			s.values[ff.key] = ff.ctl.snapshot()
			s.flagged[ff.key] = m.flagged[ff]
		}
	}
	return s
}

// Restore replays s onto the form, matching fields by Key. A State from a
// form never built restores nothing.
func (m *Model) Restore(s State) {
	if !s.built {
		return
	}
	for _, st := range m.steps {
		for _, f := range st.Fields {
			ff := f.field()
			if v, ok := s.values[ff.key]; ok {
				ff.ctl.restore(v)
			}
			if s.flagged[ff.key] {
				m.flagged[ff] = true
				ff.err = ff.check()
			}
		}
	}
	if s.step < len(m.steps) {
		m.step = s.step
	}
	m.focusField(s.focus)
}

// Height is how many rows the form needs at width w: the card, and on an
// intro screen the banner above it and the help line below.
func (m Model) Height(w int) int {
	cw := min(m.maxW, w)
	h := m.cardHeight(cw)
	if !m.modal {
		h++
		if m.banner.Fits(w) {
			h += m.banner.Height() + 1
		}
	}
	return h
}

// lines is the rows of a string, for joining.
func lines(s string) []string { return strings.Split(s, "\n") }

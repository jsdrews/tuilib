// Package form is a vertical form component: a sequence of Text, Password,
// Select, and Confirm fields with tab/shift-tab focus cycling and a submit
// button.
//
// Each field renders as a bordered, titled component (Text and Password wrap
// pkg/input, Confirm wraps pkg/toggle, Select owns its own pane), so the Label
// sits on the border as a pane title. Focus is signalled by the border color
// flipping from BorderInactive to BorderActive — there's no "▸" prefix or
// inline label line.
//
// Usage:
//
//	f := form.New(theme.Dark().Form().With([]form.Field{
//	    form.Text(form.TextOptions{Key: "name", Label: "Name"}),
//	    form.Password(form.PasswordOptions{Key: "pass", Label: "Password"}),
//	    form.Select(form.SelectOptions{Key: "role", Label: "Role",
//	        Options: []string{"admin", "user"}}),
//	    form.Confirm(form.ConfirmOptions{Key: "agree", Label: "I agree"}),
//	}))
//
// The form emits form.SubmittedMsg (with all values keyed by Field.Key) when
// the user presses enter on the submit button, and form.CancelledMsg on esc.
// The enclosing screen's IsCapturingKeys should mirror Model.IsCapturingKeys.
package form

import (
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/input"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/toggle"
)

// SubmittedMsg is emitted on enter over the submit button. Values maps each
// field's Key to its Value() — type depends on the field (string for Text
// and Select, bool for Confirm).
type SubmittedMsg struct{ Values map[string]any }

// CancelledMsg is emitted when the user presses esc.
type CancelledMsg struct{}

// InvalidMsg is emitted when a submit is refused because one or more fields
// failed validation. Keys names them, in field order. The form has already
// flagged the fields and moved focus to the first offender — a screen can
// ignore this entirely, and only needs it to react (a log line, a count, a
// statusbar note).
type InvalidMsg struct{ Keys []string }

// ErrRequired is what a Required field reports while it is empty, and what a
// Select with RequirePick reports until something is chosen.
var ErrRequired = errors.New("required")

// Styles bundles the visual knobs the form passes down to its fields and
// uses for the submit button. Populate via theme.Form() or set directly on
// Options.
type Styles struct {
	// Input styles the text inside text fields (mapped to input.TextStyle).
	Input lipgloss.Style
	// Placeholder styles the placeholder text inside empty text fields.
	Placeholder lipgloss.Style
	// CursorColor is the foreground for the text-input cursor glyph.
	CursorColor lipgloss.TerminalColor

	// Selected styles the active item in Select and the chosen side of Confirm.
	Selected lipgloss.Style

	// PaneActiveColor / PaneInactiveColor color the field's border, focused
	// and unfocused. Forwarded to each field's pane (input.SetActiveColor /
	// SetInactiveColor, or the pane's own defaults).
	PaneActiveColor   lipgloss.TerminalColor
	PaneInactiveColor lipgloss.TerminalColor

	// FieldBorderActive / FieldBorderInactive shape the field's border,
	// focused and unfocused. Every other component takes its shape from the
	// theme; before these existed a form's fields were the one chrome in the
	// library that could not be reached at all, not even by an explicit
	// override. Zero values leave each field's own default in place.
	FieldBorderActive   lipgloss.Border
	FieldBorderInactive lipgloss.Border

	// ErrorColor tints a field's border while it is showing a validation
	// error, regardless of focus — an invalid field is invalid whether or
	// not you are standing on it.
	ErrorColor lipgloss.TerminalColor
	// ErrorText styles the message rendered on the field's border.
	ErrorText lipgloss.Style

	// Submit and SubmitActive style the submit button (unfocused / focused).
	Submit       lipgloss.Style
	SubmitActive lipgloss.Style
}

// Options configures a new Form. All fields are optional except Fields.
type Options struct {
	Width, Height int
	Fields        []Field
	// SubmitText is the label on the submit button. Defaults to "Submit".
	SubmitText string
	// FieldSpacing is the number of blank lines between adjacent fields and
	// before the submit button. Defaults to 0 (borders touch). Set higher
	// for a looser layout.
	FieldSpacing *int
	Styles       Styles
}

// With returns a copy of opts with Fields replaced — handy when chaining
// from a theme builder: `theme.Dark().Form().With([]Field{…})`.
func (opts Options) With(fields []Field) Options {
	opts.Fields = fields
	return opts
}

// Model is the form component.
type Model struct {
	fields []Field
	// flagged marks fields that have failed a submit and are therefore in
	// live re-validation. Parallel to fields.
	flagged    []bool
	focus      int // 0..len(fields)-1 is a field; len(fields) is the submit button
	w, h       int
	rect       geom.Rect
	submitText string
	// submitRect is where the submit button last drew. It is a pointer
	// because View takes a value receiver — recording it on the copy would
	// be discarded before Update could hit-test it.
	submitRect *geom.Rect
	spacing    int
	styles     Styles
}

// New constructs a Form. Focuses the first field.
func New(opts Options) Model {
	if opts.SubmitText == "" {
		opts.SubmitText = "Submit"
	}
	spacing := 0
	if opts.FieldSpacing != nil {
		spacing = *opts.FieldSpacing
	}
	m := Model{
		fields:     opts.Fields,
		flagged:    make([]bool, len(opts.Fields)),
		w:          opts.Width,
		h:          opts.Height,
		submitText: opts.SubmitText,
		submitRect: new(geom.Rect),
		spacing:    spacing,
		styles:     opts.Styles,
	}
	m.propagateStyles()
	return m
}

// Init returns the textinput cursor-blink command so text fields animate.
func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.focusCurrent())
}

// Update handles tab/shift-tab focus cycling, enter, esc, and forwards
// everything else to the focused field.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "tab":
			m.focus = (m.focus + 1) % (len(m.fields) + 1)
			return m, m.focusCurrent()
		case "shift+tab":
			m.focus--
			if m.focus < 0 {
				m.focus = len(m.fields)
			}
			return m, m.focusCurrent()
		case "esc":
			return m, func() tea.Msg { return CancelledMsg{} }
		case "enter":
			if m.onSubmit() {
				return m, m.submit()
			}
			m.focus++
			if m.focus > len(m.fields) {
				m.focus = 0
			}
			return m, m.focusCurrent()
		}
	}

	// Mouse goes to every field so each can test the click against its own
	// rect, and a press inside one moves the form's focus there. Routing it
	// to the focused field alone — the way keys are routed — would make a
	// click on any other field do nothing at all.
	if e, ok := msg.(mouse.Msg); ok {
		var cmds []tea.Cmd
		if e.IsPress() && m.submitRect.Hit(e.X, e.Y) {
			m.focus = len(m.fields)
			return m, m.submit()
		}
		if e.IsPress() {
			for i, f := range m.fields {
				if !f.Rect().Hit(e.X, e.Y) {
					continue
				}
				// Stop at the first field containing the point, whether or
				// not it is the focused one. Skipping past the focused field
				// would let the click fall through to another.
				if i != m.focus {
					m.focus = i
					cmds = append(cmds, m.focusCurrent())
				}
				break
			}
		}
		for i, f := range m.fields {
			next, cmd := f.Update(msg)
			m.fields[i] = next
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	if m.focus < len(m.fields) {
		f, cmd := m.fields[m.focus].Update(msg)
		m.fields[m.focus] = f
		m.revalidateFlagged()
		return m, cmd
	}
	return m, nil
}

// View renders the form vertically: each field, FieldSpacing blank lines
// between, then the submit button.
func (m Model) View() string {
	sep := "\n" + strings.Repeat("\n", m.spacing)
	var b strings.Builder
	// Fields are stacked, so each one's origin is the running total of what
	// came before plus the separators. Height is only known after rendering,
	// which is why y advances by the measured view rather than a prediction.
	y := m.rect.Y
	for i, f := range m.fields {
		if i > 0 {
			b.WriteString(sep)
			// The field above ended on its last row without a trailing
			// newline, and h already accounted for that row — so the
			// separator advances by spacing, not 1+spacing. Counting its
			// first newline again drifts every field down by one more row
			// than the last, which puts a click near a boundary on the
			// wrong field.
			y += m.spacing
		}
		v := f.View(geom.Rect{X: m.rect.X, Y: y, W: m.rect.W, H: m.rect.H, Gen: m.rect.Gen}, i == m.focus)
		h := lipgloss.Height(v)
		// Record the rows the field actually drew, now that they are known.
		f.SetRect(geom.Rect{X: m.rect.X, Y: y, W: m.rect.W, H: h, Gen: m.rect.Gen})
		b.WriteString(v)
		y += h
	}
	b.WriteString(sep)
	y += m.spacing
	label := "[ " + m.submitText + " ]"
	// The submit button is a focus stop like any field, so it needs a rect
	// to be clickable. It isn't a Field — nothing else would record one.
	*m.submitRect = geom.Rect{
		X: m.rect.X, Y: y, W: lipgloss.Width(label), H: 1, Gen: m.rect.Gen,
	}
	if m.onSubmit() {
		b.WriteString(m.styles.SubmitActive.Render(label))
	} else {
		b.WriteString(m.styles.Submit.Render(label))
	}
	return b.String()
}

// SetRect satisfies layout.Sizer so the form can be placed into a layout
// tree via layout.Sized(&formModel). The rect is retained so View can hand
// each field its own absolute position as it stacks them.
func (m *Model) SetRect(r geom.Rect) { m.rect = r; m.w, m.h = r.W, r.H }

// Rect returns the rect the form was last placed at.
func (m Model) Rect() geom.Rect { return m.rect }

// validate checks every field, flags the failures, and returns their keys in
// field order along with the index of the first offender.
//
// Flagging is what switches a field into live re-validation: once a field has
// been told it is wrong, every subsequent keystroke re-checks it so the error
// clears the moment it is fixed. Before the first submit nothing is flagged,
// so nobody is nagged about a field they have not finished filling in.
func (m *Model) validate() (keys []string, first int) {
	first = -1
	for i, f := range m.fields {
		err := f.Validate()
		f.SetError(err)
		if err == nil {
			m.flagged[i] = false
			continue
		}
		m.flagged[i] = true
		keys = append(keys, f.Key())
		if first < 0 {
			first = i
		}
	}
	return keys, first
}

// revalidateFlagged re-checks only the fields already showing an error, so a
// correction is acknowledged immediately without complaining about fields the
// user has not reached yet.
func (m *Model) revalidateFlagged() {
	for i, f := range m.fields {
		if !m.flagged[i] {
			continue
		}
		err := f.Validate()
		f.SetError(err)
		if err == nil {
			m.flagged[i] = false
		}
	}
}

// submit validates and either emits the values or refuses.
//
// Refusing rather than handing back invalid values with an error attached
// means a screen cannot act on bad input by forgetting to check. Focus moves
// to the first offender so the fix is a keystroke away rather than a hunt.
func (m *Model) submit() tea.Cmd {
	keys, first := m.validate()
	if len(keys) == 0 {
		return func() tea.Msg { return SubmittedMsg{Values: m.Values()} }
	}
	m.focus = first
	cmd := m.focusCurrent()
	return tea.Batch(cmd, func() tea.Msg { return InvalidMsg{Keys: keys} })
}

// FocusedIndex returns which field currently has the keyboard;
// len(Fields) means the submit button.
func (m Model) FocusedIndex() int { return m.focus }

// SubmitRect returns where the submit button last rendered. Empty before
// the first View.
func (m Model) SubmitRect() geom.Rect {
	if m.submitRect == nil {
		return geom.Rect{}
	}
	return *m.submitRect
}

// FieldRect returns where field i last rendered. Empty before the first
// View, and for an out-of-range index.
func (m Model) FieldRect(i int) geom.Rect {
	if i < 0 || i >= len(m.fields) {
		return geom.Rect{}
	}
	return m.fields[i].Rect()
}

// SetStyles replaces the form's styles and propagates them to every field.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.propagateStyles()
}

// IsCapturingKeys always returns true while the form is active — use it
// from the enclosing screen's IsCapturingKeys() so the app shell keeps its
// global keys (q / t / esc) out of the form.
func (m Model) IsCapturingKeys() bool { return true }

// Help returns the form's own bindings plus the focused field's. The
// enclosing screen typically returns these directly from its own Help().
func (m Model) Help() []key.Binding {
	out := []key.Binding{
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "next")),
		key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "prev")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "submit")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
	if m.focus < len(m.fields) {
		out = append(out, m.fields[m.focus].Help()...)
	}
	return out
}

// Values returns every field's value keyed by Field.Key.
func (m Model) Values() map[string]any {
	out := make(map[string]any, len(m.fields))
	for _, f := range m.fields {
		out[f.Key()] = f.Value()
	}
	return out
}

// Value returns a single field's value by key, or nil if no such field.
func (m Model) Value(key string) any {
	for _, f := range m.fields {
		if f.Key() == key {
			return f.Value()
		}
	}
	return nil
}

// String returns a field's value as a string, or "" if the field is missing
// or the value isn't a string.
func (m Model) String(key string) string {
	if s, ok := m.Value(key).(string); ok {
		return s
	}
	return ""
}

// Bool returns a field's value as a bool, or false if the field is missing
// or the value isn't a bool.
func (m Model) Bool(key string) bool {
	if b, ok := m.Value(key).(bool); ok {
		return b
	}
	return false
}

// onSubmit reports whether the submit button is the focused "field".
func (m Model) onSubmit() bool { return m.focus == len(m.fields) }

// focusCurrent calls Focus on the focused field and Blur on all others.
func (m *Model) focusCurrent() tea.Cmd {
	var cmd tea.Cmd
	for i, f := range m.fields {
		if i == m.focus {
			cmd = f.Focus()
		} else {
			f.Blur()
		}
	}
	return cmd
}

func (m *Model) propagateStyles() {
	for _, f := range m.fields {
		f.SetStyles(&m.styles)
	}
}

// ---- Field interface ------------------------------------------------------

// Field is the contract each form entry satisfies. Use the Text, Select, and
// Confirm constructors — don't implement Field yourself unless you need a
// custom entry kind. The form stores fields by pointer and mutates them in
// place on Update/Focus/Blur.
type Field interface {
	Key() string
	Update(tea.Msg) (Field, tea.Cmd)
	// View renders the field into the given rect. Fields that occupy a
	// fixed number of rows may ignore r.H and use their own. focused tells
	// the field whether it currently owns input — fields use it to flip
	// border colors and expand inline (e.g. Select shows all options when
	// focused).
	View(r geom.Rect, focused bool) string
	// Rect reports where the field last rendered, so the form can work out
	// which one a click landed in.
	Rect() geom.Rect
	// SetRect records that area. The form calls it after rendering, because
	// a field's height is only known once it has drawn — handing every
	// field the form's full height would overlap them all and make a click
	// resolve to whichever the search reached first.
	SetRect(geom.Rect)
	Value() any
	// Validate reports why the field's current value is unacceptable, or
	// nil when it is fine.
	Validate() error
	// SetError shows err on the field; nil clears it. The form calls this,
	// so a field never decides on its own when to complain.
	SetError(err error)
	Focus() tea.Cmd
	Blur()
	SetStyles(*Styles)
	// Help returns the keys this field responds to — typically delegates
	// to the embedded component's Help().
	Help() []key.Binding
}

// ---- Text field -----------------------------------------------------------

// TextOptions configures a Text field.
type TextOptions struct {
	Key, Label, Placeholder, Initial string

	// Required rejects an empty value. The label gains a "*" so the
	// obligation is visible before anyone submits.
	Required bool
	// Validate reports why the value is unacceptable, or nil. Runs after
	// Required, so a blank required field says "required" rather than
	// whatever a format check would say about "".
	Validate func(any) error
}

// errorPresenter is what a field's underlying component must offer for the
// form to paint an error on it: a border to tint and a slot to write in.
type errorPresenter interface {
	SetActiveColor(lipgloss.TerminalColor)
	SetInactiveColor(lipgloss.TerminalColor)
	SetTopRight(string)
}

// applyFieldError tints the component's border and writes the message into
// its top-right border slot, or restores the normal colors when err is nil.
//
// Both border states are set, not just the active one: an invalid field is
// invalid whether or not the cursor happens to be on it, and a red border
// that turns grey when you tab away would hide the problem you just caused.
func applyFieldError(p errorPresenter, st *Styles, err error) {
	if st == nil {
		return
	}
	if err == nil {
		p.SetActiveColor(st.PaneActiveColor)
		p.SetInactiveColor(st.PaneInactiveColor)
		p.SetTopRight("")
		return
	}
	p.SetActiveColor(st.ErrorColor)
	p.SetInactiveColor(st.ErrorColor)
	p.SetTopRight(st.ErrorText.Render(err.Error()))
}

// borderPresenter is what a field's component must offer for the form to
// shape its border. Kept separate from errorPresenter: shape is a standing
// property of the theme, while the error colors are a transient complaint.
type borderPresenter interface {
	SetActiveBorder(lipgloss.Border)
	SetInactiveBorder(lipgloss.Border)
}

// applyFieldBorder pushes the theme's field shape onto one field's component.
// Zero values are skipped so a Styles that predates these fields leaves the
// component's own default alone.
func applyFieldBorder(p borderPresenter, st *Styles) {
	if st == nil {
		return
	}
	if (st.FieldBorderActive != lipgloss.Border{}) {
		p.SetActiveBorder(st.FieldBorderActive)
	}
	if (st.FieldBorderInactive != lipgloss.Border{}) {
		p.SetInactiveBorder(st.FieldBorderInactive)
	}
}

// fieldState is the validation bookkeeping every field type shares.
type fieldState struct {
	required bool
	validate func(any) error
	err      error
}

// check runs Required first, then Validate. Required first means a blank
// required field reports "required" rather than whatever a format rule makes
// of an empty string.
func (f fieldState) check(v any) error {
	if f.required {
		if s, ok := v.(string); ok && s == "" {
			return ErrRequired
		}
	}
	if f.validate != nil {
		return f.validate(v)
	}
	return nil
}

// label appends the required marker so the obligation is legible before any
// submit, and distinct from the tinted border that complains about content.
func (f fieldState) label(s string) string {
	if f.required {
		return s + " *"
	}
	return s
}

type textField struct {
	fieldState
	rect   geom.Rect
	key    string
	input  input.Model
	styles *Styles
}

// Text returns a single-line text field backed by pkg/input. Label becomes
// the input pane's title.
func Text(opts TextOptions) Field {
	st := fieldState{required: opts.Required, validate: opts.Validate}
	in := input.New(input.Options{
		Title:       st.label(opts.Label),
		Placeholder: opts.Placeholder,
		Initial:     opts.Initial,
	})
	return &textField{fieldState: st, key: opts.Key, input: in}
}

// PasswordOptions configures a Password field. Every field here means what
// its TextOptions namesake means — Validate sees the real text, not the mask,
// and the value arrives in SubmittedMsg.Values as an ordinary string.
type PasswordOptions struct {
	Key, Label, Placeholder, Initial string

	// MaskChar is the glyph shown per typed character. Defaults to
	// input.DefaultMaskChar.
	MaskChar rune

	// Required rejects an empty value. The label gains a "*" so the
	// obligation is visible before anyone submits.
	Required bool
	// Validate reports why the value is unacceptable, or nil. Runs after
	// Required, so a blank required field says "required" rather than
	// whatever a format check would say about "".
	Validate func(any) error
}

// Password returns a masked single-line text field backed by pkg/input. It is
// Text in every respect except that typed characters render as MaskChar.
//
// Confirm-password rules have no field to attach to and so are still out of
// scope here (see the anti-pattern note on cross-field validation): compare
// the two values on SubmittedMsg in the screen.
func Password(opts PasswordOptions) Field {
	st := fieldState{required: opts.Required, validate: opts.Validate}
	in := input.New(input.Options{
		Title:       st.label(opts.Label),
		Placeholder: opts.Placeholder,
		Initial:     opts.Initial,
		Echo:        input.EchoMask,
		MaskChar:    opts.MaskChar,
	})
	return &textField{fieldState: st, key: opts.Key, input: in}
}

func (f *textField) Validate() error { return f.check(f.Value()) }

func (f *textField) SetError(err error) {
	f.err = err
	applyFieldError(&f.input, f.styles, err)
}

func (f *textField) Key() string         { return f.key }
func (f *textField) Value() any          { return f.input.Value() }
func (f *textField) Focus() tea.Cmd      { return f.input.Focus() }
func (f *textField) Blur()               { f.input.Blur() }
func (f *textField) Help() []key.Binding { return f.input.Help() }

func (f *textField) SetStyles(s *Styles) {
	f.styles = s
	if s == nil {
		return
	}
	f.input.SetTextStyle(s.Input)
	f.input.SetPlaceholderStyle(s.Placeholder)
	f.input.SetCursorColor(s.CursorColor)
	if s.PaneActiveColor != nil {
		f.input.SetActiveColor(s.PaneActiveColor)
	}
	if s.PaneInactiveColor != nil {
		f.input.SetInactiveColor(s.PaneInactiveColor)
	}
	applyFieldBorder(&f.input, s)
}

func (f *textField) Update(msg tea.Msg) (Field, tea.Cmd) {
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return f, cmd
}

func (f *textField) Rect() geom.Rect     { return f.rect }
func (f *textField) SetRect(r geom.Rect) { f.rect = r }

func (f *textField) View(r geom.Rect, _ bool) string {
	f.input.SetRect(r)
	return f.input.View()
}

// ---- Select field ---------------------------------------------------------

// SelectOptions configures a Select field.
type SelectOptions struct {
	Key, Label string
	Options    []string
	Initial    int // initial cursor index
	// Height is the total field height including borders. Defaults to
	// min(len(Options)+2, 6) — i.e. auto-fit up to 4 visible rows; longer
	// option lists scroll within the fixed height.
	Height int

	// RequirePick starts the select with nothing highlighted and rejects
	// submission until the user chooses. A select always has a cursor
	// otherwise, so "required" for it means "must be picked deliberately"
	// rather than "must be non-empty". The label gains a "*".
	RequirePick bool
	// Validate reports why the chosen option is unacceptable, or nil.
	Validate func(any) error
}

type selectField struct {
	fieldState
	rect   geom.Rect
	key    string
	list   list.Model
	height int
	styles *Styles
}

// Select returns a single-choice field backed by pkg/list. While focused,
// up/down move the cursor; Value returns the highlighted option. Label
// becomes the list pane's title. Long option lists scroll within Height.
func Select(o SelectOptions) Field {
	initial := o.Initial
	if initial < 0 || initial >= len(o.Options) {
		initial = 0
	}
	height := o.Height
	if height <= 0 {
		height = min(len(o.Options)+2, 6)
	}
	st := fieldState{required: o.RequirePick, validate: o.Validate}
	li := list.New(list.Options{
		Title:      st.label(o.Label),
		Items:      o.Options,
		Filterable: false,
	})
	li.SetCursor(initial)
	if o.RequirePick {
		// Start with nothing highlighted, so the first pick is deliberate
		// rather than inherited from wherever the cursor happened to be.
		li.Deselect()
	}
	return &selectField{fieldState: st, key: o.Key, list: li, height: height}
}

func (f *selectField) Validate() error { return f.check(f.Value()) }

func (f *selectField) SetError(err error) {
	f.err = err
	applyFieldError(&f.list, f.styles, err)
}

func (f *selectField) Key() string         { return f.key }
func (f *selectField) Focus() tea.Cmd      { return f.list.Focus() }
func (f *selectField) Blur()               { f.list.Blur() }
func (f *selectField) Help() []key.Binding { return f.list.Help() }

func (f *selectField) SetStyles(s *Styles) {
	f.styles = s
	if s == nil {
		return
	}
	if s.PaneActiveColor != nil {
		f.list.SetActiveColor(s.PaneActiveColor)
	}
	if s.PaneInactiveColor != nil {
		f.list.SetInactiveColor(s.PaneInactiveColor)
	}
	f.list.SetSelectedStyle(s.Selected)
	applyFieldBorder(&f.list, s)
}

func (f *selectField) Value() any {
	v, _ := f.list.Selected()
	return v
}

func (f *selectField) Update(msg tea.Msg) (Field, tea.Cmd) {
	var cmd tea.Cmd
	f.list, cmd = f.list.Update(msg)
	return f, cmd
}

func (f *selectField) Rect() geom.Rect     { return f.rect }
func (f *selectField) SetRect(r geom.Rect) { f.rect = r }

func (f *selectField) View(r geom.Rect, focused bool) string {
	if focused {
		f.list.Focus()
	} else {
		f.list.Blur()
	}
	r.H = f.height
	f.list.SetRect(r)
	return f.list.View()
}

// ---- Confirm field --------------------------------------------------------

// ConfirmOptions configures a Confirm field.
type ConfirmOptions struct {
	Key, Label string
	Initial    bool

	// Validate reports why the answer is unacceptable, or nil. A yes/no
	// always has a value, so there is no Required — express "must accept"
	// here instead.
	Validate func(any) error
}

type confirmField struct {
	fieldState
	rect   geom.Rect
	key    string
	toggle toggle.Model
	styles *Styles
}

// Confirm returns a yes/no field backed by pkg/toggle. Label becomes the
// toggle pane's title.
func Confirm(o ConfirmOptions) Field {
	st := fieldState{validate: o.Validate}
	t := toggle.New(toggle.Options{
		Title:   st.label(o.Label),
		Initial: o.Initial,
	})
	return &confirmField{fieldState: st, key: o.Key, toggle: t}
}

func (f *confirmField) Validate() error { return f.check(f.Value()) }

func (f *confirmField) SetError(err error) {
	f.err = err
	applyFieldError(&f.toggle, f.styles, err)
}

func (f *confirmField) Key() string         { return f.key }
func (f *confirmField) Value() any          { return f.toggle.Value() }
func (f *confirmField) Focus() tea.Cmd      { return f.toggle.Focus() }
func (f *confirmField) Blur()               { f.toggle.Blur() }
func (f *confirmField) Help() []key.Binding { return f.toggle.Help() }

func (f *confirmField) SetStyles(s *Styles) {
	f.styles = s
	if s == nil {
		return
	}
	f.toggle.SetSelectedStyle(s.Selected)
	f.toggle.SetUnselectedStyle(lipgloss.NewStyle())
	if s.PaneActiveColor != nil {
		f.toggle.SetActiveColor(s.PaneActiveColor)
	}
	if s.PaneInactiveColor != nil {
		f.toggle.SetInactiveColor(s.PaneInactiveColor)
	}
	applyFieldBorder(&f.toggle, s)
}

func (f *confirmField) Update(msg tea.Msg) (Field, tea.Cmd) {
	var cmd tea.Cmd
	f.toggle, cmd = f.toggle.Update(msg)
	return f, cmd
}

func (f *confirmField) Rect() geom.Rect     { return f.rect }
func (f *confirmField) SetRect(r geom.Rect) { f.rect = r }

func (f *confirmField) View(r geom.Rect, _ bool) string {
	f.toggle.SetRect(r)
	return f.toggle.View()
}

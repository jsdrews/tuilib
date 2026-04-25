// Package form is a vertical form component: a sequence of Text, Select,
// and Confirm fields with tab/shift-tab focus cycling and a submit button.
//
// Each field renders as a bordered, titled component (Text wraps pkg/input,
// Confirm wraps pkg/toggle, Select owns its own pane), so the field's Label
// sits on the border as a pane title. Focus is signalled by the border color
// flipping from BorderInactive to BorderActive — there's no "▸" prefix or
// inline label line.
//
// Usage:
//
//	f := form.New(theme.Dark().Form().With([]form.Field{
//	    form.Text(form.TextOptions{Key: "name", Label: "Name"}),
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
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/input"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/toggle"
)

// SubmittedMsg is emitted on enter over the submit button. Values maps each
// field's Key to its Value() — type depends on the field (string for Text
// and Select, bool for Confirm).
type SubmittedMsg struct{ Values map[string]any }

// CancelledMsg is emitted when the user presses esc.
type CancelledMsg struct{}

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

	// PaneActive / PaneInactive color the field's border. Forwarded to each
	// field's pane (input.SetActiveColor / SetInactiveColor or pane defaults).
	PaneActive   lipgloss.TerminalColor
	PaneInactive lipgloss.TerminalColor

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
	fields     []Field
	focus      int // 0..len(fields)-1 is a field; len(fields) is the submit button
	w, h       int
	submitText string
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
		w:          opts.Width,
		h:          opts.Height,
		submitText: opts.SubmitText,
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
				return m, func() tea.Msg { return SubmittedMsg{Values: m.Values()} }
			}
			m.focus++
			if m.focus > len(m.fields) {
				m.focus = 0
			}
			return m, m.focusCurrent()
		}
	}

	if m.focus < len(m.fields) {
		f, cmd := m.fields[m.focus].Update(msg)
		m.fields[m.focus] = f
		return m, cmd
	}
	return m, nil
}

// View renders the form vertically: each field, FieldSpacing blank lines
// between, then the submit button.
func (m Model) View() string {
	sep := "\n" + strings.Repeat("\n", m.spacing)
	var b strings.Builder
	for i, f := range m.fields {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(f.View(m.w, i == m.focus))
	}
	b.WriteString(sep)
	label := "[ " + m.submitText + " ]"
	if m.onSubmit() {
		b.WriteString(m.styles.SubmitActive.Render(label))
	} else {
		b.WriteString(m.styles.Submit.Render(label))
	}
	return b.String()
}

// SetDimensions satisfies layout.Sized so the form can be placed into a
// layout tree via layout.Sized(&formModel).
func (m *Model) SetDimensions(w, h int) { m.w, m.h = w, h }

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
	// View renders the field at the given outer width. focused tells the
	// field whether it currently owns input — fields use it to flip border
	// colors and expand inline (e.g. Select shows all options when focused).
	View(width int, focused bool) string
	Value() any
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
}

type textField struct {
	key    string
	input  input.Model
	styles *Styles
}

// Text returns a single-line text field backed by pkg/input. Label becomes
// the input pane's title.
func Text(opts TextOptions) Field {
	in := input.New(input.Options{
		Title:       opts.Label,
		Placeholder: opts.Placeholder,
		Initial:     opts.Initial,
	})
	return &textField{key: opts.Key, input: in}
}

func (f *textField) Key() string           { return f.key }
func (f *textField) Value() any            { return f.input.Value() }
func (f *textField) Focus() tea.Cmd        { return f.input.Focus() }
func (f *textField) Blur()                 { f.input.Blur() }
func (f *textField) Help() []key.Binding   { return f.input.Help() }

func (f *textField) SetStyles(s *Styles) {
	f.styles = s
	if s == nil {
		return
	}
	f.input.SetTextStyle(s.Input)
	f.input.SetPlaceholderStyle(s.Placeholder)
	f.input.SetCursorColor(s.CursorColor)
	if s.PaneActive != nil {
		f.input.SetActiveColor(s.PaneActive)
	}
	if s.PaneInactive != nil {
		f.input.SetInactiveColor(s.PaneInactive)
	}
}

func (f *textField) Update(msg tea.Msg) (Field, tea.Cmd) {
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return f, cmd
}

func (f *textField) View(width int, _ bool) string {
	f.input.SetWidth(width)
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
}

type selectField struct {
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
	li := list.New(list.Options{
		Title:          o.Label,
		Items:          o.Options,
		Filterable:     false,
		ActiveBorder:   lipgloss.NormalBorder(),
		InactiveBorder: lipgloss.NormalBorder(),
	})
	li.SetCursor(initial)
	return &selectField{key: o.Key, list: li, height: height}
}

func (f *selectField) Key() string           { return f.key }
func (f *selectField) Focus() tea.Cmd        { f.list.SetFocused(true); return nil }
func (f *selectField) Blur()                 { f.list.SetFocused(false) }
func (f *selectField) Help() []key.Binding   { return f.list.Help() }

func (f *selectField) SetStyles(s *Styles) {
	f.styles = s
	if s == nil {
		return
	}
	if s.PaneActive != nil {
		f.list.SetActiveColor(s.PaneActive)
	}
	if s.PaneInactive != nil {
		f.list.SetInactiveColor(s.PaneInactive)
	}
	if c := s.Selected.GetForeground(); c != nil {
		f.list.SetSelectedColor(c)
	}
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

func (f *selectField) View(width int, focused bool) string {
	f.list.SetFocused(focused)
	f.list.SetDimensions(width, f.height)
	return f.list.View()
}

// ---- Confirm field --------------------------------------------------------

// ConfirmOptions configures a Confirm field.
type ConfirmOptions struct {
	Key, Label string
	Initial    bool
}

type confirmField struct {
	key    string
	toggle toggle.Model
	styles *Styles
}

// Confirm returns a yes/no field backed by pkg/toggle. Label becomes the
// toggle pane's title.
func Confirm(o ConfirmOptions) Field {
	t := toggle.New(toggle.Options{
		Title:   o.Label,
		Initial: o.Initial,
	})
	return &confirmField{key: o.Key, toggle: t}
}

func (f *confirmField) Key() string           { return f.key }
func (f *confirmField) Value() any            { return f.toggle.Value() }
func (f *confirmField) Focus() tea.Cmd        { return f.toggle.Focus() }
func (f *confirmField) Blur()                 { f.toggle.Blur() }
func (f *confirmField) Help() []key.Binding   { return f.toggle.Help() }

func (f *confirmField) SetStyles(s *Styles) {
	f.styles = s
	if s == nil {
		return
	}
	f.toggle.SetSelectedStyle(s.Selected)
	f.toggle.SetUnselectedStyle(lipgloss.NewStyle())
	if s.PaneActive != nil {
		f.toggle.SetActiveColor(s.PaneActive)
	}
	if s.PaneInactive != nil {
		f.toggle.SetInactiveColor(s.PaneInactive)
	}
}

func (f *confirmField) Update(msg tea.Msg) (Field, tea.Cmd) {
	var cmd tea.Cmd
	f.toggle, cmd = f.toggle.Update(msg)
	return f, cmd
}

func (f *confirmField) View(width int, _ bool) string {
	f.toggle.SetWidth(width)
	return f.toggle.View()
}

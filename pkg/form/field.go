package form

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/glyph"
)

// ErrRequired is what a Required field reports while it is empty, and what a
// Select with RequirePick or a MultiSelect short of Min reports until enough
// is chosen.
var ErrRequired = errors.New("required")

// Field is one entry in a form. Build it with Text, Password, Select,
// Confirm, MultiSelect or TextArea; the set is closed, because the form draws
// every field's label, note, well and focus bar itself.
type Field interface {
	// Key is what the field's value is filed under in Values.
	Key() string
	field() *field
}

// field is what every kind shares: the label and note the form draws around
// it, validation, and the control that draws and edits the value.
type field struct {
	key, label, note string
	required         bool
	validate         func(any) error
	err              error
	ctl              control
}

func (f *field) Key() string   { return f.key }
func (f *field) field() *field { return f }

// check runs the field's obligation first, then Validate, so a blank
// required field says "required" rather than whatever a format rule makes of
// an empty value.
func (f *field) check() error {
	if f.required && f.ctl.empty() {
		return ErrRequired
	}
	if f.validate != nil {
		return f.validate(f.ctl.value())
	}
	return nil
}

// title is the label as drawn: a required field carries a "*", a standing
// property distinct from the error tint, which is a complaint.
func (f *field) title() string {
	if f.required {
		return f.label + " *"
	}
	return f.label
}

// paint is what a control draws its rows with.
type paint struct {
	st      *Styles
	g       glyph.Set
	focused bool
	// well is the background behind a typed value; nil draws none.
	well lipgloss.TerminalColor
}

func (p paint) on(s lipgloss.Style) lipgloss.Style {
	if p.well != nil {
		return s.Background(p.well)
	}
	return s
}

// control draws and edits one kind of value. Its rows are a fixed number, so
// nothing reflows while the user edits.
type control interface {
	value() any
	empty() bool
	height() int
	// rows draws the value in w cells per row, height() rows.
	rows(w int, p paint) []string
	key(tea.KeyMsg) tea.Cmd
	// other forwards a non-key message (the cursor blink).
	other(tea.Msg) tea.Cmd
	focus() tea.Cmd
	blur()
	// click handles a press on value row row, col cells in.
	click(row, col int)
	help() []key.Binding
	// takesEnter reports whether enter edits the value instead of moving on.
	takesEnter() bool
	// wells reports whether the value sits in a well.
	wells() bool
	snapshot() any
	restore(any)
}

// fitWell pads s to w cells on the well, with a cell of the well either side.
func fitWell(s string, w int, p paint) string {
	pad := p.on(lipgloss.NewStyle())
	inner := max(0, w-2)
	if n := ansi.StringWidth(s); n > inner {
		s = ansi.Truncate(s, inner, "…")
	} else {
		s += pad.Render(strings.Repeat(" ", inner-n))
	}
	return pad.Render(" ") + s + pad.Render(" ")
}

// ---- Text and Password ----------------------------------------------------

// TextOptions configures a Text field.
type TextOptions struct {
	Key, Label, Placeholder, Initial string
	// Note says what the field is for, drawn muted under it and wrapped to
	// its width.
	Note string
	// CharLimit caps the value's length; zero is no limit.
	CharLimit int

	// Required rejects an empty value. The label gains a "*" so the
	// obligation is visible before anyone submits.
	Required bool
	// Validate reports why the value is unacceptable, or nil. Runs after
	// Required.
	Validate func(any) error
}

// PasswordOptions configures a Password field. Every field means what its
// TextOptions namesake means; Validate sees the real text, not the mask.
type PasswordOptions struct {
	Key, Label, Placeholder, Initial string
	Note                             string
	CharLimit                        int
	// MaskChar is the glyph shown per typed character. Defaults to '•'.
	MaskChar rune
	Required bool
	Validate func(any) error
}

type textCtl struct{ in textinput.Model }

// Text is a single-line text field.
func Text(o TextOptions) Field {
	return &field{
		key: o.Key, label: o.Label, note: o.Note, required: o.Required, validate: o.Validate,
		ctl: newText(o.Placeholder, o.Initial, o.CharLimit, 0),
	}
}

// Password is a Text field whose characters draw as MaskChar. Its value
// arrives in SubmittedMsg as the real text.
func Password(o PasswordOptions) Field {
	mask := o.MaskChar
	if mask == 0 {
		mask = '•'
	}
	return &field{
		key: o.Key, label: o.Label, note: o.Note, required: o.Required, validate: o.Validate,
		ctl: newText(o.Placeholder, o.Initial, o.CharLimit, mask),
	}
}

func newText(placeholder, initial string, limit int, mask rune) *textCtl {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = placeholder
	in.CharLimit = limit
	in.SetValue(initial)
	if mask != 0 {
		in.EchoMode = textinput.EchoPassword
		in.EchoCharacter = mask
	}
	return &textCtl{in: in}
}

func (c *textCtl) value() any          { return c.in.Value() }
func (c *textCtl) empty() bool         { return c.in.Value() == "" }
func (c *textCtl) height() int         { return 1 }
func (c *textCtl) takesEnter() bool    { return false }
func (c *textCtl) wells() bool         { return true }
func (c *textCtl) focus() tea.Cmd      { return c.in.Focus() }
func (c *textCtl) blur()               { c.in.Blur() }
func (c *textCtl) help() []key.Binding { return nil }
func (c *textCtl) click(_, col int) {
	c.in.SetCursor(max(0, col-1))
}

func (c *textCtl) key(k tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	c.in, cmd = c.in.Update(k)
	return cmd
}

func (c *textCtl) other(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	c.in, cmd = c.in.Update(msg)
	return cmd
}

func (c *textCtl) rows(w int, p paint) []string {
	c.in.Width = max(1, w-3)
	c.in.TextStyle = p.on(p.st.Text)
	c.in.PlaceholderStyle = p.on(p.st.Placeholder)
	c.in.PromptStyle = p.on(lipgloss.NewStyle())
	c.in.Cursor.Style = p.on(lipgloss.NewStyle().Foreground(p.st.CursorColor))
	c.in.Cursor.TextStyle = p.on(p.st.Text)
	return []string{fitWell(c.in.View(), w, p)}
}

type textSnap struct {
	value string
	pos   int
}

func (c *textCtl) snapshot() any { return textSnap{c.in.Value(), c.in.Position()} }
func (c *textCtl) restore(s any) {
	if t, ok := s.(textSnap); ok {
		c.in.SetValue(t.value)
		c.in.SetCursor(t.pos)
	}
}

// ---- TextArea ---------------------------------------------------------------

// TextAreaOptions configures a TextArea field.
type TextAreaOptions struct {
	Key, Label, Placeholder, Initial string
	Note                             string
	// Rows is how many lines the field shows; longer text scrolls inside
	// them. Defaults to 3.
	Rows      int
	CharLimit int
	Required  bool
	Validate  func(any) error
}

type areaCtl struct {
	ta   textarea.Model
	rowN int
}

// TextArea is multi-line text. Enter inserts a newline; tab and shift+tab
// leave the field.
func TextArea(o TextAreaOptions) Field {
	n := o.Rows
	if n <= 0 {
		n = 3
	}
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.Placeholder = o.Placeholder
	ta.CharLimit = o.CharLimit
	ta.SetHeight(n)
	ta.SetValue(o.Initial)
	return &field{
		key: o.Key, label: o.Label, note: o.Note, required: o.Required, validate: o.Validate,
		ctl: &areaCtl{ta: ta, rowN: n},
	}
}

func (c *areaCtl) value() any       { return c.ta.Value() }
func (c *areaCtl) empty() bool      { return strings.TrimSpace(c.ta.Value()) == "" }
func (c *areaCtl) height() int      { return c.rowN }
func (c *areaCtl) takesEnter() bool { return true }
func (c *areaCtl) wells() bool      { return true }
func (c *areaCtl) focus() tea.Cmd   { return c.ta.Focus() }
func (c *areaCtl) blur()            { c.ta.Blur() }
func (c *areaCtl) click(int, int)   {}

func (c *areaCtl) help() []key.Binding {
	return []key.Binding{key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "newline"))}
}

func (c *areaCtl) key(k tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	c.ta, cmd = c.ta.Update(k)
	return cmd
}

func (c *areaCtl) other(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	c.ta, cmd = c.ta.Update(msg)
	return cmd
}

func (c *areaCtl) rows(w int, p paint) []string {
	c.ta.SetWidth(max(1, w-2))
	base := p.on(lipgloss.NewStyle())
	s := textarea.Style{
		Base: base, Text: p.on(p.st.Text), CursorLine: p.on(p.st.Text),
		Placeholder: p.on(p.st.Placeholder), EndOfBuffer: base, Prompt: base,
	}
	c.ta.FocusedStyle, c.ta.BlurredStyle = s, s
	c.ta.Cursor.Style = p.on(lipgloss.NewStyle().Foreground(p.st.CursorColor))
	c.ta.Cursor.TextStyle = p.on(p.st.Text)
	lines := strings.Split(c.ta.View(), "\n")
	out := make([]string, c.rowN)
	for i := range out {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out[i] = fitWell(l, w, p)
	}
	return out
}

func (c *areaCtl) snapshot() any { return c.ta.Value() }
func (c *areaCtl) restore(s any) {
	if v, ok := s.(string); ok {
		c.ta.SetValue(v)
	}
}

// ---- Select -------------------------------------------------------------

// DefaultVisible is how many options a Select or MultiSelect shows before it
// scrolls.
const DefaultVisible = 5

// SelectOptions configures a Select field.
type SelectOptions struct {
	Key, Label string
	Note       string
	Options    []string
	// Initial is the option chosen to begin with, or — with RequirePick —
	// the one chosen when the field is first reached.
	Initial int
	// Visible is how many options show at once; past it the list scrolls
	// under a fixed "↑ n · ↓ n more" row. Defaults to DefaultVisible.
	Visible int

	// RequirePick starts with nothing chosen. Reaching the field chooses
	// Initial, so the choice is never invisible, and a form submitted
	// without reaching it is refused. The label gains a "*".
	RequirePick bool
	// Validate reports why the chosen option is unacceptable, or nil.
	Validate func(any) error
}

// listView is the scrolling window a Select and a MultiSelect share.
type listView struct {
	opts    []string
	visible int
	top     int
}

func newListView(opts []string, visible int) listView {
	if visible <= 0 {
		visible = DefaultVisible
	}
	return listView{opts: opts, visible: min(visible, max(1, len(opts)))}
}

func (l listView) scrolls() bool { return len(l.opts) > l.visible }

func (l listView) height() int {
	if l.scrolls() {
		return l.visible + 1
	}
	return max(1, len(l.opts))
}

// follow keeps row i in the window.
func (l *listView) follow(i int) {
	if i < l.top {
		l.top = i
	}
	if i >= l.top+l.visible {
		l.top = i - l.visible + 1
	}
	l.top = max(0, min(l.top, len(l.opts)-l.visible))
}

// at maps a value row to an option index, or -1 for the "more" row.
func (l listView) at(row int) int {
	if row < 0 || row >= l.visible || l.top+row >= len(l.opts) {
		return -1
	}
	return l.top + row
}

// draw renders the window, each option through line, plus the "more" row.
func (l listView) draw(w int, p paint, line func(i int) string) []string {
	var out []string
	for i := l.top; i < min(len(l.opts), l.top+l.visible); i++ {
		out = append(out, line(i))
	}
	if l.scrolls() {
		var more []string
		if l.top > 0 {
			more = append(more, fmt.Sprintf("↑ %d", l.top))
		}
		if below := len(l.opts) - l.top - l.visible; below > 0 {
			more = append(more, fmt.Sprintf("↓ %d more", below))
		}
		out = append(out, p.st.Note.Render("  "+strings.Join(more, " · ")))
	}
	return out
}

type selectCtl struct {
	listView
	chosen, initial int
}

// Select is a single choice from a list that is always open. The option
// under the cursor is the choice: ↑/↓ move it.
func Select(o SelectOptions) Field {
	initial := o.Initial
	if initial < 0 || initial >= len(o.Options) {
		initial = 0
	}
	c := &selectCtl{listView: newListView(o.Options, o.Visible), chosen: initial, initial: initial}
	if o.RequirePick || len(o.Options) == 0 {
		c.chosen = -1
	}
	c.follow(max(0, c.chosen))
	return &field{key: o.Key, label: o.Label, note: o.Note, required: o.RequirePick, validate: o.Validate, ctl: c}
}

func (c *selectCtl) value() any {
	if c.chosen < 0 {
		return ""
	}
	return c.opts[c.chosen]
}

func (c *selectCtl) empty() bool           { return c.chosen < 0 }
func (c *selectCtl) takesEnter() bool      { return false }
func (c *selectCtl) wells() bool           { return false }
func (c *selectCtl) other(tea.Msg) tea.Cmd { return nil }
func (c *selectCtl) blur()                 {}

// focus chooses Initial on arrival when nothing is chosen yet, so a
// RequirePick select is never focused with no visible choice.
func (c *selectCtl) focus() tea.Cmd {
	if c.chosen < 0 && len(c.opts) > 0 {
		c.choose(c.initial)
	}
	return nil
}

func (c *selectCtl) choose(i int) {
	c.chosen = max(0, min(i, len(c.opts)-1))
	c.follow(c.chosen)
}

func (c *selectCtl) key(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "up", "k":
		c.choose(c.chosen - 1)
	case "down", "j":
		c.choose(c.chosen + 1)
	case "home", "g":
		c.choose(0)
	case "end", "G":
		c.choose(len(c.opts) - 1)
	}
	return nil
}

func (c *selectCtl) click(row, _ int) {
	if i := c.at(row); i >= 0 {
		c.choose(i)
	}
}

func (c *selectCtl) help() []key.Binding {
	return []key.Binding{key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("↑↓", "choose"))}
}

func (c *selectCtl) rows(w int, p paint) []string {
	return c.draw(w, p, func(i int) string {
		g, gs, ts := p.g.RadioOff, p.st.Unchosen, p.st.Text
		if i == c.chosen {
			g, gs = p.g.RadioOn, p.st.Chosen
			ts = ts.Bold(p.focused)
		}
		return gs.Render(g) + " " + ts.Render(c.opts[i])
	})
}

func (c *selectCtl) snapshot() any { return c.chosen }
func (c *selectCtl) restore(s any) {
	if i, ok := s.(int); ok && i < len(c.opts) {
		c.chosen = i
		c.follow(max(0, i))
	}
}

// ---- MultiSelect ------------------------------------------------------------

// MultiSelectOptions configures a MultiSelect field.
type MultiSelectOptions struct {
	Key, Label string
	Note       string
	Options    []string
	// Initial are the options chosen to begin with, by index.
	Initial []int
	Visible int
	// Min is how many must be chosen; a positive Min makes the field
	// required and puts a "*" on its label. Max caps the choice; zero is no
	// cap, and a toggle past it is ignored.
	Min, Max int
	Validate func(any) error
}

type multiCtl struct {
	listView
	cursor   int
	marked   map[int]bool
	min, max int
}

// MultiSelect is any number of choices from a list that is always open. ↑/↓
// move the cursor and space toggles the option under it; the value is the
// chosen options in list order.
func MultiSelect(o MultiSelectOptions) Field {
	c := &multiCtl{listView: newListView(o.Options, o.Visible), marked: map[int]bool{}, min: o.Min, max: o.Max}
	for _, i := range o.Initial {
		if i >= 0 && i < len(o.Options) {
			c.marked[i] = true
		}
	}
	validate := o.Validate
	if o.Min > 1 {
		inner := validate
		validate = func(v any) error {
			if n := len(v.([]string)); n < o.Min {
				return fmt.Errorf("pick at least %d", o.Min)
			}
			if inner != nil {
				return inner(v)
			}
			return nil
		}
	}
	return &field{key: o.Key, label: o.Label, note: o.Note, required: o.Min > 0, validate: validate, ctl: c}
}

func (c *multiCtl) value() any {
	out := []string{}
	for i, o := range c.opts {
		if c.marked[i] {
			out = append(out, o)
		}
	}
	return out
}

func (c *multiCtl) empty() bool           { return len(c.marked) == 0 }
func (c *multiCtl) takesEnter() bool      { return false }
func (c *multiCtl) wells() bool           { return false }
func (c *multiCtl) other(tea.Msg) tea.Cmd { return nil }
func (c *multiCtl) focus() tea.Cmd        { return nil }
func (c *multiCtl) blur()                 {}

func (c *multiCtl) move(i int) {
	c.cursor = max(0, min(i, len(c.opts)-1))
	c.follow(c.cursor)
}

func (c *multiCtl) toggle(i int) {
	if c.marked[i] {
		delete(c.marked, i)
		return
	}
	if c.max > 0 && len(c.marked) >= c.max {
		return
	}
	c.marked[i] = true
}

func (c *multiCtl) key(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "up", "k":
		c.move(c.cursor - 1)
	case "down", "j":
		c.move(c.cursor + 1)
	case " ", "x":
		if len(c.opts) > 0 {
			c.toggle(c.cursor)
		}
	}
	return nil
}

func (c *multiCtl) click(row, _ int) {
	if i := c.at(row); i >= 0 {
		c.move(i)
		c.toggle(i)
	}
}

func (c *multiCtl) help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys(" ", "x"), key.WithHelp("space", "toggle")),
	}
}

func (c *multiCtl) rows(w int, p paint) []string {
	return c.draw(w, p, func(i int) string {
		cur := "  "
		if p.focused && i == c.cursor {
			cur = p.st.Bar.Render(p.g.Cursor) + " "
		}
		g, gs := p.g.CheckOff, p.st.Unchosen
		if c.marked[i] {
			g, gs = p.g.CheckOn, p.st.Chosen
		}
		return cur + gs.Render(g) + " " + p.st.Text.Bold(p.focused && i == c.cursor).Render(c.opts[i])
	})
}

type multiSnap struct {
	cursor int
	marked []int
}

func (c *multiCtl) snapshot() any {
	s := multiSnap{cursor: c.cursor}
	for i := range c.opts {
		if c.marked[i] {
			s.marked = append(s.marked, i)
		}
	}
	return s
}

func (c *multiCtl) restore(s any) {
	m, ok := s.(multiSnap)
	if !ok {
		return
	}
	c.marked = map[int]bool{}
	for _, i := range m.marked {
		if i < len(c.opts) {
			c.marked[i] = true
		}
	}
	c.move(m.cursor)
}

// ---- Confirm ----------------------------------------------------------------

// ConfirmOptions configures a Confirm field.
type ConfirmOptions struct {
	Key, Label string
	Note       string
	Initial    bool
	// Yes and No label the two sides. Default "yes" and "no".
	Yes, No string
	// Validate reports why the answer is unacceptable, or nil. A yes/no
	// always has a value, so there is no Required — say "must accept" here.
	Validate func(any) error
}

type confirmCtl struct {
	yes         bool
	yesL, noL   string
	yesAt, noAt int // where each side starts, for clicks
}

// Confirm is a yes/no answer: ←/→ (or y/n) pick a side and space flips it.
func Confirm(o ConfirmOptions) Field {
	c := &confirmCtl{yes: o.Initial, yesL: o.Yes, noL: o.No}
	if c.yesL == "" {
		c.yesL = "yes"
	}
	if c.noL == "" {
		c.noL = "no"
	}
	return &field{key: o.Key, label: o.Label, note: o.Note, validate: o.Validate, ctl: c}
}

func (c *confirmCtl) value() any            { return c.yes }
func (c *confirmCtl) empty() bool           { return false }
func (c *confirmCtl) height() int           { return 1 }
func (c *confirmCtl) takesEnter() bool      { return false }
func (c *confirmCtl) wells() bool           { return false }
func (c *confirmCtl) other(tea.Msg) tea.Cmd { return nil }
func (c *confirmCtl) focus() tea.Cmd        { return nil }
func (c *confirmCtl) blur()                 {}

func (c *confirmCtl) key(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "left", "h", "y":
		c.yes = true
	case "right", "l", "n":
		c.yes = false
	case " ":
		c.yes = !c.yes
	}
	return nil
}

func (c *confirmCtl) click(_, col int) {
	switch {
	case col >= c.noAt:
		c.yes = false
	case col >= c.yesAt:
		c.yes = true
	}
}

func (c *confirmCtl) help() []key.Binding {
	return []key.Binding{key.NewBinding(key.WithKeys("left", "right", "h", "l", "y", "n"), key.WithHelp("←→", "choose"))}
}

func (c *confirmCtl) rows(w int, p paint) []string {
	side := func(label string, chosen bool) string {
		if chosen {
			return p.st.Chosen.Render(p.g.RadioOn) + " " + p.st.Text.Bold(p.focused).Render(label)
		}
		return p.st.Unchosen.Render(p.g.RadioOff) + " " + p.st.Text.Render(label)
	}
	yes := side(c.yesL, c.yes)
	c.yesAt, c.noAt = 0, ansi.StringWidth(yes)+2
	return []string{yes + "    " + side(c.noL, !c.yes)}
}

func (c *confirmCtl) snapshot() any { return c.yes }
func (c *confirmCtl) restore(s any) {
	if b, ok := s.(bool); ok {
		c.yes = b
	}
}

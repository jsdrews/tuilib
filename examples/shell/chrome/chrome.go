// Package chrome demonstrates theme-owned border shape: the two slots a
// Theme carries for chrome, and the fact that changing one re-skins every
// component drawn from that theme without touching any component.
//
// The left column picks Theme.BorderShapeActive/Inactive (ordinary
// components) and Theme.BorderShapeOverlay (things that float above content).
// Everything on screen — the pickers included — is rebuilt from the modified
// theme, so what you are looking at is the same code path any app gets.
//
// The two slots are deliberately separate: an overlay reads as raised because
// its border differs from the pane it covers. Set both pickers to the same
// shape and the modal flattens into the content behind it, which is the thing
// the split exists to prevent.
package chrome

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/confirm"
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/input"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/toggle"
)

// glyphSets pairs a label with a glyph vocabulary. The zero Set resolves to
// glyph.Default, which is why "Default" carries no fields.
var glyphSets = []struct {
	name string
	set  glyph.Set
}{
	{"Default", glyph.Set{}},
	{"ASCII", glyph.Set{
		Cursor: ">", Mark: "*", ExpandOpen: "v", ExpandClosed: ">",
		Rule: "-", ScrollThumb: "#", ScrollTrack: ".",
		SortAsc: "^", SortDesc: "v", ColumnSep: "|", Placeholder: ".",
	}},
	{"Heavy", glyph.Set{
		Cursor: "➤", Mark: "✔", ExpandOpen: "▼", ExpandClosed: "▶",
		Rule: "━", ScrollThumb: "▓", ScrollTrack: "▒",
	}},
	{"Minimal", glyph.Set{
		Cursor: "·", Mark: "+", ExpandOpen: "-", ExpandClosed: "+",
		Rule: "┈", ScrollThumb: "│", ScrollTrack: " ",
	}},
}

func glyphNames() []string {
	out := make([]string, len(glyphSets))
	for i, g := range glyphSets {
		out[i] = g.name
	}
	return out
}

// shape pairs a label with the lipgloss border it names. Hidden is included
// because it is the one that shows what the title slots do on their own: the
// glyphs go away and the labels stay.
var shapes = []struct {
	name   string
	border lipgloss.Border
}{
	{"Normal", lipgloss.NormalBorder()},
	{"Rounded", lipgloss.RoundedBorder()},
	{"Thick", lipgloss.ThickBorder()},
	{"Double", lipgloss.DoubleBorder()},
	{"Block", lipgloss.BlockBorder()},
	{"Hidden", lipgloss.HiddenBorder()},
}

func shapeNames() []string {
	out := make([]string, len(shapes))
	for i, s := range shapes {
		out[i] = s.name
	}
	return out
}

type Screen struct {
	base theme.Theme

	// component / overlay are indices into shapes — the state the whole
	// screen is derived from.
	component int
	overlay   int
	glyphs    int

	compPick  list.Model
	ovlPick   list.Model
	glyphPick list.Model

	rows   list.Model
	field  input.Model
	choice toggle.Model
	modal  confirm.Model

	focus focus.Group
}

func New(t theme.Theme) screen.Screen {
	s := &Screen{component: 0, overlay: 2} // Normal components, Thick overlay
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string       { return "Chrome" }
func (s *Screen) OnEnter(any) tea.Cmd { return nil }

func (s *Screen) Init() tea.Cmd { return s.focus.Init() }

func (s *Screen) IsCapturingKeys() bool { return s.focus.IsCapturingKeys() }

// themed returns the base theme with the two shape slots set from the
// pickers. This is the whole feature: four lines, and every builder below
// picks it up.
func (s *Screen) themed() theme.Theme {
	t := s.base
	t.BorderShapeActive = shapes[s.component].border
	t.BorderShapeInactive = shapes[s.component].border
	t.BorderShapeOverlay = shapes[s.overlay].border
	t.Glyphs = glyphSets[s.glyphs].set
	return t
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	before := [3]int{s.component, s.overlay, s.glyphs}
	var cmds []tea.Cmd

	// The group owns tab cycling and focus grants; it does not forward to
	// the components, so the screen still has to (rule 6).
	var gcmd tea.Cmd
	s.focus, gcmd = s.focus.Update(msg)
	cmds = append(cmds, gcmd)

	// Keys go to the focused component alone; a click goes to every one of
	// them, since each tests the position against its own rect.
	if _, isMouse := msg.(mouse.Msg); isMouse {
		cmds = append(cmds, s.forwardAll(msg)...)
	} else {
		cmds = append(cmds, s.forwardFocused(msg))
	}

	s.component, s.overlay, s.glyphs = s.compPick.Cursor(), s.ovlPick.Cursor(), s.glyphPick.Cursor()
	if [3]int{s.component, s.overlay, s.glyphs} != before {
		// The pickers are drawn from the theme they edit, so a cursor move
		// rebuilds them too — cursors and focus carried across exactly as in
		// a theme swap (rule 4).
		s.rebuild()
	}
	return s, tea.Batch(cmds...)
}

// forwardAll hands a message to every component. Mouse only: each one tests
// the click against its own rect and the one it landed in acts.
func (s *Screen) forwardAll(msg tea.Msg) []tea.Cmd {
	var cmds []tea.Cmd
	var c tea.Cmd
	s.compPick, c = s.compPick.Update(msg)
	cmds = append(cmds, c)
	s.ovlPick, c = s.ovlPick.Update(msg)
	cmds = append(cmds, c)
	s.glyphPick, c = s.glyphPick.Update(msg)
	cmds = append(cmds, c)
	s.rows, c = s.rows.Update(msg)
	cmds = append(cmds, c)
	s.field, c = s.field.Update(msg)
	cmds = append(cmds, c)
	s.choice, c = s.choice.Update(msg)
	return append(cmds, c)
}

// forwardFocused hands a message to the focused component alone, so typing in
// the input does not also drive a list cursor.
func (s *Screen) forwardFocused(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch {
	case s.focus.Is(&s.compPick):
		s.compPick, cmd = s.compPick.Update(msg)
	case s.focus.Is(&s.ovlPick):
		s.ovlPick, cmd = s.ovlPick.Update(msg)
	case s.focus.Is(&s.glyphPick):
		s.glyphPick, cmd = s.glyphPick.Update(msg)
	case s.focus.Is(&s.rows):
		s.rows, cmd = s.rows.Update(msg)
	case s.focus.Is(&s.field):
		s.field, cmd = s.field.Update(msg)
	case s.focus.Is(&s.choice):
		s.choice, cmd = s.choice.Update(msg)
	}
	return cmd
}

func (s *Screen) Layout() layout.Node {
	return layout.HStack(
		layout.Fixed(24, layout.VStack(
			layout.Flex(1, layout.Sized(&s.compPick)),
			layout.Flex(1, layout.Sized(&s.ovlPick)),
			layout.Flex(1, layout.Sized(&s.glyphPick)),
		)),
		layout.Flex(1, layout.ZStack(
			layout.VStack(
				layout.Flex(1, layout.Sized(&s.rows)),
				layout.Fixed(3, layout.Sized(&s.field)),
				layout.Fixed(3, layout.Sized(&s.choice)),
			),
			layout.Center(36, 7, layout.Sized(&s.modal)),
		)),
	)
}

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

func (s *Screen) HelpSections() []help.Section {
	return help.SectionsOf(&s.focus, help.Group("Chrome",
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	))
}

// SetTheme takes the app's palette and rebuilds from it, keeping the two
// shape choices the user made.
func (s *Screen) SetTheme(t theme.Theme) {
	s.base = t
	s.rebuild()
}

// rebuild constructs every component from the derived theme, preserving
// cursors and which pane had focus.
func (s *Screen) rebuild() {
	t := s.themed()
	compCur, ovlCur, glyphCur, rowCur := s.component, s.overlay, s.glyphs, s.rows.Cursor()
	value := s.field.Value()
	idx := s.focus.Index()

	compOpts := t.List()
	compOpts.Title = "component shape"
	compOpts.Items = shapeNames()
	s.compPick = list.New(compOpts)
	s.compPick.SetCursor(compCur)

	ovlOpts := t.List()
	ovlOpts.Title = "overlay shape"
	ovlOpts.Items = shapeNames()
	s.ovlPick = list.New(ovlOpts)
	s.ovlPick.SetCursor(ovlCur)

	glyphOpts := t.List()
	glyphOpts.Title = "glyphs"
	glyphOpts.Items = glyphNames()
	s.glyphPick = list.New(glyphOpts)
	s.glyphPick.SetCursor(glyphCur)

	rowOpts := t.List()
	rowOpts.Title = "a list"
	rowOpts.Filterable = true
	rowOpts.Markable = true
	s.rows = list.New(rowOpts)
	s.rows.SetKeyedItems(previewRows())
	// Marked declaratively, not by ToggleMark: rebuild runs on every picker
	// move, and a toggle would flip the mark on and off as you browse.
	s.rows.SetMarks([]string{"worker-pool"})
	s.rows.SetCursor(rowCur)

	fieldOpts := t.Input()
	fieldOpts.Title = "an input"
	fieldOpts.Placeholder = "type here…"
	s.field = input.New(fieldOpts)
	s.field.SetValue(value)

	toggleOpts := t.Toggle()
	toggleOpts.Title = "a toggle"
	s.choice = toggle.New(toggleOpts)

	modalOpts := t.Confirm()
	modalOpts.Title = "an overlay"
	modalOpts.Message = "Its border comes from a different slot."
	s.modal = confirm.New(modalOpts)

	// The input is a focus stop like any other. It reports IsCapturingKeys
	// while focused, so tab cannot cycle off it — press esc to leave the
	// field, then tab.
	s.focus = focus.NewGroup(&s.compPick, &s.ovlPick, &s.glyphPick, &s.rows, &s.field, &s.choice)
	s.focus.SetIndex(idx)
}

// previewRows are keyed so the list can carry a mark; anonymous string items
// silently cannot be marked.
func previewRows() []list.KeyedItem {
	names := []string{
		"api-gateway", "worker-pool", "scheduler", "ingest",
		"web", "cache", "queue", "cron", "search", "billing",
	}
	out := make([]list.KeyedItem, len(names))
	for i, n := range names {
		out[i] = list.KeyedItem{Key: n, Display: n}
	}
	return out
}

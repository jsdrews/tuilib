// Package surfaces is a live surface picker: moving the cursor redraws the
// whole app in that surface — line border, filled card, header band, drop
// shadow — through app.SetSurface, the way shell/themes previews palettes.
//
// The preview shows what each component is handed. The list gets the surface
// itself; the input and the toggle get its field form, which fits a
// three-row slot; the modal — opened by the toggle — gets its overlay form,
// which reads as raised above the pane it covers. The "pinned" pane ignores the app's choice
// — a per-component override, t.Resolve(surface.Outlined) — so there is
// always an Outlined pane to compare against.
//
// Two of the surfaces on offer are not built in. Ruled and Lipped are defined
// below as struct literals, and the launcher offers them in its
// Options.Surfaces beside the built-ins: a custom surface goes through
// exactly the path a built-in does. See docs/surfaces.md.
package surfaces

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/confirm"
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/input"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/surface"
	"github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/toggle"
)

// Ruled is a custom surface: a rule along the top carrying the title and one
// along the bottom carrying the slots, no sides, no fill. Every surface needs
// both text rows, so a rule on each is the least a surface can draw.
var Ruled = func() surface.Surface {
	rule := surface.Side{
		Cell: surface.Cell{Glyph: "─", FG: surface.Edge, FGFocused: surface.EdgeActive, BG: surface.Parent},
		Text: true,
	}
	pad := surface.Side{Cell: surface.Cell{Glyph: " ", BG: surface.Parent}}
	return surface.Surface{
		Name: "ruled",
		Rings: []surface.Ring{
			{Top: rule, Bottom: rule},
			{Left: pad, Right: pad},
		},
		Title: surface.Text, TitleFocused: surface.Accent, TitleBold: true,
		Slot:      surface.TextMuted,
		Divider:   surface.DividerStyle{Glyph: "┄", FG: surface.Divider, FGActive: surface.EdgeActive, BG: surface.Parent},
		Selection: surface.SelectionStyle{BG: surface.Selection, BGBlurred: surface.SelectionDim, FG: surface.Text},
		Bar:       surface.Screen,
		Needs:     surface.Needs{Profile: termenv.ANSI},
	}
}()

// Lipped is Filled with thin eighth-block lips instead of half rows: the same
// cell cost, a sharper edge. It starts from the built-in and changes what
// differs — copying the rings first, since they are shared with Filled.
var Lipped = func() surface.Surface {
	s := surface.Filled
	s.Name = "lipped"
	s.Rings = append([]surface.Ring(nil), surface.Filled.Rings...)
	s.Rings[1].Top.Glyph, s.Rings[1].Bottom.Glyph = "▁", "▔"
	return s
}()

// Surfaces is what the picker offers: the built-ins, two Outlined variants,
// and the two custom surfaces. The launcher passes the same list as
// app.Options.Surfaces, so every name the picker sends resolves.
func Surfaces() []surface.Surface {
	rounded := surface.OutlinedWith(lipgloss.RoundedBorder())
	rounded.Name = "outlined-rounded"
	thick := surface.OutlinedWith(lipgloss.ThickBorder())
	thick.Name = "outlined-thick"
	return append(surface.Builtins(), rounded, thick, Ruled, Lipped)
}

// glyphSets pairs a label with a glyph vocabulary. The zero Set resolves to
// glyph.Default, which is why "Default" carries no fields. Glyphs are the one
// part of a theme this screen overrides for itself.
var glyphSets = []struct {
	name string
	set  glyph.Set
}{
	{"Default", glyph.Set{}},
	{"ASCII", glyph.Set{
		Cursor: ">", Mark: "*", ExpandOpen: "v", ExpandClosed: ">",
		ScrollThumb: "#", ScrollTrack: ".",
		SortAsc: "^", SortDesc: "v", ColumnSep: "|", Placeholder: ".",
	}},
	{"Heavy", glyph.Set{
		Cursor: "➤", Mark: "✔", ExpandOpen: "▼", ExpandClosed: "▶",
		ScrollThumb: "▓", ScrollTrack: "▒",
	}},
}

type Screen struct {
	base   theme.Theme
	glyphs int

	surfPick  list.Model
	glyphPick list.Model

	rows   list.Model
	field  input.Model
	choice toggle.Model
	pinned textview.Model
	modal  confirm.Model
	// modalUp is whether the overlay preview is open. The toggle opens it;
	// the modal's own y/n/esc closes it.
	modalUp bool

	focus focus.Group
}

func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	s.surfPick.SetCursor(indexOf(t.Surface.Surface().Name))
	return s
}

func indexOf(name string) int {
	for i, s := range Surfaces() {
		if s.Name == name {
			return i
		}
	}
	return 0
}

func (s *Screen) Title() string       { return "Surfaces" }
func (s *Screen) OnEnter(any) tea.Cmd { return nil }

func (s *Screen) Init() tea.Cmd { return s.focus.Init() }

// IsCapturingKeys holds the keys while the overlay is up, so esc closes it
// rather than leaving the screen (rule 22).
func (s *Screen) IsCapturingKeys() bool { return s.modalUp || s.focus.IsCapturingKeys() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	// While the overlay is up it gets every message and nothing else does
	// (rule 22); its answer closes it and puts the toggle back.
	if s.modalUp {
		switch msg.(type) {
		case confirm.ConfirmedMsg, confirm.CancelledMsg:
			s.modalUp = false
			s.choice.SetValue(false)
			return s, nil
		}
		var cmd tea.Cmd
		s.modal, cmd = s.modal.Update(msg)
		return s, cmd
	}

	surfBefore, glyphsBefore := s.surfPick.Cursor(), s.glyphs
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

	// Live preview: the surface is the app's, so a cursor move asks the
	// shell to switch, and the shell's SetTheme rebuilds this screen in it.
	if c := s.surfPick.Cursor(); c != surfBefore {
		cmds = append(cmds, app.SetSurface(Surfaces()[c].Name))
	}
	if s.glyphs = s.glyphPick.Cursor(); s.glyphs != glyphsBefore {
		s.rebuild()
	}
	if s.choice.Value() && !s.modalUp {
		s.modalUp = true
	}
	return s, tea.Batch(cmds...)
}

// forwardAll hands a message to every component. Mouse only: each one tests
// the click against its own rect and the one it landed in acts.
func (s *Screen) forwardAll(msg tea.Msg) []tea.Cmd {
	var cmds []tea.Cmd
	var c tea.Cmd
	s.surfPick, c = s.surfPick.Update(msg)
	cmds = append(cmds, c)
	s.glyphPick, c = s.glyphPick.Update(msg)
	cmds = append(cmds, c)
	s.rows, c = s.rows.Update(msg)
	cmds = append(cmds, c)
	s.field, c = s.field.Update(msg)
	cmds = append(cmds, c)
	s.choice, c = s.choice.Update(msg)
	cmds = append(cmds, c)
	s.pinned, c = s.pinned.Update(msg)
	return append(cmds, c)
}

// forwardFocused hands a message to the focused component alone, so typing in
// the input does not also drive a list cursor.
func (s *Screen) forwardFocused(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch {
	case s.focus.Is(&s.surfPick):
		s.surfPick, cmd = s.surfPick.Update(msg)
	case s.focus.Is(&s.glyphPick):
		s.glyphPick, cmd = s.glyphPick.Update(msg)
	case s.focus.Is(&s.rows):
		s.rows, cmd = s.rows.Update(msg)
	case s.focus.Is(&s.field):
		s.field, cmd = s.field.Update(msg)
	case s.focus.Is(&s.choice):
		s.choice, cmd = s.choice.Update(msg)
	case s.focus.Is(&s.pinned):
		s.pinned, cmd = s.pinned.Update(msg)
	}
	return cmd
}

func (s *Screen) Layout() layout.Node {
	var overlay layout.Node
	if s.modalUp {
		overlay = layout.Center(42, 9, layout.Sized(&s.modal))
	}
	return layout.HStack(
		layout.Fixed(26, layout.VStack(
			layout.Flex(2, layout.Sized(&s.surfPick)),
			layout.Flex(1, layout.Sized(&s.glyphPick)),
		)),
		layout.Flex(1, layout.ZStack(
			layout.VStack(
				layout.Flex(1, layout.HStack(
					layout.Flex(1, layout.Sized(&s.rows)),
					layout.Flex(1, layout.Sized(&s.pinned)),
				)),
				layout.Fixed(3, layout.Sized(&s.field)),
				layout.Fixed(3, layout.Sized(&s.choice)),
			),
			overlay,
		)),
	)
}

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

func (s *Screen) HelpSections() []help.Section {
	if s.modalUp {
		return help.SectionsOf(&s.modal)
	}
	return help.SectionsOf(&s.focus, help.Group("Surfaces",
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	))
}

// SetTheme takes the app's theme — its palette and its surface — and
// rebuilds from it, keeping the glyph choice made here.
func (s *Screen) SetTheme(t theme.Theme) {
	s.base = t
	s.rebuild()
}

// rebuild constructs every component from the theme, carrying state across
// as in any theme swap (rule 4).
func (s *Screen) rebuild() {
	t := s.base
	t.Glyphs = glyphSets[s.glyphs].set
	idx := s.focus.Index()

	st := s.surfPick.State()
	o := t.List()
	o.Title = "surface" + drawnNote(t)
	o.Items = surfaceNames()
	s.surfPick = list.New(o)
	s.surfPick.Restore(st)

	st = s.glyphPick.State()
	o = t.List()
	o.Title = "glyphs"
	for _, g := range glyphSets {
		o.Items = append(o.Items, g.name)
	}
	s.glyphPick = list.New(o)
	s.glyphPick.Restore(st)

	st = s.rows.State()
	o = t.List()
	o.Title = "a list"
	o.Filterable, o.Markable = true, true
	s.rows = list.New(o)
	if s.rows.Restore(st); len(s.rows.Items()) == 0 {
		s.rows.SetKeyedItems(previewRows())
		s.rows.SetMarks([]string{"worker-pool"})
	}

	pst := s.pinned.State()
	po := t.TextView()
	po.Surface = t.Resolve(surface.Outlined) // pinned: ignores the app's surface
	po.Title = "pinned: outlined"
	po.Content = "This pane sets Options.Surface itself:\n\n  t.Resolve(surface.Outlined)\n\nso it stays Outlined whatever\nthe app is drawn in."
	s.pinned = textview.New(po)
	s.pinned.Restore(pst)

	fst := s.field.State()
	fo := t.Input()
	fo.Title = "an input: field form"
	fo.Placeholder = "type here…"
	s.field = input.New(fo)
	s.field.Restore(fst)

	cst := s.choice.State()
	co := t.Toggle()
	co.Title = "show the overlay? (a toggle: field form)"
	s.choice = toggle.New(co)
	s.choice.Restore(cst)

	mo := t.Confirm()
	mo.Title = "an overlay"
	mo.Message = "Drawn in the surface's overlay form,\none step above the cards. y/n/esc."
	s.modal = confirm.New(mo)

	// The input is a focus stop like any other. It reports IsCapturingKeys
	// while focused, so tab cannot cycle off it — press esc to leave the
	// field, then tab.
	s.focus = focus.NewGroup(&s.surfPick, &s.glyphPick, &s.rows, &s.pinned, &s.field, &s.choice)
	s.focus.SetIndex(idx)
}

func surfaceNames() []string {
	var out []string
	for _, s := range Surfaces() {
		out = append(out, s.Name)
	}
	return out
}

// drawnNote names the surface actually drawn when the terminal couldn't draw
// the one chosen: Filled without block glyphs is drawn Banded. The shell's
// output console has the reason.
func drawnNote(t theme.Theme) string {
	if _, _, ok := t.Surface.FellBack(); ok {
		return " · drawn " + t.Surface.Name()
	}
	return ""
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

// Package appearance is the look showcase: palettes, surfaces, glyphs and the
// components whose subject is how they are drawn, so a change that breaks an
// older look shows up where you can see it.
//
//	Look     palette, surface and glyph pickers that redraw the whole app
//	         live (rule 35), beside what each kind of component is handed —
//	         a list (the surface), an input and a toggle (its field form), a
//	         modal (its overlay form), and a pane pinned to Outlined
//	Form     a titled pkg/form: required fields, validation on submit, a
//	         select that must be picked, and on a filled surface each field a
//	         card a step lighter than the frame, its buttons chips
//	Text     pkg/textview: a document to read, search and wrap
//	Panes    border styles, title positions and slot brackets side by side
//	Layouts  pkg/layout trees — VStack, HStack, ZStack, Center, Fixed, Flex —
//	         each pushed as its own screen (rule 2)
//
// The tabs are pkg/tab (rule 21): shift+←/→ or 1–5 switch. Each tab is its
// own package beside this one.
package appearance

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/examples/appearance/form"
	"github.com/jsdrews/tuilib/examples/appearance/layouts"
	"github.com/jsdrews/tuilib/examples/appearance/look"
	"github.com/jsdrews/tuilib/examples/appearance/pane"
	"github.com/jsdrews/tuilib/examples/appearance/textview"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/surface"
	"github.com/jsdrews/tuilib/pkg/tab"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// Palettes is what the palette picker offers. An app hosting the picker
// passes the same list as app.Options.Palettes, so every name it sends
// resolves.
func Palettes() []palette.Palette { return look.Palettes() }

// Surfaces is what the surface picker offers, two custom surfaces among them;
// pass it as app.Options.Surfaces for the same reason.
func Surfaces() []surface.Surface { return look.Surfaces() }

// New returns the appearance screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

// Screen holds each body so a theme rebuild reuses it — the swap is what the
// Look tab exists to cause, so every other tab is rebuilt in place under it.
type Screen struct {
	bodies []tab.Tab
	tabs   tab.Model
}

func (s *Screen) Title() string         { return "Appearance › " + s.tabs.ActiveLabel() }
func (s *Screen) Init() tea.Cmd         { return tea.Batch(textinput.Blink, s.tabs.Init()) }
func (s *Screen) OnEnter(r any) tea.Cmd { return s.tabs.OnEnterActive(r) }
func (s *Screen) IsCapturingKeys() bool { return s.tabs.IsCapturingKeys() }
func (s *Screen) Layout() layout.Node   { return layout.Sized(&s.tabs) }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.tabs, cmd = s.tabs.Update(msg)
	return s, cmd
}

func (s *Screen) Help() []key.Binding          { return help.Flatten(s.HelpSections()) }
func (s *Screen) HelpSections() []help.Section { return s.tabs.HelpSections() }

func (s *Screen) SetTheme(t theme.Theme) {
	if s.bodies == nil {
		s.bodies = []tab.Tab{
			{Label: "Look", Body: look.New(t)},
			{Label: "Form", Body: form.New(t)},
			{Label: "Text", Body: textview.New(t)},
			{Label: "Panes", Body: pane.New(t)},
			{Label: "Layouts", Body: layouts.New(t)},
		}
	} else {
		for _, b := range s.bodies {
			b.Body.SetTheme(t)
		}
	}
	active := s.tabs.ActiveTab() // 0 on the first call
	s.tabs = tab.New(tab.Options{Theme: t, Initial: active, Tabs: s.bodies})
}

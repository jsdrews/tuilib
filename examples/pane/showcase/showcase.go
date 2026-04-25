// Package showcase demonstrates pane border styles, title positions, and
// slot-bracket variants in a single screen. Four panes arranged 2x2, each
// with a different combination. No interaction beyond esc-to-back.
package showcase

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

type Screen struct {
	t              theme.Theme
	tl, tr, bl, br pane.Pane
}

// New constructs a showcase screen with the given initial theme.
func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string                           { return "Panes" }
func (s *Screen) Init() tea.Cmd                           { return nil }
func (s *Screen) Update(tea.Msg) (screen.Screen, tea.Cmd) { return s, nil }
func (s *Screen) OnEnter(any) tea.Cmd                     { return nil }
func (s *Screen) IsCapturingKeys() bool                   { return false }

func (s *Screen) Layout() layout.Node {
	return layout.VStack(
		layout.Flex(1, layout.HStack(
			layout.Flex(1, layout.Sized(&s.tl)),
			layout.Flex(1, layout.Sized(&s.tr)),
		)),
		layout.Flex(1, layout.HStack(
			layout.Flex(1, layout.Sized(&s.bl)),
			layout.Flex(1, layout.Sized(&s.br)),
		)),
	)
}

func (s *Screen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	s.tl = mk(t, paneSpec{
		border:   lipgloss.ThickBorder(),
		title:    "thick",
		titlePos: pane.TopMiddleBorder,
		brackets: pane.SlotBracketsCorners,
		body:     "border:   thick\ntitle:    top-middle\nbrackets: corners",
	})
	s.tr = mk(t, paneSpec{
		border:   lipgloss.RoundedBorder(),
		title:    "rounded",
		titlePos: pane.TopLeftBorder,
		brackets: pane.SlotBracketsNone,
		body:     "border:   rounded\ntitle:    top-left\nbrackets: none (default)",
	})
	s.bl = mk(t, paneSpec{
		border:   lipgloss.DoubleBorder(),
		title:    "double",
		titlePos: pane.TopRightBorder,
		brackets: pane.SlotBracketsTees,
		body:     "border:   double\ntitle:    top-right\nbrackets: tees",
	})
	s.br = mk(t, paneSpec{
		border:   lipgloss.HiddenBorder(),
		title:    "(hidden border / bottom title)",
		titlePos: pane.BottomLeftBorder,
		brackets: pane.SlotBracketsNone,
		body:     "border:   hidden\ntitle:    bottom-left\nbrackets: none",
	})
}

type paneSpec struct {
	border   lipgloss.Border
	title    string
	titlePos pane.BorderPosition
	brackets pane.SlotBracketStyle
	body     string
}

func mk(t theme.Theme, sp paneSpec) pane.Pane {
	opts := t.Pane()
	opts.Focused = true
	opts.ActiveBorder = sp.border
	opts.Title = sp.title
	opts.TitlePosition = sp.titlePos
	opts.SlotBrackets = sp.brackets
	p := pane.New(opts)
	p.SetContent(sp.body + "\n\nthe quick brown fox\njumps over the lazy dog")
	return p
}

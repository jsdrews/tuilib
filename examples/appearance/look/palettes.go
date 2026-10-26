package look

// The palette half of the picker: the palettes on offer, and the detail screen
// enter opens, which dumps every palette field with a swatch and the value
// rendered in that colour.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// Palettes is the picker's full palette set: palette.Terminal() first (read
// from the terminal at startup), then every built-in. Launchers can reuse
// this as the app-wide Palettes list so SetPalette names resolve.
func Palettes() []palette.Palette {
	return append([]palette.Palette{palette.Terminal()}, palette.All()...)
}

func paletteNames(ps []palette.Palette) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}

// ---- detail screen --------------------------------------------------------

type detailScreen struct {
	t    theme.Theme
	pick theme.Theme
	body pane.Pane
}

func newDetailScreen(picked theme.Theme) *detailScreen {
	s := &detailScreen{pick: picked}
	s.SetTheme(picked)
	return s
}

func (s *detailScreen) Title() string                           { return s.pick.Name }
func (s *detailScreen) Init() tea.Cmd                           { return nil }
func (s *detailScreen) Update(tea.Msg) (screen.Screen, tea.Cmd) { return s, nil }
func (s *detailScreen) OnEnter(any) tea.Cmd                     { return nil }
func (s *detailScreen) IsCapturingKeys() bool                   { return false }

func (s *detailScreen) Layout() layout.Node { return layout.Sized(&s.body) }

func (s *detailScreen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

// SetTheme on the detail screen ignores t and re-renders using the frozen
// picked palette — the whole point of "detail view" is showing that one.
func (s *detailScreen) SetTheme(theme.Theme) {
	opts := s.pick.Pane()
	opts.Title = s.pick.Name
	opts.TitlePosition = pane.TopMiddleBorder
	opts.Focused = true
	opts.ActiveBorder = lipgloss.NormalBorder()
	s.body = pane.New(opts)
	s.body.SetContent(renderFields(s.pick))
	s.body.SetBottomLeft("13 fields")
}

func renderFields(th theme.Theme) string {
	type field struct {
		label string
		color lipgloss.TerminalColor
	}
	fields := []field{
		{"BarBG", th.BarBG},
		{"BarFG", th.BarFG},
		{"Current", th.Current},
		{"Muted", th.Muted},
		{"Subtle", th.Subtle},
		{"KeyFG", th.KeyFG},
		{"BorderActive", th.BorderActive},
		{"BorderInactive", th.BorderInactive},
		{"InfoBG", th.InfoBG},
		{"InfoFG", th.InfoFG},
		{"ErrorBG", th.ErrorBG},
		{"ErrorFG", th.ErrorFG},
		{"Accent", th.Accent},
	}

	label := lipgloss.NewStyle().Foreground(th.Muted).Width(18)
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(th.Current)

	var b strings.Builder
	b.WriteString(label.Render("Name") + nameStyle.Render(th.Name) + "\n\n")
	for _, f := range fields {
		swatch := lipgloss.NewStyle().Background(f.color).Render("   ")
		value := lipgloss.NewStyle().Foreground(f.color).Render(colorString(f.color))
		b.WriteString(label.Render(f.label) + swatch + "  " + value + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func colorString(c lipgloss.TerminalColor) string {
	switch c := c.(type) {
	case lipgloss.Color:
		return string(c)
	case lipgloss.AdaptiveColor:
		return fmt.Sprintf("adaptive(light=%s dark=%s)", c.Light, c.Dark)
	case lipgloss.CompleteColor:
		return fmt.Sprintf("complete(%s)", c.TrueColor)
	default:
		return fmt.Sprintf("%v", c)
	}
}

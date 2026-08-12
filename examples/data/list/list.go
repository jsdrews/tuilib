// Package list demonstrates a filterable list.Model as a single-screen app.
//
// The list is deliberately long — far more rows than fit — so the scroll
// affordances are all reachable:
//
//   - Drag the scrollbar thumb on the right edge, or click anywhere on its
//     track to jump there. Every row is numbered, so the number under the
//     cursor after a drag tells you whether the jump landed where the thumb
//     says it should.
//   - Roll the wheel over the list, focused or not.
//   - g / G jump to the ends, ctrl+u / ctrl+d by half-pages.
//   - Double-click a row to open it (the mouse spelling of enter, rule 14).
//   - "/" filters; the filter is a row inside the pane rather than a
//     separate box above it.
package list

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

var cities = []string{
	"New York", "San Francisco", "Toronto", "Vancouver", "Chicago",
	"London", "Berlin", "Paris", "Madrid", "Amsterdam", "Lisbon", "Prague",
	"Tokyo", "Singapore", "Seoul", "Mumbai", "Bangkok", "Hong Kong", "Taipei",
	"Sydney", "Melbourne", "Auckland",
	"São Paulo", "Buenos Aires", "Mexico City", "Lima", "Bogotá",
	"Nairobi", "Cape Town", "Lagos", "Cairo",
}

// items builds a list long enough that the scrollbar thumb is small and a
// drag is worth testing. Each row carries its ordinal so the position after
// a jump is readable at a glance — without that, "did the drag land in the
// right place?" is guesswork.
func items() []string {
	const rounds = 12
	out := make([]string, 0, len(cities)*rounds)
	for i := 0; i < len(cities)*rounds; i++ {
		out = append(out, fmt.Sprintf("%3d · %s", i+1, cities[i%len(cities)]))
	}
	return out
}

type Screen struct {
	t    theme.Theme
	list list.Model
}

func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string         { return "Cities" }
func (s *Screen) Init() tea.Cmd         { return textinput.Blink }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.list.IsCapturingKeys() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	// Enter and double-click are one verb (rule 14), so both arrive here.
	if s.list.IsActivate(msg) {
		if item, ok := s.list.Selected(); ok {
			return s, app.Info("opened " + item)
		}
	}

	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *Screen) Layout() layout.Node { return layout.Sized(&s.list) }

func (s *Screen) Help() []key.Binding {
	return append(s.list.Help(),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		// Mouse affordances carry sentinel keys so they flow through
		// help.Compile without colliding and never match a real KeyMsg.
		key.NewBinding(key.WithKeys("mouse:drag"), key.WithHelp("drag bar", "scroll")),
		key.NewBinding(key.WithKeys("mouse:wheel"), key.WithHelp("wheel", "scroll")),
		key.NewBinding(key.WithKeys("mouse:dblclick"), key.WithHelp("2×click", "open")),
	)
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	cursor, value := s.list.Cursor(), s.list.Value()
	opts := t.List()
	opts.Title = "Cities"
	opts.Filterable = true
	opts.Filter.Placeholder = "filter cities…"
	opts.Items = items()
	s.list = list.New(opts)
	if value != "" {
		s.list.SetValue(value)
	}
	s.list.SetCursor(cursor)
}

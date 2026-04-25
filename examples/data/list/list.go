// Package list demonstrates a filterable list.Model as a single-screen app.
package list

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

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

type Screen struct {
	t    theme.Theme
	list list.Model
}

func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string          { return "Cities" }
func (s *Screen) Init() tea.Cmd          { return textinput.Blink }
func (s *Screen) OnEnter(any) tea.Cmd    { return nil }
func (s *Screen) IsCapturingKeys() bool  { return s.list.Filtering() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *Screen) Layout() layout.Node { return layout.Sized(&s.list) }

func (s *Screen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	cursor, value := s.list.Cursor(), s.list.Value()
	opts := t.List()
	opts.Title = "Cities"
	opts.Filterable = true
	opts.Filter.Placeholder = "filter cities…"
	opts.Items = cities
	s.list = list.New(opts)
	if value != "" {
		s.list.SetValue(value)
	}
	s.list.SetCursor(cursor)
}

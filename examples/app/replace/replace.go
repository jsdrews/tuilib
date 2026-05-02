// Package replace demonstrates screen.Replace — atomic top-of-stack swap.
//
// Two screens, each with mutable state the user can see change:
//
//	cityList: a filterable list of cities. The filter and cursor are state.
//	          Press r → Replace with a fresh cityList. Filter clears, cursor
//	          jumps to top, breadcrumb depth stays at 1 (no flicker).
//
//	cityDetail: pushed via enter on the list. Holds a visit counter that
//	            bumps on `+`. Press r → Replace with a fresh cityDetail for
//	            the same city. Counter resets to 0; the cityList beneath
//	            stays put — its OnEnter does NOT fire (which is the whole
//	            point: pop+push would fire it on the parent).
//
// Compare against pop+push, which would (a) flicker visibly between frames
// and (b) call OnEnter on the parent that briefly becomes the top, often
// retriggering side effects (refetch, scroll-to-cursor, …).
package replace

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the demo's root screen.
func New(t theme.Theme) screen.Screen { return newCityList(t) }

var cities = []string{
	"London", "Paris", "Berlin", "Madrid", "Amsterdam",
	"Tokyo", "Singapore", "Seoul", "Bangkok", "Mumbai",
	"New York", "San Francisco", "Toronto", "Vancouver",
	"Sydney", "Melbourne", "São Paulo", "Buenos Aires",
	"Mexico City", "Nairobi", "Cape Town", "Cairo",
}

// ---- cityList (root) ------------------------------------------------------

type cityList struct {
	t    theme.Theme
	list list.Model
	hint pane.Pane
}

func newCityList(t theme.Theme) *cityList {
	s := &cityList{}
	s.SetTheme(t)
	return s
}

func (s *cityList) Title() string         { return "Cities" }
func (s *cityList) Init() tea.Cmd         { return textinput.Blink }
func (s *cityList) OnEnter(any) tea.Cmd   { return nil }
func (s *cityList) IsCapturingKeys() bool { return s.list.Filtering() }

func (s *cityList) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && !s.list.Filtering() {
		switch k.String() {
		case "enter":
			if city, ok := s.list.Selected(); ok {
				return s, screen.Push(newCityDetail(city, s.t))
			}
		case "r":
			return s, screen.Replace(newCityList(s.t))
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *cityList) Layout() layout.Node {
	return layout.VStack(
		layout.Fixed(5, layout.Sized(&s.hint)),
		layout.Flex(1, layout.Sized(&s.list)),
	)
}

func (s *cityList) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reset (Replace)")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

func (s *cityList) SetTheme(t theme.Theme) {
	s.t = t

	s.hint = pane.New(t.Pane())
	s.hint.SetTitle("replace")
	s.hint.SetContent(
		"Filter or move the cursor, then press r — the screen swaps for a\n" +
			"fresh instance. Breadcrumb depth stays at 1: it's a Replace, not\n" +
			"a pop+push.",
	)

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

// ---- cityDetail (pushed) --------------------------------------------------

type cityDetail struct {
	t      theme.Theme
	city   string
	visits int
	info   pane.Pane
}

func newCityDetail(city string, t theme.Theme) *cityDetail {
	s := &cityDetail{city: city}
	s.SetTheme(t)
	return s
}

func (s *cityDetail) Title() string         { return s.city }
func (s *cityDetail) Init() tea.Cmd         { return nil }
func (s *cityDetail) OnEnter(any) tea.Cmd   { return nil }
func (s *cityDetail) IsCapturingKeys() bool { return false }

func (s *cityDetail) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "+", "=":
			s.visits++
			s.rebuildInfo()
		case "r":
			return s, screen.Replace(newCityDetail(s.city, s.t))
		}
	}
	return s, nil
}

func (s *cityDetail) Layout() layout.Node { return layout.Sized(&s.info) }

func (s *cityDetail) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("+"), key.WithHelp("+", "visit")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reset (Replace)")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *cityDetail) SetTheme(t theme.Theme) {
	s.t = t
	s.rebuildInfo()
}

func (s *cityDetail) rebuildInfo() {
	s.info = pane.New(s.t.Pane())
	s.info.SetTitle("city")
	s.info.SetContent(fmt.Sprintf(
		"City:    %s\nVisits:  %d\n\n"+
			"+/= bumps the visit counter (this screen's local state).\n"+
			"r calls screen.Replace(newCityDetail(city, theme)) — the\n"+
			"counter resets to 0, but the cityList beneath this screen\n"+
			"is untouched: its OnEnter does not fire. A pop+push would\n"+
			"flicker AND retrigger the parent's OnEnter side effects.",
		s.city, s.visits,
	))
}

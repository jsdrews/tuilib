// Package stack demonstrates a screen stack with data flowing in two
// directions:
//
//	Parent → child: via the constructor. cityList pushes newCityDetail(city).
//	Child → parent: via Pop(result). timezonePicker calls Pop(chosenTZ),
//	                which lands in cityDetail.OnEnter(chosenTZ).
//
// Each screen uses a different layout to show that the stack doesn't care
// what its children look like — it just hosts layout.Node trees.
package stack

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the stack demo's root screen.
func New(t theme.Theme) screen.Screen {
	s := &cityList{}
	s.SetTheme(t)
	return s
}

var cities = []string{
	"London", "Paris", "Berlin", "Madrid", "Amsterdam",
	"Tokyo", "Singapore", "Seoul", "Bangkok", "Mumbai",
	"New York", "San Francisco", "Toronto", "Vancouver",
	"Sydney", "Melbourne",
	"São Paulo", "Buenos Aires", "Mexico City",
	"Nairobi", "Cape Town", "Cairo",
}

var timezones = []string{
	"UTC-12:00", "UTC-11:00", "UTC-10:00", "UTC-09:00", "UTC-08:00",
	"UTC-07:00", "UTC-06:00", "UTC-05:00", "UTC-04:00", "UTC-03:00",
	"UTC-02:00", "UTC-01:00", "UTC+00:00",
	"UTC+01:00", "UTC+02:00", "UTC+03:00", "UTC+04:00", "UTC+05:00",
	"UTC+06:00", "UTC+07:00", "UTC+08:00", "UTC+09:00", "UTC+10:00",
	"UTC+11:00", "UTC+12:00",
}

// ---- CityList (root) -------------------------------------------------------
//
// Layout: single Flex. list.Model absorbs its filter internally.

type cityList struct {
	t    theme.Theme
	list list.Model
}

func (s *cityList) Title() string          { return "Cities" }
func (s *cityList) Init() tea.Cmd          { return textinput.Blink }
func (s *cityList) OnEnter(any) tea.Cmd    { return nil }
func (s *cityList) IsCapturingKeys() bool  { return s.list.Filtering() }

func (s *cityList) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && !s.list.Filtering() && k.String() == "enter" {
		if city, ok := s.list.Selected(); ok {
			return s, screen.Push(newCityDetail(city, s.t))
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *cityList) Layout() layout.Node { return layout.Sized(&s.list) }

func (s *cityList) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

func (s *cityList) SetTheme(t theme.Theme) {
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

// ---- CityDetail ------------------------------------------------------------
//
// Layout: HStack(Flex(1, info), Flex(1, actions)). The actions pane is a
// list; enter pushes whatever the selected action demands.

type cityDetail struct {
	t       theme.Theme
	city    string
	chosen  string // last-picked timezone (or "")
	info    pane.Pane
	actions list.Model
}

const actionPickTZ = "Pick timezone"

func newCityDetail(city string, t theme.Theme) *cityDetail {
	s := &cityDetail{city: city}
	s.SetTheme(t)
	return s
}

func (s *cityDetail) Title() string         { return s.city }
func (s *cityDetail) Init() tea.Cmd         { return nil }
func (s *cityDetail) IsCapturingKeys() bool { return s.actions.Filtering() }

func (s *cityDetail) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && !s.actions.Filtering() && k.String() == "enter" {
		if action, ok := s.actions.Selected(); ok {
			switch action {
			case actionPickTZ:
				return s, screen.Push(newTimezonePicker(s.t))
			}
		}
	}
	var cmd tea.Cmd
	s.actions, cmd = s.actions.Update(msg)
	return s, cmd
}

func (s *cityDetail) Layout() layout.Node {
	return layout.HStack(
		layout.Flex(1, layout.Sized(&s.info)),
		layout.Flex(1, layout.Sized(&s.actions)),
	)
}

func (s *cityDetail) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "run")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

// OnEnter receives the timezone when TimezonePicker pops with a value.
// On initial push, result is nil — we just render with the empty chosen tz.
func (s *cityDetail) OnEnter(result any) tea.Cmd {
	if tz, ok := result.(string); ok && tz != "" {
		s.chosen = tz
		s.rebuildInfo()
	}
	return nil
}

func (s *cityDetail) SetTheme(t theme.Theme) {
	s.t = t
	s.rebuildInfo()

	cursor := s.actions.Cursor()
	opts := t.List()
	opts.Title = "actions"
	opts.Items = []string{actionPickTZ}
	s.actions = list.New(opts)
	s.actions.SetCursor(cursor)
}

// rebuildInfo constructs the info pane from the current (theme, city,
// chosen) state. Called from SetTheme (theme swap) and OnEnter (new tz).
func (s *cityDetail) rebuildInfo() {
	s.info = pane.New(s.t.Pane())
	s.info.SetTitle("city")
	tz := s.chosen
	if tz == "" {
		tz = "— (run 'Pick timezone')"
	}
	s.info.SetContent(strings.Join([]string{
		"Name:       " + s.city,
		"Timezone:   " + tz,
		"",
		"The city name was passed into this screen via its",
		"constructor (NewCityDetail(city)). The timezone",
		"arrived via OnEnter after the picker screen popped",
		"with the selected value.",
	}, "\n"))
}

// ---- TimezonePicker -------------------------------------------------------
//
// Layout: VStack with a hint pane on top and a filterable list below.

type timezonePicker struct {
	t    theme.Theme
	hint pane.Pane
	list list.Model
}

func newTimezonePicker(t theme.Theme) *timezonePicker {
	s := &timezonePicker{}
	s.SetTheme(t)
	return s
}

func (s *timezonePicker) Title() string          { return "Timezone" }
func (s *timezonePicker) Init() tea.Cmd          { return textinput.Blink }
func (s *timezonePicker) OnEnter(any) tea.Cmd    { return nil }
func (s *timezonePicker) IsCapturingKeys() bool  { return s.list.Filtering() }

func (s *timezonePicker) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && !s.list.Filtering() && k.String() == "enter" {
		if tz, ok := s.list.Selected(); ok {
			return s, screen.Pop(tz) // child → parent data flow
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *timezonePicker) Layout() layout.Node {
	return layout.VStack(
		layout.Fixed(5, layout.Sized(&s.hint)),
		layout.Flex(1, layout.Sized(&s.list)),
	)
}

func (s *timezonePicker) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "pick")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *timezonePicker) SetTheme(t theme.Theme) {
	s.t = t
	cursor, value := s.list.Cursor(), s.list.Value()

	s.hint = pane.New(t.Pane())
	s.hint.SetTitle("picker")
	s.hint.SetContent("Enter commits the selection and pops back to the\n" +
		"city detail, passing the timezone up via screen.Pop(tz).")

	opts := t.List()
	opts.Title = "Timezones"
	opts.Filterable = true
	opts.Filter.Placeholder = "filter timezones…"
	opts.Items = timezones
	s.list = list.New(opts)
	if value != "" {
		s.list.SetValue(value)
	}
	s.list.SetCursor(cursor)
}


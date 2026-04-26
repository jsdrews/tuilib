// Package drilldown extends the master-detail pattern from
// examples/data/detail with a third level: pressing enter on a focused
// attribute pushes a new screen that describes it. Esc pops back to
// the parent with all of its state intact (focus index, fetched
// detail, cursor position) — that's free because screen.Stack keeps
// parent screens alive while a child is on top.
//
// Enter has a single conceptual meaning across the whole screen — "open
// the focused selection" — and a different side effect per pane:
//
//   - Focus on the cities list: enter loads that city's attributes
//     into the detail list and moves focus to the right.
//   - Focus on the detail list: enter pushes a child screen for the
//     highlighted attribute.
//
// Tab/shift-tab cycles focus explicitly. The cities list itself loads
// on Init via tea.Tick; detail fetches are tagged with a request ID so
// stale results are dropped on arrival.
package drilldown

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the drilldown demo's root screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t      theme.Theme
	cities list.Model
	detail list.Model

	focus int // 0=cities, 1=detail

	reqID int    // monotonic; only the latest detail fetch's result is applied
	shown string // city currently rendered in the detail list (or "")
	attrs []attribute
}

const focusCount = 2

type attribute struct {
	key   string
	value string
	body  string // expanded paragraph shown on the level-3 screen
}

type citiesFetchedMsg struct{ items []string }

type detailFetchedMsg struct {
	id    int
	city  string
	attrs []attribute
}

func (s *Screen) Title() string       { return "Drilldown" }
func (s *Screen) Init() tea.Cmd       { return tea.Batch(textinput.Blink, s.startFetches()) }
func (s *Screen) OnEnter(any) tea.Cmd { return nil }

// IsCapturingKeys claims keys whenever the focused list is filter-typing.
// Detail isn't filterable, so only the cities filter matters.
func (s *Screen) IsCapturingKeys() bool { return s.focus == 0 && s.cities.Filtering() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case citiesFetchedMsg:
		s.cities.SetItems(m.items)
		s.cities.SetLoading(false)
		return s, nil

	case detailFetchedMsg:
		if m.id != s.reqID {
			return s, nil // stale — a newer enter has been pressed
		}
		s.shown = m.city
		s.attrs = m.attrs
		s.detail.SetTitle("detail · " + m.city)
		s.detail.SetItems(attrRows(m.attrs))
		s.detail.SetLoading(false)
		return s, nil

	case tea.KeyMsg:
		if !s.IsCapturingKeys() {
			switch m.String() {
			case "tab", "shift+tab":
				s.focus = 1 - s.focus
				s.applyFocus()
				return s, nil
			case "r":
				return s, s.startFetches()
			case "enter":
				return s, s.handleEnter()
			}
		}
		return s, s.routeKey(msg)
	}

	// Spinner ticks, resize, etc. — fan out to both components.
	var cc, dc tea.Cmd
	s.cities, cc = s.cities.Update(msg)
	s.detail, dc = s.detail.Update(msg)
	return s, tea.Batch(cc, dc)
}

// handleEnter drills in on the focused selection. Both branches mean
// "open the focused thing" — the side effect just differs by pane.
func (s *Screen) handleEnter() tea.Cmd {
	switch s.focus {
	case 0: // cities → load detail + move focus right
		city, ok := s.cities.Selected()
		if !ok || s.cities.Loading() {
			return nil
		}
		s.focus = 1
		s.applyFocus()
		if city == s.shown {
			return nil // already loaded; just transferred focus
		}
		return s.fetchDetail(city)

	case 1: // detail → push the attribute screen
		if s.shown == "" || s.detail.Loading() {
			return nil
		}
		idx := s.detail.Cursor()
		if idx < 0 || idx >= len(s.attrs) {
			return nil
		}
		return screen.Push(newAttrScreen(s.shown, s.attrs[idx], s.t))
	}
	return nil
}

func (s *Screen) routeKey(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch s.focus {
	case 0:
		s.cities, cmd = s.cities.Update(msg)
	case 1:
		s.detail, cmd = s.detail.Update(msg)
	}
	return cmd
}

func (s *Screen) Layout() layout.Node {
	return layout.HStack(
		layout.Flex(2, layout.Sized(&s.cities)),
		layout.Flex(3, layout.Sized(&s.detail)),
	)
}

func (s *Screen) Help() []key.Binding {
	base := []key.Binding{
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "next pane")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refetch")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
	switch s.focus {
	case 0:
		return append(base, s.cities.Help()...)
	case 1:
		return append(base, s.detail.Help()...)
	}
	return base
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	// cities — preserve cursor/value/items across theme rebuilds.
	ccursor, cvalue := s.cities.Cursor(), s.cities.Value()
	citems := s.cities.Items()
	copts := t.List()
	copts.Title = "cities"
	copts.Filterable = true
	copts.Filter.Placeholder = "filter cities…"
	copts.LoadingLabel = "loading cities…"
	copts.Items = citems
	s.cities = list.New(copts)
	if cvalue != "" {
		s.cities.SetValue(cvalue)
	}
	s.cities.SetCursor(ccursor)

	// detail — six attribute rows when shown, otherwise empty.
	dcursor := s.detail.Cursor()
	dopts := t.List()
	dopts.Title = "detail"
	if s.shown != "" {
		dopts.Title = "detail · " + s.shown
	}
	dopts.LoadingLabel = "fetching…"
	dopts.Items = attrRows(s.attrs)
	s.detail = list.New(dopts)
	s.detail.SetCursor(dcursor)

	s.applyFocus()
}

func (s *Screen) applyFocus() {
	s.cities.SetFocused(s.focus == 0)
	s.detail.SetFocused(s.focus == 1)
}

// startFetches resets the cities list into loading state and schedules
// the initial cities fetch. The detail pane sits idle (empty, no
// spinner) until the user picks a city — spinning before there's
// anything to fetch would be misleading.
func (s *Screen) startFetches() tea.Cmd {
	s.shown = ""
	s.attrs = nil
	s.reqID++ // invalidate any in-flight detail fetch
	s.cities.SetItems(nil)
	s.detail.SetItems(nil)
	s.detail.SetTitle("detail")
	s.detail.SetLoading(false)
	startCities := s.cities.SetLoading(true)
	return tea.Batch(startCities, fetchCities())
}

func (s *Screen) fetchDetail(city string) tea.Cmd {
	s.reqID++
	id := s.reqID
	s.detail.SetItems(nil)
	s.detail.SetTitle("detail · " + city)
	startSpinner := s.detail.SetLoading(true)
	delay := fakeLatency(city)
	return tea.Batch(
		startSpinner,
		tea.Tick(delay, func(time.Time) tea.Msg {
			return detailFetchedMsg{id: id, city: city, attrs: attributesFor(city)}
		}),
	)
}

func fetchCities() tea.Cmd {
	return tea.Tick(900*time.Millisecond, func(time.Time) tea.Msg {
		return citiesFetchedMsg{items: cityNames()}
	})
}

// attrRows formats attributes as "key   value" rows for the detail list.
func attrRows(attrs []attribute) []string {
	out := make([]string, len(attrs))
	for i, a := range attrs {
		out[i] = fmt.Sprintf("%-12s %s", a.key, a.value)
	}
	return out
}

// ---- level-3 attribute screen --------------------------------------------

type attrScreen struct {
	t    theme.Theme
	body pane.Pane
	city string
	attr attribute
}

func newAttrScreen(city string, attr attribute, t theme.Theme) screen.Screen {
	s := &attrScreen{city: city, attr: attr}
	s.SetTheme(t)
	return s
}

func (s *attrScreen) Title() string         { return s.city + " · " + s.attr.key }
func (s *attrScreen) Init() tea.Cmd         { return nil }
func (s *attrScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *attrScreen) IsCapturingKeys() bool { return false } // shell handles esc-pop

func (s *attrScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.body, cmd = s.body.Update(msg)
	return s, cmd
}

func (s *attrScreen) Layout() layout.Node { return layout.Sized(&s.body) }

func (s *attrScreen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *attrScreen) SetTheme(t theme.Theme) {
	s.t = t
	opts := t.Pane()
	s.body = pane.New(opts)
	s.body.SetTitle(s.city + " · " + s.attr.key)
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", s.attr.value)
	b.WriteString(s.attr.body)
	s.body.SetContent(b.String())
}

// ---- synthetic data ------------------------------------------------------

type cityInfo struct {
	country    string
	timezone   string
	population int
	currency   string
	languages  []string
	founded    int
	notes      string
	latencyMs  int
}

var cities = map[string]cityInfo{
	"New York":      {"United States", "America/New_York", 8336000, "USD", []string{"English"}, 1624, "Five boroughs; financial capital; the subway runs 24/7.", 600},
	"San Francisco": {"United States", "America/Los_Angeles", 815000, "USD", []string{"English"}, 1776, "Built on 50+ hills; Golden Gate Bridge spans the strait to Marin.", 450},
	"Toronto":       {"Canada", "America/Toronto", 2930000, "CAD", []string{"English", "French"}, 1793, "Most multicultural city in the world by some measures.", 800},
	"Vancouver":     {"Canada", "America/Vancouver", 675000, "CAD", []string{"English"}, 1886, "Coastal seaport bordered by the Pacific and the Coast Mountains.", 700},
	"London":        {"United Kingdom", "Europe/London", 8982000, "GBP", []string{"English"}, 47, "Founded by the Romans as Londinium; oldest underground rail network.", 1100},
	"Berlin":        {"Germany", "Europe/Berlin", 3645000, "EUR", []string{"German"}, 1237, "Wall fell 1989; reunified capital since 1990.", 1300},
	"Paris":         {"France", "Europe/Paris", 2161000, "EUR", []string{"French"}, -250, "Lutetia under the Romans; Eiffel Tower built for the 1889 World's Fair.", 1000},
	"Madrid":        {"Spain", "Europe/Madrid", 3266000, "EUR", []string{"Spanish"}, 865, "Highest capital city in Europe at 667m elevation.", 1400},
	"Tokyo":         {"Japan", "Asia/Tokyo", 13960000, "JPY", []string{"Japanese"}, 1457, "World's largest metropolitan area by population.", 1700},
	"Singapore":     {"Singapore", "Asia/Singapore", 5454000, "SGD", []string{"English", "Mandarin", "Malay", "Tamil"}, 1819, "City-state; one of the four official languages is on every street sign.", 900},
	"Seoul":         {"South Korea", "Asia/Seoul", 9776000, "KRW", []string{"Korean"}, -18, "One of the world's most-wired cities; 99% broadband penetration.", 1200},
	"Sydney":        {"Australia", "Australia/Sydney", 5312000, "AUD", []string{"English"}, 1788, "Largest city in Oceania; the harbour bridge opened 1932.", 1900},
}

func cityNames() []string {
	return []string{
		"New York", "San Francisco", "Toronto", "Vancouver",
		"London", "Berlin", "Paris", "Madrid",
		"Tokyo", "Singapore", "Seoul", "Sydney",
	}
}

func attributesFor(city string) []attribute {
	c, ok := cities[city]
	if !ok {
		return nil
	}
	founded := fmt.Sprintf("%d CE", c.founded)
	if c.founded < 0 {
		founded = fmt.Sprintf("%d BCE", -c.founded)
	}
	return []attribute{
		{
			key:   "country",
			value: c.country,
			body:  fmt.Sprintf("%s is in %s. The country sets the legal framework, currency, and most cultural touchpoints — but the city itself often has its own civic identity that diverges from the national one.\n\n%s", city, c.country, c.notes),
		},
		{
			key:   "timezone",
			value: c.timezone,
			body:  fmt.Sprintf("IANA tz database key: %s. UTC offset varies seasonally where DST applies; meeting schedulers and log timestamps should round-trip through this string rather than a fixed offset, because the offset itself can change without the zone being renamed.", c.timezone),
		},
		{
			key:   "population",
			value: commaInt(c.population),
			body:  fmt.Sprintf("Approximately %s people live in the %s metro area. Population fluctuates with migration, employment cycles, and birth/death rates; cited counts can vary by 10%% or more depending on whether the figure is the city proper, the metro area, or the urban agglomeration.", commaInt(c.population), city),
		},
		{
			key:   "currency",
			value: c.currency,
			body:  fmt.Sprintf("Local trade clears in %s. Exchange rates against major currencies affect tourism flows, import costs, and the local cost of living for residents paid in foreign currencies. Most card terminals accept tap-to-pay and contactless EMV.", c.currency),
		},
		{
			key:   "languages",
			value: strings.Join(c.languages, ", "),
			body:  fmt.Sprintf("Languages in active civic use: %s. Multilingual cities accommodate signage, public services, and education in each official language; everyday workplace use may be narrower, often defaulting to one lingua franca regardless of how many are technically official.", strings.Join(c.languages, ", ")),
		},
		{
			key:   "founded",
			value: founded,
			body:  fmt.Sprintf("First settled %s. The original site has typically shifted, expanded, and been rebuilt multiple times since — \"founding\" dates often refer to the first recognized civic charter rather than the first human settlement, which may predate it by centuries.", founded),
		},
	}
}

func fakeLatency(city string) time.Duration {
	if c, ok := cities[city]; ok {
		return time.Duration(c.latencyMs) * time.Millisecond
	}
	return 800 * time.Millisecond
}

func commaInt(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	out := make([]byte, 0, len(s)+len(s)/3)
	pre := len(s) % 3
	if pre > 0 {
		out = append(out, s[:pre]...)
		if len(s) > pre {
			out = append(out, ',')
		}
	}
	for i := pre; i < len(s); i += 3 {
		out = append(out, s[i:i+3]...)
		if i+3 < len(s) {
			out = append(out, ',')
		}
	}
	return string(out)
}

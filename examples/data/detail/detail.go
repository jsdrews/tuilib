// Package detail demonstrates a master-detail screen with async fetches
// at both levels:
//
//   - The cities list itself is fetched on Init (SetLoading(true) + a
//     tea.Tick that delivers the city slice). Until it resolves the
//     left pane spins; the right pane sits idle with a "select a city"
//     prompt.
//   - Pressing enter on a city fires another async fetch. While the
//     detail is in flight the right pane shows a centered spinner;
//     when it resolves, the body is rendered and the spinner is
//     dismissed.
//
// Each detail fetch is tagged with a monotonically increasing request
// ID. The screen ignores any result whose ID isn't the most recent one
// — that way, hammering enter on different cities doesn't race the
// slowest fetch into the pane after a newer selection has been made.
// This is the standard "stale-response cancellation" pattern in Bubble
// Tea: you can't stop the in-flight tea.Cmd, so you discard its result
// on arrival instead.
package detail

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

const idlePrompt = "Pick a city on the left and press enter to load its details."

// New returns the master-detail demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t      theme.Theme
	list   list.Model
	detail pane.Pane

	// reqID counts detail fetches issued; only the result whose ID
	// matches the latest is applied. Older results are dropped.
	reqID int
	// shown is the city currently rendered in the detail pane (or "" if
	// nothing has been loaded yet). Used to skip re-fetching when enter
	// is pressed on the same row twice.
	shown string
}

type citiesFetchedMsg struct{ items []string }

type detailFetchedMsg struct {
	id   int
	city string
	body string
}

func (s *Screen) Title() string         { return "Detail" }
func (s *Screen) Init() tea.Cmd         { return tea.Batch(textinput.Blink, s.startFetches()) }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.list.Filtering() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case citiesFetchedMsg:
		s.list.SetItems(m.items)
		s.list.SetLoading(false)
		return s, nil

	case detailFetchedMsg:
		if m.id != s.reqID {
			return s, nil // stale — a newer enter has been pressed
		}
		s.shown = m.city
		s.detail.SetTitle("detail · " + m.city)
		s.detail.SetContent(m.body)
		s.detail.SetLoading(false)
		return s, nil

	case tea.KeyMsg:
		// "r" refetches both panes; only outside filter mode so it
		// doesn't steal "r" keystrokes from the search input.
		if !s.list.Filtering() && m.String() == "r" {
			return s, s.startFetches()
		}
		// Enter on a populated, non-filtering list triggers a detail
		// fetch for the cursor's city. Forward to the list anyway so
		// the list also gets a chance to act on enter (it commits a
		// pending filter).
		var cmd tea.Cmd
		s.list, cmd = s.list.Update(msg)
		if !s.list.Filtering() && !s.list.Loading() && m.String() == "enter" {
			if city, ok := s.list.Selected(); ok && city != s.shown {
				return s, tea.Batch(cmd, s.fetchDetail(city))
			}
		}
		return s, cmd
	}

	// Spinner ticks, resize, etc. — fan out to both components.
	var lc, dc tea.Cmd
	s.list, lc = s.list.Update(msg)
	s.detail, dc = s.detail.Update(msg)
	return s, tea.Batch(lc, dc)
}

func (s *Screen) Layout() layout.Node {
	return layout.HStack(
		layout.Flex(2, layout.Sized(&s.list)),
		layout.Flex(3, layout.Sized(&s.detail)),
	)
}

func (s *Screen) Help() []key.Binding {
	base := []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "load detail")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refetch")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
	return append(base, s.list.Help()...)
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	// cities list — preserve cursor/value/items across theme rebuilds.
	cursor, value := s.list.Cursor(), s.list.Value()
	items := s.list.Items()
	lopts := t.List()
	lopts.Title = "cities"
	lopts.Filterable = true
	lopts.Filter.Placeholder = "filter cities…"
	lopts.LoadingLabel = "loading cities…"
	lopts.Items = items
	s.list = list.New(lopts)
	if value != "" {
		s.list.SetValue(value)
	}
	s.list.SetCursor(cursor)

	// detail pane — reuse current title + content if a detail has been
	// loaded; otherwise show the idle prompt.
	popts := t.Pane()
	popts.LoadingLabel = "fetching…"
	s.detail = pane.New(popts)
	if s.shown != "" {
		s.detail.SetTitle("detail · " + s.shown)
		s.detail.SetContent(detailBody(s.shown))
	} else {
		s.detail.SetTitle("detail")
		s.detail.SetContent(idlePrompt)
	}
}

// startFetches resets the cities list into loading state, clears any
// previously-shown detail, and schedules the cities fetch.
func (s *Screen) startFetches() tea.Cmd {
	s.shown = ""
	s.reqID++ // invalidate any in-flight detail fetch
	s.list.SetItems(nil)
	s.detail.SetTitle("detail")
	s.detail.SetContent(idlePrompt)
	s.detail.SetLoading(false)
	startCities := s.list.SetLoading(true)
	return tea.Batch(startCities, fetchCities())
}

// fetchDetail bumps reqID, flips the detail pane into loading state,
// and schedules a delayed message that carries the body back. The
// ID-tag means a faster newer fetch racing past a slower older one
// will still win.
func (s *Screen) fetchDetail(city string) tea.Cmd {
	s.reqID++
	id := s.reqID
	s.detail.SetTitle("detail · " + city)
	s.detail.SetContent("")
	startSpinner := s.detail.SetLoading(true)
	delay := fakeLatency(city)
	return tea.Batch(
		startSpinner,
		tea.Tick(delay, func(time.Time) tea.Msg {
			return detailFetchedMsg{id: id, city: city, body: detailBody(city)}
		}),
	)
}

func fetchCities() tea.Cmd {
	return tea.Tick(900*time.Millisecond, func(time.Time) tea.Msg {
		return citiesFetchedMsg{items: cityNames()}
	})
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
	// Stable order so the cursor doesn't jump on theme swap or refetch.
	return []string{
		"New York", "San Francisco", "Toronto", "Vancouver",
		"London", "Berlin", "Paris", "Madrid",
		"Tokyo", "Singapore", "Seoul", "Sydney",
	}
}

func detailBody(city string) string {
	c, ok := cities[city]
	if !ok {
		return "no record"
	}
	founded := fmt.Sprintf("%d", c.founded)
	if c.founded < 0 {
		founded = fmt.Sprintf("%d BCE", -c.founded)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Country     %s\n", c.country)
	fmt.Fprintf(&b, "Timezone    %s\n", c.timezone)
	fmt.Fprintf(&b, "Population  %s\n", commaInt(c.population))
	fmt.Fprintf(&b, "Currency    %s\n", c.currency)
	fmt.Fprintf(&b, "Languages   %s\n", strings.Join(c.languages, ", "))
	fmt.Fprintf(&b, "Founded     %s\n", founded)
	b.WriteString("\n")
	b.WriteString(c.notes)
	return b.String()
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

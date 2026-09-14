package integration

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/demoapi"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/query"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/source"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// sourceScreen is examples/patterns/remote with one thing added: the test
// decides how slow each query is. The example uses a fixed latency, which is
// right for a demo and cannot produce the asymmetry these tests need.
type sourceScreen struct {
	tab table.Model
	src source.Model
	api *http.Client

	// slow reports how long the server should take over a given query.
	slow func(raw string) time.Duration

	// dropped counts pages Deliver refused. It is the observable that
	// distinguishes "the stale page was discarded" from "the stale page never
	// arrived", which are very different bugs.
	dropped int
	landed  int
}

func newSourceScreen(t *testing.T, slow func(string) time.Duration) *sourceScreen {
	t.Helper()
	s := &sourceScreen{
		api:  demoapi.Client(demoapi.New(demoapi.Options{Seed: 3, Apps: 800})),
		src:  source.New(source.Options{PageSize: 50}),
		slow: slow,
	}
	o := theme.Dark().Table()
	o.Title = "apps"
	o.Filterable = true
	o.FilterMode = table.FilterRemote
	o.SortMode = table.SortRemote
	o.Columns = []table.Column{
		{Title: "Name", Width: 24, Sortable: true},
		{Title: "Region", Width: 12, Sortable: true},
		{Title: "Sync", Width: 12},
		{Title: "Health", Width: 13},
	}
	s.tab = table.New(o)
	return s
}

func (s *sourceScreen) Title() string          { return "source" }
func (s *sourceScreen) Init() tea.Cmd          { return s.src.Init() }
func (s *sourceScreen) OnEnter(any) tea.Cmd    { return nil }
func (s *sourceScreen) Layout() layout.Node    { return layout.Sized(&s.tab) }
func (s *sourceScreen) IsCapturingKeys() bool  { return s.tab.IsCapturingKeys() }
func (s *sourceScreen) Help() []key.Binding    { return s.tab.Help() }
func (s *sourceScreen) SetTheme(t theme.Theme) {}

type pageMsg struct {
	page source.Page
	rows []table.Row
}

func (s *sourceScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case table.ViewportChangedMsg:
		return s, s.src.Viewport(m.FirstVisible, m.LastVisible)

	case table.QueryChangedMsg:
		s.tab.SetCursor(0)
		return s, s.src.SetQuery(m.Raw, m.Terms, m.Sort, m.Desc)

	case source.RequestMsg:
		return s, s.fetch(m.Query)

	case pageMsg:
		if !s.src.Deliver(m.page) {
			s.dropped++
			return s, nil
		}
		s.landed++
		s.tab.SetWindow(m.rows, m.page.Offset, m.page.Total)
		return s, nil
	}

	var cmd tea.Cmd
	s.tab, cmd = s.tab.Update(msg)
	return s, cmd
}

func (s *sourceScreen) fetch(q source.Query) tea.Cmd {
	u := url.Values{}
	u.Set("offset", strconv.Itoa(q.Offset))
	u.Set("limit", strconv.Itoa(q.Limit))
	if d := s.slow(q.Raw); d > 0 {
		u.Set("latency", d.String())
	}
	for _, term := range q.Terms {
		if term.Title != "" && term.Regex == nil {
			u.Set(strings.ToLower(term.Title), term.Value)
		}
	}
	client, gen := s.api, q.Gen
	return func() tea.Msg {
		resp, err := client.Get("http://demoapi/apps?" + u.Encode())
		if err != nil {
			return pageMsg{page: source.Page{Gen: gen}}
		}
		defer resp.Body.Close()
		var body struct {
			Total  int `json:"total"`
			Offset int `json:"offset"`
			Rows   []struct {
				Name, Region, Sync, Health string
			} `json:"rows"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		rows := make([]table.Row, len(body.Rows))
		for i, a := range body.Rows {
			rows[i] = table.Row{a.Name, a.Region, a.Sync, a.Health}
		}
		return pageMsg{
			page: source.Page{Gen: gen, Offset: body.Offset, Count: len(rows), Total: body.Total},
			rows: rows,
		}
	}
}

// queryFor builds the message pkg/table emits on commit. Constructed rather
// than typed into the filter because the parse is the same either way —
// query.Parse is what the table itself calls — and the interesting part of
// this test is what happens after the commit, not the commit.
func queryFor(raw string) table.QueryChangedMsg {
	cols := []string{"Name", "Region", "Sync", "Health"}
	return table.QueryChangedMsg{Raw: raw, Terms: query.Parse(raw, cols)}
}

// The race source.Deliver's generation check exists for, produced rather than
// simulated: a filter the user abandoned is answered slowly, the filter they
// actually want is answered fast, and the slow reply lands second.
//
// A screen that ignored Deliver's bool would show eu-west rows under a us-east
// filter — and only sometimes, depending on the network.
func TestStalePageIsDroppedWhenItLandsLast(t *testing.T) {
	s := newSourceScreen(t, func(raw string) time.Duration {
		if strings.Contains(raw, "eu-west") {
			return 700 * time.Millisecond
		}
		return 0
	})
	h := newHarness(t, s)
	h.pumpFor(150 * time.Millisecond) // the initial unfiltered page

	// The user types a filter, then changes their mind before it comes back.
	h.send(queryFor("region:eu-west"))
	time.Sleep(30 * time.Millisecond) // the slow request is now in flight
	h.send(queryFor("region:us-east"))

	// Long enough that the abandoned reply has definitely arrived.
	h.pumpFor(1200 * time.Millisecond)

	if s.dropped == 0 {
		// Two very different faults read the same here: the stale reply never
		// arrived (so nothing was tested) or Deliver accepted it (so the guard
		// is gone). The row assertions below distinguish them.
		t.Error("no page was refused — either the stale reply never arrived, or Deliver accepted it")
	}

	view := h.render()
	if strings.Contains(view, "eu-west") {
		t.Errorf("the abandoned filter's rows are on screen:\n%s", view)
	}
	if !strings.Contains(view, "us-east") {
		t.Errorf("the current filter's rows are not on screen:\n%s", view)
	}
}

// The same asymmetry the other way round: when the slow reply is the one the
// user still wants, it must be accepted however late it is.
func TestSlowReplyForTheCurrentQueryIsAccepted(t *testing.T) {
	s := newSourceScreen(t, func(raw string) time.Duration {
		if strings.Contains(raw, "eu-west") {
			return 500 * time.Millisecond
		}
		return 0
	})
	h := newHarness(t, s)
	h.pumpFor(150 * time.Millisecond)

	h.send(queryFor("region:eu-west"))
	h.pumpFor(1000 * time.Millisecond)

	if s.dropped != 0 {
		t.Errorf("dropped %d pages answering the live query", s.dropped)
	}
	if view := h.render(); !strings.Contains(view, "eu-west") {
		t.Errorf("the slow reply was never shown:\n%s", view)
	}
}

// Scrolling ahead of the data must show placeholders rather than wrong rows,
// and the cursor must not move when a window lands under it.
func TestScrollingAheadOfTheDataShowsPlaceholders(t *testing.T) {
	s := newSourceScreen(t, func(string) time.Duration { return 400 * time.Millisecond })
	h := newHarness(t, s)
	h.pumpFor(700 * time.Millisecond)

	// Through a key, not SetCursor. The table reports its viewport from
	// Update's returned command, so a cursor moved by a setter alone asks the
	// source for nothing — which is a fair description of the bug this test
	// would otherwise have been written around.
	h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})

	last := s.tab.Cursor()
	if last < 700 {
		t.Fatalf("G left the cursor at %d; the table is not addressing all 800 rows", last)
	}
	if view := h.render(); !strings.Contains(view, "·") {
		t.Errorf("no placeholders while scrolled past the held window:\n%s", view)
	}
	if _, ok := s.tab.Selected(); ok {
		t.Error("Selected reported a row the window does not hold")
	}

	h.pumpFor(1500 * time.Millisecond)

	if got := s.tab.Cursor(); got != last {
		t.Errorf("cursor moved from %d to %d when the page arrived under it", last, got)
	}
	if _, ok := s.tab.Selected(); !ok {
		t.Error("the row the cursor is on never arrived")
	}
	if view := h.render(); strings.Contains(view, "·") {
		t.Errorf("placeholders remain after the window landed:\n%s", view)
	}
}

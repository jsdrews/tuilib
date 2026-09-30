// Package remote demonstrates the full windowed-source loop: pkg/source
// coordinating a sparse pkg/table over a real paged HTTP API.
//
// The server is demoapi, and it is a genuine http.Handler reached through a
// genuine *http.Client — the request is built, encoded, routed, answered and
// decoded. It holds 5,000 applications and only ever answers one page at a
// time, with a deliberate latency so the seams are visible — 250ms by default,
// and L cycles it up to 8s. Nothing about the table's 5,000 rows is real: it
// holds one 100-row window and draws "·" everywhere else, which is what you
// see for a moment when you scroll faster than the source answers.
//
// What a slow source looks like, none of it written here: a committed filter
// or sort leaves the rows on screen, dimmed, with the border naming the query
// they answer and the one loading; [ ] s wait for a quiet moment before
// asking; scrolling waits until it settles; and every superseded request is
// cancelled — watch the output console (o) for each query's outcome.
//
// The loop, in this file:
//
//	Init            → src.Init()            → RequestMsg
//	RequestMsg      → fetch (a tea.Cmd)     → fetchedMsg
//	fetchedMsg      → src.Deliver + SetWindow (or SetFailed)
//	SetWindow       → ViewportChangedMsg    → src.Viewport → RequestMsg?
//	QueryChangedMsg → src.SetQuery          → RequestMsg
//
// The filter and the sort are answered by the source, not by the table:
// FilterRemote / SortRemote mean the table reports what the user asked for and
// displays whatever comes back. Type "region:eu-west" and it leaves here as
// "?region=eu-west" — Term.Title is the resolved column title, so a scoped
// term becomes a query parameter with no lookup on this side.
//
// Two things a slice and a time.Sleep could not demonstrate, and which this
// crosses a request boundary to reach:
//
//   - A request can fail. Press "e" and the next fetch asks the server for a
//     503; the screen has to handle a status code, which is what a screen
//     talking to anything real must do.
//   - Replies can arrive out of order. src.Deliver's generation check drops a
//     page answering a query the user has already moved past — and it can only
//     be a real drop if a real slow reply really does land second.
//
// Keys: / filters (enter commits — the request goes out then, not per
// keystroke), [ ] s sort, r refetches the current window, e arms one failure,
// L cycles the latency, and the usual j/k/g/G/^u/^d scroll through all 5,000
// logical rows.
package remote

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/demoapi"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/source"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

const pageSize = 100

// latencies are what L cycles through. The first is the default.
var latencies = []time.Duration{250 * time.Millisecond, 2 * time.Second, 4 * time.Second, 8 * time.Second}

// New returns the remote-source demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{
		// In-process by default, or a running demoapi when TUILIB_DEMO_API is
		// set — `task demo` does the latter. Nothing below changes either way:
		// it is the same client interface over the same wire format.
		api: demoapi.From(demoapi.Options{Seed: 4}),
		src: source.New(source.Options{PageSize: pageSize}),
	}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t   theme.Theme
	tab table.Model
	api demoapi.Target
	src source.Model

	latIdx   int
	failNext bool
}

func (s *Screen) Title() string         { return "Remote" }
func (s *Screen) IsCapturingKeys() bool { return s.tab.Filtering() }

func (s *Screen) Init() tea.Cmd {
	return tea.Batch(s.src.Init(), s.fetchFacets())
}

func (s *Screen) OnEnter(result any) tea.Cmd {
	if _, closed := result.(app.OutputClosed); closed {
		return nil
	}
	return nil
}

func (s *Screen) Layout() layout.Node {
	return layout.VStack(layout.Flex(1, layout.Sized(&s.tab)))
}

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

// HelpSections passes the table's own groups through and adds this screen's
// verbs, which are not the table's.
func (s *Screen) HelpSections() []help.Section {
	return help.SectionsOf(&s.tab, help.Group("Source",
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refetch")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "fail next fetch")),
		key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "cycle latency")),
	))
}

// fetchedMsg carries one page back from the server — or the error it got
// instead, in page.Err. The query rides along because the table is told
// which query a window answers.
type fetchedMsg struct {
	query source.Query
	page  source.Page
	rows  []table.Row
}

// facetsMsg carries the values the filter can complete against.
type facetsMsg struct {
	regions []string
	err     error
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmds []tea.Cmd
	switch m := msg.(type) {

	// The table says which logical rows are on screen. The coordinator
	// decides whether that needs a fetch, and waits for scrolling to settle.
	case table.ViewportChangedMsg:
		cmds = append(cmds, s.src.Viewport(m.FirstVisible, m.LastVisible))

	// The user committed a filter or a sort. Same query object the source
	// will answer, terms already resolved to column titles. The table keeps
	// its rows on screen, dimmed, until the answer lands.
	case table.QueryChangedMsg:
		cmds = append(cmds, s.src.SetQuery(m.Raw, m.Terms, m.Sort, m.Desc))

	// The one place I/O happens. The coordinator never does this itself.
	case source.RequestMsg:
		return s, s.fetch(m.Query)

	// Deliver arbitrates successes and failures alike: a reply to a query
	// the user has moved past is dropped either way. Its command carries the
	// query history the shell writes to the output console.
	case fetchedMsg:
		ok, cmd := s.src.Deliver(m.page)
		switch {
		case !ok:
		case m.page.Err != nil:
			s.tab.SetFailed(answered(m.query))
		default:
			s.tab.SetWindow(m.rows, m.page.Offset, m.page.Total, answered(m.query))
		}
		return s, cmd

	case facetsMsg:
		if m.err != nil {
			return s, app.ErrorOf(fmt.Errorf("fetch facets: %w", m.err))
		}
		// The source knows every region; the window on screen does not. Feeding
		// completions from resident rows would offer answers that are wrong
		// rather than merely incomplete.
		s.tab.SetDistinct(1, m.regions)
		return s, nil
	}

	if km, ok := msg.(tea.KeyMsg); ok && !s.tab.IsCapturingKeys() {
		switch km.String() {
		case "r":
			return s, s.src.Refresh()
		case "e":
			s.failNext = true
			return s, tea.Batch(app.Info("next fetch will fail"), s.src.Refresh())
		case "L":
			s.latIdx = (s.latIdx + 1) % len(latencies)
			return s, app.Info("latency " + latencies[s.latIdx].String())
		}
	}

	cmds = append(cmds, s.src.Update(msg))
	var cmd tea.Cmd
	s.tab, cmd = s.tab.Update(msg)
	return s, tea.Batch(append(cmds, cmd)...)
}

// answered names the query a page answers, in the table's terms.
func answered(q source.Query) table.Answer {
	return table.Answer{Raw: q.Raw, Sort: q.Sort, Desc: q.Desc}
}

// fetch is an ordinary tea.Cmd making an ordinary HTTP request.
func (s *Screen) fetch(q source.Query) tea.Cmd {
	u := url.Values{}
	u.Set("offset", strconv.Itoa(q.Offset))
	u.Set("limit", strconv.Itoa(q.Limit))
	u.Set("latency", latencies[s.latIdx].String())
	if q.Sort != "" {
		u.Set("sort", q.Sort)
		if q.Desc {
			u.Set("desc", "true")
		}
	}
	// A scoped term is already resolved to its column, so it becomes a
	// parameter directly; a bare one has no column to name and rides ?q=.
	var bare []string
	for _, t := range q.Terms {
		if t.Title != "" && t.Regex == nil {
			u.Set(strings.ToLower(t.Title), t.Value)
			continue
		}
		bare = append(bare, t.Raw)
	}
	if len(bare) > 0 {
		u.Set("q", strings.Join(bare, " "))
	}
	if s.failNext {
		u.Set("fail", "503")
		s.failNext = false
	}

	api := s.api
	failed := func(err error) tea.Msg {
		return fetchedMsg{query: q, page: source.Page{Gen: q.Gen, Err: err}}
	}
	return func() tea.Msg {
		// q.Ctx is cancelled the moment a newer request supersedes this one,
		// which abandons the request on the wire as well as the reply here.
		req, err := http.NewRequestWithContext(q.Ctx, http.MethodGet, api.URL("/apps?"+u.Encode()), nil)
		if err != nil {
			return failed(err)
		}
		resp, err := api.Client.Do(req)
		if err != nil {
			return failed(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var e struct {
				Error string `json:"error"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&e)
			return failed(fmt.Errorf("fetch page: %s: %s", resp.Status, e.Error))
		}

		var body struct {
			Total  int `json:"total"`
			Offset int `json:"offset"`
			Rows   []struct {
				Name   string `json:"name"`
				Region string `json:"region"`
				Sync   string `json:"sync"`
				Health string `json:"health"`
			} `json:"rows"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return failed(fmt.Errorf("decode page: %w", err))
		}

		rows := make([]table.Row, len(body.Rows))
		for i, a := range body.Rows {
			rows[i] = table.Row{a.Name, a.Region, a.Sync, a.Health}
		}
		return fetchedMsg{
			query: q,
			page:  source.Page{Gen: q.Gen, Offset: body.Offset, Count: len(rows), Total: body.Total},
			rows:  rows,
		}
	}
}

func (s *Screen) fetchFacets() tea.Cmd {
	api := s.api
	return func() tea.Msg {
		resp, err := api.Client.Get(api.URL("/apps/facets?field=Region"))
		if err != nil {
			return facetsMsg{err: err}
		}
		defer resp.Body.Close()
		var body struct {
			Values []string `json:"values"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return facetsMsg{err: err}
		}
		return facetsMsg{regions: body.Values}
	}
}

// SetTheme rebuilds the table and replays its state onto it (rule 4): the
// filter and committed sort, the window with the query it answers, a
// failure, and a sort still inside its quiet period.
func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	prev, rebuilt := s.tab, s.tab.Columns() != nil

	opts := t.Table()
	opts.Title = "Applications"
	if s.api.Live {
		// Worth saying on the border: against a running server this screen and
		// the activity demo are looking at one world, and a curl can change it
		// underneath them.
		opts.Title = "Applications · live"
	}
	opts.Filterable = true
	opts.FilterMode = table.FilterRemote
	opts.SortMode = table.SortRemote
	// Fixed and Flex widths only: content-auto would reflow the columns every
	// time a window swapped underneath the user.
	opts.Columns = []table.Column{
		{Title: "Name", Width: 22, Flex: 2, Sortable: true},
		{Title: "Region", Width: 12, Sortable: true},
		{Title: "Sync", Width: 12, Sortable: true},
		{Title: "Health", Width: 13},
	}
	s.tab = table.New(opts)
	if !rebuilt {
		return
	}

	s.tab.SetValue(prev.Value())
	s.tab.SetSort(prev.CommittedSort())
	if a, ok := prev.Answered(); ok {
		off, _, total := prev.Window()
		s.tab.SetWindow(prev.Rows(), off, total, a)
	}
	if prev.Failed() {
		s.tab.SetFailed(prev.Committed())
	}
	if col, desc, ok := prev.StagedSort(); ok {
		s.tab.SetStagedSort(col, desc)
	}
	s.tab.SetCursor(prev.Cursor())
}

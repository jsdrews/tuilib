// Package remote demonstrates the full windowed-source loop: pkg/source
// coordinating a sparse pkg/table over a real paged HTTP API.
//
// The server is demoapi, and it is a genuine http.Handler reached through a
// genuine *http.Client — the request is built, encoded, routed, answered and
// decoded. It holds 5,000 applications and only ever answers one page at a
// time, with a deliberate 250ms of latency so the seams are visible. Nothing
// about the table's 5,000 rows is real: it holds one 100-row window and draws
// "·" everywhere else, which is what you see for a moment when you scroll
// faster than the source answers.
//
// The loop, in this file:
//
//	Init            → src.Init()            → RequestMsg
//	RequestMsg      → fetch (a tea.Cmd)     → fetchedMsg
//	fetchedMsg      → src.Deliver + SetWindow
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
// and the usual j/k/g/G/^u/^d scroll through all 5,000 logical rows.
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

const (
	pageSize = 100
	latency  = 250 * time.Millisecond
)

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

	lastReq  string
	failNext bool
}

func (s *Screen) Title() string         { return "Remote" }
func (s *Screen) IsCapturingKeys() bool { return s.tab.Filtering() }

func (s *Screen) Init() tea.Cmd {
	return tea.Batch(s.src.Init(), s.tab.SetLoading(true), s.fetchFacets())
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
	))
}

// fetchedMsg carries one page back from the server.
type fetchedMsg struct {
	page source.Page
	rows []table.Row
}

// fetchErrMsg is the other outcome, and the one a fake in-process database
// never had. Gen is carried so a failure answering an abandoned query can be
// dropped as quietly as a success would be.
type fetchErrMsg struct {
	gen int
	err error
}

// facetsMsg carries the values the filter can complete against.
type facetsMsg struct{ regions []string }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch m := msg.(type) {

	// The table says which logical rows are on screen. The coordinator
	// decides whether that needs a fetch; usually it doesn't.
	case table.ViewportChangedMsg:
		return s, s.src.Viewport(m.FirstVisible, m.LastVisible)

	// The user committed a filter or asked for a sort. Same query object the
	// source will answer, terms already resolved to column titles.
	case table.QueryChangedMsg:
		s.tab.SetCursor(0)
		return s, tea.Batch(s.src.SetQuery(m.Raw, m.Terms, m.Sort, m.Desc), s.tab.SetLoading(true))

	// The one place I/O happens. The coordinator never does this itself.
	case source.RequestMsg:
		s.lastReq = describe(m.Query)
		return s, s.fetch(m.Query)

	case fetchedMsg:
		if !s.src.Deliver(m.page) {
			return s, nil // answers a query the user has already moved past
		}
		s.tab.SetWindow(m.rows, m.page.Offset, m.page.Total)
		s.tab.SetTitle(fmt.Sprintf("Applications — %s", s.lastReq))
		return s, s.tab.SetLoading(false)

	case fetchErrMsg:
		// Deliver is the arbiter for failures too: a 503 answering a query the
		// user has moved past is as stale as a page would be, and reporting it
		// would put an error on screen for something nobody is waiting for.
		if !s.src.Deliver(source.Page{Gen: m.gen}) {
			return s, nil
		}
		return s, tea.Batch(s.tab.SetLoading(false), app.ErrorOf(m.err))

	case facetsMsg:
		// The source knows every region; the window on screen does not. Feeding
		// completions from resident rows would offer answers that are wrong
		// rather than merely incomplete.
		s.tab.SetDistinct(1, m.regions)
		return s, nil
	}

	if km, ok := msg.(tea.KeyMsg); ok && !s.tab.IsCapturingKeys() {
		switch km.String() {
		case "r":
			return s, tea.Batch(s.src.Refresh(), s.tab.SetLoading(true))
		case "e":
			s.failNext = true
			return s, tea.Batch(app.Info("next fetch will fail"), s.src.Refresh(), s.tab.SetLoading(true))
		}
	}

	var cmd tea.Cmd
	s.tab, cmd = s.tab.Update(msg)
	return s, cmd
}

// fetch is an ordinary tea.Cmd making an ordinary HTTP request.
func (s *Screen) fetch(q source.Query) tea.Cmd {
	u := url.Values{}
	u.Set("offset", strconv.Itoa(q.Offset))
	u.Set("limit", strconv.Itoa(q.Limit))
	u.Set("latency", latency.String())
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

	api, gen := s.api, q.Gen
	return func() tea.Msg {
		resp, err := api.Client.Get(api.URL("/apps?" + u.Encode()))
		if err != nil {
			return fetchErrMsg{gen: gen, err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var e struct {
				Error string `json:"error"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&e)
			return fetchErrMsg{gen: gen, err: fmt.Errorf("fetch page: %s: %s", resp.Status, e.Error)}
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
			return fetchErrMsg{gen: gen, err: fmt.Errorf("decode page: %w", err)}
		}

		rows := make([]table.Row, len(body.Rows))
		for i, a := range body.Rows {
			rows[i] = table.Row{a.Name, a.Region, a.Sync, a.Health}
		}
		return fetchedMsg{
			page: source.Page{Gen: gen, Offset: body.Offset, Count: len(rows), Total: body.Total},
			rows: rows,
		}
	}
}

func (s *Screen) fetchFacets() tea.Cmd {
	api := s.api
	return func() tea.Msg {
		resp, err := api.Client.Get(api.URL("/apps/facets?field=Region"))
		if err != nil {
			return fetchErrMsg{err: err}
		}
		defer resp.Body.Close()
		var body struct {
			Values []string `json:"values"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return fetchErrMsg{err: err}
		}
		return facetsMsg{regions: body.Values}
	}
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	cursor, val := 0, ""
	sortCol, sortDesc := -1, false
	if s.tab.Columns() != nil {
		cursor, val = s.tab.Cursor(), s.tab.Value()
		sortCol, sortDesc = s.tab.SortColumn(), s.tab.SortDescending()
	}
	off, _, total := s.tab.Window()
	rows := s.tab.Rows()

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

	if rows != nil {
		s.tab.SetWindow(rows, off, total)
	}
	s.tab.SetValue(val)
	s.tab.SetSort(sortCol, sortDesc)
	s.tab.SetCursor(cursor)
}

func describe(q source.Query) string {
	parts := []string{fmt.Sprintf("rows %d–%d", q.Offset, q.Offset+q.Limit-1)}
	if q.Raw != "" {
		parts = append(parts, "filter "+q.Raw)
	}
	if q.Sort != "" {
		dir := "▲"
		if q.Desc {
			dir = "▼"
		}
		parts = append(parts, "sort "+q.Sort+dir)
	}
	return strings.Join(parts, " · ")
}

// Package remote demonstrates a table over a real paged HTTP API, bound
// with pkg/remote: the screen writes the HTTP call and nothing else.
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
// The filter and the sort are answered by the server, not by the table. Type
// "region:eu-west" and it leaves here as "?region=eu-west" — Term.Title is
// the resolved column title, so a scoped term becomes a query parameter with
// no lookup on this side.
//
// Two things a slice and a time.Sleep could not demonstrate, and which this
// crosses a request boundary to reach:
//
//   - A request can fail. Press "e" and the next fetch asks the server for a
//     503; the page function returns the error and the table says so.
//   - Replies can arrive out of order. A page answering a query the user has
//     already moved past is dropped — and it can only be a real drop if a real
//     slow reply really does land second.
//
// Keys: / filters (enter commits — the request goes out then, not per
// keystroke), [ ] s sort, r refetches the current window, e arms one failure,
// L cycles the latency, and the usual j/k/g/G/^u/^d scroll through all 5,000
// logical rows.
package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/demoapi"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	bind "github.com/jsdrews/tuilib/pkg/remote"
	"github.com/jsdrews/tuilib/pkg/screen"
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
	}
	s.tab = bind.NewTable(s.options(t), bind.Seekable[table.KeyedRow]{Page: s.page, PageSize: pageSize})
	return s
}

type Screen struct {
	tab *bind.Table
	api demoapi.Target

	latIdx   atomic.Int32
	failNext atomic.Bool
}

// facetsMsg carries the values the filter can complete against.
type facetsMsg struct {
	regions []string
	err     error
}

func (s *Screen) Title() string         { return "Remote" }
func (s *Screen) IsCapturingKeys() bool { return s.tab.Filtering() }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) Layout() layout.Node   { return layout.Sized(s.tab) }
func (s *Screen) Help() []key.Binding   { return help.Flatten(s.HelpSections()) }

func (s *Screen) Init() tea.Cmd { return tea.Batch(s.tab.Init(), s.fetchFacets()) }

// HelpSections passes the table's own groups through and adds this screen's
// verbs, which are not the table's.
func (s *Screen) HelpSections() []help.Section {
	return help.SectionsOf(s.tab, help.Group("Source",
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refetch")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "fail next fetch")),
		key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "cycle latency")),
	))
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if m, ok := msg.(facetsMsg); ok {
		if m.err != nil {
			return s, app.ErrorOf(fmt.Errorf("fetch facets: %w", m.err))
		}
		// The source knows every region; the window on screen does not.
		s.tab.SetDistinct(1, m.regions)
		return s, nil
	}
	if km, ok := msg.(tea.KeyMsg); ok && !s.tab.IsCapturingKeys() {
		switch km.String() {
		case "r":
			return s, s.tab.Refresh()
		case "e":
			s.failNext.Store(true)
			return s, tea.Batch(app.Info("next fetch will fail"), s.tab.Refresh())
		case "L":
			i := (s.latIdx.Load() + 1) % int32(len(latencies))
			s.latIdx.Store(i)
			return s, app.Info("latency " + latencies[i].String())
		}
	}
	return s, s.tab.Update(msg)
}

func (s *Screen) SetTheme(t theme.Theme) { s.tab.Restyle(s.options(t)) }

func (s *Screen) options(t theme.Theme) table.Options {
	opts := t.Table()
	opts.Title = "Applications"
	if s.api.Live {
		// Worth saying on the border: against a running server this screen and
		// the activity demo are looking at one world, and a curl can change it
		// underneath them.
		opts.Title = "Applications · live"
	}
	opts.Filterable = true
	// Fixed and Flex widths only: content-auto would reflow the columns every
	// time a window swapped underneath the user.
	opts.Columns = []table.Column{
		{Title: "Name", Width: 22, Flex: 2, Sortable: true},
		{Title: "Region", Width: 12, Sortable: true},
		{Title: "Sync", Width: 12, Sortable: true},
		{Title: "Health", Width: 13},
	}
	return opts
}

// page is an ordinary HTTP request, under a context the binding cancels
// the moment a newer request supersedes this one.
func (s *Screen) page(ctx context.Context, w bind.Window) ([]table.KeyedRow, int, error) {
	u := url.Values{}
	u.Set("offset", strconv.Itoa(w.Offset))
	u.Set("limit", strconv.Itoa(w.Limit))
	u.Set("latency", latencies[s.latIdx.Load()].String())
	if w.Sort != "" {
		u.Set("sort", w.Sort)
		if w.Desc {
			u.Set("desc", "true")
		}
	}
	// A scoped term is already resolved to its column, so it becomes a
	// parameter directly; a bare one has no column to name and rides ?q=.
	var bare []string
	for _, t := range w.Terms {
		if t.Title != "" && t.Regex == nil {
			u.Set(strings.ToLower(t.Title), t.Value)
			continue
		}
		bare = append(bare, t.Raw)
	}
	if len(bare) > 0 {
		u.Set("q", strings.Join(bare, " "))
	}
	if s.failNext.Swap(false) {
		u.Set("fail", "503")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.api.URL("/apps?"+u.Encode()), nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := s.api.Client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return nil, 0, fmt.Errorf("fetch page: %s: %s", resp.Status, e.Error)
	}
	var body struct {
		Total int `json:"total"`
		Rows  []struct {
			Name   string `json:"name"`
			Region string `json:"region"`
			Sync   string `json:"sync"`
			Health string `json:"health"`
		} `json:"rows"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, 0, fmt.Errorf("decode page: %w", err)
	}
	rows := make([]table.KeyedRow, len(body.Rows))
	for i, a := range body.Rows {
		rows[i] = table.KeyedRow{Key: a.Name, Cells: []string{a.Name, a.Region, a.Sync, a.Health}}
	}
	return rows, body.Total, nil
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

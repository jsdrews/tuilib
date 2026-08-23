// Package remote demonstrates the full windowed-source loop: pkg/source
// coordinating a sparse pkg/table over a simulated paged HTTP API.
//
// The "server" holds 5,000 cities and only ever answers one page at a
// time, after a deliberate 250ms delay so the seams are visible. Nothing
// about the table's 5,000 rows is real — it holds one 100-row window and
// draws "·" everywhere else, which is what you see for a moment when you
// scroll faster than the source answers.
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
// FilterRemote / SortRemote mean the table reports what the user asked
// for and displays whatever comes back. Type "region:europe" and the
// term arrives at the fake server already resolved to its column.
//
// Keys: / filters (enter commits — the request goes out then, not per
// keystroke), [ ] s sort, r refetches the current window, and the usual
// j/k/g/G/^u/^d scroll through all 5,000 logical rows.
package remote

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/query"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/source"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

const (
	pageSize  = 100
	latency   = 250 * time.Millisecond
	datasetN  = 5000
)

// New returns the remote-source demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{
		db:  newFakeDB(datasetN),
		src: source.New(source.Options{PageSize: pageSize}),
	}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t   theme.Theme
	tab table.Model
	db  *fakeDB
	src source.Model

	lastReq string
}

func (s *Screen) Title() string         { return "Remote" }
func (s *Screen) IsCapturingKeys() bool { return s.tab.Filtering() }

func (s *Screen) Init() tea.Cmd {
	return tea.Batch(s.src.Init(), s.tab.SetLoading(true))
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

func (s *Screen) Help() []key.Binding {
	return append(s.tab.Help(),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refetch")),
	)
}

// fetchedMsg carries one page back from the fake server.
type fetchedMsg struct {
	page source.Page
	rows []table.Row
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch m := msg.(type) {

	// The table says which logical rows are on screen. The coordinator
	// decides whether that needs a fetch; usually it doesn't.
	case table.ViewportChangedMsg:
		return s, s.src.Viewport(m.FirstVisible, m.LastVisible)

	// The user committed a filter or asked for a sort. Same query object
	// the source will answer, terms already resolved to column titles.
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
		s.tab.SetTitle(fmt.Sprintf("Cities — %s", s.lastReq))
		return s, s.tab.SetLoading(false)
	}

	if km, ok := msg.(tea.KeyMsg); ok && !s.tab.IsCapturingKeys() {
		if km.String() == "r" {
			return s, tea.Batch(s.src.Refresh(), s.tab.SetLoading(true))
		}
	}

	var cmd tea.Cmd
	s.tab, cmd = s.tab.Update(msg)
	return s, cmd
}

// fetch is an ordinary tea.Cmd. Swap the body for an http.Client call and
// nothing else in this file changes.
func (s *Screen) fetch(q source.Query) tea.Cmd {
	db := s.db
	return func() tea.Msg {
		time.Sleep(latency)
		rows, total := db.page(q)
		return fetchedMsg{
			page: source.Page{Gen: q.Gen, Offset: q.Offset, Count: len(rows), Total: total},
			rows: rows,
		}
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
	opts.Title = "Cities"
	opts.Filterable = true
	opts.FilterMode = table.FilterRemote
	opts.SortMode = table.SortRemote
	// Fixed and Flex widths only: content-auto would reflow the columns
	// every time a window swapped underneath the user.
	opts.Columns = []table.Column{
		{Title: "Name", Width: 18, Flex: 2, Sortable: true},
		{Title: "Region", Width: 12, Sortable: true},
		{Title: "Population", Width: 12, Align: lipgloss.Right, Sortable: true},
		{Title: "Status", Width: 10},
	}
	s.tab = table.New(opts)

	if rows != nil {
		s.tab.SetWindow(rows, off, total)
	}
	s.tab.SetValue(val)
	s.tab.SetSort(sortCol, sortDesc)
	s.tab.SetCursor(cursor)
	// The source knows every region; the window on screen doesn't.
	s.tab.SetDistinct(1, regions)
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

// ---- the "server" ----

var regions = []string{"Africa", "Americas", "Asia", "Europe", "Oceania"}

var statuses = []string{"Healthy", "Degraded", "Down"}

type city struct {
	name   string
	region string
	pop    int
	status string
}

type fakeDB struct{ all []city }

func newFakeDB(n int) *fakeDB {
	out := make([]city, n)
	for i := range out {
		out[i] = city{
			name:   fmt.Sprintf("City %04d", i),
			region: regions[i%len(regions)],
			pop:    (i*7919)%9_000_000 + 50_000,
			status: statuses[i%len(statuses)],
		}
	}
	return &fakeDB{all: out}
}

// page answers a Query the way a REST endpoint would: filter, then sort,
// then slice. Note it filters with pkg/query — the same parse the table
// used — so "region:europe" needs no translation on this side either.
func (d *fakeDB) page(q source.Query) ([]table.Row, int) {
	matched := make([]city, 0, len(d.all))
	for _, c := range d.all {
		if query.MatchAll(cells(c), q.Terms) {
			matched = append(matched, c)
		}
	}
	if q.Sort != "" {
		sortCities(matched, q.Sort, q.Desc)
	}
	total := len(matched)
	start := min(q.Offset, total)
	end := min(start+q.Limit, total)
	rows := make([]table.Row, 0, end-start)
	for _, c := range matched[start:end] {
		rows = append(rows, cells(c))
	}
	return rows, total
}

func cells(c city) table.Row {
	return table.Row{c.name, c.region, fmt.Sprintf("%d", c.pop), c.status}
}

func sortCities(cs []city, col string, desc bool) {
	less := func(i, j int) bool { return cs[i].name < cs[j].name }
	switch col {
	case "Region":
		less = func(i, j int) bool { return cs[i].region < cs[j].region }
	case "Population":
		less = func(i, j int) bool { return cs[i].pop < cs[j].pop }
	}
	sort.SliceStable(cs, func(i, j int) bool {
		if desc {
			return less(j, i)
		}
		return less(i, j)
	})
}

// Package anchored demonstrates pkg/eventlog over Anchored data: a fake log
// index shaped like Elasticsearch behind search_after. There are no offsets
// and no total — only "the page older than this document" and "the page
// newer than that one" — so the eventlog holds a span, grown at whichever
// edge the viewport nears, and trimmed at the far end past 5,000 documents.
//
// The index already holds 20,000 documents and keeps receiving more, so the
// view opens at the newest (source.Newest) and follows. The loop:
//
//	ViewportChangedMsg → src.Viewport(ToOlder, ToNewer)   (extend an edge)
//	QueryChangedMsg    → src.SetQuery                     (the filter narrows)
//	FindMsg            → src.Find                         (n/N past the span)
//	RequestMsg         → fetch(query, log.Edges())        (cursors come from the span)
//	fetchedMsg         → src.Deliver, then Prepend / Append / SetNew / FoundAt
//
// A search hit beyond the span re-anchors there: the span is replaced by the
// hit and its surroundings, which is Kibana's "surrounding documents" view.
// The cursor is the document id as a string; the library never reads it.
package anchored

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
	elog "github.com/jsdrews/tuilib/pkg/eventlog"
	"github.com/jsdrews/tuilib/pkg/help"
	insp "github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/source"
	"github.com/jsdrews/tuilib/pkg/theme"
)

const (
	pageSize = 200
	seeded   = 20000
	perSec   = 5
)

var latencies = []time.Duration{300 * time.Millisecond, 2 * time.Second, 4 * time.Second}

// New returns the anchored-source demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{
		started: time.Now(),
		src:     source.NewAnchored(source.AnchoredOptions{PageSize: pageSize, Follow: time.Second}),
	}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t       theme.Theme
	log     elog.Model
	src     source.Anchored
	started time.Time
	latIdx  int
}

func (s *Screen) Title() string         { return "Anchored" }
func (s *Screen) IsCapturingKeys() bool { return s.log.IsCapturingKeys() }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) Layout() layout.Node   { return layout.Sized(&s.log) }
func (s *Screen) Help() []key.Binding   { return help.Flatten(s.HelpSections()) }

func (s *Screen) HelpSections() []help.Section {
	return help.SectionsOf(&s.log, help.Group("Source",
		key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "cycle latency")),
	))
}

func (s *Screen) Init() tea.Cmd {
	s.log.SetGrowing(true)
	return tea.Batch(s.src.Init(), s.src.SetGrowing(true))
}

type fetchedMsg struct {
	query source.Query
	page  source.Page
	items []elog.Item
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmds []tea.Cmd
	switch m := msg.(type) {
	case elog.ViewportChangedMsg:
		cmds = append(cmds, s.src.Viewport(m.ToOlder, m.ToNewer))

	case elog.QueryChangedMsg:
		cmds = append(cmds, s.src.SetQuery(m.Raw, m.Terms, "", false))

	case elog.FindMsg:
		dir := source.Older
		if m.Newer {
			dir = source.Newer
		}
		cmds = append(cmds, s.src.Find(m.Term, dir))

	case source.RequestMsg:
		older, newer := s.log.Edges()
		return s, s.fetch(m.Query, older, newer)

	case fetchedMsg:
		ok, cmd := s.src.Deliver(m.page)
		a := elog.Answer{Raw: m.query.Raw}
		switch {
		case !ok:
		case m.query.Find && m.page.Found:
			// Jump there: the span is replaced by the hit and its context.
			s.log.FoundAt(true, m.page.Cursor)
			s.log.Reanchor()
			cmds = append(cmds, s.src.SetAnchor(source.At(m.page.Cursor)))
		case m.query.Find:
			s.log.FoundAt(false, "")
		case m.page.Err != nil:
			s.log.SetFailed(a)
		case m.query.Probe:
			s.log.SetNew(m.page.Count, m.page.More)
		case m.query.Dir == source.Older:
			s.log.Prepend(m.items, a)
		default:
			s.log.Append(m.items, a)
		}
		s.log.SetMore(s.src.More())
		cmds = append(cmds, cmd)
	}

	if s.log.IsActivate(msg) {
		if it, ok := s.log.Selected(); ok {
			return s, screen.Push(newDetail(s.t, it))
		}
	}
	if km, ok := msg.(tea.KeyMsg); ok && !s.log.IsCapturingKeys() && km.String() == "L" {
		s.latIdx = (s.latIdx + 1) % len(latencies)
		return s, app.Info("latency " + latencies[s.latIdx].String())
	}

	cmds = append(cmds, s.src.Update(msg))
	var cmd tea.Cmd
	s.log, cmd = s.log.Update(msg)
	return s, tea.Batch(append(cmds, cmd)...)
}

// ---- the fake index -----------------------------------------------------

// count is how many documents the index holds now.
func (s *Screen) count() int { return seeded + int(time.Since(s.started).Seconds()*perSec) }

var (
	levels   = []string{"info", "info", "info", "warn", "info", "error"}
	services = []string{"gateway", "orders", "payments", "search"}
	t0       = time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
)

type doc struct {
	id      int
	ts      time.Time
	level   string
	service string
	msg     string
}

func document(id int) doc {
	d := doc{
		id:      id,
		ts:      t0.Add(time.Duration(id) * 200 * time.Millisecond),
		level:   levels[(id*7)%len(levels)],
		service: services[(id*3)%len(services)],
	}
	switch d.level {
	case "error":
		d.msg = fmt.Sprintf("upstream timeout after 5000ms (request %d)", id)
	case "warn":
		d.msg = fmt.Sprintf("slow query: %dms", 200+id%900)
	default:
		d.msg = fmt.Sprintf("handled request %d in %dms", id, 3+id%40)
	}
	return d
}

func (d doc) item() elog.Item {
	line := fmt.Sprintf("%-5s %-8s %s", strings.ToUpper(d.level), d.service, d.msg)
	return elog.Item{
		Key: strconv.Itoa(d.id),
		// No positions in Anchored data: the gutter shows the document's
		// @timestamp — the server's time for the event, not the TUI's —
		// which stays true as pages arrive above it. These messages carry
		// no time of their own; if yours do, leave Mark empty instead.
		Mark:  d.ts.Format("15:04:05.000"),
		Lines: []string{line},
		Data: map[string]any{
			"_id": d.id, "@timestamp": d.ts.Format(time.RFC3339Nano),
			"log.level": d.level, "service.name": d.service, "message": d.msg,
		},
	}
}

// matches is the index's query: "level:error", "service:orders", or text.
func matches(raw string, d doc) bool {
	for _, term := range strings.Fields(strings.ToLower(raw)) {
		switch {
		case strings.HasPrefix(term, "level:"):
			if d.level != strings.TrimPrefix(term, "level:") {
				return false
			}
		case strings.HasPrefix(term, "service:"):
			if d.service != strings.TrimPrefix(term, "service:") {
				return false
			}
		default:
			if !strings.Contains(strings.ToLower(d.msg), term) {
				return false
			}
		}
	}
	return true
}

// fetch answers q the way search_after would: from the edge cursor the
// request's direction names, never by offset.
func (s *Screen) fetch(q source.Query, olderEdge, newerEdge string) tea.Cmd {
	lat, n := latencies[s.latIdx], s.count()
	return func() tea.Msg {
		select {
		case <-q.Ctx.Done():
			return fetchedMsg{query: q, page: source.Page{Gen: q.Gen, Err: context.Canceled}}
		case <-time.After(lat):
		}
		if strings.Contains(q.Raw, "boom") {
			return fetchedMsg{query: q, page: source.Page{Gen: q.Gen, Err: errors.New("search: 503 Service Unavailable")}}
		}
		var ids []int
		for id := 0; id < n; id++ {
			if matches(q.Raw, document(id)) {
				ids = append(ids, id)
			}
		}
		// pos is where a cursor sits in the filtered ids.
		pos := func(c string) int {
			id, _ := strconv.Atoi(c)
			return sort.SearchInts(ids, id)
		}

		cursor := q.Cursor
		if !q.FromAnchor {
			// An edge request: extend from the span's own edge.
			if q.Dir == source.Older {
				cursor = olderEdge
			} else {
				cursor = newerEdge
			}
		}

		if q.Find {
			term := strings.ToLower(q.Term)
			p := pos(cursor)
			if q.Dir == source.Older {
				for i := p - 1; i >= 0; i-- {
					if strings.Contains(strings.ToLower(document(ids[i]).msg), term) {
						return fetchedMsg{query: q, page: source.Page{Gen: q.Gen, Found: true, Cursor: strconv.Itoa(ids[i])}}
					}
				}
			} else {
				for i := p + 1; i < len(ids); i++ {
					if strings.Contains(strings.ToLower(document(ids[i]).msg), term) {
						return fetchedMsg{query: q, page: source.Page{Gen: q.Gen, Found: true, Cursor: strconv.Itoa(ids[i])}}
					}
				}
			}
			return fetchedMsg{query: q, page: source.Page{Gen: q.Gen}}
		}

		var from, to int
		var more bool
		switch {
		case q.Dir == source.Older:
			to = len(ids) // Newest: from the tail
			if cursor != "" {
				to = pos(cursor)
				if q.Inclusive {
					to++
				}
			}
			from = max(0, to-q.Limit)
			more = from > 0
		default:
			from = 0 // Oldest: from the head
			if cursor != "" {
				from = pos(cursor) + 1
			}
			to = min(len(ids), from+q.Limit)
			more = to < len(ids)
		}
		to = min(to, len(ids))
		from = min(from, to)
		var items []elog.Item
		for _, id := range ids[from:to] {
			items = append(items, document(id).item())
		}
		return fetchedMsg{query: q, page: source.Page{Gen: q.Gen, Count: len(items), More: more}, items: items}
	}
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	prev, rebuilt := s.log, s.log.Title() != ""
	opts := t.Eventlog()
	opts.Title = "logs-* · search_after"
	opts.Anchored = true
	opts.Filterable = true
	opts.Searchable = true
	s.log = elog.New(opts)
	s.log.SetGrowing(true)
	if !rebuilt {
		return
	}
	s.log.SetValue(prev.Value())
	s.log.SetTerm(prev.Term())
	if a, ok := prev.Answered(); ok {
		_, items := prev.Items()
		s.log.Append(items, a)
		s.log.SetMore(s.src.More())
	}
	if !prev.Following() {
		s.log.SetCursor(prev.Cursor())
	}
}

// detail shows one document's fields.
type detail struct{ ins insp.Model }

func newDetail(t theme.Theme, it elog.Item) screen.Screen {
	opts := t.Inspector()
	opts.Title = "_id " + it.Key
	if m, ok := it.Data.(map[string]any); ok {
		opts.Fields = insp.FromMap(m)
	}
	return &detail{ins: insp.New(opts)}
}

func (d *detail) Title() string         { return "Document" }
func (d *detail) Init() tea.Cmd         { return nil }
func (d *detail) OnEnter(any) tea.Cmd   { return nil }
func (d *detail) IsCapturingKeys() bool { return d.ins.Searching() }
func (d *detail) Layout() layout.Node   { return layout.Sized(&d.ins) }
func (d *detail) Help() []key.Binding   { return d.ins.Help() }
func (d *detail) SetTheme(theme.Theme)  {}
func (d *detail) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	d.ins, cmd = d.ins.Update(msg)
	return d, cmd
}

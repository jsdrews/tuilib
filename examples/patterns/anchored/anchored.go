// Package anchored demonstrates pkg/eventlog over Anchored data: a log index
// read the way Elasticsearch (behind ECK) is read past its first 10,000
// hits — with search_after, over HTTP, against demoapi's
// POST /logs-app/_search. There are no offsets and no total, only "the page
// older than these sort values" and "the page newer than those", so the
// eventlog holds a span grown at whichever edge the viewport nears, trimmed
// at the far end past 5,000 documents.
//
// The index holds 20,000 documents and keeps receiving more, so the view
// opens at the newest and follows. The screen is bound with pkg/remote and
// writes only the search client, which is shaped like one for a real
// cluster:
//
//   - The cursor is a hit's sort values, @timestamp and the _shard_doc
//     tiebreaker; several documents share a timestamp, so paging on the
//     timestamp alone would lose the ones at a page's edge.
//   - Older pages are the same query with the sort reversed, the hits put
//     back in time order.
//   - Tailing newer documents rewinds a few seconds past the edge: some
//     arrive after their own @timestamp, and a tail strictly after the
//     newest one held would never see them. The eventlog drops the
//     repeats by key.
//   - Whether an edge has more is asked for, not computed: one hit more
//     than the page.
//   - A search hit beyond the span re-anchors there — the span is replaced
//     by the hit and its surroundings, Kibana's "surrounding documents" —
//     and the binding does that part.
//
// The gutter shows each document's @timestamp: the server's time for the
// event, not the TUI's. Enter opens a document; L cycles latency.
package anchored

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/demoapi"
	"github.com/jsdrews/tuilib/pkg/app"
	elog "github.com/jsdrews/tuilib/pkg/eventlog"
	"github.com/jsdrews/tuilib/pkg/help"
	insp "github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/remote"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

const (
	pageSize = 200
	// overlap is how far a tail rewinds past the newest document held, to
	// catch documents ingested after their own @timestamp.
	overlap = 5 * time.Second
)

var latencies = []time.Duration{300 * time.Millisecond, 2 * time.Second, 4 * time.Second}

// New returns the anchored-source demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{t: t, api: demoapi.From(demoapi.Options{Seed: 4})}
	s.log = remote.NewEventlog(options(t), remote.Anchored[elog.Item]{
		Edge:     s.edge,
		Find:     s.find,
		PageSize: pageSize,
		Follow:   time.Second,
	})
	return s
}

type Screen struct {
	t      theme.Theme
	log    *remote.Eventlog
	api    demoapi.Target
	latIdx atomic.Int32
}

func (s *Screen) Title() string         { return "Anchored" }
func (s *Screen) IsCapturingKeys() bool { return s.log.IsCapturingKeys() }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) Layout() layout.Node   { return layout.Sized(s.log) }
func (s *Screen) Help() []key.Binding   { return help.Flatten(s.HelpSections()) }

func (s *Screen) HelpSections() []help.Section {
	return help.SectionsOf(s.log, help.Group("Source",
		key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "cycle latency")),
	))
}

func (s *Screen) Init() tea.Cmd { return tea.Batch(s.log.Init(), s.log.SetGrowing(true)) }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if s.log.IsActivate(msg) {
		if it, ok := s.log.Selected(); ok {
			return s, screen.Push(newDetail(s.t, it))
		}
	}
	if km, ok := msg.(tea.KeyMsg); ok && !s.log.IsCapturingKeys() && km.String() == "L" {
		i := (s.latIdx.Load() + 1) % int32(len(latencies))
		s.latIdx.Store(i)
		return s, app.Info("latency " + latencies[i].String())
	}
	return s, s.log.Update(msg)
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	s.log.Restyle(options(t))
}

func options(t theme.Theme) elog.Options {
	opts := t.Eventlog()
	opts.Title = "logs-* · search_after"
	opts.Filterable = true
	opts.Searchable = true
	return opts
}

// ---- the search client: the only part a real screen writes ---------------

type hit struct {
	ID     string         `json:"_id"`
	Source map[string]any `json:"_source"`
	Sort   []int64        `json:"sort"`
}

// search posts body to the index's _search.
func (s *Screen) search(ctx context.Context, body map[string]any) ([]hit, error) {
	b, _ := json.Marshal(body)
	u := s.api.URL("/logs-app/_search?latency=" + latencies[s.latIdx.Load()].String())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.api.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Error *struct {
			Reason string `json:"reason"`
		} `json:"error"`
		Hits struct {
			Hits []hit `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("search: %s: %w", resp.Status, err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("search: %s: %s", resp.Status, out.Error.Reason)
	}
	return out.Hits.Hits, nil
}

// queryOf turns the filter bar into query_string: level:error and
// service:orders scope by field, other words search the message.
func queryOf(f remote.Filter, extra ...string) map[string]any {
	var parts []string
	for _, t := range f.Terms {
		raw := t.Raw
		switch {
		case strings.HasPrefix(strings.ToLower(raw), "level:"):
			parts = append(parts, "log.level:"+raw[len("level:"):])
		case strings.HasPrefix(strings.ToLower(raw), "service:"):
			parts = append(parts, "service.name:"+raw[len("service:"):])
		default:
			parts = append(parts, raw)
		}
	}
	parts = append(parts, extra...)
	if len(parts) == 0 {
		return map[string]any{"match_all": map[string]any{}}
	}
	return map[string]any{"query_string": map[string]any{"query": strings.Join(parts, " AND ")}}
}

func sortBy(newer bool) []map[string]any {
	order := "desc"
	if newer {
		order = "asc"
	}
	return []map[string]any{{"@timestamp": order}, {"_shard_doc": order}}
}

// cursor encodes a hit's sort values as the item's key; after decodes it.
func cursor(sortValues []int64) string {
	return strconv.FormatInt(sortValues[0], 10) + "|" + strconv.FormatInt(sortValues[1], 10)
}

func after(c string) []int64 {
	ts, seq, _ := strings.Cut(c, "|")
	t, _ := strconv.ParseInt(ts, 10, 64)
	n, _ := strconv.ParseInt(seq, 10, 64)
	return []int64{t, n}
}

// edge answers the page beyond a cursor, oldest first.
func (s *Screen) edge(ctx context.Context, e remote.Edge) ([]elog.Item, bool, error) {
	body := map[string]any{
		"size": e.Limit + 1, "sort": sortBy(e.Newer), "track_total_hits": false, "query": queryOf(e.Filter),
	}
	if e.Cursor != "" {
		sa := after(e.Cursor)
		switch {
		case e.Newer:
			sa = []int64{sa[0] - overlap.Milliseconds(), 0}
		case e.Inclusive:
			sa[1]++ // a descending walk from just past the anchor includes it
		}
		body["search_after"] = sa
	}
	hits, err := s.search(ctx, body)
	if err != nil {
		return nil, false, err
	}
	more := len(hits) > e.Limit
	hits = hits[:min(len(hits), e.Limit)]
	if !e.Newer {
		slices.Reverse(hits)
	}
	items := make([]elog.Item, len(hits))
	for i, h := range hits {
		items[i] = itemOf(h)
	}
	return items, more, nil
}

// find asks for the first message matching the term beyond the edge.
func (s *Screen) find(ctx context.Context, f remote.Find) (string, bool, error) {
	body := map[string]any{
		"size": 1, "sort": sortBy(f.Newer), "track_total_hits": false,
		"query": queryOf(f.Filter, fmt.Sprintf("message:%q", f.Term)),
	}
	if f.Cursor != "" {
		body["search_after"] = after(f.Cursor)
	}
	hits, err := s.search(ctx, body)
	if err != nil || len(hits) == 0 {
		return "", false, err
	}
	return cursor(hits[0].Sort), true, nil
}

// itemOf is how a document reads: level, service and message, keyed by its
// sort values and marked with its own @timestamp.
func itemOf(h hit) elog.Item {
	src := h.Source
	line := fmt.Sprintf("%-5s %-8s %s", strings.ToUpper(fmt.Sprint(src["log.level"])), src["service.name"], src["message"])
	mark := ""
	if ts, err := time.Parse(time.RFC3339Nano, fmt.Sprint(src["@timestamp"])); err == nil {
		mark = ts.Local().Format("15:04:05.000")
	}
	data := map[string]any{"_id": h.ID}
	for k, v := range src {
		data[k] = v
	}
	return elog.Item{Key: cursor(h.Sort), Mark: mark, Lines: []string{line}, Data: data}
}

// detail shows one document's fields.
type detail struct{ ins insp.Model }

func newDetail(t theme.Theme, it elog.Item) screen.Screen {
	opts := t.Inspector()
	opts.Title = "document"
	if m, ok := it.Data.(map[string]any); ok {
		opts.Title = fmt.Sprint(m["_id"])
	}
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

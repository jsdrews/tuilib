// Package eventlog demonstrates pkg/eventlog over Seekable, Growing data: an
// AWX job's events, read over HTTP from demoapi's AWX-shaped endpoints
// (/api/v2/jobs/{id}/job_events/). Events are numbered by counter, carry
// 0 to n lines of stdout, and — while the job runs — keep arriving, some
// of them saved late. L cycles the server's latency.
//
// The screen is bound with pkg/remote, so all it writes is the AWX client:
// page and find, two ordinary functions taking a context and a typed
// request. They are written the way a client of the real AWX would be:
//
//   - Unfiltered, a page is a counter range (counter__gt / counter__lte),
//     and the total is the highest counter, not count — counters missing
//     from the range are events not saved yet, shown as holes.
//   - Filtered, counters no longer line up with positions, so a page is
//     AWX's page mode, and a search hit is turned into a position with one
//     more request for count.
//   - A search asks for "the first match past this counter"
//     (stdout__icontains, counter__gt, page_size=1).
//   - A running job is done when event_processing_finished says so, not
//     when status does: the last events are saved after the job ends.
//
// The routing between the view and its source — paging as you scroll,
// following, landing search hits, dropping replies nobody waits for — is
// the binding's.
//
// New is a running job: following is on while the view is on the newest
// event; scroll up and it stops, the border counts what arrived since, and
// G returns. NewFinished is a job that has already ended — the
// troubleshooting case — opened at its first event: search (/, then n/N)
// walks the loaded events and, past them, asks AWX for the next match.
// Enter opens an event's fields in an inspector.
package eventlog

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
	elog "github.com/jsdrews/tuilib/pkg/eventlog"
	"github.com/jsdrews/tuilib/pkg/help"
	insp "github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/remote"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

const (
	pageSize = 100
	// awxPage is AWX's largest page.
	awxPage = 200
	// The fixture's jobs: one running, one long finished.
	liveJob     = 4242
	finishedJob = 4187
)

var latencies = []time.Duration{300 * time.Millisecond, 2 * time.Second, 4 * time.Second}

// New returns the demo over a job that is still running.
func New(t theme.Theme) screen.Screen { return newScreen(t, liveJob) }

// NewFinished returns the demo over a job that has already finished.
func NewFinished(t theme.Theme) screen.Screen { return newScreen(t, finishedJob) }

func newScreen(t theme.Theme, job int) screen.Screen {
	s := &Screen{t: t, job: job, live: job == liveJob, api: demoapi.From(demoapi.Options{Seed: 4})}
	s.log = remote.NewEventlog(s.options(t), remote.Seekable[elog.Item]{
		Page:     s.page,
		Find:     s.find,
		PageSize: pageSize,
		Follow:   time.Second,
	})
	if !s.live {
		// A finished job is read from its first event, not tailed.
		s.log.SetFollow(false)
	}
	return s
}

type Screen struct {
	t      theme.Theme
	log    *remote.Eventlog
	api    demoapi.Target
	job    int
	live   bool
	latIdx atomic.Int32
	// processed is set by the page function once AWX reports the job's
	// events all saved; Update turns it into SetGrowing(false).
	processed atomic.Bool
	done      bool
}

func (s *Screen) Title() string         { return "Eventlog" }
func (s *Screen) IsCapturingKeys() bool { return s.log.IsCapturingKeys() }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) Layout() layout.Node   { return layout.Sized(s.log) }
func (s *Screen) Help() []key.Binding   { return help.Flatten(s.HelpSections()) }

func (s *Screen) HelpSections() []help.Section {
	return help.SectionsOf(s.log, help.Group("Source",
		key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "cycle latency")),
	))
}

func (s *Screen) Init() tea.Cmd {
	if !s.live {
		return s.log.Init()
	}
	return tea.Batch(s.log.Init(), s.log.SetGrowing(true))
}

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
	cmd := s.log.Update(msg)
	if s.live && !s.done && s.processed.Load() {
		s.done = true
		return s, tea.Batch(cmd, s.log.SetGrowing(false), app.Info("job finished"))
	}
	return s, cmd
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	s.log.Restyle(s.options(t))
}

func (s *Screen) options(t theme.Theme) elog.Options {
	opts := t.Eventlog()
	opts.Title = fmt.Sprintf("job %d · events", s.job)
	if !s.live {
		opts.Title += " · finished"
	}
	opts.Filterable = true
	opts.Searchable = true
	return opts
}

// ---- the AWX client: the only part a real screen writes -------------------

// event is one record of /api/v2/jobs/{id}/job_events/.
type event struct {
	Counter  int    `json:"counter"`
	Event    string `json:"event"`
	Stdout   string `json:"stdout"`
	HostName string `json:"host_name"`
	Task     string `json:"task"`
	Failed   bool   `json:"failed"`
	Changed  bool   `json:"changed"`
	Created  string `json:"created"`
}

type eventPage struct {
	Count   int     `json:"count"`
	Results []event `json:"results"`
}

// get fetches path with params, as AWX would be asked.
func (s *Screen) get(ctx context.Context, path string, params url.Values, into any) error {
	params.Set("latency", latencies[s.latIdx.Load()].String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.api.URL(path+"?"+params.Encode()), nil)
	if err != nil {
		return err
	}
	resp, err := s.api.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("GET %s: %s: %s", path, resp.Status, e.Error)
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

func (s *Screen) eventsPath() string { return fmt.Sprintf("/api/v2/jobs/%d/job_events/", s.job) }

// filterParams turns the filter bar into AWX's: "failed" and "changed"
// select by outcome, host:web-1 and task:deploy scope by field, and any
// other text searches stdout.
func filterParams(f remote.Filter) url.Values {
	v := url.Values{}
	var text []string
	for _, t := range f.Terms {
		raw := t.Raw
		switch {
		case strings.EqualFold(raw, "failed"):
			v.Set("failed", "true")
		case strings.EqualFold(raw, "changed"):
			v.Set("changed", "true")
		case strings.HasPrefix(strings.ToLower(raw), "host:"):
			v.Set("host_name", raw[len("host:"):])
		case strings.HasPrefix(strings.ToLower(raw), "task:"):
			v.Set("task__icontains", raw[len("task:"):])
		default:
			text = append(text, raw)
		}
	}
	if len(text) > 0 {
		v.Set("stdout__icontains", strings.Join(text, " "))
	}
	return v
}

// page answers a window of positions.
func (s *Screen) page(ctx context.Context, w remote.Window) ([]elog.Item, int, error) {
	if s.live {
		if err := s.checkProcessed(ctx); err != nil {
			return nil, 0, err
		}
	}
	if w.Raw == "" {
		return s.pageByCounter(ctx, w)
	}
	return s.pageByPage(ctx, w)
}

// pageByCounter reads an unfiltered window as a counter range: position i
// is counter i+1, the total is the highest counter, and a counter the
// range lacks is an event not saved yet.
func (s *Screen) pageByCounter(ctx context.Context, w remote.Window) ([]elog.Item, int, error) {
	var top eventPage
	if err := s.get(ctx, s.eventsPath(), url.Values{"order_by": {"-counter"}, "page_size": {"1"}}, &top); err != nil {
		return nil, 0, err
	}
	total := 0
	if len(top.Results) > 0 {
		total = top.Results[0].Counter
	}
	end := min(w.Offset+w.Limit, total)
	got := map[int]event{}
	for lo := w.Offset; lo < end; lo += awxPage {
		var p eventPage
		params := url.Values{"order_by": {"counter"}, "page_size": {strconv.Itoa(awxPage)},
			"counter__gt": {strconv.Itoa(lo)}, "counter__lte": {strconv.Itoa(min(lo+awxPage, end))}}
		if err := s.get(ctx, s.eventsPath(), params, &p); err != nil {
			return nil, 0, err
		}
		for _, e := range p.Results {
			got[e.Counter] = e
		}
	}
	var items []elog.Item
	for pos := w.Offset; pos < end; pos++ {
		if e, ok := got[pos+1]; ok {
			items = append(items, itemOf(e))
		} else {
			c := strconv.Itoa(pos + 1)
			items = append(items, elog.Item{Key: c, Mark: c, Hole: true})
		}
	}
	return items, total, nil
}

// pageByPage reads a filtered window with AWX's page mode — a page of 200
// or two — and slices out the positions asked for.
func (s *Screen) pageByPage(ctx context.Context, w remote.Window) ([]elog.Item, int, error) {
	params := filterParams(w.Filter)
	params.Set("order_by", "counter")
	params.Set("page_size", strconv.Itoa(awxPage))
	first := w.Offset / awxPage
	var events []event
	total := 0
	for pg := first; pg*awxPage < w.Offset+w.Limit; pg++ {
		params.Set("page", strconv.Itoa(pg+1))
		var p eventPage
		if err := s.get(ctx, s.eventsPath(), params, &p); err != nil {
			return nil, 0, err
		}
		total = p.Count
		events = append(events, p.Results...)
		if (pg+1)*awxPage >= total {
			break
		}
	}
	from := w.Offset - first*awxPage
	var items []elog.Item
	for i := from; i < min(len(events), from+w.Limit); i++ {
		items = append(items, itemOf(events[i]))
	}
	return items, total, nil
}

// find asks AWX for the first match past the counter the search starts
// from. Under a filter the hit's position takes one more request: how
// many filtered events come up to it.
func (s *Screen) find(ctx context.Context, f remote.Find) (int, bool, error) {
	past, _ := strconv.Atoi(f.Cursor)
	if past == 0 {
		past = f.From + 1
	}
	params := filterParams(f.Filter)
	params.Set("page_size", "1")
	params.Set("stdout__icontains", strings.TrimSpace(params.Get("stdout__icontains")+" "+f.Term))
	if f.Newer {
		params.Set("order_by", "counter")
		params.Set("counter__gt", strconv.Itoa(past))
	} else {
		params.Set("order_by", "-counter")
		params.Set("counter__lt", strconv.Itoa(past))
	}
	var hit eventPage
	if err := s.get(ctx, s.eventsPath(), params, &hit); err != nil || len(hit.Results) == 0 {
		return 0, false, err
	}
	counter := hit.Results[0].Counter
	if f.Raw == "" {
		return counter - 1, true, nil
	}
	upTo := filterParams(f.Filter)
	upTo.Set("page_size", "1")
	upTo.Set("counter__lte", strconv.Itoa(counter))
	var p eventPage
	if err := s.get(ctx, s.eventsPath(), upTo, &p); err != nil {
		return 0, false, err
	}
	return p.Count - 1, true, nil
}

// checkProcessed notes when AWX has saved every event of the running job.
func (s *Screen) checkProcessed(ctx context.Context) error {
	var job struct {
		Processed bool `json:"event_processing_finished"`
	}
	if err := s.get(ctx, fmt.Sprintf("/api/v2/jobs/%d/", s.job), url.Values{}, &job); err != nil {
		return err
	}
	if job.Processed {
		s.processed.Store(true)
	}
	return nil
}

// itemOf is how an event reads in the timeline: its stdout, one line per
// line, keyed and marked by its counter.
func itemOf(e event) elog.Item {
	c := strconv.Itoa(e.Counter)
	var lines []string
	if e.Stdout != "" {
		lines = strings.Split(strings.ReplaceAll(e.Stdout, "\r\n", "\n"), "\n")
	}
	return elog.Item{Key: c, Mark: c, Lines: lines, Data: map[string]any{
		"counter": e.Counter, "event": e.Event, "host": e.HostName, "task": e.Task,
		"failed": e.Failed, "changed": e.Changed, "created": e.Created,
	}}
}

// detail shows one event's fields.
type detail struct {
	ins insp.Model
}

func newDetail(t theme.Theme, it elog.Item) screen.Screen {
	opts := t.Inspector()
	opts.Title = "event " + it.Key
	if m, ok := it.Data.(map[string]any); ok {
		m["stdout"] = strings.Join(it.Lines, "\n")
		opts.Fields = insp.FromMap(m)
	}
	return &detail{ins: insp.New(opts)}
}

func (d *detail) Title() string         { return "Event" }
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

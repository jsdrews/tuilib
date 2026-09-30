// Package eventlog demonstrates pkg/eventlog over Seekable, Growing data:
// a fake job shaped like an AWX job's events. Each event has a counter, 0
// to n lines of output and some fields; the job keeps emitting while it
// runs, answers one page at a time with latency (L cycles it), and can
// search its own output.
//
// The loop, in this file:
//
//	ViewportChangedMsg → src.SetHeld + src.Viewport  (pages, after a settle)
//	QueryChangedMsg    → src.SetQuery                (the filter narrows)
//	FindMsg            → src.Find                    (n/N past what is held)
//	RequestMsg         → fetch                       (the one place I/O happens)
//	fetchedMsg         → src.Deliver, then Found / SetFailed / SetPage
//
// Following is on while the view is on the newest event: new events arrive
// under it. Scroll up and it stops; the border counts what arrived since,
// and G returns. Enter opens the event's fields in an inspector.
//
// NewFinished is the same screen over a job that has already ended — the
// troubleshooting case. Nothing grows, nothing polls, and it opens at the
// first event: search (/, then n/N) walks the loaded events and, past
// them, asks the job for the next match, which is how you find the one
// failure in 8,000 events without paging through them.
package eventlog

import (
	"context"
	"errors"
	"fmt"
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
	pageSize = 100
	// The job emits this many events a second until it reaches jobLength.
	rate      = 8
	jobLength = 3000
	// A finished job is larger: there is no waiting for it to fill up.
	finishedLength = 8000
)

var latencies = []time.Duration{300 * time.Millisecond, 2 * time.Second, 4 * time.Second}

// New returns the demo over a job that is still running.
func New(t theme.Theme) screen.Screen { return newScreen(t, true) }

// NewFinished returns the demo over a job that has already finished.
func NewFinished(t theme.Theme) screen.Screen { return newScreen(t, false) }

func newScreen(t theme.Theme, live bool) screen.Screen {
	s := &Screen{
		live:    live,
		done:    !live,
		started: time.Now(),
		src: source.New(source.Options{
			PageSize: pageSize,
			MaxHeld:  elog.DefaultMaxItems,
			Follow:   time.Second,
		}),
	}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t       theme.Theme
	log     elog.Model
	src     source.Model
	started time.Time
	latIdx  int
	live    bool
	done    bool
}

func (s *Screen) Title() string         { return "Eventlog" }
func (s *Screen) IsCapturingKeys() bool { return s.log.IsCapturingKeys() }

func (s *Screen) Init() tea.Cmd {
	if !s.live {
		return s.src.Init()
	}
	s.log.SetGrowing(true)
	return tea.Batch(s.src.Init(), s.src.SetGrowing(true))
}

func (s *Screen) OnEnter(any) tea.Cmd { return nil }

func (s *Screen) Layout() layout.Node { return layout.Sized(&s.log) }

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

func (s *Screen) HelpSections() []help.Section {
	return help.SectionsOf(&s.log, help.Group("Source",
		key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "cycle latency")),
	))
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
		s.src.SetHeld(m.HeldStart, m.HeldCount)
		cmds = append(cmds, s.src.Viewport(m.FirstVisible, m.LastVisible))

	case elog.QueryChangedMsg:
		cmds = append(cmds, s.src.SetQuery(m.Raw, m.Terms, "", false))

	case elog.FindMsg:
		dir := source.Older
		if m.Newer {
			dir = source.Newer
		}
		cmds = append(cmds, s.src.Find(m.Term, dir, m.From))

	case source.RequestMsg:
		return s, s.fetch(m.Query)

	case fetchedMsg:
		ok, cmd := s.src.Deliver(m.page)
		a := elog.Answer{Raw: m.query.Raw}
		switch {
		case !ok:
		case m.query.Find:
			s.log.Found(m.page.Found, m.page.Offset)
		case m.page.Err != nil:
			s.log.SetFailed(a)
		default:
			s.log.SetPage(m.items, m.page.Offset, m.page.Total, a)
			if !s.done && s.emitted() >= jobLength {
				s.done = true
				s.log.SetGrowing(false)
				cmds = append(cmds, s.src.SetGrowing(false), app.Info("job finished"))
			}
		}
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

// emitted is how many events the job has produced so far.
func (s *Screen) emitted() int {
	if !s.live {
		return finishedLength
	}
	return min(jobLength, int(time.Since(s.started).Seconds()*rate)+40)
}

// fetch answers q from the fake job after the configured latency, under
// q.Ctx — a superseded request is abandoned, as a real HTTP call would be.
func (s *Screen) fetch(q source.Query) tea.Cmd {
	lat, total := latencies[s.latIdx], s.emitted()
	return func() tea.Msg {
		select {
		case <-q.Ctx.Done():
			return fetchedMsg{query: q, page: source.Page{Gen: q.Gen, Err: context.Canceled}}
		case <-time.After(lat):
		}
		match := matcher(q.Raw)
		// The filter narrows the set; offsets are into what survives it.
		var ids []int
		for i := 1; i <= total; i++ {
			if match(i) {
				ids = append(ids, i)
			}
		}
		if q.Find {
			term := strings.ToLower(q.Term)
			step, from := 1, q.Offset+1
			if q.Dir == source.Older {
				step, from = -1, q.Offset-1
			}
			for j := from; j >= 0 && j < len(ids); j += step {
				if strings.Contains(strings.ToLower(strings.Join(event(ids[j]).Lines, "\n")), term) {
					return fetchedMsg{query: q, page: source.Page{Gen: q.Gen, Found: true, Offset: j}}
				}
			}
			return fetchedMsg{query: q, page: source.Page{Gen: q.Gen}}
		}
		if strings.Contains(q.Raw, "boom") {
			return fetchedMsg{query: q, page: source.Page{Gen: q.Gen, Err: errors.New("fetch events: 503 Service Unavailable")}}
		}
		end := min(q.Offset+q.Limit, len(ids))
		var items []elog.Item
		for j := q.Offset; j < end; j++ {
			items = append(items, event(ids[j]))
		}
		return fetchedMsg{
			query: q,
			page:  source.Page{Gen: q.Gen, Offset: q.Offset, Count: len(items), Total: len(ids)},
			items: items,
		}
	}
}

// matcher is the server's filter: "failed", "changed", or a host name.
func matcher(raw string) func(int) bool {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return func(int) bool { return true }
	}
	return func(i int) bool {
		return strings.Contains(strings.ToLower(strings.Join(event(i).Lines, "\n")), raw)
	}
}

var hosts = []string{"web-1", "web-2", "db-1", "cache-1"}
var tasks = []string{"Gathering Facts", "install packages", "render config", "restart service", "wait for health"}

// event is the job's event i: a task header every so often, a result line
// per host, and now and then an event with no output at all.
func event(i int) elog.Item {
	key := fmt.Sprint(i)
	mark := key // AWX's counter: stable, and what you'd quote
	task := tasks[(i/5)%len(tasks)]
	host := hosts[i%len(hosts)]
	status := "ok"
	switch {
	case i%37 == 0:
		status = "failed"
	case i%7 == 0:
		status = "changed"
	}
	data := map[string]any{"counter": i, "task": task, "host": host, "status": status}
	switch {
	case i%5 == 0:
		return elog.Item{Key: key, Mark: mark, Data: data, Lines: []string{
			"",
			fmt.Sprintf("TASK [%s] %s", task, strings.Repeat("*", 30)),
		}}
	case i%11 == 0:
		return elog.Item{Key: key, Mark: mark, Data: data} // a runner_on_start: no output
	case status == "failed":
		return elog.Item{Key: key, Mark: mark, Data: data, Lines: []string{
			fmt.Sprintf("fatal: [%s]: FAILED! => {\"msg\": \"timeout waiting for %s\"}", host, task),
			"  retrying in 5s",
		}}
	}
	return elog.Item{Key: key, Mark: mark, Data: data, Lines: []string{fmt.Sprintf("%s: [%s]", status, host)}}
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	prev, rebuilt := s.log, s.log.Title() != ""
	opts := t.Eventlog()
	opts.Title = "job 4242 · events"
	if !s.live {
		opts.Title = "job 4187 · events · finished"
	}
	opts.Filterable = true
	opts.Searchable = true
	s.log = elog.New(opts)
	if !s.live {
		// A finished job is read from its first event, not tailed.
		s.log.SetFollow(false)
	}
	if !rebuilt {
		return
	}
	s.log.SetValue(prev.Value())
	s.log.SetTerm(prev.Term())
	s.log.SetGrowing(!s.done)
	if a, ok := prev.Answered(); ok {
		start, items := prev.Items()
		s.log.SetPage(items, start, s.src.Total(), a)
	}
	if !prev.Following() {
		s.log.SetCursor(prev.Cursor())
	}
}

// detail shows one event's fields.
type detail struct {
	ins insp.Model
}

func newDetail(t theme.Theme, it elog.Item) screen.Screen {
	opts := t.Inspector()
	opts.Title = "event " + it.Key
	if m, ok := it.Data.(map[string]any); ok {
		m["output"] = strings.Join(it.Lines, "\n")
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

// Package liveops is the live-data showcase: everything that changes while
// you watch, on one screen, so a change that breaks an older feature shows up
// where you can see it.
//
//	┌ applications ──────────────────────┐┌ workloads ─────────────────────┐
//	│ demoapi over HTTP: per-row         ││ pkg/metrics: Ratio, Badge,     │
//	│ activity, marking, the verbs       ││ Bar, Spark                     │
//	└────────────────────────────────────┘└────────────────────────────────┘
//	┌ events ──────────────────────────────────────────────────────────────┐
//	│ what each poll changed, searchable                                   │
//	└──────────────────────────────────────────────────────────────────────┘
//
// What it exercises:
//
//   - One pkg/poll drives every pane (rule 24): each tick fetches the
//     applications from demoapi and advances the workloads. Both tables swap
//     their rows by key, so the cursor stays on its row while the order moves
//     under it. The verbs on the workloads and events panes pause and resume
//     the poll, refresh now, and change its cadence.
//   - Loading (rule 17): every pane shows its spinner until its first data
//     lands, and "Reload" on the workloads pane clears it and shows it again.
//   - Row activity (rule 33) on the applications, against an API shaped like
//     Argo CD. The server is the source of truth: rows spin because the
//     server says so (demoapi schedules syncs of its own every few seconds),
//     and when the user starts work the row is told at once. One decision
//     per verb — who knows when the work ends:
//     Sync is Observed (the POST answers at once; the phase says when it
//     ends). Refresh is Held (a GET that blocks until reconciled and shows
//     nothing meanwhile). Sync and follow is Held on a job handle it streams.
//     Quick sync ends between two polls and is reported with UnobservedMsg.
//     Fail a sync shows the failure path, and Sync (dropped) a job that was
//     accepted and never ran.
//   - Reads that fail are reported once, and the title carries "reads
//     failing" beside "refreshed Ns ago", which keeps counting.
//   - pkg/metrics' inline primitives in table cells, coloured with
//     foreground-only escapes so the selected row's background runs through
//     them (rule 19).
//   - A capped, searchable logview fed by what changed (rule 13).
//   - focus.Group over the three panes; keys go to the focused pane, the
//     mouse and everything else to all of them (rule 6).
package liveops

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/demoapi"
	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/poll"
	"github.com/jsdrews/tuilib/pkg/runner"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

const (
	pollInterval = 2 * time.Second
	minInterval  = 500 * time.Millisecond
	maxInterval  = 10 * time.Second
	appCount     = 6
	eventsCap    = 500
)

type Screen struct {
	t      theme.Theme
	table  table.Model // the applications
	work   table.Model // the workloads
	events logview.Model
	focus  focus.Group
	poll   poll.Model

	api  demoapi.Target
	apps []appRow
	// ops maps a run's tag to the operations it dispatched — one per row, so
	// the server refusing one row stops that row alone — so the run's answer
	// can close them.
	ops map[string][]activity.Op

	workloads []workload
	rng       *rand.Rand
	// loaded says each pane has had its first data, which is what ends its
	// spinner and what SetTheme carries.
	appsLoaded, workLoaded, eventsLoaded bool
	// fetchingWork stops a slow workloads fetch from being asked for twice.
	fetchingWork bool
}

// New returns the live-ops screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{
		// In-process by default, or a running demoapi when TUILIB_DEMO_API is
		// set. A short schedule, so rows spin because the *server* said so
		// while you are watching.
		api:       demoapi.From(demoapi.Options{Seed: 11, Apps: appCount, Schedule: 3 * time.Second}),
		poll:      poll.New(poll.Options{Interval: pollInterval}),
		ops:       map[string][]activity.Op{},
		workloads: seedWorkloads(),
		rng:       rand.New(rand.NewSource(7)),
	}
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string         { return "Live ops" }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.focus.IsCapturingKeys() }

func (s *Screen) Init() tea.Cmd {
	return tea.Batch(
		s.focus.Init(), s.poll.Init(), titleTick(),
		s.table.SetLoading(true), s.work.SetLoading(true), s.events.SetLoading(true),
		s.fetch(), s.fetchWork(),
	)
}

// titleTickMsg re-renders the titles once a second, so "refreshed Ns ago"
// counts between polls — and through an outage, when no poll lands.
type titleTickMsg struct{}

func titleTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return titleTickMsg{} })
}

// fetchedMsg carries one poll's worth of rows and the token its fetch was
// stamped with when issued.
type fetchedMsg struct {
	read activity.Read
	apps []appRow
	err  error
}

func (s *Screen) fetchWork() tea.Cmd {
	if s.fetchingWork {
		return nil
	}
	s.fetchingWork = true
	return fetchWorkloads(s.workloads, s.rng)
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmds []tea.Cmd
	var c tea.Cmd
	cmds = append(cmds, s.poll.Update(msg))
	s.focus, c = s.focus.Update(msg)
	cmds = append(cmds, c)

	switch m := msg.(type) {
	case poll.RefreshMsg:
		cmds = append(cmds, s.fetch(), s.fetchWork())

	case titleTickMsg:
		s.refreshTitles()
		cmds = append(cmds, titleTick())

	case action.ChosenMsg:
		v, ok := verbs[m.Action.Label]
		if !ok || m.Action.Run == nil {
			break
		}
		// One operation per row, so one refusal stops one row.
		var ops []activity.Op
		for _, k := range m.Targets {
			op, cmd := s.table.Dispatch([]string{k}, v.label, v.mode)
			ops = append(ops, op)
			cmds = append(cmds, cmd)
		}
		s.ops[action.RunKey(m.Action, m.Target)] = ops

	case runner.Captured:
		// Under the shell, an action's Run answering arrives as this. The
		// shell has already posted the run's receipt or its error; what is
		// left is closing each row's operation with that row's own result.
		ops, ours := s.ops[m.Tag]
		if !ours {
			break
		}
		delete(s.ops, m.Tag)
		ended := false
		for _, op := range ops {
			if s.table.Done(op, errFor(m.Err, op.Keys[0])) == activity.Ended {
				ended = true
			}
		}
		if ended {
			// The work is over and the rows do not know yet; ask now rather
			// than leave them settled on the pre-refresh value for a poll.
			cmds = append(cmds, s.poll.Refresh())
		}

	case activity.UnobservedMsg:
		// Every Observed verb here is a sync, so name it; m.Label is the
		// claim's label, "Running", not the verb.
		if m.Changed {
			cmds = append(cmds, app.Info("Sync finished between polls"))
		} else {
			cmds = append(cmds, app.Info("Sync requested — no change observed"))
		}

	case fetchedMsg:
		cmds = append(cmds, s.applyApps(m))

	case workloadsMsg:
		s.fetchingWork = false
		s.workloads = m.rows
		s.work.SetKeyedRows(workloadRows(m.rows))
		s.logEvents(m.events)
		if !s.workLoaded {
			s.workLoaded = true
			cmds = append(cmds, s.work.SetLoading(false))
		}
		s.refreshTitles()
	}

	if _, isKey := msg.(tea.KeyMsg); isKey {
		cmds = append(cmds, s.forwardFocused(msg))
	} else {
		cmds = append(cmds, s.forwardAll(msg)...)
	}
	return s, tea.Batch(cmds...)
}

// applyApps lands a read of the applications: rows when it was taken in
// order and succeeded, and the outage otherwise.
func (s *Screen) applyApps(m fetchedMsg) tea.Cmd {
	wasFailing := s.table.ReadsFailing()
	if !s.table.ApplyRead(m.read, rowsOf(m.apps), m.err) {
		var cmd tea.Cmd
		if m.err != nil && !wasFailing {
			// Once, when reads start failing; the title says it for as long
			// as the outage lasts.
			cmd = app.ErrorOf(m.err)
		}
		s.refreshTitles()
		return cmd
	}
	s.logEvents(appChanges(s.apps, m.apps))
	s.apps = m.apps
	s.poll.MarkRefreshed()
	var cmds []tea.Cmd
	if !s.appsLoaded {
		s.appsLoaded = true
		cmds = append(cmds, s.table.SetLoading(false))
	}
	if !s.eventsLoaded {
		s.eventsLoaded = true
		s.events.Append(stamp(fmt.Sprintf("%d applications loaded", len(m.apps))))
		cmds = append(cmds, s.events.SetLoading(false))
	}
	s.refreshTitles()
	return tea.Batch(cmds...)
}

// appChanges is what moved between two reads. Sync and health only: the
// phase is what the row's indicator already shows.
func appChanges(prev, next []appRow) []string {
	was := map[string]appRow{}
	for _, a := range prev {
		was[a.ID] = a
	}
	var out []string
	for _, a := range next {
		p, ok := was[a.ID]
		if !ok {
			continue
		}
		if p.Sync != a.Sync {
			out = append(out, fmt.Sprintf("%s: %s → %s", a.Name, p.Sync, a.Sync))
		}
		if p.Health != a.Health {
			out = append(out, fmt.Sprintf("%s: health %s → %s", a.Name, p.Health, a.Health))
		}
	}
	return out
}

func (s *Screen) logEvents(lines []string) {
	for _, l := range lines {
		s.events.Append(stamp(l))
	}
}

func stamp(line string) string { return time.Now().Format("15:04:05") + "  " + line }

func (s *Screen) forwardFocused(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch {
	case s.focus.Is(&s.table):
		s.table, cmd = s.table.Update(msg)
	case s.focus.Is(&s.work):
		s.work, cmd = s.work.Update(msg)
	case s.focus.Is(&s.events):
		s.events, cmd = s.events.Update(msg)
	}
	return cmd
}

// forwardAll hands mouse events, timers and everything else to every pane, so
// a covered pane keeps its spinner ticking and a click lands where it fell.
func (s *Screen) forwardAll(msg tea.Msg) []tea.Cmd {
	cmds := make([]tea.Cmd, 3)
	s.table, cmds[0] = s.table.Update(msg)
	s.work, cmds[1] = s.work.Update(msg)
	s.events, cmds[2] = s.events.Update(msg)
	return cmds
}

func (s *Screen) Layout() layout.Node {
	return layout.VStack(
		layout.Flex(3, layout.HStack(
			layout.Flex(4, layout.Sized(&s.table)),
			layout.Flex(5, layout.Sized(&s.work)),
		)),
		layout.Flex(2, layout.Sized(&s.events)),
	)
}

func (s *Screen) Help() []key.Binding          { return s.focus.Help() }
func (s *Screen) HelpSections() []help.Section { return s.focus.HelpSections() }

// Actions satisfies action.Provider: the application verbs on the
// applications, and the poll's on the other two panes.
func (s *Screen) Actions() action.Set {
	if s.focus.Is(&s.table) {
		return s.appActions()
	}
	return s.pollActions()
}

func (s *Screen) pollActions() action.Set {
	set := action.Set{Target: "refresh"}
	if s.poll.Paused() {
		set.Actions = append(set.Actions, action.Action{Label: "Resume", Desc: "poll again", Group: "poll", Do: s.poll.Resume})
	} else {
		set.Actions = append(set.Actions, action.Action{Label: "Pause", Desc: "stop polling", Group: "poll", Do: func() tea.Cmd {
			s.poll.Pause()
			s.refreshTitles()
			return nil
		}})
	}
	set.Actions = append(set.Actions,
		action.Action{Label: "Refresh now", Group: "poll", Do: s.poll.Refresh},
		action.Action{Label: "Faster", Desc: "halve the interval", Group: "poll", Do: func() tea.Cmd {
			return s.setInterval(max(minInterval, s.poll.Interval()/2))
		}},
		action.Action{Label: "Slower", Desc: "double the interval", Group: "poll", Do: func() tea.Cmd {
			return s.setInterval(min(maxInterval, s.poll.Interval()*2))
		}},
		action.Action{Label: "Reload workloads", Desc: "show the spinner again", Group: "loading", Do: func() tea.Cmd {
			s.work.SetKeyedRows(nil)
			s.workLoaded = false
			return tea.Batch(s.work.SetLoading(true), s.fetchWork())
		}},
		action.Action{Label: "Clear events", Group: "loading", Do: func() tea.Cmd {
			s.events.Clear()
			return nil
		}},
	)
	return set
}

func (s *Screen) setInterval(d time.Duration) tea.Cmd {
	cmd := s.poll.SetInterval(d)
	s.refreshTitles()
	return tea.Batch(cmd, app.Info("Polling every "+d.String()))
}

func (s *Screen) refreshTitles() {
	s.table.SetTitle(s.appsTitle())
	s.work.SetTitle("workloads · " + s.cadence())
	s.events.SetTitle("events")
}

func (s *Screen) appsTitle() string {
	name := "applications"
	if s.api.Live {
		name += " · live"
	}
	if n := s.table.ActivityCount(); n > 0 {
		name += fmt.Sprintf(" · %d working", n)
	}
	// Both, never one instead of the other: "reads failing" says the rows
	// can't be refreshed, and "refreshed Ns ago" says how stale they are —
	// counted from the last read that landed, so it climbs through an outage.
	if s.table.ReadsFailing() {
		name += " · reads failing"
	}
	if last := s.poll.LastRefresh(); !last.IsZero() {
		name += fmt.Sprintf(" · refreshed %ds ago", int(time.Since(last).Seconds()))
	}
	return name
}

func (s *Screen) cadence() string {
	if s.poll.Paused() {
		return "paused"
	}
	return "every " + s.poll.Interval().String()
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	ast := s.table.State()
	o := t.Table()
	o.Filterable = true
	o.Markable = true
	o.Filter.Placeholder = "filter…"
	o.Columns = []table.Column{
		{Title: "Name", Width: 14, Flex: 1},
		// Fixed, and wide enough for "⣾ Refreshing": widths come from the
		// rows, never from the indicator.
		{Title: "Sync", Width: 14},
		{Title: "Health", Width: 11},
	}
	o.ActivityColumn = "Sync"
	o.BusyWhen = func(r table.KeyedRow) (string, bool) {
		phase := r.Data.(appRow).Phase
		return phase, phase == demoapi.PhaseRunning
	}
	// Activity.Settle stays 0: demoapi reports the phase inline, the moment a
	// sync is accepted. Against a real Argo controller, which takes a moment
	// to pick an operation up, set it to 1 or 2.
	// Revision moves whenever an operation on the app finishes, so a Quick
	// sync that ends where it began still reads as done on the first poll
	// after it. Argo's equivalent is operationState.finishedAt.
	o.Revision = func(r table.KeyedRow) string { return strconv.FormatInt(r.Data.(appRow).Rev, 10) }
	s.table = table.New(o)
	s.table.SetKeyedRows(s.rows())
	s.table.Restore(ast)

	wst := s.work.State()
	wo := t.Table()
	wo.Filterable = true
	wo.Filter.Placeholder = "filter…"
	wo.Columns = []table.Column{
		{Title: "Workload", Width: 14},
		{Title: "Ready", Width: 5, Sortable: true, Less: ratioLess, Align: lipgloss.Right},
		{Title: "Pods", Width: 8},
		{Title: "CPU", Width: 11, Sortable: true, Less: cpuLess},
		{Title: "History", Width: sparkWidth},
	}
	s.work = table.New(wo)
	if s.workLoaded {
		s.work.SetKeyedRows(workloadRows(s.workloads))
	}
	s.work.Restore(wst)

	est := s.events.State()
	eo := t.Logview()
	eo.Searchable = true
	eo.MaxLines = eventsCap
	eo.Filter.Placeholder = "search…"
	s.events = logview.New(eo)
	s.events.Restore(est)

	at := s.focus.Index()
	s.focus = focus.NewGroup(&s.table, &s.work, &s.events).WithKeys(focus.Keys{
		Next: key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "next pane")),
		Prev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "prev pane")),
	})
	s.focus.SetIndex(at)
	s.refreshTitles()
}

// stripped is a rendered cell without its colour, for the comparators.
func stripped(s string) string { return strings.TrimSpace(xansi.Strip(s)) }

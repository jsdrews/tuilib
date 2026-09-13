// Package activity demonstrates per-row in-flight state: the spinner and
// status label a row shows while something is working on it.
//
// The shape is an argocd/AWX dashboard — applications with a sync status that
// changes for three different reasons, all of which have to look right:
//
//  1. **You did it.** Press "a" and pick Sync. The row starts spinning before
//     any request has been answered, because the shell broadcasts the action's
//     Busy label against the keys the screen put in action.Set.Targets. This
//     screen writes one extra line to get that — Targets — and no wiring at
//     all.
//
//  2. **Something else did it.** Every few seconds a scheduled sync fires on
//     its own. Nobody pressed anything, so nothing was broadcast; the row
//     spins because Options.ActivityWhen recognises "Syncing" in the data the
//     poll brought back. A read-only dashboard gets this and needs no actions.
//
//  3. **It happened while nobody was looking.** Some operations finish between
//     two polls, so the TUI never observes them running. Those rows flash "•"
//     instead, driven by Options.ActivityRevision over a hidden column — the
//     only way to notice a change the sampled data carries no other evidence
//     of.
//
// # The case worth watching for
//
// Refresh is deliberately instant server-side: it is done long before the next
// poll. Press it and the row keeps spinning anyway, until the poll lands. That
// is not a lie about the work — it is the truth about what the TUI knows. The
// alternative, clearing the moment the request returns, shows a tick and then
// a cell that reads exactly as it did before the user acted, which is
// indistinguishable from nothing having happened.
//
// Sync is the opposite: slow enough server-side that the poll catches it, so
// the local "syncing" hands over to the data's own "Syncing" mid-flight and
// the row never stops moving.
//
// # What this screen does not contain
//
// No spinner. No ticker. No re-pushing rows per frame to animate anything. The
// rows are pushed once per poll, exactly as the poll example pushes them, and
// the indicator is drawn over them by the component.
package activity

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/ansi"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/poll"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

const pollInterval = 2 * time.Second

// Column order. Rev is hidden: it is the revision ActivityRevision watches,
// and "the status worth watching is not always the status worth showing".
const (
	colName = iota
	colSync
	colHealth
	colRev
)

// Sync states. The two the predicate treats as in-flight are the two a server
// reports while it is working.
const (
	syncSynced    = "Synced"
	syncOutOfSync = "OutOfSync"
	syncSyncing   = "Syncing"
	syncRefresh   = "Refreshing"
)

// app is one row's server-side truth.
type app struct {
	name   string
	sync   string
	health string
	rev    int64
}

// job is work the fake server is doing. ticks is how many polls it survives —
// zero means it finishes before the next one, which is the case decision 19 of
// docs/activity.md exists for.
type job struct {
	label  string
	ticks  int
	result string
	health string
}

// backend stands in for the API. It is mutex-guarded because an action.Func
// runs on its own goroutine: Sync and Refresh are called from there, while
// tick and snapshot are called from Update. That is not test scaffolding — it
// is the same discipline a real client needs, and the reason an action is
// handed a context and a writer rather than the model.
type backend struct {
	mu   sync.Mutex
	apps []app
	jobs map[string]*job
	rng  *rand.Rand
}

func newBackend() *backend {
	return &backend{
		apps: []app{
			{"api-server", syncSynced, "Healthy", 1},
			{"web-frontend", syncOutOfSync, "Degraded", 1},
			{"worker-pool", syncSynced, "Healthy", 1},
			{"cache-redis", syncSynced, "Healthy", 1},
			{"metrics-agent", syncOutOfSync, "Progressing", 1},
		},
		jobs: map[string]*job{},
		rng:  rand.New(rand.NewSource(7)),
	}
}

// Sync starts slow server-side work — slow enough that the poll observes it,
// so the local indicator hands over to the data mid-flight.
func (b *backend) Sync(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.jobs[name] = &job{label: syncSyncing, ticks: 2, result: syncSynced, health: "Healthy"}
}

// Refresh is instant server-side: it is finished long before the next poll can
// see it. The row keeps its indicator until that poll lands anyway.
func (b *backend) Refresh(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.jobs[name] = &job{label: syncRefresh, ticks: 0, result: syncOutOfSync, health: "Progressing"}
}

// tick advances the server by one poll interval and occasionally invents work
// nobody asked for.
func (b *backend) tick() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for name, j := range b.jobs {
		if j.ticks > 0 {
			j.ticks--
			continue
		}
		for i := range b.apps {
			if b.apps[i].name == name {
				b.apps[i].sync = j.result
				b.apps[i].health = j.health
				b.apps[i].rev++
			}
		}
		delete(b.jobs, name)
	}

	switch n := b.rng.Intn(6); {
	case n == 0:
		// A schedule fires. Slow, so it is visible as a spinner nobody started.
		a := &b.apps[b.rng.Intn(len(b.apps))]
		if _, busy := b.jobs[a.name]; !busy {
			b.jobs[a.name] = &job{label: syncSyncing, ticks: 2, result: syncSynced, health: "Healthy"}
		}
	case n == 1:
		// Something completed entirely between two polls. The only evidence is
		// the revision, which is what the flash is for.
		a := &b.apps[b.rng.Intn(len(b.apps))]
		if _, busy := b.jobs[a.name]; !busy {
			a.rev++
			if a.health == "Healthy" {
				a.health = "Degraded"
			} else {
				a.health = "Healthy"
			}
		}
	}
}

// snapshot is what a list endpoint would return: current state, with in-flight
// work reported as a status like any other.
func (b *backend) snapshot() []app {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]app, len(b.apps))
	copy(out, b.apps)
	for i := range out {
		if j, ok := b.jobs[out[i].name]; ok {
			out[i].sync = j.label
		}
	}
	return out
}

// Screen is a table over the backend, plus a poll. Everything about the
// indicators is declared in SetTheme and Actions; nothing here drives them.
type Screen struct {
	t     theme.Theme
	table table.Model
	poll  poll.Model
	back  *backend
}

// New returns the activity demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{
		back: newBackend(),
		poll: poll.New(poll.Options{Interval: pollInterval}),
	}
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string         { return "Activity" }
func (s *Screen) Init() tea.Cmd         { return s.poll.Init() }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.table.IsCapturingKeys() }
func (s *Screen) Layout() layout.Node   { return layout.Sized(&s.table) }

// Update forwards everything to both the table and the poll. The activity
// broadcasts the shell sends arrive here like any other message and reach the
// table because rule 6 says forward all of them — which is the whole of this
// screen's involvement in the feature.
func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmds []tea.Cmd

	cmds = append(cmds, s.poll.Update(msg))

	if _, ok := msg.(poll.RefreshMsg); ok {
		s.back.tick()
		s.table.SetKeyedRows(s.rows())
		s.poll.MarkRefreshed()
	}

	var tcmd tea.Cmd
	s.table, tcmd = s.table.Update(msg)
	cmds = append(cmds, tcmd)

	return s, tea.Batch(cmds...)
}

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

func (s *Screen) HelpSections() []help.Section {
	return help.SectionsOf(&s.table, help.Group("Applications",
		key.NewBinding(key.WithKeys("mouse:right"), key.WithHelp("right-click", "actions")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	))
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	cursor, value := s.table.Cursor(), s.table.Value()
	act := s.table.ActivityState()

	o := t.Table()
	o.Title = "applications"
	o.Filterable = true
	o.Filter.Placeholder = "filter…"
	o.Columns = []table.Column{
		{Title: "Name", Width: 16},
		// Fixed, and wide enough for the longest label it will carry: widths
		// come from the rows, so an indicator is truncated to fit rather than
		// widening anything, and an auto-sized column fitted to "OutOfSync"
		// would show the glyph and swallow the word.
		{Title: "Sync", Width: 14},
		{Title: "Health", Width: 13},
		{Title: "Rev", Hidden: true},
	}

	// The Sync cell is what changes: Synced → ⠹ syncing → Synced reads as one
	// cell changing its mind, rather than decoration appearing beside it.
	o.ActivityColumn = "Sync"

	// Derived state, for work this session did not start.
	o.ActivityWhen = func(c table.Row) (string, bool) {
		return activity.Busy(syncSyncing, syncRefresh)(c[colSync])
	}

	// And for work it never even saw running.
	o.ActivityRevision = func(c table.Row) string { return c[colRev] }

	s.table = table.New(o)
	s.table.SetKeyedRows(s.rows())
	if value != "" {
		s.table.SetValue(value)
	}
	s.table.SetCursor(cursor)
	// Rule 4: carry the work in flight across the rebuild along with the
	// cursor and the filter. The returned tick is dropped here only because
	// SetTheme has nowhere to return one; the next poll re-arms it.
	_ = s.table.SetActivityState(act)
}

// rows are keyed by name, which is what lets an indicator stay on its own row
// when a refresh reorders the set — and what Selection() reports for Targets.
func (s *Screen) rows() []table.KeyedRow {
	apps := s.back.snapshot()
	out := make([]table.KeyedRow, len(apps))
	for i, a := range apps {
		out[i] = table.KeyedRow{
			Key: a.name,
			Cells: []string{
				a.name,
				a.sync,
				healthCell(a.health),
				strconv.FormatInt(a.rev, 10),
			},
		}
	}
	return out
}

// healthCell colours through pkg/ansi rather than lipgloss, so the selected
// row's background survives the cell (rule 19).
func healthCell(h string) string {
	switch h {
	case "Healthy":
		return ansi.CellColor(2, h)
	case "Degraded":
		return ansi.CellColor(1, h)
	default:
		return ansi.CellColor(3, h)
	}
}

// Actions is this screen's whole contribution to the indicators it shows for
// work the user starts: Targets, and a Busy label per verb.
func (s *Screen) Actions() action.Set {
	targets := s.table.Selection()
	if len(targets) == 0 {
		return action.Set{}
	}
	name := targets[0]

	return action.Set{
		Target: s.table.SelectionLabel(),
		// The keys, as opposed to the label above. Without these the verbs
		// still run and still log; the rows just never say so.
		Targets: targets,
		Count:   len(targets),
		Actions: []action.Action{
			{
				Label: "Sync",
				Desc:  "reconcile to git",
				// What the row says. Without it the label is used, lowercased.
				Busy:      "syncing",
				Exclusive: true,
				Run: func(ctx context.Context, out io.Writer) error {
					fmt.Fprintf(out, "sync requested for %s\n", name)
					// Progress rides the writer the action already holds. It
					// relabels the row and is deliberately not logged: a run
					// reporting progress ten times would otherwise post ten
					// records into an event the badge counts as one.
					for i, phase := range []string{"comparing", "applying", "pruning"} {
						select {
						case <-ctx.Done():
							return ctx.Err()
						case <-time.After(400 * time.Millisecond):
						}
						activity.Progress(out, fmt.Sprintf("%s %d/3", phase, i+1))
						fmt.Fprintf(out, "%s\n", phase)
					}
					s.back.Sync(name)
					fmt.Fprintln(out, "operation submitted")
					return nil
				},
			},
			{
				Label: "Refresh",
				Desc:  "re-read from git",
				Busy:  "refreshing",
				// Returns almost at once, and the server is done before the
				// next poll. Watch the row keep moving until that poll lands:
				// the indicator covers the gap between asking and being told,
				// not the work.
				Run: func(_ context.Context, out io.Writer) error {
					fmt.Fprintf(out, "refresh requested for %s\n", name)
					s.back.Refresh(name)
					return nil
				},
			},
			{
				Label: "Fail a sync",
				Desc:  "see the failure path",
				Busy:  "syncing",
				Run: func(_ context.Context, out io.Writer) error {
					fmt.Fprintf(out, "sync requested for %s\n", name)
					activity.Progress(out, "comparing 1/3")
					time.Sleep(600 * time.Millisecond)
					// A failed dispatch started nothing, so there is nothing
					// for the data to confirm: the row reports ✗ at once
					// rather than waiting for an observation.
					return fmt.Errorf("rpc error: application %q not found", name)
				},
			},
		},
	}
}

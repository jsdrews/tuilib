// Package activity demonstrates per-row in-flight state: the spinner and
// status label a row shows while something is working on it.
//
// It talks to demoapi over HTTP — a real handler, a real client, real request
// boundaries — so the timings below are the server's, not a sleep in this
// file's goroutine.
//
// Three reasons a row's state changes, all of which have to look right:
//
//  1. **You did it.** Press "a" and pick Sync. The row starts spinning before
//     the POST has been answered, because the screen put its Selection() into
//     action.Set.Targets and the shell broadcast the verb's Busy label against
//     those keys. This screen writes one extra line to get that — Targets —
//     and no wiring at all.
//
//     Mark several rows first ("x", or "A" for all of them) and one run drives
//     all of them: one log event, one statusbar receipt, every marked row
//     spinning. They finish together, because a run has one outcome — retiring
//     them as each target lands is the refinement docs/activity.md decision 11
//     deferred until something needed it.
//
//  2. **Something else did it.** demoapi fires a scheduled sync every few
//     seconds. Nobody pressed anything, so nothing was broadcast; the row
//     spins because Options.ActivityWhen recognises "Syncing" in the data the
//     poll brought back. A read-only dashboard gets this and needs no actions.
//
//  3. **It happened while nobody was looking.** Some of the server's work
//     finishes between two polls, so the TUI never observes it running. Those
//     rows flash "•" instead, driven by Options.ActivityRevision over a hidden
//     column carrying the server's revision — the only way to notice a change
//     the sampled data carries no other evidence of.
//
// # The case worth watching for
//
// Refresh is instant server-side: it is done long before the next poll. Press
// it and the row keeps spinning anyway, until the poll lands. That is not a
// lie about the work — it is the truth about what the TUI knows. The
// alternative, clearing the moment the request returns, shows a tick and then
// a cell reading exactly as it did before the user acted, which is
// indistinguishable from nothing having happened.
//
// Sync is the opposite: the server takes four seconds, so the poll catches it
// and the local "syncing" hands over to the data's own "Syncing" mid-flight.
// The action itself returns earlier than that, having streamed the job's log
// into the console — so the handoff really is covering a gap.
//
// # What this screen does not contain
//
// No spinner. No ticker. No re-pushing rows per frame to animate anything. The
// rows are pushed once per poll and the indicator is drawn over them by the
// component.
package activity

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/demoapi"
	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/ansi"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/poll"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

const (
	pollInterval = 2 * time.Second
	appCount     = 6
)

// Column order. Rev is hidden: it is the revision ActivityRevision watches,
// and the status worth watching is not always the status worth showing.
const (
	colName = iota
	colSync
	colHealth
	colRev
)

// appRow is one row as the server reports it.
type appRow struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Sync   string `json:"sync"`
	Health string `json:"health"`
	Rev    int64  `json:"rev"`
}

// Screen is a table over demoapi, plus a poll. Everything about the indicators
// is declared in SetTheme and Actions; nothing here drives them.
type Screen struct {
	t     theme.Theme
	table table.Model
	poll  poll.Model
	api   demoapi.Target
	apps  []appRow

	// gen counts requests; seen is the newest reply applied.
	gen, seen int
}

// New returns the activity demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{
		// In-process by default, or a running demoapi when TUILIB_DEMO_API is
		// set — `task demo` does the latter, and then this screen and the
		// remote demo are watching one world that a curl can also change.
		// A short schedule, so the thing this screen exists to show — a row
		// spinning because the *server* said so, with nobody having pressed
		// anything — happens while you are watching rather than once a minute.
		api:  demoapi.From(demoapi.Options{Seed: 11, Apps: appCount, Schedule: 3 * time.Second}),
		poll: poll.New(poll.Options{Interval: pollInterval}),
	}
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string         { return "Activity" }
func (s *Screen) IsCapturingKeys() bool { return s.table.IsCapturingKeys() }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) Layout() layout.Node   { return layout.Sized(&s.table) }

func (s *Screen) Init() tea.Cmd { return tea.Batch(s.poll.Init(), s.fetch()) }

// fetchedMsg carries one poll's worth of rows.
//
// gen is the request's sequence number. Replies can arrive out of order over a
// real network, and an older one landing last would put stale rows on screen —
// the hazard source.Deliver's generation check exists for, which a plainly
// polled screen has to handle itself because nothing is coordinating it.
type fetchedMsg struct {
	gen  int
	apps []appRow
	err  error
}

// Update forwards everything to both the table and the poll. The activity
// broadcasts the shell sends arrive here like any other message and reach the
// table because rule 6 says forward all of them — which is the whole of this
// screen's involvement in the feature.
func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmds []tea.Cmd
	cmds = append(cmds, s.poll.Update(msg))

	switch m := msg.(type) {
	case poll.RefreshMsg:
		cmds = append(cmds, s.fetch())

	case fetchedMsg:
		if m.gen < s.seen {
			break // a slower request overtaken by a newer one
		}
		s.seen = m.gen
		if m.err != nil {
			cmds = append(cmds, app.ErrorOf(m.err))
			break
		}
		s.apps = m.apps
		// One push per poll. The indicators are an overlay the component
		// draws on top; nothing here re-pushes rows to animate anything.
		s.table.SetKeyedRows(s.rows())
		// Rule 24's idiom: a persistent indicator goes on the component's
		// title, not in the statusbar, which is for transient messages.
		s.table.SetTitle(s.title())
		s.poll.MarkRefreshed()
	}

	var tcmd tea.Cmd
	s.table, tcmd = s.table.Update(msg)
	cmds = append(cmds, tcmd)

	return s, tea.Batch(cmds...)
}

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

func (s *Screen) HelpSections() []help.Section {
	// The table contributes its own groups, marking included, so nothing here
	// restates x / X / A / D.
	return help.SectionsOf(&s.table, help.Group("Applications",
		key.NewBinding(key.WithKeys("mouse:right"), key.WithHelp("right-click", "actions")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	))
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	cursor, value := s.table.Cursor(), s.table.Value()
	marks := s.table.Marks()
	act := s.table.ActivityState()

	o := t.Table()
	o.Title = s.title()
	o.Filterable = true
	// Marking, so one verb can act on several rows and all of them say so.
	// The rows are keyed, which marking requires (rule 32) and which the
	// indicators require for the same reason.
	o.Markable = true
	o.Filter.Placeholder = "filter…"
	o.Columns = []table.Column{
		{Title: "Name", Width: 22},
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
	//
	// Settled names the statuses that mean "nothing is happening" and treats
	// everything else as in flight — which is how an API like this documents
	// itself, and which keeps working when the server learns a new in-progress
	// status. Listing the busy ones instead would quietly stop spinning for it.
	//
	// The label is whatever the server said. The verbs below use the same
	// strings for their Busy text, so a row shows one word from the moment it
	// is asked to the moment the poll says it is done — no mapping to keep in
	// step, and no handoff visible as a change of wording.
	settled := activity.Settled(demoapi.SyncSynced, demoapi.SyncOutOfSync)
	o.ActivityWhen = func(c table.Row) (string, bool) { return settled(c[colSync]) }

	// And for work it never even saw running.
	o.ActivityRevision = func(c table.Row) string { return c[colRev] }

	s.table = table.New(o)
	s.table.SetKeyedRows(s.rows())
	if value != "" {
		s.table.SetValue(value)
	}
	s.table.SetCursor(cursor)
	s.table.SetMarks(marks)
	// Rule 4: carry the work in flight across the rebuild along with the
	// cursor, the filter and the marks. The returned tick is dropped here only because
	// SetTheme has nowhere to return one; the next poll re-arms it.
	_ = s.table.SetActivityState(act)
}

// working describes what is in flight and, of that, how much this session did
// not start — which is the whole point of deriving state from the data and is
// otherwise indistinguishable on screen, since both layers use the server's
// vocabulary.
//
// An entry started by the shell's broadcast carries the run's id; one built by
// ActivityWhen from a poll result carries none. That is what separates them.
// (A screen calling SetActivity directly would also report no run, which is
// why this reads as "not started by an action" rather than "from the server".)
// title is the pane's border text: what this is, whether it is talking to a
// real server, and what is in flight.
func (s *Screen) title() string {
	name := "applications"
	if s.api.Live {
		name += " · live"
	}
	if w := s.working(); w != "" {
		name += " · " + w
	}
	return name
}

func (s *Screen) working() string {
	st := s.table.ActivityState()
	var mine, theirs int
	for _, a := range s.apps {
		e, ok := st.State(a.ID)
		if !ok || e.Done {
			continue
		}
		if e.RunID != 0 {
			mine++
		} else {
			theirs++
		}
	}
	switch {
	case mine+theirs == 0:
		return ""
	case theirs == 0:
		return fmt.Sprintf("%d working", mine)
	case mine == 0:
		return fmt.Sprintf("%d working, from the server", theirs)
	default:
		return fmt.Sprintf("%d working, %d from the server", mine+theirs, theirs)
	}
}

// rows are keyed by the server's id, which is what lets an indicator stay on
// its own row when a refresh reorders the set — and what Selection() reports
// for Targets, and what the POST paths are built from.
func (s *Screen) rows() []table.KeyedRow {
	out := make([]table.KeyedRow, len(s.apps))
	for i, a := range s.apps {
		out[i] = table.KeyedRow{
			Key: a.ID,
			Cells: []string{
				a.Name,
				a.Sync,
				healthCell(a.Health),
				strconv.FormatInt(a.Rev, 10),
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

func (s *Screen) fetch() tea.Cmd {
	s.gen++
	api, gen := s.api, s.gen
	return func() tea.Msg {
		resp, err := api.Client.Get(api.URL("/apps?limit=" + strconv.Itoa(appCount)))
		if err != nil {
			return fetchedMsg{gen: gen, err: err}
		}
		defer resp.Body.Close()
		var body struct {
			Rows []appRow `json:"rows"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return fetchedMsg{gen: gen, err: err}
		}
		return fetchedMsg{gen: gen, apps: body.Rows}
	}
}

// Actions is this screen's whole contribution to the indicators it shows for
// work the user starts: Targets, and a Busy label per verb.
func (s *Screen) Actions() action.Set {
	// Selection() is the marked set, or the cursor row when nothing is marked,
	// so this reads the same at either arity — which is the whole reason a
	// screen calls it rather than branching on Marks() itself.
	targets := s.table.Selection()
	if len(targets) == 0 {
		return action.Set{}
	}

	// Best-effort conflict avoidance: a row the last poll reported busy gets its
	// verbs dimmed with the reason, so the menu says why rather than letting the
	// user fire into a 409.
	//
	// Best-effort is the honest description. This reads the *last* observation,
	// which can be a poll interval old, and a schedule can fire inside that
	// window — so the server still has to reject, and the action still has to
	// surface it. Exclusive is no help: it gates runs this session launched and
	// knows nothing about work the server started on its own.
	busy := s.busyTargets(targets)

	return action.Set{
		Target: s.table.SelectionLabel(),
		// The keys, as opposed to the label above. Without these the verbs
		// still run and still log; the rows just never say so.
		Targets: targets,
		Count:   len(targets),
		Actions: []action.Action{
			{
				Label:    "Sync",
				Desc:     "reconcile to git",
				Disabled: busy,
				// The server's own word, so the local indicator and the derived
				// one that replaces it read identically.
				Busy: demoapi.SyncSyncing,
				// This verb dispatches: it returns once the server has accepted
				// the operation, which is well before the server has finished
				// it. "Sync completed" would claim otherwise while the row is
				// still visibly syncing.
				Receipt: "Sync requested",
				// One run over several rows: one log event, one statusbar
				// receipt, and every marked row spinning (decision 11 of
				// docs/activity.md). Exclusive is now held per target, so
				// syncing {a, b} and {b, c} collide on b alone.
				Multi:     true,
				Exclusive: true,
				Run:       s.run(targets, "sync"),
			},
			{
				Label:    "Refresh",
				Desc:     "re-read from git",
				Disabled: busy,
				Busy:     demoapi.SyncRefresh,
				Receipt:  "Refresh requested",
				Multi:    true,
				// Returns almost at once, and the server is done before the
				// next poll. Watch the rows keep moving until that poll lands:
				// the indicator covers the gap between asking and being told,
				// not the work.
				Run: s.refresh(targets),
			},
			{
				Label:    "Fail a sync",
				Desc:     "see the failure path",
				Disabled: busy,
				Busy:     demoapi.SyncSyncing,
				Multi:    true,
				// A failed dispatch started nothing for the data to confirm,
				// so the rows report ✗ at once rather than waiting.
				Run: s.run(targets, "fail"),
			},
		},
	}
}

// busyTargets reports why the verbs are unavailable, or "" when they are not.
//
// It reads derived state rather than the cells, so it means "the last poll said
// the server was working on this" — the same source the row's indicator uses. A
// local entry (RunID non-zero) is this session's own run, which Exclusive
// already guards.
func (s *Screen) busyTargets(targets []string) string {
	st := s.table.ActivityState()
	for _, key := range targets {
		if e, ok := st.State(key); ok && !e.Done && e.RunID == 0 {
			return "already " + strings.ToLower(e.Label)
		}
	}
	return ""
}

// run POSTs the action and then streams the server's job log into out.
//
// ctx is threaded into both requests, so killing the run from the console ("o"
// then "x") cancels a request in flight rather than merely abandoning it. That
// is the reason an action is handed a context rather than the model.
func (s *Screen) run(ids []string, kind string) action.Func {
	api := s.api
	return func(ctx context.Context, out io.Writer) error {
		// No activity.Progress anywhere, deliberately. The row shows the verb's
		// Busy label — the server's own "Syncing" — and holds it until the poll
		// finds a status the server calls settled. One word, the same at every
		// arity, and the same word the server would have used.
		//
		// This screen used to relabel as it went, and it was worse twice over.
		// A single-row run read "submitting" then "applying" while a
		// multi-row run counted "1/3", "2/3" — two vocabularies for one verb,
		// so the row said something different depending on how many things you
		// had marked. And the counter was run-scoped but drawn per row, so
		// every marked row showed "2/3" as though that were its own progress.
		//
		// activity.Progress is for an action with genuinely long, genuinely
		// per-target phases. A 1.6-second sync has neither, and a label that
		// changes three times in that window is harder to read than one that
		// does not change at all. The console is where the detail belongs.
		var failed error
		for _, id := range ids {
			job, err := launch(ctx, api, id, kind)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%s: accepted as %s\n", id, job)

			if err := s.follow(ctx, out, id, job, len(ids) > 1); err != nil {
				// One target failing does not abandon the others — but the run
				// reports it, so the rows show ✗ and the statusbar says so.
				failed = err
			}
		}
		return failed
	}
}

// follow streams one job's log into out, prefixing lines with the app when a
// run covers several so the console stays readable.
func (s *Screen) follow(ctx context.Context, out io.Writer, id, job string, prefix bool) error {
	resp, err := get(ctx, s.api, s.api.URL("/jobs/"+job+"/log"))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if prefix {
			fmt.Fprintf(out, "%s: %s\n", id, line)
		} else {
			fmt.Fprintln(out, line)
		}
		if strings.HasPrefix(line, "error:") {
			return fmt.Errorf("%s: %s", id, strings.TrimPrefix(line, "error: "))
		}
	}
	return sc.Err()
}

// refresh is the fire-and-forget shape: one POST, no waiting.
func (s *Screen) refresh(ids []string) action.Func {
	api := s.api
	return func(ctx context.Context, out io.Writer) error {
		for _, id := range ids {
			job, err := launch(ctx, api, id, "refresh")
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%s: accepted as %s\n", id, job)
		}
		return nil
	}
}

func launch(ctx context.Context, api demoapi.Target, id, kind string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api.URL("/apps/"+id+"/"+kind), nil)
	if err != nil {
		return "", err
	}
	resp, err := api.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return "", fmt.Errorf("%s %s: %s", kind, resp.Status, e.Error)
	}
	var body struct {
		Job string `json:"job"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.Job, nil
}

func get(ctx context.Context, api demoapi.Target, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return api.Client.Do(req)
}

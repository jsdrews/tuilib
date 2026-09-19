// Package activity demonstrates per-row in-flight state: the spinner and
// status label a row shows while the server says something is working on it.
//
// It talks to demoapi over HTTP — a real handler, a real client, real request
// boundaries — so the timings below are the server's, not a sleep in this
// file's goroutine.
//
// # The feature is four lines, and they are all in SetTheme
//
//	o.ActivityColumn = "Sync"
//	settled := activity.Settled(demoapi.SyncSynced, demoapi.SyncOutOfSync)
//	o.ActivityWhen = func(c table.Row) (string, bool) { return settled(c[colSync]) }
//
// demoapi starts a scheduled sync every few seconds. Nobody pressed anything;
// those rows spin because the poll brought back "Syncing" and the predicate
// recognised it as work. That is the entire mechanism. A read-only dashboard
// — a poll, a keyed swap and a predicate — needs nothing else in this file.
//
// # So what is everything else doing here
//
// The action menu, marking, the console streaming and the generation-stamped
// fetch are composition *around* the indicator rather than part of it. They
// are here because this is the screen shape the feature exists for, and
// because two of them are easy to get wrong in ways that break it:
//
//   - The rows are keyed (SetKeyedRows), which activity requires for the same
//     reason marking does — a refresh reorders the set constantly, and an
//     indicator held by index would drift onto a neighbour.
//   - Replies are stamped and stale ones dropped. Over a real network they do
//     arrive out of order, and a stale page painted over a newer one is the
//     most common way an indicator appears to flap.
//
// # The case worth watching for
//
// Press "a" and pick Sync, and the row does not move. It keeps saying
// OutOfSync for up to a poll interval, and only then — when the server has
// been asked and has answered — does it start spinning "Syncing". Refresh is
// more pointed still: the server finishes it long before the next poll, so it
// may produce no visible indicator at all.
//
// That gap is the honest cost of deriving state from data, and it is a
// deliberate trade rather than a bug. The TUI does not know the row is busy
// until it is told, and the alternative — a client-side claim that the row is
// working because the user pressed something — is the layer docs/activity.md
// removed, along with the six mechanisms it took to keep that claim from
// disagreeing with the server. Open question 2 there is whether a dispatching
// verb should show anything in the meantime; Action.Receipt ("Sync requested"
// in the statusbar) is what covers it today.
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

// Column order. colSync is the one the predicate reads and the one the
// indicator replaces; they are the same column here, but they need not be —
// ActivityWhen takes the whole row precisely so the status worth watching can
// be a field the table never shows.
const (
	colName = iota
	colSync
	colHealth
)

// appRow is one row as the server reports it.
type appRow struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Sync   string `json:"sync"`
	Health string `json:"health"`
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

// Update forwards everything to both the table and the poll. Nothing here
// touches the indicators: the table observes its own rows when SetKeyedRows
// hands them over, and animates them off the spinner ticks rule 6 delivers.
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

// title is the pane's border text: what this is, whether it is talking to a
// real server, and how much of it the last poll said was working.
func (s *Screen) title() string {
	name := "applications"
	if s.api.Live {
		name += " · live"
	}
	if n := s.table.ActivityCount(); n > 0 {
		name += fmt.Sprintf(" · %d working", n)
	}
	return name
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
// It reads the observed set rather than the cells, so it means "the last poll
// said the server was working on this" — the same source the row's indicator
// uses, and the same reason it is best-effort: an observation can be a poll
// interval old, which is why the server still answers 409.
func (s *Screen) busyTargets(targets []string) string {
	st := s.table.ActivityState()
	for _, key := range targets {
		if e, ok := st.State(key); ok {
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

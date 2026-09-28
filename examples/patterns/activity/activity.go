// Package activity demonstrates per-row in-flight state against an API shaped
// like Argo CD: the spinner and status label a row shows while work runs on it.
//
// It talks to demoapi over HTTP — a real handler, a real client, real request
// boundaries — so the timings below are the server's, not a sleep here.
//
// # The model
//
// The server is the source of truth. When the user starts work locally, the
// row is told at once, and that claim lasts until the truth can take over.
// Everything below is one of those two things.
//
// # Observation: one predicate, over the row's own data
//
//	o.ActivityColumn = "Sync"
//	o.BusyWhen = func(r table.KeyedRow) (string, bool) {
//	    phase := r.Data.(appRow).Phase
//	    return phase, phase == demoapi.PhaseRunning
//	}
//
// In Argo, sync status is a comparison (Synced, OutOfSync) and never an
// activity: a sync in progress leaves it alone. The field that says work is
// running is operationState.phase — which this table does not show. The
// predicate reads it from Data and the indicator draws in the Sync cell.
// demoapi starts scheduled syncs every few seconds, so rows spin with nobody
// having pressed anything; a read-only dashboard needs nothing more.
//
// # Operations: one decision per verb
//
// Who knows when the work ends?
//
//   - Sync is activity.Observed. The POST answers at once and the controller
//     does the work, so the phase says when it ends.
//   - Refresh is activity.Held. Argo's refresh is a GET that holds the
//     connection until reconciled and shows nothing on the app meanwhile, so
//     no poll can say it is running or that it stopped. Only the request knows.
//   - Sync and follow is activity.Held too — a job handle. It streams the job
//     it was given and ends when the job does.
//   - Quick sync is Observed work that finishes between two polls. Nothing a
//     poll returns can show it, so the table reports it with UnobservedMsg
//     rather than leaving the row looking ignored.
//
// Dispatch opens the claim, Done closes the request, and BeginRead / ApplyRead
// stamp every fetch so the table — not this screen — decides whether a read
// taken across a write can end a claim. There are no counters here.
package activity

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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
	"github.com/jsdrews/tuilib/pkg/runner"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

const (
	pollInterval = 2 * time.Second
	appCount     = 6
)

// appRow is one row as the server reports it.
type appRow struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Sync   string `json:"sync"`
	Health string `json:"health"`
	Phase  string `json:"phase"`
	Rev    int64  `json:"rev"`
}

// verb is what a menu entry claims while it runs, and who ends the claim.
type verb struct {
	label string
	mode  activity.Mode
}

// The sync verbs claim with the server's own word, so the row reads the same
// from the keypress to the observation that confirms it. Refresh has no server
// word — Argo shows nothing while one runs — so it names itself.
var verbs = map[string]verb{
	"Sync":            {demoapi.PhaseRunning, activity.Observed},
	"Quick sync":      {demoapi.PhaseRunning, activity.Observed},
	"Fail a sync":     {demoapi.PhaseRunning, activity.Observed},
	"Refresh":         {"Refreshing", activity.Held},
	"Sync and follow": {demoapi.PhaseRunning, activity.Held},
}

// Screen is a table over demoapi, plus a poll.
type Screen struct {
	t     theme.Theme
	table table.Model
	poll  poll.Model
	api   demoapi.Target
	apps  []appRow

	// ops maps a run's tag to the operations it dispatched — one per row, so
	// the server refusing one row stops that row alone — so the run's answer
	// can close them.
	ops map[string][]activity.Op
}

// New returns the activity demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{
		// In-process by default, or a running demoapi when TUILIB_DEMO_API is
		// set. A short schedule, so rows spin because the *server* said so
		// while you are watching.
		api:  demoapi.From(demoapi.Options{Seed: 11, Apps: appCount, Schedule: 3 * time.Second}),
		poll: poll.New(poll.Options{Interval: pollInterval}),
		ops:  map[string][]activity.Op{},
	}
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string         { return "Activity" }
func (s *Screen) IsCapturingKeys() bool { return s.table.IsCapturingKeys() }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) Layout() layout.Node   { return layout.Sized(&s.table) }

func (s *Screen) Init() tea.Cmd { return tea.Batch(s.poll.Init(), s.fetch(), titleTick()) }

// titleTickMsg re-renders the title once a second, so "refreshed Ns ago"
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

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmds []tea.Cmd
	cmds = append(cmds, s.poll.Update(msg))

	switch m := msg.(type) {
	case poll.RefreshMsg:
		cmds = append(cmds, s.fetch())

	case titleTickMsg:
		s.table.SetTitle(s.title())
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
		wasFailing := s.table.ReadsFailing()
		if !s.table.ApplyRead(m.read, rowsOf(m.apps), m.err) {
			if m.err != nil {
				if !wasFailing {
					// Once, when reads start failing; the title says it for as
					// long as the outage lasts.
					cmds = append(cmds, app.ErrorOf(m.err))
				}
				s.table.SetTitle(s.title())
			}
			break
		}
		s.apps = m.apps
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
	o.Markable = true
	o.Filter.Placeholder = "filter…"
	o.Columns = []table.Column{
		{Title: "Name", Width: 22},
		// Fixed, and wide enough for "⣾ Refreshing": widths come from the
		// rows, never from the indicator.
		{Title: "Sync", Width: 14},
		{Title: "Health", Width: 13},
	}
	o.ActivityColumn = "Sync"
	o.BusyWhen = func(r table.KeyedRow) (string, bool) {
		phase := r.Data.(appRow).Phase
		return phase, phase == demoapi.PhaseRunning
	}
	// Activity.Settle stays 0: demoapi reports the phase inline, the moment a
	// sync is accepted. Against a real Argo controller, which takes a moment
	// to pick an operation up, set it to 1 or 2.
	// Moves whenever an operation on the app finishes, so a Quick sync that
	// ends where it began (Succeeded → Succeeded) still reads as done on the
	// first poll after it. Argo's equivalent is operationState.finishedAt.
	o.Revision = func(r table.KeyedRow) string { return strconv.FormatInt(r.Data.(appRow).Rev, 10) }

	s.table = table.New(o)
	s.table.SetKeyedRows(s.rows())
	if value != "" {
		s.table.SetValue(value)
	}
	s.table.SetCursor(cursor)
	s.table.SetMarks(marks)
	_ = s.table.SetActivityState(act)
}

func (s *Screen) title() string {
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

func (s *Screen) rows() []table.KeyedRow { return rowsOf(s.apps) }

func rowsOf(apps []appRow) []table.KeyedRow {
	out := make([]table.KeyedRow, len(apps))
	for i, a := range apps {
		out[i] = table.KeyedRow{
			Key:   a.ID,
			Cells: []string{a.Name, a.Sync, healthCell(a.Health)},
			Data:  a,
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
	api, rd := s.api, s.table.BeginRead()
	return func() tea.Msg {
		resp, err := api.Client.Get(api.URL("/apps?limit=" + strconv.Itoa(appCount)))
		if err != nil {
			return fetchedMsg{read: rd, err: err}
		}
		defer resp.Body.Close()
		// A 503 body decodes as an empty page; without this an outage blanks
		// the table instead of reading as a failed read.
		if resp.StatusCode != http.StatusOK {
			return fetchedMsg{read: rd, err: fmt.Errorf("list apps: %s", resp.Status)}
		}
		var body struct {
			Rows []appRow `json:"rows"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return fetchedMsg{read: rd, err: err}
		}
		return fetchedMsg{read: rd, apps: body.Rows}
	}
}

// Actions lists one verb per operation shape. What each claims, and in which
// mode, is the verbs table above.
func (s *Screen) Actions() action.Set {
	targets := s.table.Selection()
	if len(targets) == 0 {
		return action.Set{}
	}
	// Best-effort: reads the last observation, so the server still has to
	// refuse with a 409 and the action still has to surface it.
	busy := s.busyTargets(targets)

	return action.Set{
		Target:  s.table.SelectionLabel(),
		Targets: targets,
		Count:   len(targets),
		Actions: []action.Action{
			{
				Label:     "Sync",
				Desc:      "reconcile to git",
				Disabled:  busy,
				Receipt:   "Sync requested",
				Multi:     true,
				Exclusive: true,
				Run:       s.launchAll(targets, "sync", ""),
			},
			{
				Label:    "Refresh",
				Desc:     "compare with git (blocks)",
				Disabled: busy,
				Multi:    true,
				Run:      s.refresh(targets),
			},
			{
				Label:    "Sync and follow",
				Desc:     "stream the job to the end",
				Disabled: busy,
				// One row: it is Held, so each row's spinner ends only when the
				// run returns, and a run that follows several jobs in turn
				// would hold a finished row until the last one ended.
				Multi:     false,
				Exclusive: true,
				Run:       s.syncAndFollow(targets),
			},
			{
				Label:    "Quick sync",
				Desc:     "done before the next poll",
				Disabled: busy,
				Receipt:  "Sync requested",
				Multi:    true,
				Run:      s.launchAll(targets, "sync", "?takes=300ms"),
			},
			{
				Label:    "Fail a sync",
				Desc:     "see the failure path",
				Disabled: busy,
				Receipt:  "Sync requested",
				Multi:    true,
				Run:      s.launchAll(targets, "fail", ""),
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

// launchAll POSTs one operation per target and returns once each is
// accepted — an Observed verb's Run is only the acknowledgement.
func (s *Screen) launchAll(ids []string, kind, query string) action.Func {
	api := s.api
	return func(ctx context.Context, out io.Writer) error {
		var errs []error
		for _, id := range ids {
			job, err := launch(ctx, api, id, kind+query)
			if err != nil {
				errs = append(errs, keyError{id, err})
				continue
			}
			fmt.Fprintf(out, "%s: accepted as %s\n", id, job)
		}
		return errors.Join(errs...)
	}
}

// keyError is one row's failure inside a run over several. The run keeps
// going past it, and the screen closes each row's operation with its own
// result (errFor) rather than failing every row on one refusal.
type keyError struct {
	key string
	err error
}

func (e keyError) Error() string { return e.err.Error() }
func (e keyError) Unwrap() error { return e.err }

// errFor is the part of a run's error that belongs to key. An error not tied
// to a row — the run was cancelled — belongs to every row.
func errFor(err error, key string) error {
	if err == nil {
		return nil
	}
	errs := []error{err}
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		errs = j.Unwrap()
	}
	tied := false
	for _, e := range errs {
		var ke keyError
		if errors.As(e, &ke) {
			tied = true
			if ke.key == key {
				return ke.err
			}
		}
	}
	if tied {
		return nil
	}
	return err
}

// refresh is Argo's blocking refresh: the GET does not answer until the
// server has re-compared the app, and the Run does not return until it does.
func (s *Screen) refresh(ids []string) action.Func {
	api := s.api
	return func(ctx context.Context, out io.Writer) error {
		var errs []error
		for _, id := range ids {
			resp, err := get(ctx, api, api.URL("/apps/"+id+"?refresh=normal"))
			if err != nil {
				errs = append(errs, keyError{id, err})
				continue
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				errs = append(errs, keyError{id, fmt.Errorf("refresh %s: %s", id, resp.Status)})
				continue
			}
			var a appRow
			err = json.NewDecoder(resp.Body).Decode(&a)
			resp.Body.Close()
			if err != nil {
				errs = append(errs, keyError{id, err})
				continue
			}
			fmt.Fprintf(out, "%s: refreshed, %s\n", id, a.Sync)
		}
		return errors.Join(errs...)
	}
}

// syncAndFollow is the job-handle shape: POST, then stream the job the server
// handed back until it closes.
func (s *Screen) syncAndFollow(ids []string) action.Func {
	api := s.api
	return func(ctx context.Context, out io.Writer) error {
		var errs []error
		for _, id := range ids {
			job, err := launch(ctx, api, id, "sync")
			if err != nil {
				errs = append(errs, keyError{id, err})
				continue
			}
			fmt.Fprintf(out, "%s: accepted as %s\n", id, job)
			if err := s.follow(ctx, out, id, job, len(ids) > 1); err != nil {
				errs = append(errs, keyError{id, err})
			}
		}
		return errors.Join(errs...)
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

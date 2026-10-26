package liveops

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

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/demoapi"
	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/ansi"
	"github.com/jsdrews/tuilib/pkg/table"
)

// The applications pane: per-row activity against an API shaped like Argo CD,
// over HTTP to demoapi, so the timings are the server's.

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
	"Sync (dropped)":  {demoapi.PhaseRunning, activity.Held},
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

// appActions lists one verb per operation shape. What each claims, and in
// which mode, is the verbs table above.
func (s *Screen) appActions() action.Set {
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
			{
				// A job handle polled to its end. It is the one shape that can
				// tell "finished at once" from "accepted and dropped": the
				// dropped job's id resolves to nothing, and the row stops
				// spinning on the error rather than waiting out a poll.
				Label:    "Sync (dropped)",
				Desc:     "accepted, never run",
				Disabled: busy,
				Multi:    true,
				Run:      s.waitJobs(targets, "?blackhole=1"),
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

// waitJobs starts a sync per target and polls the job it was handed until the
// job ends, or turns out never to have existed.
func (s *Screen) waitJobs(ids []string, query string) action.Func {
	api := s.api
	return func(ctx context.Context, out io.Writer) error {
		var errs []error
		for _, id := range ids {
			job, err := launch(ctx, api, id, "sync"+query)
			if err != nil {
				errs = append(errs, keyError{id, err})
				continue
			}
			fmt.Fprintf(out, "%s: accepted as %s\n", id, job)
			if err := waitJob(ctx, api, id, job); err != nil {
				errs = append(errs, keyError{id, err})
			}
		}
		return errors.Join(errs...)
	}
}

func waitJob(ctx context.Context, api demoapi.Target, id, job string) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		resp, err := get(ctx, api, api.URL("/jobs/"+job))
		if err != nil {
			return err
		}
		var j demoapi.Job
		err = json.NewDecoder(resp.Body).Decode(&j)
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusNotFound:
			// Short, because the statusbar shows it after "Sync (dropped)
			// failed:"; the run has already logged which app and job.
			return errors.New("job vanished")
		case err != nil:
			return err
		case j.Status == demoapi.JobFailed:
			return fmt.Errorf("sync %s failed", id)
		case j.Status != demoapi.JobRunning:
			return nil
		}
	}
}

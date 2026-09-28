package activityrecipes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/demoapi"
	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// Recipe 4 — a job handle (activity.Held).
//
// The POST answers 202 with a job id, and GET /jobs/{id} says what became of
// it. The job, not the app, knows when the work ends, so this is Held like a
// refresh: Done fires when the job does. It is also the one shape that can
// tell "finished instantly" from "was accepted and dropped" — the dropped
// job's id resolves to nothing.

type jobRecipe struct{ *observe }

type jobDoneMsg struct {
	op  activity.Op
	err error
}

func newJob(t theme.Theme) *jobRecipe { return &jobRecipe{newObserve(t, "job handle")} }

func (s *jobRecipe) Actions() action.Set {
	keys := s.selection()
	return action.Set{
		Targets: keys,
		Actions: []action.Action{
			{Label: "Sync and wait", Multi: true, Do: func() tea.Cmd { return s.syncAndWait(keys, "") }},
			{Label: "Sync (dropped)", Desc: "accepted, never run", Multi: true, Do: func() tea.Cmd { return s.syncAndWait(keys, "?blackhole=1") }},
		},
	}
}

func (s *jobRecipe) syncAndWait(keys []string, query string) tea.Cmd {
	api := s.api
	return s.table.DispatchEach(keys, demoapi.PhaseRunning, activity.Held,
		func(op activity.Op, k string) tea.Cmd {
			return func() tea.Msg { return jobDoneMsg{op, runJob(api, k, query)} }
		})
}

// runJob starts a sync and waits on its handle until it finishes.
func runJob(api demoapi.Target, app, query string) error {
	resp, err := api.Client.Post(api.URL("/apps/"+app+"/sync"+query), "", nil)
	if err != nil {
		return err
	}
	var started struct {
		Job string `json:"job"`
	}
	err = json.NewDecoder(resp.Body).Decode(&started)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("sync %s: %s", app, resp.Status)
	}
	for {
		time.Sleep(500 * time.Millisecond)
		resp, err := api.Client.Get(api.URL("/jobs/" + started.Job))
		if err != nil {
			return err
		}
		var j demoapi.Job
		err = json.NewDecoder(resp.Body).Decode(&j)
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusNotFound:
			return fmt.Errorf("sync %s: job %s vanished", app, started.Job)
		case err != nil:
			return err
		case j.Status == demoapi.JobFailed:
			return errors.New("sync " + app + " failed")
		case j.Status != demoapi.JobRunning:
			return nil
		}
	}
}

func (s *jobRecipe) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	if m, ok := msg.(jobDoneMsg); ok {
		switch s.table.Done(m.op, m.err) {
		case activity.Withdrawn:
			cmd = app.ErrorOf(m.err)
		case activity.Ended:
			cmd = app.Info("Sync finished")
		}
	}
	return s, tea.Batch(cmd, s.update(msg))
}

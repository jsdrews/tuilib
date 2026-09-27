package activityrecipes

import (
	"errors"
	"fmt"
	"net/http"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/demoapi"
	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// Recipe 2 — a fire-and-observe verb (activity.Observed).
//
// Argo's sync POST answers at once, and then the controller does the work.
// The server's phase says when the work ends, so the spinner does this:
//
//   - Dispatch starts it the moment the user asks.
//   - Done marks the POST as answered.
//   - After that, the polled phase takes over.
//
// A sync that finishes between two polls never shows as Running. The table
// sends UnobservedMsg for it, so the screen can say something instead of the
// row looking ignored.

type syncRecipe struct{ *observe }

type answeredMsg struct {
	op  activity.Op
	err error
}

func newSync(t theme.Theme) *syncRecipe { return &syncRecipe{newObserve(t, "sync")} }

func (s *syncRecipe) Actions() action.Set {
	keys := s.selection()
	return action.Set{
		Targets: keys,
		Actions: []action.Action{
			{Label: "Sync", Do: func() tea.Cmd { return s.sync(keys, "") }},
			{Label: "Quick sync", Desc: "done before the next poll", Do: func() tea.Cmd { return s.sync(keys, "?takes=300ms") }},
		},
	}
}

func (s *syncRecipe) sync(keys []string, query string) tea.Cmd {
	op, spin := s.table.Dispatch(keys, demoapi.PhaseRunning, activity.Observed)
	api := s.api
	// Every key is asked, even after one is refused: stopping at the first
	// failure would withdraw the spinners of apps that were never tried.
	return tea.Batch(spin, func() tea.Msg {
		var errs []error
		for _, k := range keys {
			resp, err := api.Client.Post(api.URL("/apps/"+k+"/sync"+query), "", nil)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusAccepted {
				errs = append(errs, fmt.Errorf("sync %s: %s", k, resp.Status))
			}
		}
		return answeredMsg{op, errors.Join(errs...)}
	})
}

func (s *syncRecipe) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	switch m := msg.(type) {
	case answeredMsg:
		s.table.Done(m.op, m.err)
		// A Do verb gets no shell receipt (that is Run's), so say it here —
		// "requested", because the 202 is not the work finishing.
		cmd = app.Info("Sync requested")
		if m.err != nil {
			cmd = app.ErrorOf(m.err)
		}
	case activity.UnobservedMsg:
		cmd = app.Info("Sync done between polls")
	}
	return s, tea.Batch(cmd, s.update(msg))
}

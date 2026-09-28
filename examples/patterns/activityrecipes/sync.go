package activityrecipes

import (
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
//   - Dispatch starts it the moment the user asks, one operation per row.
//   - Done marks the POST as answered, and returns Acknowledged: accepted,
//     still running.
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
			// Multi: the verbs act on every marked row, one operation each.
			{Label: "Sync", Multi: true, Do: func() tea.Cmd { return s.sync(keys, "") }},
			{Label: "Quick sync", Desc: "done before the next poll", Multi: true, Do: func() tea.Cmd { return s.sync(keys, "?takes=300ms") }},
		},
	}
}

func (s *syncRecipe) sync(keys []string, query string) tea.Cmd {
	api := s.api
	// One operation per row: a 409 on one row stops that row alone.
	return s.table.DispatchEach(keys, demoapi.PhaseRunning, activity.Observed,
		func(op activity.Op, k string) tea.Cmd {
			return func() tea.Msg {
				resp, err := api.Client.Post(api.URL("/apps/"+k+"/sync"+query), "", nil)
				if err != nil {
					return answeredMsg{op, err}
				}
				resp.Body.Close()
				if resp.StatusCode != http.StatusAccepted {
					return answeredMsg{op, fmt.Errorf("sync %s: %s", k, resp.Status)}
				}
				return answeredMsg{op, nil}
			}
		})
}

func (s *syncRecipe) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	switch m := msg.(type) {
	case answeredMsg:
		// Done says what the reply means. For an Observed verb it is
		// Acknowledged: the server has the work and the row keeps spinning,
		// so this says "requested", never "completed".
		switch s.table.Done(m.op, m.err) {
		case activity.Withdrawn:
			cmd = app.ErrorOf(m.err)
		case activity.Acknowledged:
			cmd = app.Info("Sync requested")
		}
	case activity.UnobservedMsg:
		// Name the verb here: m.Label is the claim's label, "Running".
		cmd = app.Info("Sync done between polls")
	}
	return s, tea.Batch(cmd, s.update(msg))
}

package activityrecipes

import (
	"fmt"
	"net/http"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// Recipe 3 — a blocking verb (activity.Held).
//
// Argo's refresh is a GET that holds the connection until the refresh is
// done, and the app shows nothing while it runs. No poll can say it is running
// or that it stopped; only the request knows. So Held: polls cannot end the
// spinner, and Done — when the GET answers — does.

type refreshRecipe struct{ *observe }

type refreshedMsg struct {
	op  activity.Op
	err error
}

func newRefresh(t theme.Theme) *refreshRecipe { return &refreshRecipe{newObserve(t, "refresh")} }

func (s *refreshRecipe) Actions() action.Set {
	keys := s.selection()
	return action.Set{
		Targets: keys,
		Actions: []action.Action{{Label: "Refresh", Do: func() tea.Cmd { return s.refresh(keys) }}},
	}
}

func (s *refreshRecipe) refresh(keys []string) tea.Cmd {
	op, spin := s.table.Dispatch(keys, "Refreshing", activity.Held)
	api := s.api
	return tea.Batch(spin, func() tea.Msg {
		for _, k := range keys {
			resp, err := api.Client.Get(api.URL("/apps/" + k + "?refresh=normal"))
			if err != nil {
				return refreshedMsg{op, err}
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return refreshedMsg{op, fmt.Errorf("refresh %s: %s", k, resp.Status)}
			}
		}
		return refreshedMsg{op, nil}
	})
}

func (s *refreshRecipe) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	if m, ok := msg.(refreshedMsg); ok {
		s.table.Done(m.op, m.err)
		cmd = s.poll.Refresh() // show what the refresh found now, not in a poll
		if m.err != nil {
			cmd = app.ErrorOf(m.err)
		}
	}
	return s, tea.Batch(cmd, s.update(msg))
}

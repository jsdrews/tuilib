// The example, driven the way a person drives it.
//
// Every other test here builds a screen for the occasion, which is what let
// the real one ship broken: pkg/activity was right, the component contract was
// right, the ordering rules were right, and the shell consumed action.ChosenMsg
// without forwarding it — so the example's claim was unreachable code and
// pressing Sync did nothing until the next poll.
//
// Nothing short of driving the actual screen through the actual shell would
// have caught that, so this does.
package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/demoapi"
	demoactivity "github.com/jsdrews/tuilib/examples/patterns/activity"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// Press the verb and the row says so, without waiting for a poll to agree.
func TestTheExampleClaimsTheRowOnDispatch(t *testing.T) {
	m := app.New(app.Options{
		Root:       demoactivity.New(theme.Dark()),
		Themes:     []theme.Theme{theme.Dark()},
		SkipConfig: true,
		Mouse:      app.MouseClick,
		ActionsKey: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "actions")),
		OutputKey:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "output")),
	})
	h := newAppHarness(t, m)

	// The first page, so there is a row to act on. demoapi seeds most apps
	// Synced, and its own scheduled work is 3s away — comfortably outside the
	// window this test measures in.
	if !h.pumpUntil(3*time.Second, func() bool {
		return strings.Contains(h.render(), demoapi.SyncSynced)
	}) {
		t.Fatal("no rows arrived, so there is nothing to dispatch against")
	}
	if strings.Contains(h.render(), demoapi.SyncSyncing) {
		t.Skip("a scheduled sync is already running; this test measures the claim alone")
	}

	h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !strings.Contains(h.render(), "Sync") {
		t.Fatal("the action menu did not open")
	}
	h.send(tea.KeyMsg{Type: tea.KeyEnter})

	// One message hop, not a poll: the menu reports its pick as a command, so
	// action.ChosenMsg arrives on the next turn of the loop. 150ms is long
	// enough for that and for the POST, and far short of the 2s poll and the
	// 3s schedule — so a "Syncing" on screen now can only be the claim, not an
	// observation of one.
	h.pumpFor(150 * time.Millisecond)

	if !strings.Contains(h.render(), demoapi.SyncSyncing) {
		t.Error("the row says nothing after the verb was dispatched; Expect never reached it")
	}
}

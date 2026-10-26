//go:build integration

package integration

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"

	"github.com/jsdrews/tuilib/demoapi"
	"github.com/jsdrews/tuilib/examples/liveops"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// The operation shapes the activity recipes demonstrated one per tab, driven
// through the shell against the live-ops screen that now carries them all.
func newRecipesApp(t *testing.T) *harness {
	t.Helper()
	srv := httptest.NewServer(demoapi.New(demoapi.Options{Seed: 31, Apps: 6}))
	t.Cleanup(srv.Close)
	t.Setenv(demoapi.EnvBase, srv.URL)
	h := newAppHarness(t, app.New(app.Options{
		Root:       liveops.New(theme.New(palette.Dark())),
		Palettes:   []palette.Palette{palette.Dark()},
		SkipConfig: true,
		ActionsKey: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "actions")),
	}))
	if !h.pumpUntil(5*time.Second, func() bool {
		v := h.render()
		return strings.Contains(v, demoapi.SyncSynced) || strings.Contains(v, demoapi.SyncOutOfSync)
	}) {
		t.Fatalf("no rows arrived:\n%s", h.render())
	}
	return h
}

func TestRecipeRefreshSpinsForTheHold(t *testing.T) {
	h := newRecipesApp(t)
	h.pick("Refresh")
	if !h.pumpUntil(time.Second, func() bool { return strings.Contains(h.render(), "Refreshing") }) {
		t.Fatalf("the refresh never showed:\n%s", h.render())
	}
	h.pumpFor(1500 * time.Millisecond)
	if !strings.Contains(h.render(), "Refreshing") {
		t.Errorf("stopped while the GET was still held:\n%s", h.render())
	}
	if !h.pumpUntil(3*time.Second, func() bool { return !strings.Contains(h.render(), "Refreshing") }) {
		t.Errorf("still spinning after the GET answered:\n%s", h.render())
	}
}

func TestRecipeQuickSyncIsReported(t *testing.T) {
	h := newRecipesApp(t)
	h.pick("Quick sync")
	if !h.pumpUntil(8*time.Second, func() bool { return strings.Contains(h.render(), "Sync finished between") }) {
		t.Errorf("a sync nobody saw ended in silence:\n%s", h.render())
	}
}

func TestRecipeDroppedJobIsAnError(t *testing.T) {
	h := newRecipesApp(t)
	h.pick("Sync (dropped)")
	if !h.pumpUntil(3*time.Second, func() bool { return strings.Contains(h.render(), "vanish") }) {
		t.Errorf("a dropped job was not reported:\n%s", h.render())
	}
	if !h.pumpUntil(time.Second, func() bool { return spinnerGlyph(h.render()) == "" }) {
		t.Errorf("the row kept spinning for a job that does not exist:\n%s", h.render())
	}
}

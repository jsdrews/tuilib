package componenttest

import (
	"strings"
	"testing"

	exactivity "github.com/jsdrews/tuilib/examples/patterns/activity"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"github.com/jsdrews/tuilib/pkg/poll"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// The activity example's package comment promises three sources of row state
// on one screen. A demo that quietly stops demonstrating its subject is worse
// than no demo — the reader concludes the feature does not work — so the
// promises are asserted rather than trusted. Same reasoning as
// multiselect_example_test.go.

func newActivityExample(t *testing.T) screen.Screen {
	t.Helper()
	s := exactivity.New(theme.Dark())
	s.Layout().Render(geom.New(0, 0, exW, exH))
	return s
}

func drawActivity(t *testing.T, s screen.Screen) string {
	t.Helper()
	return s.Layout().Render(geom.New(0, 0, exW, exH))
}

func tickActivity(t *testing.T, s screen.Screen, n int) screen.Screen {
	t.Helper()
	for i := 0; i < n; i++ {
		s, _ = s.Update(poll.RefreshMsg{})
		drawActivity(t, s)
	}
	return s
}

func TestActivityExampleRendersItsApps(t *testing.T) {
	s := newActivityExample(t)
	if v := drawActivity(t, s); !strings.Contains(v, "api-server") {
		t.Fatalf("no rows rendered:\n%s", v)
	}
}

// Promise 2: a status the poll brings back spins on its own, with no action
// and no broadcast anywhere.
func TestActivityExampleSpinsFromPolledData(t *testing.T) {
	s := newActivityExample(t)
	s = tickActivity(t, s, 12)

	// Twelve intervals is well past the point where the example's scheduled
	// work has fired at least once.
	var sawSpinner bool
	for i := 0; i < 12 && !sawSpinner; i++ {
		if strings.Contains(drawActivity(t, s), "Syncing") {
			sawSpinner = true
			break
		}
		s = tickActivity(t, s, 1)
	}
	if !sawSpinner {
		t.Errorf("no scheduled work ever appeared:\n%s", drawActivity(t, s))
	}
}

// Promise 3: work that begins and ends between two polls flashes rather than
// passing unnoticed.
func TestActivityExampleFlashesUnseenChanges(t *testing.T) {
	s := newActivityExample(t)
	flash := glyph.Default().ActivityChanged

	var sawFlash bool
	for i := 0; i < 40 && !sawFlash; i++ {
		s = tickActivity(t, s, 1)
		if strings.Contains(drawActivity(t, s), flash) {
			sawFlash = true
		}
	}
	if !sawFlash {
		t.Errorf("ActivityRevision never flashed an unobserved change:\n%s", drawActivity(t, s))
	}
}

package componenttest

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	appfilters "github.com/jsdrews/tuilib/examples/app/filters"
	appmouse "github.com/jsdrews/tuilib/examples/app/mouse"
	appoutput "github.com/jsdrews/tuilib/examples/app/output"
	dataloading "github.com/jsdrews/tuilib/examples/data/loading"
	datarunlog "github.com/jsdrews/tuilib/examples/data/runlog"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// A component can only claim a click it actually receives. Screens route
// keys to the focused component — correctly, or "/" would open every filter
// at once — and it is easy to route mouse events the same way by accident.
// When that happens the component under the pointer never sees the press, so
// it cannot take focus or hand input back from its filter, and the symptom
// looks like a component bug rather than a routing one.
//
// This drives real example screens rather than components, because that is
// the layer where the mistake lives. It caught runlog, drilldown and loading
// forwarding mouse events only to whichever pane already had focus.
func TestScreensFanMouseOutToEveryComponent(t *testing.T) {
	const (
		w = 100
		h = 30
	)

	screens := map[string]func(theme.Theme) screen.Screen{
		"app/mouse":    appmouse.New,
		"app/filters":  appfilters.New,
		"data/loading": dataloading.New,
		"data/runlog":  datarunlog.New,
		"app/output":   appoutput.New,
	}

	for name, build := range screens {
		t.Run(name, func(t *testing.T) {
			s := build(theme.Dark())
			s.Init()
			s.OnEnter(nil)
			// Render once so every component holds a rect stamped in the
			// current generation; a stale rect declines events by design.
			geom.NextGen()
			s.Layout().Render(geom.New(0, 0, w, h))

			// Sweep the screen. Every press inside it should reach *some*
			// component and be acted on — a press that produces nothing
			// anywhere means the event never arrived.
			claimed := 0
			for y := 1; y < h-1; y++ {
				for x := 1; x < w-1; x += 7 {
					next, cmd := s.Update(press(x, y))
					s = next
					if cmd != nil {
						claimed++
					}
					// Re-render so rects stay in the current generation.
					geom.NextGen()
					s.Layout().Render(geom.New(0, 0, w, h))
				}
			}

			if claimed == 0 {
				t.Errorf("no press anywhere on the screen produced a command — " +
					"mouse events are not reaching the components")
			}
		})
	}
}

// The narrower version of the same failure: with focus on one pane, a press
// on a *different* pane must still reach it.
func TestClickOnUnfocusedPaneIsDelivered(t *testing.T) {
	const w, h = 100, 30

	for name, build := range map[string]func(theme.Theme) screen.Screen{
		"app/filters":  appfilters.New,
		"data/loading": dataloading.New,
		"data/runlog":  datarunlog.New,
		// app/output is deliberately absent: its right-hand side is a
		// read-only pane.Pane, so there is no second focusable component
		// for a click to be misrouted away from. It is covered by the
		// fan-out test above, which is the general assertion.
	} {
		t.Run(name, func(t *testing.T) {
			s := build(theme.Dark())
			s.Init()
			s.OnEnter(nil)
			geom.NextGen()
			s.Layout().Render(geom.New(0, 0, w, h))

			// The right-hand side of the screen belongs to a pane that does
			// not hold focus on entry.
			var got tea.Cmd
			for y := 4; y < 10 && got == nil; y++ {
				var next screen.Screen
				next, got = s.Update(press(w-10, y))
				s = next
				geom.NextGen()
				s.Layout().Render(geom.New(0, 0, w, h))
			}

			if got == nil {
				t.Errorf("presses on the unfocused pane produced nothing — " +
					"the screen is routing mouse events to the focused component only")
			}
		})
	}
}

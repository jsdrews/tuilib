package componenttest

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	tea "github.com/charmbracelet/bubbletea"

	appfilters "github.com/jsdrews/tuilib/examples/app/filters"
	dataloading "github.com/jsdrews/tuilib/examples/data/loading"
	datarunlog "github.com/jsdrews/tuilib/examples/data/runlog"
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// Clicking a pane must move the *keyboard*, not just the highlight.
//
// A clicked component lights its own border directly, but transferring focus
// happens by way of a focus.RequestMsg that only takes effect if the screen
// forwards it to its focus.Group. A screen that calls Group.Update solely for
// tab keys drops those requests: the clicked pane looks focused while every
// keystroke still goes to the pane that was focused before. Typing into a
// filter and watching the characters land somewhere else is the symptom.
func TestClickingAPaneMovesTheKeyboard(t *testing.T) {
	const w, h = 100, 30

	for name, build := range map[string]func(theme.Theme) screen.Screen{
		"app/filters":  appfilters.New,
		"data/loading": dataloading.New,
		"data/runlog":  datarunlog.New,
	} {
		t.Run(name, func(t *testing.T) {
			s := build(theme.Dark())
			s.Init()
			s.OnEnter(nil)
			render := func() { geom.NextGen(); s.Layout().Render(geom.New(0, 0, w, h)) }
			render()

			// Screens open with focus on their left pane. Click deep into
			// the right-hand pane, running the commands the click returns so
			// the focus request is actually delivered.
			s = deliver(s, press(w-10, 6), render)

			_ = s
			// Where a typed marker lands with the click, versus without it.
			// Comparing the two is what makes this independent of how a
			// given screen splits its panes: the click has to change the
			// answer. Asserting only that *some* filter opened would pass
			// while the filter opened on the pane focus never left.
			withClick, okWith := typeMarker(build, true, w, h)
			plain, okPlain := typeMarker(build, false, w, h)

			if !okWith {
				t.Fatalf("after clicking the right-hand pane, '/' opened no filter there — "+
					"the click lit the pane but the keyboard stayed on the pane focus "+
					"never left (typed %q went nowhere visible)", marker)
			}
			if okPlain && withClick == plain {
				t.Errorf("typing after clicking the right-hand pane landed in the same "+
					"place as without the click (column %d) — the click lit the pane but "+
					"left the keyboard behind", plain)
			}
		})
	}
}

// typeMarker opens a filter with "/" and types marker, optionally clicking
// the right-hand pane first. It reports where the marker was drawn.
func typeMarker(build func(theme.Theme) screen.Screen, click bool, w, h int) (int, bool) {
	s := build(theme.Dark())
	s.Init()
	s.OnEnter(nil)
	render := func() { geom.NextGen(); s.Layout().Render(geom.New(0, 0, w, h)) }
	render()

	if click {
		s = deliver(s, press(w-10, 6), render)
	}
	s = deliver(s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}, render)
	for _, r := range marker {
		s = deliver(s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}, render)
	}
	geom.NextGen()
	return columnOf(s.Layout().Render(geom.New(0, 0, w, h)), marker)
}

// deliver applies msg and re-injects any focus request it produced, the way
// the runtime would. Only focus requests are re-injected: running every
// command would execute the examples' tea.Tick timers, which block.
// marker is a string that will not occur in any example's own content.
const marker = "zqx"

// columnOf returns the column where want appears in a rendered screen.
func columnOf(view, want string) (int, bool) {
	for _, line := range strings.Split(view, "\n") {
		plain := ansi.Strip(line)
		if i := strings.Index(plain, want); i >= 0 {
			return ansi.StringWidth(plain[:i]), true
		}
	}
	return 0, false
}

func deliver(s screen.Screen, msg tea.Msg, render func()) screen.Screen {
	next, cmd := s.Update(msg)
	s = next
	render()
	// Only mouse events produce focus requests, and inspecting a command
	// means running it — which would fire the examples' tea.Tick timers and
	// sleep. Keys need no re-injection, so don't look.
	if _, isMouse := msg.(mouse.Msg); !isMouse {
		return s
	}
	for _, m := range flatten(cmd) {
		if _, ok := m.(focus.RequestMsg); !ok {
			continue
		}
		s, _ = s.Update(m)
		render()
	}
	return s
}

func flatten(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, flatten(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

// Enter and double-click are one verb (rule 14), but a screen has to opt in:
// the component reports the activation and the screen decides what "open"
// means. A screen that only matches an enter KeyMsg silently ignores double
// clicks, which is what runlog did — the commands list looked interactive
// and double-clicking a command did nothing.
//
// The assertion watches for the "$ <label>" line the screen appends before
// launching, so nothing is actually executed.
func TestRunlogDoubleClickStartsACommand(t *testing.T) {
	const w, h = 100, 30

	s := datarunlog.New(theme.Dark())
	s.Init()
	s.OnEnter(nil)
	render := func() string {
		geom.NextGen()
		return s.Layout().Render(geom.New(0, 0, w, h))
	}
	render()

	if _, ok := columnOf(render(), "$ "); ok {
		t.Fatalf("setup: a command was already marked as started")
	}

	// The commands list is the left pane; its first row sits just inside the
	// top border. Double-click it, then deliver whatever came back.
	dbl := mouse.Msg{
		MouseMsg: tea.MouseMsg{X: 5, Y: 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		Clicks:   2,
	}
	next, cmd := s.Update(dbl)
	s = next
	render()
	for _, m := range flatten(cmd) {
		s, _ = s.Update(m)
		render()
	}

	if _, ok := columnOf(render(), "$ "); !ok {
		t.Errorf("double-clicking a command did not start it — the screen " +
			"matches an enter key but ignores the list's activation")
	}
}

package navigation

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/internal/cmdtest"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/menu"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// The navigation screen is one a reader checks after a change, so what its
// package comment promises is asserted rather than trusted.

func home(t *testing.T) *Screen {
	t.Helper()
	return New(theme.New(palette.Dark())).(*Screen)
}

// opened is the screen choosing it pushes, or nil.
func opened(s *Screen, it menu.Item) screen.Screen {
	_, cmd := s.Update(menu.ChosenMsg{Item: it, Token: s.menu.FocusToken()})
	for _, m := range cmdtest.Run(cmd) {
		if p, ok := m.(screen.PushMsg); ok {
			return p.Screen
		}
	}
	return nil
}

func TestEveryNavigationItemOpensItsScreen(t *testing.T) {
	s := home(t)
	want := map[string]string{
		"screen stack":  "Cities",
		"sign in first": "Prescreen",
		"drilldown":     "Drilldown",
		"tabbed host":   "Tabs",
	}
	for _, it := range s.menu.Items() {
		if it.Group != "navigation" {
			continue
		}
		next := opened(s, it)
		if next == nil {
			t.Errorf("%s opened nothing", it.Label)
			continue
		}
		if title := next.Title(); !strings.HasPrefix(title, want[it.Label]) {
			t.Errorf("%s opened %q, want %q", it.Label, title, want[it.Label])
		}
		delete(want, it.Label)
	}
	for label := range want {
		t.Errorf("the navigation group lost %q", label)
	}
}

func TestASectionItemOpensItsSection(t *testing.T) {
	s := home(t)
	for _, it := range s.menu.Items() {
		if it.Group == "network" {
			if next := opened(s, it); next == nil || next.Title() != it.Label {
				t.Errorf("%s opened %v", it.Label, next)
			}
			return
		}
	}
	t.Fatal("the network group is gone")
}

// driver runs the app shell the way the runtime would, delivering what each
// command answers within cmdtest's grace and leaving timers asleep.
type driver struct{ m tea.Model }

func (d *driver) send(msg tea.Msg) {
	queue := []tea.Msg{msg}
	for steps := 0; len(queue) > 0 && steps < 50; steps++ {
		var cmd tea.Cmd
		d.m, cmd = d.m.Update(queue[0])
		queue = append(queue[1:], cmdtest.Run(cmd)...)
	}
}

func (d *driver) key(k string) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	switch k {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	}
	d.send(msg)
}

func (d *driver) crumb() string {
	return strings.TrimSpace(strings.Split(ansi.Strip(d.m.View()), "\n")[0])
}

// The whole trip, through the real shell: open the stack, push a detail, and
// unwind with esc. The breadcrumb names every level on the way.
func TestATripDownTheStackAndBack(t *testing.T) {
	d := &driver{m: app.New(app.Options{
		Root:       New(theme.New(palette.Dark())),
		Palettes:   []palette.Palette{palette.Dark()},
		SkipConfig: true,
	})}
	d.send(d.m.Init()())
	d.send(tea.WindowSizeMsg{Width: 100, Height: 30})

	// The first item, under the navigation header, is the screen stack.
	d.key("enter")
	if got := d.crumb(); !strings.Contains(got, "Navigation › Cities") {
		t.Fatalf("after opening the stack: %q", got)
	}
	d.key("enter")
	if got := d.crumb(); strings.Count(got, "›") != 2 {
		t.Fatalf("after opening a city: %q", got)
	}
	d.key("esc")
	d.key("esc")
	if got := d.crumb(); !strings.HasPrefix(got, "Navigation") || strings.Contains(got, "›") {
		t.Errorf("esc twice should be home again: %q", got)
	}
}

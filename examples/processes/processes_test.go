package processes

import (
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/internal/cmdtest"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/menu"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/runner"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// The processes screen is one a reader checks after a change, so what its
// package comment promises is asserted rather than trusted.

const w, h = 100, 30

func build(t *testing.T) *Screen {
	t.Helper()
	s := New(theme.New(palette.Dark())).(*Screen)
	s.Init()
	s.OnEnter(nil)
	render(s)
	return s
}

func render(s *Screen) string {
	geom.NextGen()
	return s.Layout().Render(geom.New(0, 0, w, h))
}

func out(s *Screen) string { return ansi.Strip(s.out.View()) }

// choose runs the command labelled label, the way enter on it does.
func choose(s *Screen, label string) tea.Cmd {
	for _, it := range s.menu.Items() {
		if it.Label == label {
			_, cmd := s.Update(menu.ChosenMsg{Item: it, Token: s.menu.FocusToken()})
			render(s)
			return cmd
		}
	}
	panic("no command " + label)
}

// Enter and a double click are one verb, and a screen has to opt in to the
// second: a screen matching only enter ignores double clicks. The first row is
// the capture group's header, so the first command is the row below it.
func TestDoubleClickRunsACommand(t *testing.T) {
	s := build(t)
	dbl := mouse.Msg{
		MouseMsg: tea.MouseMsg{X: 5, Y: 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		Clicks:   2,
	}
	_, cmd := s.Update(dbl)
	render(s)
	for _, m := range cmdtest.Run(cmd) {
		if _, chosen := m.(menu.ChosenMsg); chosen {
			s.Update(m)
			render(s)
		}
	}
	if !strings.Contains(out(s), "$ "+commands[0].label) {
		t.Errorf("double-clicking a command did not start it:\n%s", out(s))
	}
}

// A capture's three messages land in the output pane, and Kill is offered
// only while one runs.
func TestCaptureStreamsIntoTheOutput(t *testing.T) {
	s := build(t)
	if k := s.Actions().Actions[0]; k.Label != "Kill" || k.Disabled == "" {
		t.Fatalf("Kill should be offered, disabled, with nothing running: %+v", k)
	}
	cmd := choose(s, "seq with sleep (50 lines)")
	if cmd == nil {
		t.Fatal("a capture command returned nothing to run")
	}
	s.Update(runner.CaptureStarted{Label: "seq"})
	s.Update(runner.CapturedLine{Text: "line 1"})
	s.Update(runner.CapturedLine{Text: "boom", Stderr: true})
	if k := s.Actions().Actions[0]; k.Disabled != "" {
		t.Errorf("Kill is disabled while a capture runs: %q", k.Disabled)
	}
	if again := choose(s, "burst (500 lines)"); again == nil {
		t.Error("starting a second capture said nothing")
	}
	s.Update(runner.Captured{Label: "seq"})
	render(s)
	v := out(s)
	for _, want := range []string{"line 1", "2> boom", "seq completed"} {
		if !strings.Contains(v, want) {
			t.Errorf("output lacks %q:\n%s", want, v)
		}
	}
}

// The tail toggles, and a tick from a tail that was stopped does nothing.
func TestTailStartsAndStops(t *testing.T) {
	s := build(t)
	choose(s, "synthetic tail (start / stop)")
	if !s.tailing {
		t.Fatal("the tail did not start")
	}
	gen := s.tailGen
	s.Update(tailMsg{gen})
	if !strings.Contains(out(s), "job#1") {
		t.Fatalf("a tick appended nothing:\n%s", out(s))
	}
	choose(s, "synthetic tail (start / stop)")
	before := out(s)
	if _, next := s.Update(tailMsg{gen}); next != nil || out(s) != before {
		t.Error("a stopped tail's tick still appended or re-armed")
	}
}

// Closing the console pops with OutputClosed, which is not an activation.
func TestOnEnterIgnoresTheConsoleClosing(t *testing.T) {
	s := build(t)
	n := s.activations
	s.OnEnter(app.OutputClosed{})
	if s.activations != n {
		t.Error("closing the console counted as an activation")
	}
	s.OnEnter(nil)
	if s.activations != n+1 {
		t.Error("a real activation was not counted")
	}
}

// A terminal command's result is shown on return.
func TestTerminalResultIsShown(t *testing.T) {
	s := build(t)
	if cmd := choose(s, "less /etc/hosts"); cmd == nil {
		t.Fatal("a terminal command returned nothing to run")
	}
	s.Update(runner.Result{Cmd: exec.Command("less", "/etc/hosts")})
	render(s)
	if v := ansi.Strip(s.about.View()); !strings.Contains(v, "/etc/hosts — exited") {
		t.Errorf("the about pane does not show the result:\n%s", v)
	}
}

// A theme swap keeps the output and the menu's cursor (rule 4).
func TestThemeSwapKeepsOutputAndCursor(t *testing.T) {
	s := build(t)
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	at, _ := s.menu.Selected()
	choose(s, "seq with sleep (50 lines)")
	s.Update(runner.CapturedLine{Text: "kept across the swap"})
	s.SetTheme(theme.New(palette.Light()))
	render(s)
	if now, _ := s.menu.Selected(); now.ID != at.ID {
		t.Errorf("menu cursor moved from %q to %q", at.ID, now.ID)
	}
	if !strings.Contains(out(s), "kept across the swap") {
		t.Error("the output was lost across the swap")
	}
}

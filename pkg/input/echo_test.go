package input

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/geom"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

// body returns the input's rendered content row, stripped of styling.
func body(t *testing.T, m Model) string {
	t.Helper()
	m.SetRect(geom.New(0, 0, 30, 3))
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if len(lines) < 2 {
		t.Fatalf("input rendered %d lines, want the 3-line pane", len(lines))
	}
	return lines[1]
}

func TestEchoNormalShowsTheText(t *testing.T) {
	m := New(Options{Initial: "hunter2"})

	if got := body(t, m); !strings.Contains(got, "hunter2") {
		t.Errorf("unmasked input rendered %q, want it to contain the value", got)
	}
}

func TestEchoMaskHidesTheTextButNotTheValue(t *testing.T) {
	m := New(Options{Initial: "hunter2", Echo: EchoMask})

	got := body(t, m)
	if strings.Contains(got, "hunter2") {
		t.Errorf("masked input rendered %q, leaking the value", got)
	}
	if want := strings.Repeat(string(DefaultMaskChar), len("hunter2")); !strings.Contains(got, want) {
		t.Errorf("masked input rendered %q, want %q", got, want)
	}
	if m.Value() != "hunter2" {
		t.Errorf("Value() = %q, want the real text — masking is display-only", m.Value())
	}
}

func TestMaskCharOverridesTheDefault(t *testing.T) {
	m := New(Options{Initial: "abc", Echo: EchoMask, MaskChar: '*'})

	if got := body(t, m); !strings.Contains(got, "***") {
		t.Errorf("input rendered %q, want the custom mask char", got)
	}
}

func TestEchoNoneRendersNothing(t *testing.T) {
	m := New(Options{Initial: "hunter2", Echo: EchoNone})

	got := strings.TrimSpace(strings.Trim(body(t, m), "│"))
	if got != "" {
		t.Errorf("EchoNone rendered %q, want an empty field", got)
	}
}

// The reveal affordance: flip a masked field open and shut with the value
// intact, so the user never retypes a password to check it.
func TestSetEchoRevealsWithoutTouchingTheValue(t *testing.T) {
	m := New(Options{Initial: "hunter2", Echo: EchoMask})

	m.SetEcho(EchoNormal)
	if got := body(t, m); !strings.Contains(got, "hunter2") {
		t.Errorf("revealed input rendered %q, want the value", got)
	}
	if m.Echo() != EchoNormal {
		t.Errorf("Echo() = %v, want EchoNormal", m.Echo())
	}

	m.SetEcho(EchoMask)
	if got := body(t, m); strings.Contains(got, "hunter2") {
		t.Errorf("re-masked input rendered %q, leaking the value", got)
	}
	if m.Value() != "hunter2" {
		t.Errorf("Value() = %q, want the value to survive both flips", m.Value())
	}
}

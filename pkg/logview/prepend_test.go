package logview

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jsdrews/tuilib/pkg/geom"
)

func numbered(from, to int) []string {
	var out []string
	for i := from; i < to; i++ {
		out = append(out, fmt.Sprintf("line %03d", i))
	}
	return out
}

func newScrolled(t *testing.T, maxLines int) Model {
	t.Helper()
	m := New(Options{Title: "logs", MaxLines: maxLines, Searchable: true})
	m.SetRect(geom.New(0, 0, 40, 12))
	m.AppendLines(numbered(100, 200))
	m.SetFollow(false)
	m.body.SetYOffset(10)
	return m
}

func TestPrependKeepsTheViewOnWhatWasRead(t *testing.T) {
	m := newScrolled(t, 0)
	first := strings.Split(m.body.View(), "\n")
	m.Prepend(numbered(50, 100))
	if m.Lines()[0] != "line 050" || len(m.Lines()) != 150 {
		t.Fatalf("lines = %d starting %q", len(m.Lines()), m.Lines()[0])
	}
	if got := m.body.YOffset(); got != 60 {
		t.Errorf("offset = %d, want 60 — the same lines on screen", got)
	}
	if after := strings.Split(m.body.View(), "\n"); after[3] != first[3] {
		t.Errorf("the view moved:\n%s\nvs\n%s", after[3], first[3])
	}
}

func TestPrependPastTheCapDropsTheNewestWhileScrolledBack(t *testing.T) {
	m := newScrolled(t, 120)
	m.Prepend(numbered(50, 100))
	lines := m.Lines()
	if len(lines) != 120 || lines[0] != "line 050" || lines[len(lines)-1] != "line 169" {
		t.Errorf("kept %q … %q (%d)", lines[0], lines[len(lines)-1], len(lines))
	}
}

func TestPrependKeepsTheCurrentMatch(t *testing.T) {
	m := newScrolled(t, 0)
	m.SetQuery("line 15")
	cur := m.matches[m.matchIdx].line
	curText := m.Lines()[cur]
	m.Prepend([]string{"line 150 again", "other"})
	if got := m.Lines()[m.matches[m.matchIdx].line]; got != curText {
		t.Errorf("current match is %q, want %q", got, curText)
	}
}

func TestAppendMarker(t *testing.T) {
	m := New(Options{Title: "logs"})
	m.SetRect(geom.New(0, 0, 60, 6))
	m.AppendMarker("── reconnected after 42s ──")
	if !strings.Contains(m.Lines()[0], "reconnected after 42s") {
		t.Errorf("marker = %q", m.Lines()[0])
	}
}

func TestLineNumbersSkipMarkers(t *testing.T) {
	m := New(Options{Title: "logs", LineNumbers: true})
	m.SetRect(geom.New(0, 0, 60, 10))
	m.AppendLines([]string{"alpha", "beta"})
	m.AppendMarker("-- reconnected --")
	m.Append("gamma")
	view := m.body.View()
	for _, want := range []string{"1 │ alpha", "2 │ beta", "3 │ gamma"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "4 │") {
		t.Error("the marker must not be numbered")
	}
	m.Prepend([]string{"older"})
	if !strings.Contains(m.body.View(), "1 │ older") || !strings.Contains(m.body.View(), "4 │ gamma") {
		t.Errorf("numbers count from the start of what is loaded:\n%s", m.body.View())
	}
}

package output

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/theme"
)

func press(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func newTestScreen(t *testing.T, buf *Buffer) *Screen {
	t.Helper()
	opts := OptionsFrom(theme.Dark())
	opts.ExportDir = t.TempDir()
	s := NewScreen(buf, opts)
	// Give the logview a rect, as layout would.
	geom.NextGen()
	s.Layout().Render(geom.New(0, 0, 100, 24))
	return s
}

// The screen mirrors the buffer rather than owning it, so records appended
// from the shell while the screen is on the stack have to show up.
func TestScreenMirrorsRecordsAppendedWhileOpen(t *testing.T) {
	buf := NewBuffer(0)
	s := newTestScreen(t, buf)

	buf.Append(Record{Level: LevelInfo, Source: "Deploy", Text: "started", Head: true})
	s.Update(nil)

	lines := s.lv.Lines()
	if len(lines) != 1 || !strings.Contains(lines[0], "started") {
		t.Fatalf("record did not reach the logview: %#v", lines)
	}
}

// Trimming drops records off the front, which invalidates an append-only
// mirror. The epoch is what tells the screen to rebuild instead.
func TestScreenRebuildsAfterTrim(t *testing.T) {
	buf := NewBuffer(20)
	s := newTestScreen(t, buf)

	for i := 0; i < 200; i++ {
		buf.Append(Record{Level: LevelInfo, Source: "go", Text: "line", Head: true})
		s.Update(nil)
	}

	if got, want := len(s.lv.Lines()), buf.Len(); got != want {
		t.Errorf("logview holds %d lines, buffer holds %d — mirror drifted", got, want)
	}
}

func TestClearKeyEmptiesTheBuffer(t *testing.T) {
	buf := NewBuffer(0)
	buf.Append(Record{Level: LevelInfo, Source: "Deploy", Text: "done", Head: true})
	s := newTestScreen(t, buf)

	s.Update(press("c"))

	if buf.Len() != 0 {
		t.Errorf("buffer still holds %d records after clear", buf.Len())
	}
	if got := len(s.lv.Lines()); got != 0 {
		t.Errorf("logview still shows %d lines after clear", got)
	}
}

func TestExportWritesEveryRecordByDefault(t *testing.T) {
	buf := NewBuffer(0)
	buf.Append(Record{Level: LevelInfo, Source: "kubectl", Text: "$ kubectl apply", Head: true})
	buf.Append(Record{Level: LevelInfo, Source: "terraform", Text: "$ terraform plan", Head: true})
	s := newTestScreen(t, buf)

	body := runExport(t, s)
	if !strings.Contains(body, "kubectl") || !strings.Contains(body, "terraform") {
		t.Errorf("export dropped records:\n%s", body)
	}
	if strings.HasPrefix(body, "# filter:") {
		t.Errorf("unfiltered export carries a filter header:\n%s", body)
	}
}

// Filter mode narrows what the pane shows, so it narrows the export — and a
// narrowed export says so on its first line, or it reads as the whole log
// once it is attached to a bug report.
func TestExportRespectsFilterModeAndSaysSo(t *testing.T) {
	buf := NewBuffer(0)
	buf.Append(Record{Level: LevelInfo, Source: "kubectl", Text: "$ kubectl apply", Head: true})
	buf.Append(Record{Level: LevelInfo, Source: "terraform", Text: "$ terraform plan", Head: true})
	s := newTestScreen(t, buf)

	s.lv.SetQuery("terraform")
	s.lv.SetFilterMode(true)

	body := runExport(t, s)
	if !strings.HasPrefix(body, "# filter: terraform\n") {
		t.Errorf("filtered export missing its header:\n%s", body)
	}
	if strings.Contains(body, "kubectl") {
		t.Errorf("filtered export included non-matching records:\n%s", body)
	}
}

// A bare search query highlights in place without hiding anything, so it
// must not narrow the export either.
func TestExportIgnoresABareSearchQuery(t *testing.T) {
	buf := NewBuffer(0)
	buf.Append(Record{Level: LevelInfo, Source: "kubectl", Text: "$ kubectl apply", Head: true})
	buf.Append(Record{Level: LevelInfo, Source: "terraform", Text: "$ terraform plan", Head: true})
	s := newTestScreen(t, buf)

	s.lv.SetQuery("terraform")

	body := runExport(t, s)
	if !strings.Contains(body, "kubectl") {
		t.Errorf("a bare query narrowed the export:\n%s", body)
	}
}

func TestExportIsPlainText(t *testing.T) {
	buf := NewBuffer(0)
	buf.Append(Record{Level: LevelError, Source: "Deploy", Text: "boom", Head: true})
	s := newTestScreen(t, buf)

	if body := runExport(t, s); strings.Contains(body, "\x1b[") {
		t.Errorf("export contains ANSI escapes:\n%q", body)
	}
}

// runExport presses the export key, follows the Notice back to the path it
// reports, and returns the file's contents.
func runExport(t *testing.T, s *Screen) string {
	t.Helper()

	_, cmd := s.Update(press("w"))
	if cmd == nil {
		t.Fatal("export key produced no command")
	}
	msg, ok := cmd().(Notice)
	if !ok {
		t.Fatalf("export returned %T, want Notice", cmd())
	}
	if msg.Level == LevelError {
		t.Fatalf("export failed: %s", msg.Text)
	}
	path := strings.TrimPrefix(msg.Text, "wrote ")

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading exported file: %v", err)
	}
	return string(b)
}

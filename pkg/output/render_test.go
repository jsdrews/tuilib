package output

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/theme"
)

func TestMain(m *testing.M) {
	// Without a TTY lipgloss falls back to the Ascii profile and strips
	// every style, so a render assertion would pass no matter what the code
	// emitted.
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

func at(hh, mm, ss int) time.Time {
	return time.Date(2026, 8, 12, hh, mm, ss, 0, time.UTC)
}

func themed() Options { return OptionsFrom(theme.Dark()) }

// The prefix is repeated on every line — including continuation lines — so
// that logview's filter mode can hide non-matching lines without orphaning a
// body line from the command that produced it. That repetition is the whole
// reason the format costs ~20 columns, so it is worth a test.
func TestEveryLineCarriesTheFullPrefix(t *testing.T) {
	o := Options{}
	head := o.RenderPlain(Record{Time: at(14, 23, 1), Level: LevelError, Source: "Deploy", Text: "failed", Head: true})
	body := o.RenderPlain(Record{Time: at(14, 23, 1), Level: LevelError, Source: "Deploy", Text: "+ kubectl apply"})

	for _, line := range []string{head, body} {
		if !strings.HasPrefix(line, "14:23:01 ERR Deploy") {
			t.Errorf("line missing full prefix: %q", line)
		}
	}
	if !strings.Contains(head, "› failed") {
		t.Errorf("head line missing › glyph: %q", head)
	}
	if !strings.Contains(body, "│ + kubectl apply") {
		t.Errorf("body line missing │ glyph: %q", body)
	}
}

func TestStderrGetsItsOwnGlyphNotAnErrorLevel(t *testing.T) {
	o := Options{}
	line := o.RenderPlain(Record{Time: at(9, 0, 0), Level: LevelInfo, Source: "go", Text: "compiling", Stderr: true})

	if !strings.Contains(line, "┃ compiling") {
		t.Errorf("stderr line missing ┃ glyph: %q", line)
	}
	// Tools that log progress to stderr are common; folding that into
	// severity would leave the badge permanently red.
	if strings.Contains(line, "ERR") {
		t.Errorf("stderr line was tagged ERR: %q", line)
	}
}

func TestSourceColumnAlignsAcrossSources(t *testing.T) {
	o := Options{}
	a := o.RenderPlain(Record{Time: at(1, 2, 3), Source: "Deploy", Text: "x", Head: true})
	b := o.RenderPlain(Record{Time: at(1, 2, 3), Source: "kubectl", Text: "x"})

	if strings.Index(a, "›") != strings.Index(b, "│") {
		t.Errorf("glyph column drifts between sources:\n%q\n%q", a, b)
	}
}

func TestLongSourceIsTruncatedNotWrapped(t *testing.T) {
	o := Options{SourceWidth: 6}
	line := o.RenderPlain(Record{Time: at(1, 2, 3), Source: "averylongscreenname", Text: "x"})

	if got, want := strings.Index(line, "│"), len("01:02:03 INF ")+6+1; got != want {
		t.Errorf("glyph at %d, want %d — source was not cut to SourceWidth", got, want)
	}
}

// Rule 17's property: logview's CurrentLineStyle paints a background across
// the whole row, so a full reset inside the prefix would punch a hole in it.
func TestStyledLineClosesWithForegroundResetOnly(t *testing.T) {
	o := themed()
	line := o.Render(Record{Time: at(1, 2, 3), Level: LevelError, Source: "Deploy", Text: "boom", Head: true})

	if strings.Contains(line, "\x1b[0m") {
		t.Errorf("prefix emitted a full reset, which would clobber the current-line background: %q", line)
	}
	if !strings.Contains(line, "\x1b[39m") {
		t.Errorf("expected foreground-only resets in %q", line)
	}
}

func TestStylingDoesNotChangeVisibleWidth(t *testing.T) {
	o := themed()
	r := Record{Time: at(1, 2, 3), Level: LevelInfo, Source: "Deploy", Text: "hello", Head: true}

	if styled, plain := xansi.StringWidth(o.Render(r)), xansi.StringWidth(o.RenderPlain(r)); styled != plain {
		t.Errorf("styled width %d != plain width %d", styled, plain)
	}
}

func TestBadgeStates(t *testing.T) {
	o := Options{}

	b := NewBuffer(0)
	if got := o.Badge(b); got.Show {
		t.Errorf("badge shown for an empty buffer: %+v", got)
	}

	b.Append(Record{Level: LevelInfo, Source: "Deploy", Text: "done", Head: true})
	if got := o.Badge(b); !got.Show || got.Text != "1 output" || got.Error {
		t.Errorf("after one info event: %+v, want {1 output, no tint}", got)
	}

	// The count going to zero must not take the door with it, or a log that
	// still has content becomes unreachable.
	b.MarkRead()
	if got := o.Badge(b); !got.Show || got.Text != "output" {
		t.Errorf("after MarkRead: %+v, want a bare \"output\" affordance", got)
	}

	b.Append(Record{Level: LevelError, Source: "Deploy", Text: "boom", Head: true})
	if got := o.Badge(b); !got.Error || got.Text != "1 output" {
		t.Errorf("after an error: %+v, want tinted \"1 output\"", got)
	}

	b.StartRun(7, "kubectl", nil)
	if got := o.Badge(b); !strings.HasPrefix(got.Text, runGlyph) {
		t.Errorf("in-flight run missing its marker: %+v", got)
	}
}

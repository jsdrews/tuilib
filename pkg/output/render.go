package output

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

const (
	// headGlyph marks a summary line, bodyGlyph a continuation. They sit in
	// the same column on every line, which is what makes the log scannable
	// once several sources are interleaved.
	headGlyph = "›"
	bodyGlyph = "│"
	errGlyph  = "┃"

	// runGlyph marks the badge while a capture is streaming. Deliberately
	// static: animating it would need a tick loop repainting the statusbar
	// for the whole duration of a build.
	runGlyph = "⟳"
)

// fg wraps s in a foreground SGR derived from c, closing with the
// foreground-only reset (\x1b[39m).
//
// lipgloss is the wrong tool here: it closes with a full \x1b[0m, and
// logview's CurrentLineStyle pads the current row out to the pane width to
// paint a background — a full reset inside the prefix would punch a hole in
// that highlight. This is CLAUDE.md rule 17's reasoning applied outside
// pkg/table. Going through the active color profile (rather than emitting
// truecolor directly) keeps the degradation to 256/16 colors that lipgloss
// would have done.
func fg(c lipgloss.TerminalColor, s string) string {
	if c == nil || s == "" {
		return s
	}
	if _, plain := c.(lipgloss.NoColor); plain {
		return s
	}
	seq := lipgloss.ColorProfile().FromColor(c).Sequence(false)
	if seq == "" {
		return s
	}
	return "\x1b[" + seq + "m" + s + "\x1b[39m"
}

// prefixWidth is the visible width of everything before the record's text:
// "HH:MM:SS " + "LVL " + source + " " + glyph + " ".
func (o Options) prefixWidth() int {
	return 9 + 4 + o.SourceWidth + 3
}

// Render formats one record as a display line, with the full prefix repeated
// on every line — including continuation lines.
//
// The repetition is the point. logview's filter mode (\) shows only matching
// lines, so a body line rendered bare would surface with no timestamp, no
// level, and no indication of which command produced it. It costs roughly 20
// columns of every line, which is the trade taken deliberately.
func (o Options) Render(r Record) string {
	o.fillDefaults()

	lvlColor := o.InfoColor
	if r.Level == LevelError {
		lvlColor = o.ErrorColor
	}
	var b strings.Builder
	b.WriteString(fg(o.TimeColor, r.Time.Format("15:04:05")))
	b.WriteByte(' ')
	b.WriteString(fg(lvlColor, r.Level.Tag()))
	b.WriteByte(' ')
	b.WriteString(fg(o.SourceColor, padTruncate(r.Source, o.SourceWidth)))
	b.WriteByte(' ')
	b.WriteString(fg(o.GutterColor, glyphFor(r)))
	b.WriteByte(' ')
	b.WriteString(r.Text)
	return b.String()
}

// glyphFor picks the column-3 marker: › for a summary, ┃ for a captured
// stderr line, │ for anything else.
func glyphFor(r Record) string {
	switch {
	case r.Head:
		return headGlyph
	case r.Stderr:
		return errGlyph
	}
	return bodyGlyph
}

// RenderPlain is Render without any styling, for export. A file full of SGR
// escapes is not what anyone wants to attach to a bug report.
func (o Options) RenderPlain(r Record) string {
	o.fillDefaults()

	return r.Time.Format("15:04:05") + " " + r.Level.Tag() + " " +
		padTruncate(r.Source, o.SourceWidth) + " " + glyphFor(r) + " " + r.Text
}

// RenderAll formats a batch in order.
func (o Options) RenderAll(rs []Record) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = o.Render(r)
	}
	return out
}

// Badge describes the statusbar affordance for a buffer's current state.
type Badge struct {
	// Text is the affordance label, e.g. "output", "3 output", "⟳ 2 output".
	Text string
	// Error is true when any unread record was an error, selecting
	// Options.BadgeErrorStyle.
	Error bool
	// Show is false while there is nothing to look at, in which case the
	// affordance is not rendered at all.
	Show bool
}

// Badge computes the affordance state.
//
// Two visible states rather than one: the count disappearing must not take
// the door with it, or a log that still has content in it becomes
// unreachable. So "output" bare once everything has been seen, "3 output"
// when it hasn't, and nothing at all only while there is genuinely nothing
// buffered and nothing running.
func (o Options) Badge(b *Buffer) Badge {
	if b == nil || (b.Len() == 0 && b.InFlight() == 0) {
		return Badge{}
	}
	var sb strings.Builder
	if b.InFlight() > 0 {
		sb.WriteString(runGlyph)
		sb.WriteByte(' ')
	}
	if n := b.Unread(); n > 0 {
		sb.WriteString(strconv.Itoa(n))
		sb.WriteByte(' ')
	}
	sb.WriteString("output")
	return Badge{Text: sb.String(), Error: b.UnreadError(), Show: true}
}

// RenderBadge returns the styled affordance, or "" when it should not be
// shown. pkg/app places it in the statusbar's right slot, ahead of the
// version string.
func (o Options) RenderBadge(b *Buffer) string {
	bd := o.Badge(b)
	if !bd.Show {
		return ""
	}
	if bd.Error {
		return o.BadgeErrorStyle.Render(bd.Text)
	}
	return o.BadgeStyle.Render(bd.Text)
}

// padTruncate pads s with spaces to w visible cells, or cuts it to fit.
// ANSI-aware, though sources are normally plain.
func padTruncate(s string, w int) string {
	if w <= 0 {
		return s
	}
	vw := xansi.StringWidth(s)
	if vw > w {
		return xansi.Cut(s, 0, w)
	}
	return s + strings.Repeat(" ", w-vw)
}

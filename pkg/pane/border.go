package pane

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type BorderPosition int

const (
	TopLeftBorder BorderPosition = iota
	TopMiddleBorder
	TopRightBorder
	BottomLeftBorder
	BottomMiddleBorder
	BottomRightBorder
)

// SlotBracketStyle controls how embedded slot text (titles, labels, scroll
// percentages) is bracketed against the surrounding border line.
type SlotBracketStyle int

const (
	// SlotBracketsCorners wraps slot text with inverted corner glyphs so it
	// looks like a tab dipping into the pane. On a normal border: `┐ text ┌`.
	// This is the default and matches pug's aesthetic.
	SlotBracketsCorners SlotBracketStyle = iota
	// SlotBracketsNone draws the border edge straight through the slot region,
	// so text sits inline on the border line: `── text ──`.
	SlotBracketsNone
	// SlotBracketsTees uses junction tees pointing into the slot text:
	// `─┤ text ├─`.
	SlotBracketsTees
)

// Borderize wraps content in the given border with up to six embedded text
// slots (top/bottom × left/middle/right). Embedded text is flanked by
// inverted corner glyphs so it renders like `┤ title ├`. The border and all
// slot glyphs are drawn in color; embedded text passes through verbatim, so
// pre-style it with lipgloss if you want bold/colored labels.
func Borderize(
	content string,
	border lipgloss.Border,
	color lipgloss.TerminalColor,
	embedded map[BorderPosition]string,
	slotBrackets SlotBracketStyle,
) string {
	if embedded == nil {
		embedded = make(map[BorderPosition]string)
	}
	style := lipgloss.NewStyle().Foreground(color)
	width := lipgloss.Width(content)

	var leftBracket, rightBracket string
	switch slotBrackets {
	case SlotBracketsTees:
		leftBracket = border.MiddleRight
		rightBracket = border.MiddleLeft
	case SlotBracketsNone:
		leftBracket, rightBracket = "", ""
	default: // SlotBracketsCorners
		leftBracket = border.TopRight
		rightBracket = border.TopLeft
	}

	enclose := func(text string) string {
		if text == "" {
			return ""
		}
		return fmt.Sprintf("%s%s%s",
			style.Render(leftBracket),
			text,
			style.Render(rightBracket),
		)
	}

	buildRow := func(left, mid, right, leftCorner, inbetween, rightCorner string) string {
		left = enclose(left)
		mid = enclose(mid)
		right = enclose(right)
		remaining := max(0, width-lipgloss.Width(left)-lipgloss.Width(mid)-lipgloss.Width(right))
		leftLen := max(0, (width/2)-lipgloss.Width(left)-(lipgloss.Width(mid)/2))
		rightLen := max(0, remaining-leftLen)
		s := left +
			style.Render(strings.Repeat(inbetween, leftLen)) +
			mid +
			style.Render(strings.Repeat(inbetween, rightLen)) +
			right
		s = lipgloss.NewStyle().Inline(true).MaxWidth(width).Render(s)
		return style.Render(leftCorner) + s + style.Render(rightCorner)
	}

	return strings.Join([]string{
		buildRow(
			embedded[TopLeftBorder], embedded[TopMiddleBorder], embedded[TopRightBorder],
			border.TopLeft, border.Top, border.TopRight,
		),
		lipgloss.NewStyle().
			BorderForeground(color).
			Border(border, false, true, false, true).
			Render(content),
		buildRow(
			embedded[BottomLeftBorder], embedded[BottomMiddleBorder], embedded[BottomRightBorder],
			border.BottomLeft, border.Bottom, border.BottomRight,
		),
	}, "\n")
}

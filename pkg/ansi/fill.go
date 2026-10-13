package ansi

import "strings"

// Fill wraps s in the SGR sequence open, and puts back whatever part of open
// s turns off along the way. A full reset (\x1b[0m) inside s re-opens all of
// it; a background reset (\x1b[49m) re-opens only the background; a bold
// reset (\x1b[22m) only the weight; and so on for each attribute open sets.
// The result closes with a full reset.
//
// This is how a highlight survives the content inside it. A selected row
// painted with one lipgloss.Style.Render loses its background at the first
// cell that carries its own style, because lipgloss closes every style with
// \x1b[0m. Fill makes the highlight span the row whatever the row contains.
//
// Sequences are parsed parameter by parameter, so the operands of extended
// colours are never mistaken for resets: the 0 in \x1b[38;2;255;0;0m is a
// colour component, and \x1b[0;31m turns the foreground back on after the
// reset rather than having Fill overwrite it. Escapes other than SGR, such as
// OSC 8 hyperlinks, pass through untouched.
//
// An open with no SGR attributes returns s unchanged.
func Fill(s, open string) string {
	want := sgrTokens(open)
	if len(want.tokens) == 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + len(open) + 8)
	b.WriteString(open)
	for i := 0; i < len(s); {
		params, end, ok := sgrAt(s, i)
		if !ok {
			b.WriteByte(s[i])
			i++
			continue
		}
		b.WriteString(s[i:end])
		if cleared := clearedBy(params); len(cleared) > 0 {
			b.WriteString(reopen(want, cleared))
		}
		i = end
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

// RenderFill is Fill for a renderer such as lipgloss.Style.Render, without
// this package importing lipgloss: the renderer's opening sequence is found
// by rendering a probe. Text with no escapes in it renders exactly as render
// would, so plain rows are byte-identical to an ordinary Render.
func RenderFill(render func(...string) string, s string) string {
	if !strings.ContainsRune(s, '\x1b') {
		return render(s)
	}
	const probe = "\x00tuilib-fill-probe\x00"
	r := render(probe)
	i := strings.Index(r, probe)
	if i <= 0 {
		return render(s)
	}
	return Fill(s, r[:i])
}

// attr is one independently resettable SGR attribute.
type attr int

const (
	attrFG attr = iota
	attrBG
	attrUnderlineColor
	attrBold
	attrFaint
	attrItalic
	attrUnderline
	attrBlink
	attrReverse
	attrConceal
	attrStrike
	attrOverline
	attrCount
)

// sgrAt reports whether an SGR sequence (CSI … m) starts at s[i], returning
// its parameter string and the index just past it.
func sgrAt(s string, i int) (params string, end int, ok bool) {
	if i+1 >= len(s) || s[i] != '\x1b' || s[i+1] != '[' {
		return "", 0, false
	}
	j := i + 2
	for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == ';' || s[j] == ':') {
		j++
	}
	if j >= len(s) || s[j] != 'm' {
		return "", 0, false
	}
	return s[i+2 : j], j + 1, true
}

// openAttrs is what an opening sequence sets: per attribute, the parameters
// that set it (operands included), and the order the attributes appeared in.
type openAttrs struct {
	tokens map[attr][]string
	order  []attr
}

// sgrTokens collects what open sets.
func sgrTokens(open string) openAttrs {
	out := openAttrs{tokens: map[attr][]string{}}
	for i := 0; i < len(open); {
		params, end, ok := sgrAt(open, i)
		if !ok {
			i++
			continue
		}
		walkSGR(params, func(a attr, set bool, tokens []string) {
			if !set {
				delete(out.tokens, a)
				return
			}
			if _, seen := out.tokens[a]; !seen {
				out.order = append(out.order, a)
			}
			out.tokens[a] = tokens
		})
		i = end
	}
	return out
}

// clearedBy is the set of attributes params leaves turned off: cleared, and
// not set again later in the same sequence.
func clearedBy(params string) map[attr]bool {
	cleared := map[attr]bool{}
	walkSGR(params, func(a attr, set bool, _ []string) {
		if set {
			delete(cleared, a)
		} else {
			cleared[a] = true
		}
	})
	return cleared
}

func reopen(want openAttrs, cleared map[attr]bool) string {
	var parts []string
	for _, a := range want.order {
		if cleared[a] && len(want.tokens[a]) > 0 {
			parts = append(parts, want.tokens[a]...)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(parts, ";") + "m"
}

// walkSGR reports each attribute an SGR parameter string sets or clears, in
// order. Set reports carry the parameter tokens that did the setting.
func walkSGR(params string, f func(a attr, set bool, tokens []string)) {
	toks := strings.Split(params, ";")
	clearAll := func() {
		for a := attr(0); a < attrCount; a++ {
			f(a, false, nil)
		}
	}
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		head, _, sub := strings.Cut(t, ":")
		switch head {
		case "", "0":
			if !sub {
				clearAll()
			}
		case "38", "48", "58":
			a := map[string]attr{"38": attrFG, "48": attrBG, "58": attrUnderlineColor}[head]
			n := 1
			if !sub && i+1 < len(toks) {
				switch toks[i+1] {
				case "5":
					n = 3
				case "2":
					n = 5
				}
			}
			n = min(n, len(toks)-i)
			f(a, true, toks[i:i+n])
			i += n - 1
		case "39":
			f(attrFG, false, nil)
		case "49":
			f(attrBG, false, nil)
		case "59":
			f(attrUnderlineColor, false, nil)
		case "1":
			f(attrBold, true, []string{t})
		case "2":
			f(attrFaint, true, []string{t})
		case "22":
			f(attrBold, false, nil)
			f(attrFaint, false, nil)
		case "3":
			f(attrItalic, true, []string{t})
		case "23":
			f(attrItalic, false, nil)
		case "4", "21":
			f(attrUnderline, true, []string{t})
		case "24":
			f(attrUnderline, false, nil)
		case "5", "6":
			f(attrBlink, true, []string{t})
		case "25":
			f(attrBlink, false, nil)
		case "7":
			f(attrReverse, true, []string{t})
		case "27":
			f(attrReverse, false, nil)
		case "8":
			f(attrConceal, true, []string{t})
		case "28":
			f(attrConceal, false, nil)
		case "9":
			f(attrStrike, true, []string{t})
		case "29":
			f(attrStrike, false, nil)
		case "53":
			f(attrOverline, true, []string{t})
		case "55":
			f(attrOverline, false, nil)
		default:
			switch {
			case len(head) == 2 && head[0] == '3' && head[1] <= '7',
				len(head) == 2 && head[0] == '9' && head[1] <= '7':
				f(attrFG, true, []string{t})
			case len(head) == 2 && head[0] == '4' && head[1] <= '7',
				len(head) == 3 && head[:2] == "10" && head[2] <= '7':
				f(attrBG, true, []string{t})
			}
		}
	}
}

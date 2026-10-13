// PROTOTYPE — throwaway. Delete once the "material surfaces" question is settled.
//
// Question: does a card/surface look (panes with a filled background, not just
// a line border) read well enough in a terminal to be worth building into the
// theme system?
//
// Five variants of one realistic screen (list + table + detail), switchable
// live. Nothing here uses tuilib's components: every cell is painted by hand
// so the variants can disagree about structure freely.
//
//	task prototype:surfaces
//
// ←/→ surface   t/T palette   tab focus   j/k move   m modal   q quit
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/theme"
)

type palette struct {
	name                          string
	screen, card, cardFocus       lipgloss.TerminalColor
	header, shadow                lipgloss.TerminalColor
	border, borderActive          lipgloss.TerminalColor
	fg, muted, accent             lipgloss.TerminalColor
	sel, selFG, selDim            lipgloss.TerminalColor
	bar, barFG                    lipgloss.TerminalColor
	ok, warn, bad                 lipgloss.TerminalColor
	modal, modalBorder, modalHead lipgloss.TerminalColor
}

// derive builds the surface palette from an existing tuilib Theme. Only the
// surface tones (screen, card, header, shadow, selection) are invented; every
// other colour is the theme's own token. This is the question under test:
// can surfaces be derived, or must every palette hand-pick them?
func derive(t theme.Theme) palette {
	rgb := func(c lipgloss.TerminalColor) [3]float64 {
		s := fmt.Sprint(c)
		if lc, ok := c.(lipgloss.Color); ok {
			s = string(lc)
		}
		k := termenv.ConvertToRGB(termenv.TrueColor.Color(s))
		return [3]float64{k.R, k.G, k.B}
	}
	hex := func(v [3]float64) lipgloss.TerminalColor {
		return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", int(v[0]*255+.5), int(v[1]*255+.5), int(v[2]*255+.5)))
	}
	mix := func(a, b [3]float64, f float64) [3]float64 {
		return [3]float64{a[0] + (b[0]-a[0])*f, a[1] + (b[1]-a[1])*f, a[2] + (b[2]-a[2])*f}
	}
	lum := func(v [3]float64) float64 { return .2126*v[0] + .7152*v[1] + .0722*v[2] }
	black, white := [3]float64{}, [3]float64{1, 1, 1}

	base, fg := rgb(t.BarBG), rgb(t.BarFG)
	isDark := lum(base) < lum(fg)

	var screen, card, cardFocus, header, shadow, modal [3]float64
	if isDark {
		card = base
		screen = mix(base, black, .35)
		cardFocus = mix(base, fg, .06)
		header = mix(base, fg, .12)
		shadow = mix(screen, black, .6)
		modal = mix(base, fg, .10)
	} else {
		card = mix(base, white, .85)
		screen = mix(card, black, .07)
		cardFocus = mix(base, white, .95)
		header = mix(card, black, .10)
		shadow = mix(screen, black, .22)
		modal = white
	}
	name := t.Name
	if !isDark {
		name += " (light)"
	}
	return palette{
		name: name, screen: hex(screen), card: hex(card), cardFocus: hex(cardFocus),
		header: hex(header), shadow: hex(shadow),
		border: t.BorderInactive, borderActive: t.BorderActive,
		fg: t.BarFG, muted: t.Muted, accent: t.Accent,
		sel: hex(mix(card, rgb(t.BorderActive), .45)), selFG: t.Current,
		selDim: hex(mix(card, fg, .12)),
		bar:    hex(screen), barFG: t.BarFG,
		ok: t.InfoBG, warn: t.KeyFG, bad: t.ErrorBG,
		modal: hex(modal), modalBorder: t.Accent, modalHead: hex(mix(modal, fg, .08)),
	}
}

// --- spans: every visible cell carries an explicit background --------------

type span struct {
	text string
	fg   lipgloss.TerminalColor
	bold bool
}

type line struct {
	spans    []span
	selected bool
}

func paint(text string, fg, bg lipgloss.TerminalColor, bold bool) string {
	s := lipgloss.NewStyle().Background(bg).Bold(bold)
	if fg != nil {
		s = s.Foreground(fg)
	}
	return s.Render(text)
}

func renderLine(l line, w int, bg, fg lipgloss.TerminalColor) string {
	var b strings.Builder
	used := 0
	for _, sp := range l.spans {
		if used >= w {
			break
		}
		t := sp.text
		if n := len([]rune(t)); used+n > w {
			t = string([]rune(t)[:w-used])
		}
		used += len([]rune(t))
		f := sp.fg
		if f == nil {
			f = fg
		}
		b.WriteString(paint(t, f, bg, sp.bold))
	}
	if used < w {
		b.WriteString(paint(strings.Repeat(" ", w-used), fg, bg, false))
	}
	return b.String()
}

// --- the screen's content ---------------------------------------------------

type model struct {
	w, h     int
	variant  int
	focus    int // 0 list, 1 table, 2 detail
	cursor   [3]int
	p        palette
	palettes []palette
	pi       int
	modal    bool
}

var clusters = []string{"prod-us-east", "prod-eu-west", "staging", "dev-sandbox", "perf-lab", "dr-standby", "edge-ap-south", "ci-runners"}

type app struct{ name, ns, status, ready, age string }

var apps = []app{
	{"api-gateway", "core", "Synced", "6/6", "14d"},
	{"billing", "payments", "OutOfSync", "3/4", "2d"},
	{"cache-redis", "infra", "Synced", "3/3", "41d"},
	{"auth", "core", "Degraded", "0/2", "6h"},
	{"search-indexer", "data", "Synced", "4/4", "9d"},
	{"notifications", "comms", "Running", "2/3", "1h"},
	{"reports", "data", "Synced", "1/1", "22d"},
	{"web-frontend", "web", "Synced", "8/8", "3d"},
	{"ml-scorer", "data", "OutOfSync", "2/2", "5d"},
	{"audit-log", "infra", "Synced", "2/2", "60d"},
}

func (m model) statusColor(s string) lipgloss.TerminalColor {
	switch s {
	case "Synced":
		return m.p.ok
	case "Degraded":
		return m.p.bad
	default:
		return m.p.warn
	}
}

func (m model) listLines() []line {
	var ls []line
	for i, c := range clusters {
		mark := "  "
		if i == m.cursor[0] {
			mark = "▸ "
		}
		ls = append(ls, line{spans: []span{{text: mark + c}}, selected: i == m.cursor[0]})
	}
	return ls
}

func (m model) tableLines() []line {
	col := func(s string, w int) string { return fmt.Sprintf("%-*s", w, s) }
	ls := []line{{spans: []span{{text: col("NAME", 16) + col("NAMESPACE", 11) + col("STATUS", 11) + col("READY", 7) + "AGE", fg: m.p.muted, bold: true}}}}
	for i, a := range apps {
		ls = append(ls, line{
			spans: []span{
				{text: col(a.name, 16)},
				{text: col(a.ns, 11), fg: m.p.muted},
				{text: col(a.status, 11), fg: m.statusColor(a.status)},
				{text: col(a.ready, 7)},
				{text: a.age, fg: m.p.muted},
			},
			selected: i == m.cursor[1],
		})
	}
	return ls
}

func (m model) detailLines() []line {
	a := apps[m.cursor[1]]
	kv := func(k, v string, vf lipgloss.TerminalColor) line {
		return line{spans: []span{{text: fmt.Sprintf("%-12s", k), fg: m.p.muted}, {text: v, fg: vf}}}
	}
	fields := []line{
		kv("name", a.name, nil),
		kv("cluster", clusters[m.cursor[0]], nil),
		kv("namespace", a.ns, nil),
		kv("status", a.status, m.statusColor(a.status)),
		kv("ready", a.ready, nil),
		kv("revision", "a91f3c2  (main)", m.p.accent),
		kv("synced", a.age+" ago", nil),
	}
	for i := range fields {
		fields[i].selected = i == m.cursor[2]
	}
	return fields
}

// --- cards: one function per variant ---------------------------------------

type card struct {
	title, slot string
	focused     bool
	lines       []line
}

// variantNames index renderCard's cases; B (outline + fill) was dropped.
var variantNames = map[int]string{
	0: "Outlined (today)",
	2: "Filled",
	3: "Banded",
	4: "Elevated",
	5: "Accented (candidate)",
	6: "Ruled (candidate)",
	7: "Lipped (candidate)",
	8: "Bare (candidate)",
}

var surfaces = []int{0, 2, 3, 4, 5, 6, 7, 8}

func (m model) selBG(c card) lipgloss.TerminalColor {
	if c.focused {
		return m.p.sel
	}
	return m.p.selDim
}

func (m model) body(c card, w, h int, bg lipgloss.TerminalColor, pad int) []string {
	var out []string
	padS := paint(strings.Repeat(" ", pad), nil, bg, false)
	for i := 0; i < h; i++ {
		l := line{}
		if i < len(c.lines) {
			l = c.lines[i]
		}
		lbg, lfg := bg, m.p.fg
		if l.selected {
			lbg = m.selBG(c)
			if c.focused {
				lfg = m.p.selFG
			}
		}
		out = append(out, padS+renderLine(l, w-2*pad, lbg, lfg)+padS)
	}
	return out
}

func (m model) lineBorder(c card, w, h int, bg lipgloss.TerminalColor, rounded bool) []string {
	bc := m.p.border
	if c.focused {
		bc = m.p.borderActive
	}
	tl, tr, bl, br := "┌", "┐", "└", "┘"
	if rounded {
		tl, tr, bl, br = "╭", "╮", "╰", "╯"
	}
	g := func(s string) string { return paint(s, bc, bg, false) }
	title := " " + c.title + " "
	slot := " " + c.slot + " "
	fill := w - 2 - 1 - len([]rune(title)) - len([]rune(slot)) - 1
	if fill < 0 {
		fill = 0
	}
	top := g(tl+"─") + paint(title, m.p.fg, bg, true) + g(strings.Repeat("─", fill)) + paint(slot, m.p.muted, bg, false) + g("─"+tr)
	out := []string{top}
	for _, row := range m.body(c, w-2, h-2, bg, 0) {
		out = append(out, g("│")+row+g("│"))
	}
	return append(out, g(bl+strings.Repeat("─", w-2)+br))
}

func (m model) titleRow(c card, w int, bg lipgloss.TerminalColor, tfg lipgloss.TerminalColor, pad int) string {
	padS := paint(strings.Repeat(" ", pad), nil, bg, false)
	inner := w - 2*pad
	t := paint(c.title, tfg, bg, true)
	s := paint(c.slot, m.p.muted, bg, false)
	gap := inner - len([]rune(c.title)) - len([]rune(c.slot))
	if gap < 1 {
		gap = 1
	}
	return padS + t + paint(strings.Repeat(" ", gap), nil, bg, false) + s + padS
}

func (m model) renderCard(c card, w, h int) []string {
	p := m.p
	switch m.variant {
	case 0:
		return m.lineBorder(c, w, h, lipgloss.NoColor{}, false)
	case 1:
		bg := p.card
		if c.focused {
			bg = p.cardFocus
		}
		return m.lineBorder(c, w, h, bg, true)
	case 2:
		bg := p.card
		tfg := p.fg
		if c.focused {
			bg = p.header
			tfg = p.accent
		}
		out := []string{paint(strings.Repeat("▄", w), bg, p.screen, false)}
		out = append(out, m.titleRow(c, w, bg, tfg, 2))
		out = append(out, m.body(c, w, h-3, bg, 2)...)
		return append(out, paint(strings.Repeat("▀", w), bg, p.screen, false))
	case 3:
		hbg, hfg := p.header, p.fg
		if c.focused {
			hbg, hfg = p.borderActive, p.selFG
		}
		out := []string{m.titleRow(c, w, hbg, hfg, 1)}
		return append(out, m.body(c, w, h-1, p.card, 1)...)
	case 5: // Accented: unfilled, left bar coloured on focus, text rows top and bottom
		bg := p.screen
		bar := paint("▌", p.border, bg, false)
		tfg := p.fg
		if c.focused {
			bar = paint("▌", p.accent, bg, false)
			tfg = p.accent
		}
		rows := append([]string{m.titleRow(c, w-1, bg, tfg, 1)}, m.body(c, w-1, h-2, bg, 1)...)
		rows = append(rows, m.footRow(w-1, bg, 1))
		var out []string
		for _, r := range rows {
			out = append(out, bar+r)
		}
		return out
	case 6: // Ruled: a title rule on top, a slot rule on the bottom, no sides, no fill
		bg := p.screen
		rc := p.border
		tfg := p.fg
		if c.focused {
			rc, tfg = p.borderActive, p.accent
		}
		rule := func(left string, lfg lipgloss.TerminalColor, right string) string {
			fill := w - len([]rune(left)) - len([]rune(right)) - 2
			if fill < 0 {
				fill = 0
			}
			return paint(left+" ", lfg, bg, true) + paint(strings.Repeat("─", fill), rc, bg, false) + paint(" "+right, p.muted, bg, false)
		}
		out := []string{rule(c.title, tfg, c.slot)}
		out = append(out, m.body(c, w, h-2, bg, 0)...)
		return append(out, rule("", nil, fmt.Sprintf("%d rows", len(c.lines))))
	case 7: // Lipped: Filled with eighth-block lips
		bg := p.card
		tfg := p.fg
		if c.focused {
			bg, tfg = p.header, p.accent
		}
		out := []string{paint(strings.Repeat("▁", w), bg, p.screen, false)}
		out = append(out, m.titleRow(c, w, bg, tfg, 2))
		out = append(out, m.body(c, w, h-4, bg, 2)...)
		out = append(out, m.footRow(w, bg, 2))
		return append(out, paint(strings.Repeat("▔", w), bg, p.screen, false))
	case 8: // Bare: edge cells kept, nothing drawn but the text rows
		bg := p.screen
		tfg := p.muted
		if c.focused {
			tfg = p.accent
		}
		out := []string{m.titleRow(c, w, bg, tfg, 1)}
		out = append(out, m.body(c, w, h-2, bg, 1)...)
		return append(out, m.footRow(w, bg, 1))
	default: // 4 — elevated
		bg := p.card
		tfg := p.fg
		if c.focused {
			bg = p.cardFocus
			tfg = p.accent
		}
		cw, ch := w-1, h-1
		var out []string
		stripe := func(i int) string {
			if c.focused {
				return paint("▎", p.accent, bg, false)
			}
			return paint(" ", nil, bg, false)
		}
		rows := append([]string{m.titleRow(c, cw-1, bg, tfg, 1)}, m.body(c, cw-1, ch-1, bg, 1)...)
		for i, r := range rows {
			sh := paint("▌", p.shadow, p.screen, false)
			if i == 0 {
				sh = paint("▖", p.shadow, p.screen, false)
			}
			out = append(out, stripe(i)+r+sh)
		}
		return append(out, paint(" ", nil, p.screen, false)+paint(strings.Repeat("▀", cw), p.shadow, p.screen, false))
	}
}

// --- composition ------------------------------------------------------------

func (m model) footRow(w int, bg lipgloss.TerminalColor, pad int) string {
	padS := paint(strings.Repeat(" ", pad), nil, bg, false)
	t := "↑↓ move  / filter"
	return padS + renderLine(line{spans: []span{{text: t, fg: m.p.muted}}}, w-2*pad, bg, m.p.muted) + padS
}

func (m model) screenBG() lipgloss.TerminalColor {
	if m.variant == 0 {
		return lipgloss.NoColor{}
	}
	return m.p.screen
}

func (m model) gap(n int) string { return paint(strings.Repeat(" ", n), nil, m.screenBG(), false) }

func (m model) bar(left, right string, bg, fg lipgloss.TerminalColor) string {
	gap := m.w - len([]rune(left)) - len([]rune(right))
	if gap < 1 {
		gap = 1
	}
	return paint(left+strings.Repeat(" ", gap)+right, fg, bg, false)
}

func (m model) View() string {
	if m.w == 0 {
		return ""
	}
	g := 0 // gutter between cards; variant A uses none, like today
	if m.variant > 0 {
		g = 1
	}
	bodyH := m.h - 2 - 2*g
	leftW := 26
	rightW := m.w - leftW - 3*g
	detailH := 11
	tableH := bodyH - detailH - g
	if tableH < 5 || rightW < 20 {
		return "terminal too small"
	}

	left := m.renderCard(card{title: "Clusters", slot: fmt.Sprintf("%d/%d", m.cursor[0]+1, len(clusters)), focused: m.focus == 0, lines: m.listLines()}, leftW, bodyH)
	right := m.renderCard(card{title: "Applications", slot: "/ filter", focused: m.focus == 1, lines: m.tableLines()}, rightW, tableH)
	if g > 0 {
		right = append(right, m.gap(rightW))
	}
	right = append(right, m.renderCard(card{title: apps[m.cursor[1]].name, slot: "inspector", focused: m.focus == 2, lines: m.detailLines()}, rightW, detailH)...)

	var rows []string
	rows = append(rows, m.bar(" tuilib › Clusters › "+clusters[m.cursor[0]], "prototype ", m.p.bar, m.p.barFG))
	for i := 0; i < g; i++ {
		rows = append(rows, m.gap(m.w))
	}
	for i := 0; i < bodyH; i++ {
		rows = append(rows, m.gap(g)+left[i]+m.gap(g)+right[i]+m.gap(g))
	}
	for i := 0; i < g; i++ {
		rows = append(rows, m.gap(m.w))
	}
	rows = append(rows, m.bar(fmt.Sprintf(" %s · %s (%d/%d) ", variantNames[m.variant], m.p.name, m.pi+1, len(m.palettes)), "←/→ surface  t/T palette  tab focus  j/k  m modal  q ", m.p.bar, m.p.barFG))

	if m.modal {
		rows = m.overlay(rows)
	}
	return strings.Join(rows, "\n")
}

func (m model) overlay(rows []string) []string {
	mw, mh := 44, 7
	c := card{title: "Sync 2 applications?", slot: "", focused: true, lines: []line{
		{},
		{spans: []span{{text: "billing, ml-scorer will be synced to main."}}},
		{},
		{spans: []span{{text: "        [ Cancel ]    ", fg: m.p.muted}, {text: "[ Sync ]", fg: m.p.accent, bold: true}}},
	}}
	saved := m.p
	m.p.card, m.p.cardFocus, m.p.header = m.p.modal, m.p.modal, m.p.modalHead
	m.p.borderActive = m.p.modalBorder
	box := m.renderCard(c, mw, mh)
	m.p = saved
	x, y := (m.w-mw)/2, (m.h-mh)/2
	for i, b := range box {
		r := y + i
		rows[r] = ansi.Cut(rows[r], 0, x) + b + ansi.Cut(rows[r], x+len([]rune(ansi.Strip(b))), m.w)
	}
	return rows
}

// --- bubbletea plumbing -----------------------------------------------------

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "right", "l", "left", "h":
			step := 1
			if k := msg.String(); k == "left" || k == "h" {
				step = len(surfaces) - 1
			}
			for i, v := range surfaces {
				if v == m.variant {
					m.variant = surfaces[(i+step)%len(surfaces)]
					break
				}
			}
		case "tab":
			m.focus = (m.focus + 1) % 3
		case "shift+tab":
			m.focus = (m.focus + 2) % 3
		case "t", "T":
			step := 1
			if msg.String() == "T" {
				step = len(m.palettes) - 1
			}
			m.pi = (m.pi + step) % len(m.palettes)
			m.p = m.palettes[m.pi]
		case "m", "esc":
			m.modal = !m.modal && msg.String() == "m"
		case "j", "down":
			m.cursor[m.focus] = min(m.cursor[m.focus]+1, []int{len(clusters), len(apps), 7}[m.focus]-1)
		case "k", "up":
			m.cursor[m.focus] = max(m.cursor[m.focus]-1, 0)
		}
	}
	return m, nil
}

func main() {
	var ps []palette
	for _, t := range theme.All() {
		ps = append(ps, derive(t))
	}
	if len(os.Args) > 1 && os.Args[1] == "-dump" {
		lipgloss.SetColorProfile(termenv.TrueColor)
		for _, p := range ps {
			fmt.Printf("%-22s screen %v card %v header %v shadow %v sel %v\n", p.name, p.screen, p.card, p.header, p.shadow, p.sel)
		}
		for _, v := range surfaces {
			for pi, p := range ps {
				m := model{w: 110, h: 34, variant: v, focus: 1, p: p, palettes: ps, pi: pi, modal: v == 4}
				for i, r := range strings.Split(m.View(), "\n") {
					if w := ansi.StringWidth(r); w != m.w {
						fmt.Printf("surface %d %s row %d width %d\n", v, p.name, i, w)
					}
				}
			}
		}
		return
	}
	if _, err := tea.NewProgram(model{p: ps[0], palettes: ps, variant: 4, focus: 1}, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

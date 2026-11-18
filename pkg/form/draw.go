package form

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/geom"
)

// geometry is where the last View put things, for hit-testing.
type geometry struct {
	card    geom.Rect
	content geom.Rect
	// rows names what each body row holds, for the whole body; offset is
	// the first one on screen.
	rows    []rowRef
	offset  int
	fields  []geom.Rect
	buttons []buttonRect
}

type buttonRect struct {
	index int
	r     geom.Rect
}

// rowRef is what a body row belongs to: a field (or -1), and which of its
// value rows (or -1 for its label and note).
type rowRef struct{ field, value int }

// body is the card's rows at content width cw, and what each one holds. The
// first column of each row is the focus bar, the second a gap.
func (m Model) body(cw int) ([]string, []rowRef) {
	st := m.styles
	lead := strings.Repeat(" ", m.gutter)
	iw := max(1, cw-2-m.gutter)
	var out []string
	var refs []rowRef
	row := func(s string, bar bool, ref rowRef) {
		b := " "
		if bar {
			b = st.Bar.Render(m.glyphs.FieldBar)
		}
		out = append(out, lead+b+" "+fit(s, iw))
		refs = append(refs, ref)
	}
	blank := func() { row("", false, rowRef{-1, -1}) }

	for i, f := range m.fields() {
		if i > 0 {
			for range m.spacing {
				blank()
			}
		}
		on := i == m.focus
		ls := st.Label
		if on {
			ls = st.LabelFocused
		}
		label := ls.Render(f.title())
		if f.err != nil {
			msg := f.err.Error()
			label = st.Error.Bold(on).Render(f.title())
			gap := max(1, iw-ansi.StringWidth(f.title())-ansi.StringWidth(msg))
			label += strings.Repeat(" ", gap) + st.Error.Render(msg)
		}
		row(label, on, rowRef{i, -1})

		p := paint{st: st, g: m.glyphs, focused: on}
		if f.ctl.wells() {
			p.well = st.Well
			if on {
				p.well = st.WellFocused
			}
		}
		for j, v := range f.ctl.rows(iw, p) {
			row(v, on, rowRef{i, j})
		}
		for _, n := range noteLines(f.note, iw) {
			row(st.Note.Render(n), on, rowRef{i, -1})
		}
	}

	blank()
	row(m.buttonRow(), false, rowRef{-1, -1})
	if m.modal {
		blank()
		row(m.helpLine(), false, rowRef{-1, -1})
	}
	return out, refs
}

func noteLines(note string, w int) []string {
	if note == "" {
		return nil
	}
	return strings.Split(ansi.Wrap(note, w, ""), "\n")
}

// fit cuts or pads s to w cells.
func fit(s string, w int) string {
	n := ansi.StringWidth(s)
	if n > w {
		return ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-n)
}

func (m Model) chip(label string, active bool) string {
	s := m.styles.Button
	if active {
		s = m.styles.ButtonActive
	}
	if m.styles.Chips {
		return s.Render("  " + label + "  ")
	}
	return s.Render("[ " + label + " ]")
}

func (m Model) buttonRow() string {
	var chips []string
	for i, b := range m.buttons() {
		chips = append(chips, m.chip(m.label(b), m.onButton() && m.focus-len(m.fields()) == i))
	}
	return strings.Join(chips, "  ")
}

// helpLine is the keys that work where focus is, read in the order a user
// reaches for them: the field's own, enter, moving, then leaving.
func (m Model) helpLine() string {
	bs := m.editKeys()
	if enter, ok := m.enterKey(); ok {
		bs = append(bs, enter)
	}
	bs = append(bs,
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "next")),
		key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "prev")),
		m.escKey())
	var parts []string
	for _, b := range bs {
		h := b.Help()
		parts = append(parts, m.styles.HelpKey.Render(h.Key)+" "+m.styles.HelpDesc.Render(h.Desc))
	}
	return strings.Join(parts, m.styles.HelpDesc.Render(" · "))
}

// progress is the step dots for the frame, or "" for a single page.
func (m Model) progress() string {
	var dots []string
	n := 0
	for i := range m.steps {
		if m.skipped(i) {
			continue
		}
		g, s := m.glyphs.RadioOff, m.styles.Unchosen
		if i <= m.step {
			g, s = m.glyphs.RadioOn, m.styles.Chosen
		}
		dots = append(dots, s.Render(g))
		n++
	}
	if n < 2 {
		return ""
	}
	return strings.Join(dots, m.styles.Unchosen.Render("──"))
}

func (m Model) frameTitle() string {
	st := m.steps[m.step].Title
	switch {
	case m.title != "" && st != "":
		return m.title + " · " + st
	case st != "":
		return st
	}
	return m.title
}

// cardHeight is how tall the card is at outer width w with nothing cut.
func (m Model) cardHeight(w int) int {
	f := m.frame
	f.SetRect(geom.Rect{W: w, H: 40})
	in := f.Insets()
	cw := f.ContentRect().W
	b, _ := m.body(cw)
	return len(b) + in.Top + in.Bottom
}

// View draws the form centred in its rect, and records where everything
// went so a click can find it.
func (m Model) View() string {
	r := m.rect
	*m.geo = geometry{}
	if r.W <= 0 || r.H <= 0 {
		return ""
	}
	w := min(m.maxW, r.W)
	cardH := min(m.cardHeight(w), r.H)

	showHelp := !m.modal && r.H > cardH
	showBanner := !m.modal && m.banner.Fits(r.W) && r.H >= cardH+m.banner.Height()+1+btoi(showHelp)
	total := cardH + btoi(showHelp)
	if showBanner {
		total += m.banner.Height() + 1
	}
	top := r.Y + (r.H-total)/2
	left := r.X + (r.W-w)/2

	var block []string
	if showBanner {
		pad := strings.Repeat(" ", (r.W-m.banner.Width())/2)
		for _, l := range lines(m.banner.View()) {
			block = append(block, pad+l)
		}
		block = append(block, "")
	}
	cardTop := top + len(block)
	card := m.drawCard(geom.Rect{X: left, Y: cardTop, W: w, H: cardH, Gen: r.Gen})
	pad := strings.Repeat(" ", left-r.X)
	for _, l := range lines(card) {
		block = append(block, pad+l)
	}
	if showHelp {
		h := m.helpLine()
		block = append(block, strings.Repeat(" ", max(0, (r.W-ansi.StringWidth(h))/2))+h)
	}

	out := make([]string, r.H)
	for i := range out {
		l := ""
		if j := i - (top - r.Y); j >= 0 && j < len(block) {
			l = block[j]
		}
		out[i] = fit(l, r.W)
	}
	return strings.Join(out, "\n")
}

// drawCard draws the frame at cr with as much of the body as fits, scrolled
// to keep the focused field in view.
func (m Model) drawCard(cr geom.Rect) string {
	f := m.frame
	f.SetRect(cr)
	f.SetTitle(m.frameTitle())
	f.SetTopRight(m.progress())
	content := f.ContentRect()
	body, refs := m.body(content.W)

	offset := 0
	if len(body) > content.H {
		first, last := -1, -1
		for i, ref := range refs {
			if ref.field == m.focus {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if m.onButton() {
			first, last = len(body)-1, len(body)-1
		}
		offset = max(0, min(last-content.H+1, first, len(body)-content.H))
	}
	visible := body[offset:min(len(body), offset+content.H)]
	f.SetContent(strings.Join(visible, "\n"))

	g := m.geo
	g.card, g.content, g.rows, g.offset = cr, content, refs, offset
	g.fields = make([]geom.Rect, len(m.fields()))
	for i, ref := range refs {
		y := content.Y + i - offset
		if ref.field < 0 || y < content.Y || y >= content.Y+content.H {
			continue
		}
		fr := &g.fields[ref.field]
		if fr.Empty() {
			*fr = geom.Rect{X: content.X, Y: y, W: content.W, H: 1, Gen: cr.Gen}
		} else {
			fr.H = y - fr.Y + 1
		}
	}
	if y := content.Y + len(body) - 1 - btoi(m.modal)*2 - offset; y >= content.Y && y < content.Y+content.H {
		x := content.X + 2 + m.gutter
		for i, b := range m.buttons() {
			wd := lipgloss.Width(m.chip(m.label(b), false))
			g.buttons = append(g.buttons, buttonRect{i, geom.Rect{X: x, Y: y, W: wd, H: 1, Gen: cr.Gen}})
			x += wd + 2
		}
	}
	return f.View()
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

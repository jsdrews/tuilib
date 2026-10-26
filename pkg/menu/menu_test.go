package menu

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

func sections() []Item {
	return []Item{
		{ID: "pg", Label: "postgres › instance", Group: "database", Description: "GET /db/pg/instance"},
		{ID: "vpc", Label: "vpc", Group: "network", Description: "GET /net/vpc"},
		{ID: "about", Label: "about"},
		{ID: "my", Label: "mysql › instance", Group: "database", Description: "GET /db/mysql/instance"},
		{ID: "fw", Label: "firewall", Group: "network"},
	}
}

func testMenu(t *testing.T, items []Item, filterable bool) Model {
	t.Helper()
	m := New(Options{
		Title:         "Sections",
		Items:         items,
		Filterable:    filterable,
		DescStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		HeaderStyle:   lipgloss.NewStyle().Bold(true),
		SelectedStyle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")),
		ActiveColor:   lipgloss.Color("12"),
		InactiveColor: lipgloss.Color("240"),
		Filter:        filter.Options{},
	})
	m.SetRect(placed())
	return m
}

func placed() geom.Rect {
	geom.NextGen()
	return geom.New(0, 0, 60, 16)
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func typeText(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func click(x, y, n int) mouse.Msg {
	return mouse.Msg{
		MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		Clicks:   n,
	}
}

// rows is the pane's content lines, border and scrollbar stripped.
func rows(m Model) []string {
	var out []string
	for _, ln := range strings.Split(ansi.Strip(m.View()), "\n") {
		if !strings.HasPrefix(ln, "│") {
			continue
		}
		ln = strings.TrimRight(strings.TrimSuffix(strings.TrimRight(ln, " "), "│"), " █░")
		ln = strings.TrimPrefix(ln, "│")
		// Blank rows below the items, and the filter row and its rule.
		if strings.Trim(ln, "─ ") == "" || strings.HasPrefix(ln, "/") {
			continue
		}
		out = append(out, ln)
	}
	return out
}

func labels(m Model) []string {
	var out []string
	for _, r := range rows(m) {
		l := strings.TrimSpace(strings.TrimPrefix(strings.Trim(r, "─ "), "▸"))
		out = append(out, strings.Split(l, "  ")[0]) // drop the description column
	}
	return out
}

func selectedID(m Model) string {
	it, _ := m.Selected()
	return it.ID
}

func TestGroupsDrawHeadersWithOtherLast(t *testing.T) {
	m := testMenu(t, sections(), false)
	var got []string
	for _, l := range labels(m) {
		got = append(got, strings.Fields(l + " ")[0])
	}
	want := "database postgres mysql network vpc firewall other about"
	if strings.Join(got, " ") != want {
		t.Errorf("rows\n got  %v\n want %s", got, want)
	}
}

func TestHeadersAreDividersAcrossTheWidth(t *testing.T) {
	rs := rows(testMenu(t, sections(), false))
	if !strings.HasPrefix(rs[0], "── database ─") || !strings.HasSuffix(rs[0], "─") {
		t.Errorf("header %q is not a divider with the name set into it", rs[0])
	}
	if ansi.StringWidth(rs[0]) <= ansi.StringWidth(rs[1]) {
		t.Errorf("header %q stops short of the rows' width", rs[0])
	}
	if !strings.HasPrefix(rs[1], "▸ postgres") || !strings.HasPrefix(rs[2], "  mysql") {
		t.Errorf("items not in the cursor column: %q / %q", rs[1], rs[2])
	}
}

func TestOneGroupIsFlat(t *testing.T) {
	m := testMenu(t, []Item{{Label: "a", Group: "x"}, {Label: "b", Group: "x"}}, false)
	if got := strings.Join(labels(m), "|"); got != "a|b" {
		t.Errorf("rows %q, want a|b with no header", got)
	}
}

func TestDescriptionIsDimmedAndCutOff(t *testing.T) {
	m := testMenu(t, sections(), false)
	m.SetRect(geom.New(0, 0, 34, 16))
	view := m.View()
	if !strings.Contains(ansi.Strip(view), "mysql › instance     GET /db…") {
		t.Errorf("description not cut off with an ellipsis:\n%s", ansi.Strip(view))
	}
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("GET /db…")
	if !strings.Contains(view, dim) {
		t.Error("description not drawn in DescStyle")
	}
}

func TestCursorSkipsHeaders(t *testing.T) {
	m := testMenu(t, sections(), false)
	var got []string
	for range 6 {
		got = append(got, selectedID(m))
		m, _ = m.Update(keyMsg("down"))
	}
	if want := "pg my vpc fw about about"; strings.Join(got, " ") != want {
		t.Errorf("down walked %v, want %s", got, want)
	}
}

func TestTabAndBracketsJumpGroups(t *testing.T) {
	m := testMenu(t, sections(), false)
	for _, step := range []struct{ key, want string }{
		{"]", "vpc"}, {"tab", "about"}, {"shift+tab", "vpc"}, {"[", "pg"},
		// Past either end the jumps wrap, the way tab cycles panes.
		{"[", "about"}, {"tab", "pg"}, {"shift+tab", "about"}, {"]", "pg"},
	} {
		m, _ = m.Update(keyMsg(step.key))
		if id := selectedID(m); id != step.want {
			t.Errorf("%s landed on %s, want %s", step.key, id, step.want)
		}
	}
}

func TestInAFocusGroupTabCyclesAndBracketsJump(t *testing.T) {
	m := testMenu(t, sections(), false)
	focus.NewGroup(&m)
	m, _ = m.Update(keyMsg("tab"))
	if id := selectedID(m); id != "pg" {
		t.Errorf("tab moved the cursor to %s; in a focus.Group it cycles focus", id)
	}
	m, _ = m.Update(keyMsg("]"))
	if id := selectedID(m); id != "vpc" {
		t.Errorf("] landed on %s, want vpc", id)
	}
	for _, b := range m.Help() {
		if h := b.Help().Key; strings.Contains(h, "tab") {
			t.Errorf("help advertises %q, which the focus.Group owns", h)
		}
	}
}

func TestEnterEmitsChosenWithThePayload(t *testing.T) {
	items := sections()
	items[0].Data = 42
	m := testMenu(t, items, false)
	_, cmd := m.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("enter returned no command")
	}
	it, ok := m.Chosen(cmd())
	if !ok || it.ID != "pg" || it.Data != 42 {
		t.Errorf("Chosen = %+v, %v; want pg with its Data", it, ok)
	}
	other := testMenu(t, items, false)
	if _, ok := other.Chosen(cmd()); ok {
		t.Error("a second menu claimed the first one's ChosenMsg")
	}
}

func TestFilterMatchesLabelAndGroupNotDescription(t *testing.T) {
	m := testMenu(t, sections(), true)
	m, _ = m.Update(keyMsg("/"))
	m = typeText(m, "network")
	if got := strings.Join(labels(m), "|"); got != "network|vpc|firewall" {
		t.Errorf("group match: %q", got)
	}

	m = testMenu(t, sections(), true)
	m, _ = m.Update(keyMsg("/"))
	m = typeText(m, "instance")
	if got := strings.Join(labels(m), "|"); !strings.HasPrefix(got, "database|postgres") || strings.Contains(got, "network") {
		t.Errorf("label match: %q — an emptied group must hide its header", got)
	}

	m = testMenu(t, sections(), true)
	m, _ = m.Update(keyMsg("/"))
	m = typeText(m, "GET")
	if got := labels(m); len(got) != 0 {
		t.Errorf("description was searched: %q", got)
	}
	if _, ok := m.Selected(); ok {
		t.Error("an empty result still has a selection")
	}
}

func TestFilterKeepsTheCursorOnItsItem(t *testing.T) {
	m := testMenu(t, sections(), true)
	m, _ = m.Update(keyMsg("]")) // vpc
	m, _ = m.Update(keyMsg("/"))
	m = typeText(m, "v")
	if id := selectedID(m); id != "vpc" {
		t.Errorf("cursor on %s after filtering, want vpc", id)
	}
	m = typeText(m, "x")
	m, _ = m.Update(keyMsg("esc"))
	if id := selectedID(m); id == "" {
		t.Error("clearing the filter left nothing selected")
	}
}

func TestFilteringCapturesKeys(t *testing.T) {
	m := testMenu(t, sections(), true)
	m, _ = m.Update(keyMsg("/"))
	if !m.IsCapturingKeys() {
		t.Fatal("focused filter is not capturing")
	}
	m, cmd := m.Update(keyMsg("enter"))
	if cmd != nil {
		if _, ok := m.Chosen(cmd()); ok {
			t.Error("enter in the filter chose an item instead of committing the filter")
		}
	}
	if m.IsCapturingKeys() {
		t.Error("enter did not hand input back")
	}
}

func TestClickMovesTheCursorAndDoubleClickChooses(t *testing.T) {
	m := testMenu(t, sections(), false)
	// Row 0 (y=1) is the database header; y=3 is mysql.
	m, _ = m.Update(click(4, 1, 1))
	if id := selectedID(m); id != "pg" {
		t.Errorf("click on a header moved the cursor to %s", id)
	}
	m, _ = m.Update(click(4, 3, 1))
	if id := selectedID(m); id != "my" {
		t.Errorf("click landed on %s, want my", id)
	}
	_, cmd := m.Update(click(4, 3, 2))
	var chosen bool
	for _, msg := range drain(cmd) {
		if it, ok := m.Chosen(msg); ok && it.ID == "my" {
			chosen = true
		}
	}
	if !chosen {
		t.Error("double click did not choose")
	}
}

func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, drain(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestSetItemsKeepsTheCursorByID(t *testing.T) {
	m := testMenu(t, sections(), false)
	m, _ = m.Update(keyMsg("]")) // vpc
	items := sections()
	items[0], items[1] = items[1], items[0]
	m.SetItems(items)
	if id := selectedID(m); id != "vpc" {
		t.Errorf("cursor on %s after SetItems, want vpc", id)
	}
}

func TestStateRestoresAcrossARebuild(t *testing.T) {
	m := testMenu(t, sections(), true)
	m, _ = m.Update(keyMsg("/"))
	m = typeText(m, "inst")
	m, _ = m.Update(keyMsg("enter"))
	m, _ = m.Update(keyMsg("down"))
	st := m.State()

	n := testMenu(t, nil, true)
	n.Restore(st)
	if n.Value() != "inst" || selectedID(n) != "my" || len(n.Items()) != 5 {
		t.Errorf("restored value=%q selected=%s items=%d", n.Value(), selectedID(n), len(n.Items()))
	}

	var zero Model
	fresh := testMenu(t, sections(), true)
	fresh.Restore(zero.State())
	if len(fresh.Items()) != 5 {
		t.Error("a State from a never-built menu overwrote the new one")
	}
}

func TestHelpListsGroupKeysOnlyWhenGrouped(t *testing.T) {
	has := func(m Model, k string) bool {
		for _, b := range m.Help() {
			for _, kk := range b.Keys() {
				if kk == k {
					return true
				}
			}
		}
		return false
	}
	if !has(testMenu(t, sections(), false), "]") {
		t.Error("grouped menu does not advertise ]")
	}
	if has(testMenu(t, []Item{{Label: "a"}, {Label: "b"}}, false), "]") {
		t.Error("flat menu advertises ]")
	}
}

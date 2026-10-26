package action

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// groupedActions is a read group, a write group, and one action with no group,
// declared interleaved so the menu has to pull each group together.
func groupedActions() Set {
	return Set{Actions: []Action{
		{Label: "Describe", Group: "read", Run: noop},
		{Label: "Delete", Group: "write", Run: noop},
		{Label: "Shell", Run: noop},
		{Label: "Logs", Group: "read", Run: noop},
		{Label: "Patch", Group: "write", Run: noop},
	}}
}

// innerRows is the menu's box with its border and the cursor glyph stripped.
func innerRows(m Menu) []string {
	var out []string
	for _, ln := range strings.Split(ansi.Strip(m.View()), "\n") {
		ln = strings.TrimSpace(ln)
		if !strings.HasPrefix(ln, "│") {
			continue
		}
		ln = strings.Trim(ln, "│ █░")
		ln = strings.TrimSpace(strings.TrimPrefix(ln, "▸"))
		out = append(out, strings.Trim(ln, "─ "))
	}
	return out
}

func TestGroupsDrawHeadersInFirstSeenOrderWithOtherLast(t *testing.T) {
	m := testMenu(t, groupedActions())
	got := strings.Join(innerRows(m), "|")
	want := "read|Describe|Logs|write|Delete|Patch|other|Shell"
	if got != want {
		t.Errorf("rows\n got  %s\n want %s", got, want)
	}
}

func TestOneGroupKeepsTheMenuFlat(t *testing.T) {
	s := threeActions()
	for i := range s.Actions {
		s.Actions[i].Group = "read"
	}
	m := testMenu(t, s)
	if got := strings.Join(innerRows(m), "|"); strings.Contains(got, "read") {
		t.Errorf("a single group drew a header: %s", got)
	}
}

func TestCursorStartsOnTheFirstActionNotAHeader(t *testing.T) {
	m := testMenu(t, groupedActions())
	if a, _ := m.Selected(); a.Label != "Describe" {
		t.Errorf("cursor on %q, want Describe", a.Label)
	}
}

func TestDownFollowsDisplayOrderAcrossHeaders(t *testing.T) {
	m := testMenu(t, groupedActions())
	var got []string
	for range 5 {
		a, _ := m.Selected()
		got = append(got, a.Label)
		m, _ = send(m, keyMsg("down"))
	}
	if want := "Describe Logs Delete Patch Shell"; strings.Join(got, " ") != want {
		t.Errorf("down walked %v, want %s", got, want)
	}
}

func TestTabJumpsBetweenGroups(t *testing.T) {
	m := testMenu(t, groupedActions())
	m, _ = send(m, keyMsg("tab"))
	if a, _ := m.Selected(); a.Label != "Delete" {
		t.Errorf("tab landed on %q, want Delete", a.Label)
	}
	m, _ = send(m, keyMsg("tab"))
	if a, _ := m.Selected(); a.Label != "Shell" {
		t.Errorf("tab landed on %q, want Shell (other)", a.Label)
	}
	m, _ = send(m, keyMsg("shift+tab"))
	if a, _ := m.Selected(); a.Label != "Delete" {
		t.Errorf("shift+tab landed on %q, want Delete", a.Label)
	}
}

func TestGroupJumpsWrapPastEitherEnd(t *testing.T) {
	m := testMenu(t, groupedActions())
	m, _ = send(m, keyMsg("shift+tab"))
	if a, _ := m.Selected(); a.Label != "Shell" {
		t.Errorf("shift+tab from the first group landed on %q, want Shell (other, the last)", a.Label)
	}
	m, _ = send(m, keyMsg("]"))
	if a, _ := m.Selected(); a.Label != "Describe" {
		t.Errorf("] from the last group landed on %q, want Describe", a.Label)
	}
}

func TestBracketsJumpBetweenGroupsLikeTab(t *testing.T) {
	m := testMenu(t, groupedActions())
	m, _ = send(m, keyMsg("]"))
	if a, _ := m.Selected(); a.Label != "Delete" {
		t.Errorf("] landed on %q, want Delete", a.Label)
	}
	m, _ = send(m, keyMsg("["))
	if a, _ := m.Selected(); a.Label != "Describe" {
		t.Errorf("[ landed on %q, want Describe", a.Label)
	}
}

func TestGroupJumpLandsOnTheFirstEnabledAction(t *testing.T) {
	s := groupedActions()
	s.Actions[1].Disabled = "read-only" // Delete
	m := testMenu(t, s)
	m, _ = send(m, keyMsg("tab"))
	if a, _ := m.Selected(); a.Label != "Patch" {
		t.Errorf("tab landed on %q, want Patch", a.Label)
	}
}

func TestClickOnAHeaderDoesNothing(t *testing.T) {
	m := testMenu(t, groupedActions())
	r := m.Rect()
	if _, msg := send(m, press(r.X+2, r.Y+1)); msg != nil {
		t.Errorf("click on the first header produced %T", msg)
	}
	// Row 2 is Describe, row 3 Logs.
	if _, msg := send(m, press(r.X+2, r.Y+3)); msg == nil || msg.(ChosenMsg).Action.Label != "Logs" {
		t.Errorf("click under the header produced %v, want Logs", msg)
	}
}

func TestGroupStyleStylesEachHeader(t *testing.T) {
	opts := testOptions()
	opts.Set = groupedActions()
	opts.HeaderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	opts.GroupStyle = func(g string) lipgloss.Style {
		if g == "write" {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	}
	m := New(opts)
	m.SetRect(placed(0, 0, 80, 24))
	view := m.View()
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("write")
	if !strings.Contains(view, red) {
		t.Errorf("write header not drawn in its group style:\n%q", view)
	}
	if strings.Contains(view, lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("read")) {
		t.Error("read header took the write group's style")
	}
}

func TestHelpListsGroupKeysOnlyWhenGrouped(t *testing.T) {
	if hasBinding(testMenu(t, threeActions()).Help(), "tab") {
		t.Error("flat menu advertises tab")
	}
	if !hasBinding(testMenu(t, groupedActions()).Help(), "tab") {
		t.Error("grouped menu does not advertise tab")
	}
}

func TestHeadersCountTowardTheBoxHeight(t *testing.T) {
	m := testMenu(t, groupedActions())
	// 5 actions + 3 headers + a top and bottom border.
	if h := m.Rect().H; h != 10 {
		t.Errorf("height %d, want 10", h)
	}
}

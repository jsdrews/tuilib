package workbench

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/internal/cmdtest"
	"github.com/jsdrews/tuilib/internal/tick"
	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// The workbench is the screen a reader checks after a change, so the
// behaviour its package comment promises is asserted rather than trusted: a
// showcase that quietly stops showing something reads as the feature being
// broken.

func build(t *testing.T) *Screen {
	t.Helper()
	tick.Instant(t)
	s := New(theme.New(palette.Dark())).(*Screen)
	s.Init()
	render(s)
	return s
}

func render(s *Screen) string {
	geom.NextGen()
	return s.Layout().Render(geom.New(0, 0, 120, 36))
}

func keyOf(k string) tea.KeyMsg {
	switch k {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// press sends each key and feeds back what its commands report, the way the
// runtime would, so a ChosenMsg or a focus grant takes effect.
func press(s *Screen, keys ...string) {
	for _, k := range keys {
		_, cmd := s.Update(keyOf(k))
		render(s)
		for _, m := range cmdtest.Run(cmd) {
			s.Update(m)
			render(s)
		}
	}
}

func typed(s *Screen, text string) {
	for _, r := range text {
		press(s, string(r))
	}
}

func set(t *testing.T, s *Screen) action.Set {
	t.Helper()
	return s.Actions()
}

func find(t *testing.T, set action.Set, label string) action.Action {
	t.Helper()
	for _, a := range set.Actions {
		if a.Label == label {
			return a
		}
	}
	t.Fatalf("action %q is gone", label)
	return action.Action{}
}

// ---- marking on the table ------------------------------------------------

func TestTableMarksReportTheirCount(t *testing.T) {
	s := build(t)
	if got := set(t, s).Count; got != 1 {
		t.Fatalf("nothing marked: Count = %d, want 1 (the cursor row)", got)
	}
	press(s, "x", "j", "x")
	got := set(t, s)
	if got.Count != 2 || got.Target != "2 items" {
		t.Errorf("two marked: Count %d, Target %q; want 2, \"2 items\"", got.Count, got.Target)
	}
	if !strings.Contains(s.table.Title(), "2 marked") {
		t.Errorf("table title %q does not count the marks", s.table.Title())
	}
}

// Describe declares no Multi, so a second mark disables it; Restart does.
func TestDescribeIsSingleAndRestartIsMulti(t *testing.T) {
	s := build(t)
	if find(t, set(t, s), "Describe").Multi {
		t.Error("Describe declares Multi; the arity gate is no longer shown")
	}
	if !find(t, set(t, s), "Restart").Multi {
		t.Error("Restart must declare Multi to contrast with Describe")
	}
}

// A mark filtered out of sight is still selected. It is the surprise most
// likely to be "fixed" by someone who thinks it is a bug.
func TestMarksSurviveFiltering(t *testing.T) {
	s := build(t)
	press(s, "x") // prod/api-server
	press(s, "/")
	typed(s, "env:staging")
	press(s, "enter")
	if strings.Contains(ansi.Strip(s.table.View()), "prod") {
		t.Fatal("env:staging no longer hides the prod rows; this test can't see a drifting selection")
	}
	if got := set(t, s).Count; got != 1 {
		t.Errorf("the filtered-away mark was dropped: Count = %d, want 1", got)
	}
}

// ---- marking on the tree -------------------------------------------------

// A marked namespace cascades to its pods for Restart: 11 in prod.
func TestTreeBranchCascadesToItsPods(t *testing.T) {
	s := build(t)
	press(s, "tab", "j", "x") // the tree, then prod
	got := set(t, s)
	if got.Count != 11 || got.Target != "11 pods" {
		t.Errorf("prod marked: Count %d, Target %q; want 11, \"11 pods\"", got.Count, got.Target)
	}
	if find(t, got, "Describe").Multi {
		t.Error("tree Describe reads the marked node and must not be Multi")
	}
	if find(t, got, "Delete").Disabled == "" {
		t.Error("Delete is enabled on a prod selection; the staging guard is gone")
	}
}

// ---- master and detail -----------------------------------------------------

func TestPodsAndDetailFollowTheTable(t *testing.T) {
	s := build(t)
	press(s, "j")
	if got := s.pods.Title(); !strings.Contains(got, "web-frontend") {
		t.Errorf("pods title %q after moving to web-frontend", got)
	}
	if !strings.Contains(s.detailTitle, "web-frontend") {
		t.Errorf("detail title %q after moving to web-frontend", s.detailTitle)
	}

	press(s, "enter")
	if !s.focus.Is(&s.pods) {
		t.Fatal("enter on a deployment did not move focus to its pods")
	}
	press(s, "j", "enter")
	if !s.focus.Is(&s.detail) {
		t.Fatal("enter on a pod did not move focus to the detail")
	}
	if !strings.Contains(s.detailTitle, "web-frontend-9b21") {
		t.Errorf("detail title %q, want the second pod", s.detailTitle)
	}
}

// ---- the operations menu in the focus.Group --------------------------------

// The press that cycles focus onto the menu reaches it too: the screen updates
// the Group first and then forwards the key to whichever pane now has focus. A
// menu that jumped sections on tab would move its cursor on that press.
func TestTabCyclesPanesWithoutMovingTheMenu(t *testing.T) {
	s := build(t)
	press(s, "shift+tab") // wraps to the menu
	if !s.focus.Is(&s.ops) {
		t.Fatal("shift+tab from the table did not wrap to the operations menu")
	}
	press(s, "]")
	at, _ := s.ops.Selected()
	if at.Group != "confirm" {
		t.Fatalf("] landed in %q, want the confirm group", at.Group)
	}
	for _, k := range []string{"tab", "shift+tab"} {
		for range 5 {
			press(s, k)
		}
		if !s.focus.Is(&s.ops) {
			t.Errorf("five %s presses did not come back to the menu", k)
		}
		if now, _ := s.ops.Selected(); now.ID != at.ID {
			t.Errorf("%s moved the menu cursor from %q to %q", k, at.ID, now.ID)
		}
	}
}

// ---- modals ------------------------------------------------------------------

func TestOperationsOpenTheModalsTheyAskFor(t *testing.T) {
	s := build(t)
	press(s, "shift+tab", "G", "enter") // Run migration 0042 fails
	if !s.alertUp || !s.IsCapturingKeys() {
		t.Fatal("a failing op did not raise the alert, or the alert does not capture keys")
	}
	press(s, "enter")
	if s.alertUp {
		t.Fatal("enter did not dismiss the alert")
	}

	press(s, "[", "enter") // back to the confirm group: Delete build cache
	if !s.confirmUp {
		t.Fatal("a destructive op did not ask first")
	}
	if s.confirm.Value() {
		t.Error("the confirm modal starts on the affirmative side")
	}
	s.SetTheme(theme.New(palette.Light()))
	if !s.confirmUp {
		t.Error("the confirm modal did not survive a theme swap")
	}
}

// ---- theme swaps ---------------------------------------------------------------

func TestThemeSwapKeepsEveryPanesState(t *testing.T) {
	s := build(t)
	press(s, "x", "j", "x")     // two deployments
	press(s, "tab", "j", "x")   // prod in the tree
	press(s, "tab", "x")        // a pod
	press(s, "tab", "tab", "]") // the menu's confirm group
	op, _ := s.ops.Selected()
	title := s.detailTitle

	s.SetTheme(theme.New(palette.Light()))
	render(s)

	if s.table.MarkCount() != 2 || s.tree.MarkCount() != 1 || s.pods.MarkCount() != 1 {
		t.Errorf("marks after the swap: table %d, tree %d, pods %d; want 2, 1, 1",
			s.table.MarkCount(), s.tree.MarkCount(), s.pods.MarkCount())
	}
	if now, _ := s.ops.Selected(); now.ID != op.ID {
		t.Errorf("menu cursor moved from %q to %q", op.ID, now.ID)
	}
	if !s.focus.Is(&s.ops) {
		t.Error("focus left the menu")
	}
	if s.detailTitle != title || !strings.Contains(ansi.Strip(s.detail.View()), strings.TrimPrefix(title, "detail · ")) {
		t.Errorf("detail lost what it was showing (%q)", title)
	}
}

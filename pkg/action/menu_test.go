package action

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/pane"
)

// Without a TTY lipgloss falls back to the Ascii profile and strips every
// style, so any render comparison would pass no matter what the code did.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

func noop(context.Context, io.Writer) error { return nil }

// testOptions mirrors what theme.Actions() produces without importing
// pkg/theme, which imports this package — an in-package test cannot close
// that loop. The styles only have to be non-empty for the render assertions
// to mean anything.
func testOptions() Options {
	return Options{
		LabelStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		DescStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		KeyStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color("111")),
		SelectedStyle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")),
		DisabledStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		ActiveColor:    lipgloss.Color("212"),
		InactiveColor:  lipgloss.Color("240"),
		ActiveBorder:   lipgloss.NormalBorder(),
		InactiveBorder: lipgloss.NormalBorder(),
		SlotBrackets:   pane.SlotBracketsNone,
		Keys:           DefaultKeys(),
	}
}

// placed stamps a rect with the current generation so Rect.Hit accepts it.
func placed(x, y, w, h int) geom.Rect {
	geom.NextGen()
	return geom.New(x, y, w, h)
}

func testMenu(t *testing.T, s Set) Menu {
	t.Helper()
	opts := testOptions()
	opts.Set = s
	m := New(opts)
	m.SetRect(placed(0, 0, 80, 24))
	return m
}

func threeActions() Set {
	return Set{
		Target: "cache-redis",
		Count:  1,
		Actions: []Action{
			{Label: "Restart", Desc: "roll the pods", Multi: true, Run: noop},
			{Label: "Scale", Run: noop},
			{Label: "Logs", Key: key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "logs")),
				Do: func() tea.Cmd { return nil }},
		},
	}
}

// send drives Update and returns the resulting message, or nil.
func send(m Menu, msg tea.Msg) (Menu, tea.Msg) {
	m, cmd := m.Update(msg)
	if cmd == nil {
		return m, nil
	}
	return m, cmd()
}

func keyMsg(s string) tea.KeyMsg {
	if len(s) == 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	panic("unmapped key " + s)
}

func press(x, y int) mouse.Msg {
	return mouse.Msg{
		MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		Clicks:   1,
	}
}

func TestEnterChoosesTheCursorAction(t *testing.T) {
	m := testMenu(t, threeActions())

	_, msg := send(m, keyMsg("enter"))
	ch, ok := msg.(ChosenMsg)
	if !ok {
		t.Fatalf("enter produced %T, want ChosenMsg", msg)
	}
	if ch.Action.Label != "Restart" {
		t.Errorf("chose %q, want Restart", ch.Action.Label)
	}
	if ch.Target != "cache-redis" {
		t.Errorf("Target = %q, want cache-redis", ch.Target)
	}
}

func TestEscCancels(t *testing.T) {
	m := testMenu(t, threeActions())
	if _, msg := send(m, keyMsg("esc")); !isCancelled(msg) {
		t.Errorf("esc produced %T, want CancelledMsg", msg)
	}
}

func isCancelled(msg tea.Msg) bool {
	_, ok := msg.(CancelledMsg)
	return ok
}

func TestDownMovesTheCursor(t *testing.T) {
	m := testMenu(t, threeActions())
	m, _ = send(m, keyMsg("down"))
	if m.Cursor() != 1 {
		t.Fatalf("Cursor = %d, want 1", m.Cursor())
	}
	_, msg := send(m, keyMsg("enter"))
	if ch := msg.(ChosenMsg); ch.Action.Label != "Scale" {
		t.Errorf("chose %q, want Scale", ch.Action.Label)
	}
}

func TestCursorStopsAtTheEndsRatherThanWrapping(t *testing.T) {
	m := testMenu(t, threeActions())
	for i := 0; i < 10; i++ {
		m, _ = send(m, keyMsg("down"))
	}
	if m.Cursor() != 2 {
		t.Errorf("Cursor = %d after running past the end, want 2", m.Cursor())
	}
	for i := 0; i < 10; i++ {
		m, _ = send(m, keyMsg("up"))
	}
	if m.Cursor() != 0 {
		t.Errorf("Cursor = %d after running past the start, want 0", m.Cursor())
	}
}

func TestCursorSkipsDisabledRows(t *testing.T) {
	s := threeActions()
	s.Actions[1].Disabled = "not allowed here"
	m := testMenu(t, s)

	m, _ = send(m, keyMsg("down"))
	if m.Cursor() != 2 {
		t.Errorf("Cursor = %d, want 2 — the disabled row must be stepped over", m.Cursor())
	}
}

func TestCursorStartsPastALeadingDisabledRow(t *testing.T) {
	s := threeActions()
	s.Actions[0].Disabled = "nope"
	m := testMenu(t, s)
	if m.Cursor() != 1 {
		t.Errorf("Cursor = %d, want 1", m.Cursor())
	}
}

func TestAllDisabledLeavesNothingSelectable(t *testing.T) {
	s := threeActions()
	for i := range s.Actions {
		s.Actions[i].Disabled = "nope"
	}
	m := testMenu(t, s)
	if m.Cursor() != -1 {
		t.Errorf("Cursor = %d, want -1", m.Cursor())
	}
	if _, msg := send(m, keyMsg("enter")); msg != nil {
		t.Errorf("enter produced %T, want nothing", msg)
	}
}

// The arity gate: a multi-selection disables everything that did not opt in.
func TestMultiSelectionDisablesSingleTargetActions(t *testing.T) {
	s := threeActions()
	s.Count, s.Target = 3, "3 items"
	m := testMenu(t, s)

	// Restart is Multi; Scale and Logs are not.
	if m.Cursor() != 0 {
		t.Fatalf("Cursor = %d, want 0 (Restart is the only Multi action)", m.Cursor())
	}
	m, _ = send(m, keyMsg("down"))
	if m.Cursor() != 0 {
		t.Errorf("Cursor = %d, want 0 — no other row is selectable", m.Cursor())
	}
	if got := m.reasonAt(1); got != DefaultMultiReason {
		t.Errorf("reason = %q, want %q", got, DefaultMultiReason)
	}
	if !strings.Contains(m.View(), "3 items") {
		t.Error("menu title should name the target so a verb states its blast radius")
	}
}

func TestSingleTargetLeavesEverythingEnabled(t *testing.T) {
	s := threeActions()
	s.Count = 1
	m := testMenu(t, s)
	for i := range s.Actions {
		if r := m.reasonAt(i); r != "" {
			t.Errorf("action %d disabled with %q on a single selection", i, r)
		}
	}
}

func TestExclusiveActionInFlightIsDisabled(t *testing.T) {
	s := threeActions()
	s.Actions[0].Exclusive = true
	m := testMenu(t, s)

	m.SetRunning(map[string]bool{RunKey(s.Actions[0], s.Target): true})

	if got := m.reasonAt(0); got != DefaultRunningReason {
		t.Errorf("reason = %q, want %q", got, DefaultRunningReason)
	}
	if m.Cursor() != 1 {
		t.Errorf("Cursor = %d, want 1 — the cursor must leave a row that just became unavailable", m.Cursor())
	}
}

// Exclusivity is per target: the same verb on a different object is free.
func TestExclusiveIsScopedToTheTarget(t *testing.T) {
	s := threeActions()
	s.Actions[0].Exclusive = true
	m := testMenu(t, s)

	m.SetRunning(map[string]bool{RunKey(s.Actions[0], "some-other-pod"): true})

	if got := m.reasonAt(0); got != "" {
		t.Errorf("reason = %q, want empty — a run against another target must not block this one", got)
	}
}

func TestActionShortcutKeyChoosesIt(t *testing.T) {
	m := testMenu(t, threeActions())
	_, msg := send(m, keyMsg("l"))
	ch, ok := msg.(ChosenMsg)
	if !ok {
		t.Fatalf("shortcut produced %T, want ChosenMsg", msg)
	}
	if ch.Action.Label != "Logs" {
		t.Errorf("chose %q, want Logs", ch.Action.Label)
	}
}

func TestDisabledActionShortcutDoesNothing(t *testing.T) {
	s := threeActions()
	s.Actions[2].Disabled = "no logs yet"
	m := testMenu(t, s)
	if _, msg := send(m, keyMsg("l")); msg != nil {
		t.Errorf("shortcut on a disabled action produced %T, want nothing", msg)
	}
}

// A menu binding must never be shadowed by an action that bound over it.
func TestBuiltInKeysWinOverActionShortcuts(t *testing.T) {
	s := threeActions()
	s.Actions[1].Key = key.NewBinding(key.WithKeys("j"), key.WithHelp("j", "scale"))
	m := testMenu(t, s)

	m2, msg := send(m, keyMsg("j"))
	if msg != nil {
		t.Fatalf("j produced %T, want nothing — j is the menu's own down key", msg)
	}
	if m2.Cursor() != 1 {
		t.Errorf("Cursor = %d, want 1 — j must still move the cursor", m2.Cursor())
	}
}

func TestSingleClickCommitsARow(t *testing.T) {
	m := testMenu(t, threeActions())
	r := m.Rect()

	// First inner row: one down from the top border.
	_, msg := send(m, press(r.X+2, r.Y+1))
	ch, ok := msg.(ChosenMsg)
	if !ok {
		t.Fatalf("click produced %T, want ChosenMsg — a menu row is chrome, not a data row", msg)
	}
	if ch.Action.Label != "Restart" {
		t.Errorf("chose %q, want Restart", ch.Action.Label)
	}
}

func TestClickOnASecondRowChoosesThatRow(t *testing.T) {
	m := testMenu(t, threeActions())
	r := m.Rect()
	_, msg := send(m, press(r.X+2, r.Y+2))
	if ch, ok := msg.(ChosenMsg); !ok || ch.Action.Label != "Scale" {
		t.Errorf("click on row 2 produced %v, want Scale", msg)
	}
}

func TestClickOnADisabledRowDoesNothing(t *testing.T) {
	s := threeActions()
	s.Actions[1].Disabled = "nope"
	m := testMenu(t, s)
	r := m.Rect()

	m2, msg := send(m, press(r.X+2, r.Y+2))
	if msg != nil {
		t.Errorf("click on a disabled row produced %T, want nothing", msg)
	}
	if m2.Cursor() != 0 {
		t.Errorf("Cursor = %d, want 0 — a disabled row must not take the cursor", m2.Cursor())
	}
}

func TestClickOutsideCancels(t *testing.T) {
	m := testMenu(t, threeActions())
	r := m.Rect()
	if _, msg := send(m, press(r.X-2, r.Y-2)); !isCancelled(msg) {
		t.Errorf("click outside produced %T, want CancelledMsg", msg)
	}
}

// A host that keeps forwarding while the menu is down must not be able to
// make it cancel itself out of a frame it was never drawn in.
func TestAStaleMenuDeclinesMouse(t *testing.T) {
	m := testMenu(t, threeActions())
	geom.NextGen() // a frame in which the menu was not drawn

	if _, msg := send(m, press(0, 0)); msg != nil {
		t.Errorf("stale menu produced %T, want nothing", msg)
	}
}

func TestWheelMovesTheCursorNotJustTheViewport(t *testing.T) {
	s := Set{Count: 1}
	for i := 0; i < 20; i++ {
		s.Actions = append(s.Actions, Action{Label: "act " + string(rune('a'+i)), Run: noop})
	}
	opts := testOptions()
	opts.Set = s
	m := New(opts)
	m.SetRect(placed(0, 0, 80, 12))

	r := m.Rect()
	down := mouse.Msg{MouseMsg: tea.MouseMsg{
		X: r.X + 2, Y: r.Y + 1,
		Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown,
	}}
	m, _ = send(m, down)
	if m.Cursor() != 1 {
		t.Errorf("Cursor = %d, want 1 — the wheel must move the cursor, or the next frame snaps back", m.Cursor())
	}
}

func TestSizesToContentAndStaysWithinCaps(t *testing.T) {
	m := testMenu(t, threeActions())
	r := m.Rect()

	if r.W >= 80 {
		t.Errorf("menu width %d fills the bounds; it should take only what its rows need", r.W)
	}
	if r.H != len(threeActions().Actions)+2 {
		t.Errorf("menu height = %d, want %d (rows + borders)", r.H, len(threeActions().Actions)+2)
	}
	// Every label and the widest gloss have to fit.
	view := m.View()
	for _, want := range []string{"Restart", "roll the pods", "Scale", "Logs"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q", want)
		}
	}
}

func TestLongActionSetScrollsRatherThanGrowing(t *testing.T) {
	s := Set{Count: 1}
	for i := 0; i < 40; i++ {
		s.Actions = append(s.Actions, Action{Label: "action", Run: noop})
	}
	opts := testOptions()
	opts.Set = s
	m := New(opts)
	m.SetRect(placed(0, 0, 80, 24))

	if h := m.Rect().H; h > 24*menuHeightNum/menuHeightDen {
		t.Errorf("menu height %d exceeds the 60%% cap", h)
	}
	if !m.CanScroll() {
		t.Error("a 40-action menu in a capped box should report that it scrolls")
	}
}

func TestAnchorPlacesTheBoxAtThePointer(t *testing.T) {
	m := testMenu(t, threeActions())
	m.Anchor(10, 4)
	r := m.Rect()
	if r.X != 10 || r.Y != 4 {
		t.Errorf("placed at (%d,%d), want (10,4)", r.X, r.Y)
	}
}

func TestAnchorNearTheEdgeFlipsBackInside(t *testing.T) {
	m := testMenu(t, threeActions())
	m.Anchor(78, 23)
	r := m.Rect()
	if r.X+r.W > 80 || r.Y+r.H > 24 {
		t.Errorf("box at (%d,%d) %dx%d hangs off an 80x24 screen", r.X, r.Y, r.W, r.H)
	}
	if r.X < 0 || r.Y < 0 {
		t.Errorf("box at (%d,%d) is off the near edge", r.X, r.Y)
	}
}

func TestCenterPlacesTheBoxInTheMiddle(t *testing.T) {
	m := testMenu(t, threeActions())
	m.Anchor(2, 2)
	m.Center()
	r := m.Rect()
	if want := geom.CenterIn(placed(0, 0, 80, 24), r.W, r.H); r.X != want.X || r.Y != want.Y {
		t.Errorf("placed at (%d,%d), want (%d,%d)", r.X, r.Y, want.X, want.Y)
	}
}

// The View must fill the bounds it was given so ZStack can composite it, and
// leave untouched rows blank so the base layer shows through.
func TestViewFillsTheBoundsAndLeavesGapsBlank(t *testing.T) {
	m := testMenu(t, threeActions())
	m.Anchor(10, 4)
	lines := strings.Split(m.View(), "\n")

	if len(lines) != 24 {
		t.Fatalf("view has %d lines, want 24", len(lines))
	}
	for i := 0; i < 4; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			t.Errorf("line %d should be blank above the box, got %q", i, lines[i])
		}
	}
	if got := ansi.StringWidth(lines[4]); got == 0 {
		t.Error("the box's first row is empty")
	}
	if !strings.HasPrefix(lines[5], strings.Repeat(" ", 10)) {
		t.Errorf("box is not indented to its anchor: %q", lines[5])
	}
}

// The cursor row is styled as one run. A per-column style nested inside a
// row-wide one closes the outer style at the first reset (rule 19), which
// would drop the highlight halfway across the row.
func TestCursorRowIsStyledAsASingleRun(t *testing.T) {
	m := testMenu(t, threeActions())
	var row string
	for _, ln := range strings.Split(m.View(), "\n") {
		if strings.Contains(ln, "Restart") {
			row = ln
			break
		}
	}
	if row == "" {
		t.Fatal("could not find the cursor row")
	}
	// The row also carries the pane's own styled border glyphs, so counting
	// resets across the whole line proves nothing. What matters is that the
	// content is one unbroken run: find the escape that opens it and assert
	// the label and the gloss both land before it closes.
	at := strings.Index(row, "▸ Restart")
	open := strings.LastIndex(row[:at], "\x1b[")
	if open < 0 {
		t.Fatal("cursor row content is not styled at all")
	}
	rest := row[open:]
	end := strings.Index(rest, "\x1b[0m")
	if end < 0 {
		t.Fatal("cursor row's style never closes")
	}
	run := rest[:end]
	if !strings.Contains(run, "Restart") || !strings.Contains(run, "roll the pods") {
		t.Errorf("highlight breaks partway across the cursor row; a nested per-column style\n"+
			"would close it at the first reset (rule 19). Run was: %q", run)
	}
}

func TestSetActionsResetsTheCursor(t *testing.T) {
	m := testMenu(t, threeActions())
	m, _ = send(m, keyMsg("down"))
	m, _ = send(m, keyMsg("down"))
	if m.Cursor() != 2 {
		t.Fatalf("setup: Cursor = %d, want 2", m.Cursor())
	}

	m.SetActions(Set{Count: 1, Actions: []Action{{Label: "Only", Run: noop}}})
	if m.Cursor() != 0 {
		t.Errorf("Cursor = %d after SetActions, want 0", m.Cursor())
	}
}

func TestEmptySetRendersWithoutPanicking(t *testing.T) {
	m := testMenu(t, Set{})
	if m.Cursor() != -1 {
		t.Errorf("Cursor = %d on an empty set, want -1", m.Cursor())
	}
	if _, msg := send(m, keyMsg("enter")); msg != nil {
		t.Errorf("enter on an empty menu produced %T, want nothing", msg)
	}
	_ = m.View()
}

func TestHelpTracksWhetherTheMenuScrolls(t *testing.T) {
	short := testMenu(t, threeActions())
	if hasBinding(short.Help(), "ctrl+u") {
		t.Error("a menu that fits should not advertise half-page scrolling")
	}

	s := Set{Count: 1}
	for i := 0; i < 40; i++ {
		s.Actions = append(s.Actions, Action{Label: "action", Run: noop})
	}
	opts := testOptions()
	opts.Set = s
	long := New(opts)
	long.SetRect(placed(0, 0, 80, 24))
	if !hasBinding(long.Help(), "ctrl+u") {
		t.Error("a scrolling menu should advertise half-page scrolling")
	}
}

func hasBinding(bs []key.Binding, want string) bool {
	for _, b := range bs {
		for _, k := range b.Keys() {
			if k == want {
				return true
			}
		}
	}
	return false
}

// The mouse affordance is advertised with a sentinel key so help.Compile can
// dedupe on it and it can never match a real KeyMsg (rule 10).
func TestHelpAdvertisesTheMouseWithASentinel(t *testing.T) {
	if !hasBinding(testMenu(t, threeActions()).Help(), "mouse:click") {
		t.Error("Help should carry the click affordance as a sentinel binding")
	}
}

func rightPress(x, y int) mouse.Msg {
	return mouse.Msg{MouseMsg: tea.MouseMsg{
		X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonRight,
	}}
}

// A right-press outside the menu is the gesture that opened it, aimed
// somewhere else. Dismissing and making the user click again charges two
// gestures for one intent.
func TestRightPressOutsideRetargets(t *testing.T) {
	m := testMenu(t, threeActions())
	r := m.Rect()

	_, msg := send(m, rightPress(r.X-4, r.Y+2))
	rt, ok := msg.(RetargetMsg)
	if !ok {
		t.Fatalf("right-press outside produced %T, want RetargetMsg", msg)
	}
	if rt.Event.X != r.X-4 || rt.Event.Y != r.Y+2 {
		t.Errorf("event carried (%d,%d), want (%d,%d) — the host reopens against this",
			rt.Event.X, rt.Event.Y, r.X-4, r.Y+2)
	}
	if !rt.Event.IsRightPress() {
		t.Error("the event should be forwarded untouched so the host can route it")
	}
}

// Inside its own rect there is no second level of menu to ask about.
func TestRightPressInsideDoesNothing(t *testing.T) {
	m := testMenu(t, threeActions())
	r := m.Rect()
	if _, msg := send(m, rightPress(r.X+2, r.Y+1)); msg != nil {
		t.Errorf("right-press inside produced %T, want nothing", msg)
	}
}

// Retargeting must not be confused with dismissing: a left press away still
// cancels, and a right press away never does.
func TestRightPressDoesNotCancel(t *testing.T) {
	m := testMenu(t, threeActions())
	r := m.Rect()
	if _, msg := send(m, rightPress(r.X-4, r.Y+2)); isCancelled(msg) {
		t.Error("a right-press must retarget, not dismiss")
	}
	if _, msg := send(m, press(r.X-4, r.Y+2)); !isCancelled(msg) {
		t.Error("a left press away must still dismiss")
	}
}

// A host that ignores RetargetMsg is left exactly where it was, which is what
// makes this safe to add to an already-shipped component.
func TestRightPressLeavesTheMenuUnchanged(t *testing.T) {
	m := testMenu(t, threeActions())
	m, _ = send(m, keyMsg("down"))
	before := m.Cursor()

	r := m.Rect()
	m2, _ := send(m, rightPress(r.X-4, r.Y+2))
	if m2.Cursor() != before {
		t.Errorf("Cursor moved to %d, want %d — retarget is the host's call", m2.Cursor(), before)
	}
}

func TestAStaleMenuDeclinesARightPress(t *testing.T) {
	m := testMenu(t, threeActions())
	geom.NextGen()
	if _, msg := send(m, rightPress(0, 0)); msg != nil {
		t.Errorf("stale menu produced %T, want nothing", msg)
	}
}

// The keyboard path centers, and a menu that was never explicitly placed
// centers too — a top-anchored variant was tried and reverted (it covered the
// host pane's filter row).
func TestCenterStillCenters(t *testing.T) {
	opts := testOptions()
	opts.Set = threeActions()
	m := New(opts)
	m.SetRect(placed(0, 0, 120, 60))
	m.Center()

	r := m.Rect()
	if want := geom.CenterIn(placed(0, 0, 120, 60), r.W, r.H); r.Y != want.Y {
		t.Errorf("box Y = %d, want %d", r.Y, want.Y)
	}
}

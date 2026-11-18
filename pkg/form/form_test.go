package form

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/internal/cmdtest"
	"github.com/jsdrews/tuilib/internal/tick"
	"github.com/jsdrews/tuilib/pkg/banner"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

var (
	enter    = tea.KeyMsg{Type: tea.KeyEnter}
	tab      = tea.KeyMsg{Type: tea.KeyTab}
	shiftTab = tea.KeyMsg{Type: tea.KeyShiftTab}
	esc      = tea.KeyMsg{Type: tea.KeyEsc}
	down     = tea.KeyMsg{Type: tea.KeyDown}
	space    = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
)

func typed(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func signIn() []Field {
	return []Field{
		Text(TextOptions{Key: "user", Label: "Username", Required: true, Note: "The name you sign in with."}),
		Password(PasswordOptions{Key: "pass", Label: "Password"}),
		Select(SelectOptions{Key: "role", Label: "Role", Options: []string{"admin", "editor", "viewer"}, RequirePick: true}),
		Confirm(ConfirmOptions{Key: "remember", Label: "Remember me?", Initial: true}),
	}
}

func placed(t *testing.T, o Options, w, h int) Model {
	t.Helper()
	m := New(o)
	draw(&m, w, h)
	return m
}

func draw(m *Model, w, h int) string {
	geom.NextGen()
	m.SetRect(geom.New(0, 0, w, h))
	return m.View()
}

// send delivers msg and every message its command produces, the way the
// runtime would, and returns what came out.
func send(t *testing.T, m *Model, msg tea.Msg) []tea.Msg {
	t.Helper()
	next, cmd := m.Update(msg)
	*m = next
	draw(m, 80, 40)
	return cmdtest.Run(cmd)
}

func find[T any](msgs []tea.Msg) (T, bool) {
	for _, msg := range msgs {
		if v, ok := msg.(T); ok {
			return v, true
		}
	}
	var zero T
	return zero, false
}

func TestViewFillsItsRectAndCentresTheCard(t *testing.T) {
	m := New(Options{Title: "Sign in", Fields: signIn()})
	v := draw(&m, 100, 40)
	lines := strings.Split(v, "\n")
	if len(lines) != 40 {
		t.Fatalf("%d lines, want 40", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 100 {
			t.Fatalf("line %d is %d wide, want 100", i, w)
		}
	}
	card := m.Rect()
	if card.W != DefaultMaxWidth {
		t.Errorf("card is %d wide, want DefaultMaxWidth", card.W)
	}
	if card.X != (100-card.W)/2 {
		t.Errorf("card at x=%d, not centred", card.X)
	}
	if !strings.Contains(ansi.Strip(v), "Sign in") {
		t.Error("title not drawn")
	}
}

// A narrower rect than MaxWidth gets the whole width.
func TestCardShrinksToANarrowRect(t *testing.T) {
	m := placed(t, Options{Fields: signIn()}, 40, 40)
	if m.Rect().W != 40 {
		t.Errorf("card is %d wide in a 40-wide rect", m.Rect().W)
	}
}

// Height is what the form draws: the rows between its first and last
// painted line in a rect tall enough to hold it.
func TestHeightIsWhatItDraws(t *testing.T) {
	for name, o := range map[string]Options{
		"plain":  {Fields: signIn()},
		"modal":  {Fields: signIn(), Modal: true},
		"banner": {Fields: signIn(), Banner: banner.Options{Text: "hi"}},
	} {
		t.Run(name, func(t *testing.T) {
			m := New(o)
			lines := strings.Split(ansi.Strip(draw(&m, 80, 80)), "\n")
			first, last := -1, -1
			for i, l := range lines {
				if strings.TrimSpace(l) != "" {
					if first < 0 {
						first = i
					}
					last = i
				}
			}
			if got, want := last-first+1, m.Height(80); got != want {
				t.Errorf("drew %d rows, Height says %d", got, want)
			}
		})
	}
}

func TestEnterMovesOnAndTheLastFieldSubmits(t *testing.T) {
	m := placed(t, Options{Fields: signIn()}, 80, 40)
	send(t, &m, typed("ada"))
	for i := 1; i < 4; i++ {
		send(t, &m, enter)
		if m.FocusedIndex() != i {
			t.Fatalf("after %d enters focus is %d", i, m.FocusedIndex())
		}
	}
	got, ok := find[SubmittedMsg](send(t, &m, enter))
	if !ok {
		t.Fatal("enter on the last field did not submit")
	}
	want := Values{"user": "ada", "pass": "", "role": "admin", "remember": true}
	if !reflect.DeepEqual(got.Values, want) {
		t.Errorf("values = %v, want %v", got.Values, want)
	}
}

func TestRefusedSubmitFlagsAndFocusesTheFirstOffender(t *testing.T) {
	m := placed(t, Options{Fields: signIn()}, 80, 40)
	m.focusField(4) // the submit button
	msgs := send(t, &m, enter)
	if _, ok := find[SubmittedMsg](msgs); ok {
		t.Fatal("an invalid form submitted")
	}
	inv, ok := find[InvalidMsg](msgs)
	if !ok || !reflect.DeepEqual(inv.Keys, []string{"user", "role"}) {
		t.Fatalf("InvalidMsg = %+v, want user and role", inv)
	}
	if m.FocusedIndex() != 0 {
		t.Errorf("focus = %d, want the first offender", m.FocusedIndex())
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "required") {
		t.Errorf("the error is not drawn:\n%s", v)
	}
	send(t, &m, typed("a"))
	if strings.Count(ansi.Strip(m.View()), "required") != 1 {
		t.Error("typing into a flagged field did not clear its error at once")
	}
}

// A RequirePick select shows no choice until it is reached; reaching it
// chooses Initial, so it is never focused with nothing visible, and leaving
// keeps that choice.
func TestRequirePickChoosesOnArrival(t *testing.T) {
	m := placed(t, Options{Fields: []Field{
		Text(TextOptions{Key: "a", Label: "A"}),
		Select(SelectOptions{Key: "s", Label: "S", Options: []string{"x", "y", "z"}, Initial: 1, RequirePick: true}),
	}}, 80, 40)
	if m.String("s") != "" {
		t.Fatalf("an unreached select has value %q", m.String("s"))
	}
	send(t, &m, tab)
	if m.String("s") != "y" {
		t.Errorf("arriving chose %q, want Initial y", m.String("s"))
	}
	send(t, &m, down)
	send(t, &m, tab)
	if m.String("s") != "z" {
		t.Errorf("leaving lost the choice: %q", m.String("s"))
	}
}

// Clicking straight to the button never reaches the select, so it is never
// chosen, and the form refuses.
func TestUnreachedRequirePickIsRefused(t *testing.T) {
	m := placed(t, Options{Fields: []Field{
		Text(TextOptions{Key: "a", Label: "A"}),
		Select(SelectOptions{Key: "s", Label: "S", Options: []string{"x"}, RequirePick: true}),
	}}, 80, 40)
	m.focus = 2
	if _, ok := find[InvalidMsg](send(t, &m, enter)); !ok {
		t.Error("a RequirePick select never reached was accepted")
	}
}

func TestValidateRunsAfterRequired(t *testing.T) {
	notEmail := func(v any) error {
		if !strings.Contains(v.(string), "@") {
			return errors.New("not an email")
		}
		return nil
	}
	m := placed(t, Options{Fields: []Field{
		Text(TextOptions{Key: "e", Label: "Email", Required: true, Validate: notEmail}),
	}}, 80, 40)
	send(t, &m, enter)
	if !strings.Contains(ansi.Strip(m.View()), "required") {
		t.Error("a blank required field should say required, not the format error")
	}
	send(t, &m, typed("ada"))
	if !strings.Contains(ansi.Strip(m.View()), "not an email") {
		t.Error("the format error should replace required once there is text")
	}
}

func TestEscCancels(t *testing.T) {
	m := placed(t, Options{Fields: signIn()}, 80, 40)
	if _, ok := find[CancelledMsg](send(t, &m, esc)); !ok {
		t.Error("esc did not cancel")
	}
}

func TestMultiSelect(t *testing.T) {
	m := placed(t, Options{Fields: []Field{
		MultiSelect(MultiSelectOptions{Key: "m", Label: "M", Options: []string{"a", "b", "c"}, Min: 1, Max: 2}),
	}}, 80, 40)
	if _, ok := find[InvalidMsg](send(t, &m, enter)); !ok {
		t.Fatal("Min 1 with nothing chosen was accepted")
	}
	send(t, &m, space)
	send(t, &m, down)
	send(t, &m, space)
	send(t, &m, down)
	send(t, &m, space) // past Max: ignored
	if got := m.Strings("m"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("chose %v, want [a b] with Max 2", got)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "■ a") || !strings.Contains(v, "□ c") {
		t.Errorf("marks not drawn as squares:\n%s", v)
	}
}

func TestTextAreaTakesEnterAndTabLeaves(t *testing.T) {
	m := placed(t, Options{Fields: []Field{
		TextArea(TextAreaOptions{Key: "bio", Label: "Bio", Rows: 3}),
		Text(TextOptions{Key: "a", Label: "A"}),
	}}, 80, 40)
	send(t, &m, typed("one"))
	send(t, &m, enter)
	send(t, &m, typed("two"))
	if m.FocusedIndex() != 0 {
		t.Fatal("enter left the text area")
	}
	if m.String("bio") != "one\ntwo" {
		t.Errorf("bio = %q, want a newline from enter", m.String("bio"))
	}
	send(t, &m, tab)
	if m.FocusedIndex() != 1 {
		t.Error("tab did not leave the text area")
	}
}

func TestTextAreaKeepsItsRows(t *testing.T) {
	f := TextArea(TextAreaOptions{Key: "bio", Label: "Bio", Rows: 4})
	m := placed(t, Options{Fields: []Field{f}}, 80, 40)
	before := m.Height(80)
	send(t, &m, typed("a\nb\nc\nd\ne\nf"))
	if m.Height(80) != before {
		t.Error("text longer than the area grew the form")
	}
}

func wizard(skipSSH bool) Options {
	return Options{Title: "Connect", Steps: []Step{
		{Title: "Host", Fields: []Field{
			Text(TextOptions{Key: "host", Label: "Host", Required: true}),
			Select(SelectOptions{Key: "auth", Label: "Auth", Options: []string{"password", "key"}}),
		}},
		{Title: "Key", Skip: func(v Values) bool { return v.String("auth") != "key" }, Fields: []Field{
			Text(TextOptions{Key: "keyfile", Label: "Key file"}),
		}},
		{Title: "Confirm", Fields: []Field{
			Confirm(ConfirmOptions{Key: "save", Label: "Save?"}),
		}},
	}}
}

func TestWizardValidatesEachStepAndGoesBack(t *testing.T) {
	m := placed(t, wizard(true), 80, 40)
	m.focusField(2) // Next
	if _, ok := find[InvalidMsg](send(t, &m, enter)); !ok || m.Step() != 0 {
		t.Fatalf("Next past an invalid step: step %d", m.Step())
	}
	send(t, &m, typed("db1"))
	send(t, &m, enter) // to auth
	send(t, &m, enter) // last field: next step
	if m.Step() != 2 {
		t.Fatalf("step %d, want 2 — the key step skips for password auth", m.Step())
	}
	if !strings.Contains(ansi.Strip(m.View()), "Connect · Confirm") {
		t.Error("the step title is not on the frame")
	}
	send(t, &m, esc)
	if m.Step() != 0 {
		t.Errorf("esc went to step %d, want back past the skipped step to 0", m.Step())
	}
	if m.String("host") != "db1" {
		t.Error("going back lost the answer")
	}
	if _, ok := find[CancelledMsg](send(t, &m, esc)); !ok {
		t.Error("esc on the first step should cancel")
	}
}

func TestWizardSubmitsEveryRunStepOnce(t *testing.T) {
	m := placed(t, wizard(false), 80, 40)
	send(t, &m, typed("db1"))
	send(t, &m, enter)
	send(t, &m, down) // auth = key: the key step now runs
	send(t, &m, enter)
	if m.Step() != 1 {
		t.Fatalf("step %d, want the key step", m.Step())
	}
	send(t, &m, typed("~/.ssh/id"))
	send(t, &m, enter)
	got, ok := find[SubmittedMsg](send(t, &m, enter))
	if !ok {
		t.Fatal("the last step did not submit")
	}
	want := Values{"host": "db1", "auth": "key", "keyfile": "~/.ssh/id", "save": false}
	if !reflect.DeepEqual(got.Values, want) {
		t.Errorf("values = %v, want %v", got.Values, want)
	}
}

func TestSkippedStepIsLeftOutOfValues(t *testing.T) {
	m := placed(t, wizard(true), 80, 40)
	send(t, &m, typed("db1"))
	send(t, &m, enter)
	send(t, &m, enter)
	got, _ := find[SubmittedMsg](send(t, &m, enter))
	if _, has := got.Values["keyfile"]; has {
		t.Errorf("a skipped step's field was submitted: %v", got.Values)
	}
}

func TestProgressCountsStepsThatRun(t *testing.T) {
	m := placed(t, wizard(true), 80, 40)
	if got := strings.Count(ansi.Strip(m.View()), "○") - 1; got != 1 {
		// one ○ is the auth select's unchosen option
		t.Errorf("%d upcoming steps drawn, want 1 (the key step skips)", got)
	}
}

func TestClicks(t *testing.T) {
	m := placed(t, Options{Fields: signIn()}, 80, 40)
	press := func(x, y int) []tea.Msg {
		return send(t, &m, mouse.Msg{MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}, Clicks: 1})
	}

	r := m.FieldRect(2)
	press(r.X+4, r.Y+3) // the role select's third option
	if m.FocusedIndex() != 2 || m.String("role") != "viewer" {
		t.Errorf("click on an option: focus %d, role %q", m.FocusedIndex(), m.String("role"))
	}

	r = m.FieldRect(0)
	press(r.X+4, r.Y)
	if m.FocusedIndex() != 0 {
		t.Errorf("click on a field's label focused %d", m.FocusedIndex())
	}

	send(t, &m, typed("ada"))
	s := m.SubmitRect()
	if _, ok := find[SubmittedMsg](press(s.X+1, s.Y)); !ok {
		t.Error("clicking Submit did not submit")
	}
}

func TestStateRestoreCarriesEverything(t *testing.T) {
	m := placed(t, wizard(false), 80, 40)
	send(t, &m, typed("db1"))
	send(t, &m, enter)
	send(t, &m, down)
	send(t, &m, enter)
	send(t, &m, typed("~/k"))

	n := New(wizard(false))
	n.Restore(m.State())
	draw(&n, 80, 40)
	if n.Step() != 1 || n.FocusedIndex() != 0 {
		t.Errorf("restored to step %d focus %d", n.Step(), n.FocusedIndex())
	}
	if !reflect.DeepEqual(n.Values(), m.Values()) {
		t.Errorf("values %v, want %v", n.Values(), m.Values())
	}

	empty := New(wizard(false))
	empty.Restore(Model{}.State())
	if empty.Step() != 0 {
		t.Error("a State from a form never built restored something")
	}
}

func TestModalDrawsHelpInsideAndNoBanner(t *testing.T) {
	m := placed(t, Options{Fields: signIn(), Modal: true, Banner: banner.Options{Text: "hi"}}, 80, 40)
	v := ansi.Strip(m.View())
	if strings.Contains(v, "██") {
		t.Error("a modal drew the banner")
	}
	card := m.Rect()
	lines := strings.Split(v, "\n")
	for y, l := range lines {
		inside := y >= card.Y && y < card.Y+card.H
		if !inside && strings.TrimSpace(l) != "" {
			t.Errorf("row %d outside the card is painted: %q", y, l)
		}
		if inside && strings.Contains(l, "esc cancel") {
			return
		}
	}
	t.Error("the help line is not inside the modal card")
}

func TestBannerDropsWhenTooNarrowOrShort(t *testing.T) {
	o := Options{Fields: signIn(), Banner: banner.Options{Text: "tuilib"}}
	if v := ansi.Strip(draw(ptr(New(o)), 80, 60)); !strings.Contains(v, "██") {
		t.Fatal("the banner is missing where it fits")
	}
	if v := ansi.Strip(draw(ptr(New(o)), 30, 60)); strings.Contains(v, "██") {
		t.Error("a banner wider than the rect was drawn")
	}
	m := New(o)
	if v := ansi.Strip(draw(&m, 80, m.Height(80)-1)); strings.Contains(v, "██") {
		t.Error("the banner was drawn where it would push the card off")
	}
}

func ptr(m Model) *Model { return &m }

// A rect too short for the card scrolls the body so the focused field stays
// on screen.
func TestShortRectKeepsTheFocusedFieldInView(t *testing.T) {
	m := New(Options{Fields: signIn()})
	draw(&m, 80, 12)
	for range 3 {
		next, _ := m.Update(tab)
		m = next
	}
	v := ansi.Strip(draw(&m, 80, 12))
	if !strings.Contains(v, "Remember me?") {
		t.Errorf("the focused field scrolled away:\n%s", v)
	}
	if r := m.FieldRect(3); r.Empty() {
		t.Error("the focused field has no rect")
	}
}

func TestBannerAnimatesThroughTheForm(t *testing.T) {
	tick.Instant(t)
	m := New(Options{Fields: signIn(), Banner: banner.Options{Text: "a", Colours: []lipgloss.TerminalColor{
		lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff")}}})
	before := draw(&m, 80, 60)
	cmd := m.Init()
	for range 10 {
		var next tea.Cmd
		for _, msg := range cmdtest.Run(cmd) {
			m, next = m.Update(msg)
		}
		cmd = next
	}
	if cmd == nil {
		t.Fatal("the banner's tick chain stopped")
	}
	if draw(&m, 80, 60) == before {
		t.Error("the banner did not move")
	}
}

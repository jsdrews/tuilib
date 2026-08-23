package app

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/output"
	"github.com/jsdrews/tuilib/pkg/runner"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/statusbar"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// recordingScreen notes what OnEnter was handed, which is how the pop
// sentinel is observed from the screen underneath.
type recordingScreen struct {
	stubScreen
	entered []any
}

func (s *recordingScreen) OnEnter(result any) tea.Cmd {
	s.entered = append(s.entered, result)
	return nil
}

func newOutputApp(t *testing.T, root screen.Screen) Model {
	t.Helper()
	m := New(Options{
		Root:       root,
		Themes:     []theme.Theme{theme.Dark()},
		Mouse:      MouseClick,
		SkipConfig: true,
		OutputKey:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "output")),
	})
	tm, _ := m.Update(tea.WindowSizeMsg{Width: termW, Height: termH})
	m = tm.(Model)
	_ = m.View()
	return m
}

func send(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	tm, _ := m.Update(msg)
	next := tm.(Model)
	_ = next.View()
	return next
}

func typeKey(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// The whole feature hangs off OutputKey. Without it there is no buffer, no
// badge, and "o" belongs to whatever the screen wants it for.
func TestOutputIsOptIn(t *testing.T) {
	m := newApp(t, 1)
	if m.outputEnabled() {
		t.Fatal("console exists without OutputKey being set")
	}

	m = send(t, m, StatusInfoMsg{Text: "hello"})
	if strings.Contains(m.rightSlot, "output") {
		t.Errorf("badge rendered with the console disabled: %q", m.rightSlot)
	}
}

// Existing app.Info / app.Error calls become recoverable with no code
// changes — that is what makes the console non-empty on day one.
func TestStatusMessagesAreCaptured(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Deploy"})

	m = send(t, m, StatusInfoMsg{Text: "run completed"})
	m = send(t, m, StatusErrorMsg{Text: "deploy failed"})

	recs := m.outBuf.Records()
	if len(recs) != 2 {
		t.Fatalf("buffered %d records, want 2", len(recs))
	}
	if recs[0].Source != "Deploy" {
		t.Errorf("source = %q, want the screen on top at arrival", recs[0].Source)
	}
	if recs[1].Level != output.LevelError {
		t.Errorf("error message stored at level %v", recs[1].Level)
	}
}

// The summary paints the bar as before; the body only ever goes to the log.
func TestDetailBodyGoesToTheLogNotTheBar(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Deploy"})

	m = send(t, m, StatusErrorMsg{Text: "deploy failed", Body: "line one\nline two"})

	recs := m.outBuf.Records()
	if len(recs) != 3 {
		t.Fatalf("buffered %d records, want head + 2 body lines", len(recs))
	}
	if !recs[0].Head || recs[1].Head || recs[2].Head {
		t.Errorf("body lines were recorded as separate events: %+v", recs)
	}
	if got := m.outBuf.Unread(); got != 1 {
		t.Errorf("Unread() = %d — a summary plus its body is one event", got)
	}
	if msg, _ := m.sb.Message(); msg != "deploy failed" {
		t.Errorf("statusbar shows %q, want the summary alone", msg)
	}
}

func TestErrorOfUnwrapsTheChain(t *testing.T) {
	inner := errors.New("connection refused")
	wrapped := fmt.Errorf("deploy: %w", fmt.Errorf("api call: %w", inner))

	msg, ok := ErrorOf(wrapped)().(StatusErrorMsg)
	if !ok {
		t.Fatalf("ErrorOf returned %T", ErrorOf(wrapped)())
	}
	if msg.Text != wrapped.Error() {
		t.Errorf("summary = %q, want the outermost message", msg.Text)
	}
	if n := len(strings.Split(msg.Body, "\n")); n != 3 {
		t.Errorf("body has %d lines, want one per wrap:\n%s", n, msg.Body)
	}

	// An error that wraps nothing has no chain worth showing, so it should
	// be indistinguishable from a plain Error.
	plain, ok := ErrorOf(inner)().(StatusErrorMsg)
	if !ok || plain.Body != "" {
		t.Errorf("unwrapped error produced a body: %+v", plain)
	}
}

func TestBadgeAppearsOnlyOnceSomethingIsLogged(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Deploy"})
	if strings.Contains(m.rightSlot, "output") {
		t.Fatalf("badge shown for an empty buffer: %q", m.rightSlot)
	}

	m = send(t, m, StatusInfoMsg{Text: "hello"})
	if !strings.Contains(m.rightSlot, "1 output") {
		t.Errorf("badge missing after an event: %q", m.rightSlot)
	}
	if m.badgeW == 0 {
		t.Error("badge width not recorded; clicks could not be resolved")
	}
}

// Opening pushes, pressing again pops — and the pop carries the sentinel, so
// the screen underneath can tell a glance at the log from a fresh
// activation.
func TestOutputKeyTogglesAndPopsWithSentinel(t *testing.T) {
	root := &recordingScreen{stubScreen: stubScreen{name: "Deploy"}}
	m := newOutputApp(t, root)

	m = send(t, m, typeKey("o"))
	if !m.outputOnTop() {
		t.Fatal("output key did not open the console")
	}
	if got := m.stack.Depth(); got != 2 {
		t.Errorf("stack depth = %d, want 2", got)
	}

	m = send(t, m, typeKey("o"))
	if m.outputOnTop() {
		t.Fatal("output key did not close the console")
	}
	if len(root.entered) == 0 {
		t.Fatal("screen underneath never received OnEnter")
	}
	if _, ok := root.entered[len(root.entered)-1].(OutputClosed); !ok {
		t.Errorf("OnEnter got %#v, want OutputClosed", root.entered[len(root.entered)-1])
	}
}

// esc has to close it the same way the key does, or half the exits deliver
// nil and the sentinel is worthless.
func TestEscClosesTheConsoleWithTheSentinel(t *testing.T) {
	root := &recordingScreen{stubScreen: stubScreen{name: "Deploy"}}
	m := newOutputApp(t, root)

	m = send(t, m, typeKey("o"))
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.outputOnTop() {
		t.Fatal("esc did not close the console")
	}
	if _, ok := root.entered[len(root.entered)-1].(OutputClosed); !ok {
		t.Errorf("esc popped with %#v, want OutputClosed", root.entered[len(root.entered)-1])
	}
}

func TestClosingMarksEverythingRead(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Deploy"})
	m = send(t, m, StatusInfoMsg{Text: "hello"})

	m = send(t, m, typeKey("o"))
	if got := m.outBuf.Unread(); got != 1 {
		t.Errorf("Unread() = %d while the console is open, want 1 (read is marked on close)", got)
	}

	m = send(t, m, typeKey("o"))
	if got := m.outBuf.Unread(); got != 0 {
		t.Errorf("Unread() = %d after closing, want 0", got)
	}
	if !strings.Contains(m.rightSlot, "output") || strings.Contains(m.rightSlot, "1 output") {
		t.Errorf("affordance should stay as a bare door once read: %q", m.rightSlot)
	}
}

// A subprocess that failed used to produce nothing at all unless the screen
// author checked Result.Err.
func TestRunnerResultIsLogged(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Deploy"})

	cmd := exec.Command("/usr/bin/true")
	m = send(t, m, runner.Result{Cmd: cmd, Err: errors.New("exit status 1")})

	recs := m.outBuf.Records()
	if len(recs) != 1 {
		t.Fatalf("buffered %d records, want 1", len(recs))
	}
	if recs[0].Level != output.LevelError || !strings.Contains(recs[0].Text, "exit status 1") {
		t.Errorf("exit status not recorded as an error: %+v", recs[0])
	}
}

// Captured lines are attributed to the command, not to whatever screen
// happened to be on top — for captured output that is the honest answer.
func TestCapturedLinesAreSourcedToTheCommand(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Deploy"})

	m = send(t, m, runner.CapturedLine{RunID: 1, Label: "kubectl", Text: "applying"})

	recs := m.outBuf.Records()
	if len(recs) != 1 || recs[0].Source != "kubectl" {
		t.Fatalf("captured line source = %+v, want kubectl", recs)
	}
}

// stderr is not severity. Plenty of well-behaved tools log progress there,
// and tinting on it would leave the badge permanently red.
func TestStderrDoesNotTintTheBadge(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Deploy"})

	m = send(t, m, runner.CapturedLine{RunID: 1, Label: "go", Text: "compiling", Stderr: true})
	if m.outBuf.UnreadError() {
		t.Error("a stderr line tinted the badge")
	}

	m = send(t, m, runner.Captured{RunID: 1, Label: "go", Err: errors.New("exit status 2")})
	if !m.outBuf.UnreadError() {
		t.Error("a non-zero exit did not tint the badge")
	}
}

// One run is one event however many lines it emitted, so a 3000-line build
// must not render as "3000 output".
func TestCaptureCountsAsOneEvent(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Deploy"})

	m = send(t, m, runner.CaptureStarted{RunID: 1, Label: "go", Cmd: exec.Command("/usr/bin/true")})
	for i := 0; i < 50; i++ {
		m = send(t, m, runner.CapturedLine{RunID: 1, Label: "go", Text: "compiling"})
	}
	m = send(t, m, runner.Captured{RunID: 1, Label: "go"})

	if got := m.outBuf.Unread(); got != 1 {
		t.Errorf("Unread() = %d after a 50-line run, want 1", got)
	}
	if m.outBuf.InFlight() != 0 {
		t.Errorf("run still in flight after Captured")
	}
}

func TestInFlightRunMarksTheBadge(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Deploy"})

	m = send(t, m, runner.CaptureStarted{RunID: 1, Label: "go", Cmd: exec.Command("/usr/bin/true")})
	if m.outBuf.InFlight() != 1 {
		t.Fatal("run not registered as in flight")
	}
	if !strings.Contains(m.rightSlot, "⟳") {
		t.Errorf("badge missing the in-flight marker: %q", m.rightSlot)
	}
}

// The badge is the "you missed something" signal, so it and the version must
// survive on the bar rather than being pushed off the end by the slot maths.
func TestFooterFitsWithBadgeAndVersion(t *testing.T) {
	for _, w := range []int{60, 80, 100, 140} {
		for _, expanded := range []bool{false, true} {
			m := New(Options{
				Root:       &stubScreen{name: "Deploy"},
				Themes:     []theme.Theme{theme.Dark()},
				Version:    "v1.4.0",
				SkipConfig: true,
				OutputKey:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "output")),
			})
			tm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: termH})
			m = tm.(Model)
			m = send(t, m, StatusInfoMsg{Text: "hello"})
			if expanded {
				// "? close" is two cells wider than "? help", and the panel
				// changes the slot arithmetic — this is where the version
				// used to get clipped.
				m = send(t, m, typeKey("?"))
			}

			rows := strings.Split(m.View(), "\n")
			foot := rows[len(rows)-1]
			plain := stripANSI(foot)

			if got := lipglossWidth(foot); got != w {
				t.Errorf("w=%d expanded=%v: footer width = %d", w, expanded, got)
			}
			if !strings.Contains(plain, "v1.4.0") {
				t.Errorf("w=%d expanded=%v: version pushed off the bar:\n%q", w, expanded, plain)
			}
			if !strings.Contains(plain, "output") {
				t.Errorf("w=%d expanded=%v: badge pushed off the bar:\n%q", w, expanded, plain)
			}
			// The message only survives while it is on screen; "?" is a
			// KeyMsg and auto-clears it, which is expected.
			if !expanded && !strings.Contains(plain, "hello") {
				t.Errorf("w=%d: status message squeezed out of the bar:\n%q", w, plain)
			}
		}
	}
}

func lipglossWidth(s string) int { return lipgloss.Width(s) }

func stripANSI(s string) string { return xansi.Strip(s) }

// Screens shouldn't have to restate the output key — it is opt-in, so an
// author copying an existing screen has no reason to know it exists. Left to
// them it would be advertised on exactly the screens whose authors
// remembered.
func TestOutputKeyIsAdvertisedWithoutTheScreenListingIt(t *testing.T) {
	root := &stubScreen{name: "Deploy"}
	if listsKey(root.Help(), "o") {
		t.Fatal("the stub screen already advertises o; the test proves nothing")
	}

	m := newOutputApp(t, root)
	if !listsKey(m.helpBindings(root), "o") {
		t.Error("output key missing from the advertised bindings")
	}

	// And it must not be advertised when the feature is off.
	plain := newApp(t, 1)
	if listsKey(plain.helpBindings(plain.stack.Current()), "o") {
		t.Error("output key advertised with the console disabled")
	}
}

// Same key, opposite meaning, so the hint has to track which way it goes.
func TestOutputKeyAdvertisesCloseWhileOpen(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Deploy"})
	m = send(t, m, typeKey("o"))

	for _, b := range m.helpBindings(m.stack.Current()) {
		if b.Help().Key == "o" {
			if got := b.Help().Desc; got != "close output" {
				t.Errorf("hint reads %q while the console is open", got)
			}
			return
		}
	}
	t.Error("output key not advertised while the console is open")
}

func listsKey(bindings []key.Binding, want string) bool {
	for _, b := range bindings {
		for _, k := range b.Keys() {
			if k == want {
				return true
			}
		}
	}
	return false
}

// bindingHeavyScreen has the sort of binding count a deep component
// composition produces (table + filter + sort + pane scroll).
type bindingHeavyScreen struct{ stubScreen }

func (s *bindingHeavyScreen) Help() []key.Binding {
	labels := []string{"up", "down", "half up", "half down", "top", "bottom",
		"scroll left", "scroll right", "start", "end", "filter", "search",
		"next match", "prev match", "sort col", "sort dir", "expand all",
		"collapse all", "toggle", "wrap", "follow", "refresh", "pause", "open"}
	out := make([]key.Binding, 0, len(labels))
	for i, l := range labels {
		k := string(rune('A' + i))
		out = append(out, key.NewBinding(key.WithKeys(k), key.WithHelp(k, l)))
	}
	return out
}

// The expanded panel is capped at HelpMaxRows, so its tail is simply not
// drawn. A binding placed at the end of the list is therefore the first
// casualty on exactly the screens that need help most — which is how this
// shipped broken the first time: fine on a sparse screen at 120 columns,
// gone on a table at 80.
func TestOutputKeySurvivesACappedHelpPanel(t *testing.T) {
	for _, w := range []int{60, 80, 100, 120} {
		m := New(Options{
			Root:       &bindingHeavyScreen{stubScreen{name: "Table"}},
			Themes:     []theme.Theme{theme.Dark()},
			Version:    "v1.4.0",
			SkipConfig: true,
			OutputKey:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "output")),
		})
		tm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 40})
		m = tm.(Model)
		_ = m.View()

		tm, _ = m.Update(typeKey("?"))
		m = tm.(Model)

		if !strings.Contains(stripANSI(m.View()), "o output") {
			t.Errorf("w=%d: output key dropped off the end of the capped help panel", w)
		}
	}
}

// openConsole opens the console either by key or by clicking the badge, so
// tests can assert the two entry points agree.
func openConsole(t *testing.T, m Model, byMouse bool) Model {
	t.Helper()
	if !byMouse {
		return send(t, m, typeKey("o"))
	}
	m.placeChrome()
	slot := m.sb.RightContentRect()
	return send(t, m, tea.MouseMsg{
		X: slot.X, Y: slot.Y,
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
}

// The statusbar message is a truncated echo of what the console shows in
// full, so opening the console retires it. Both entry points must agree —
// the key path cleared only as a side effect of being a keypress, which left
// clicking the badge behaving differently for the same action.
func TestOpeningTheConsoleClearsTheStatusMessage(t *testing.T) {
	for _, byMouse := range []bool{false, true} {
		m := newOutputApp(t, &stubScreen{name: "Deploy"})
		m = send(t, m, StatusErrorMsg{Text: "deploy failed: exit 1"})
		if _, kind := m.sb.Message(); kind == statusbar.MessageNone {
			t.Fatalf("byMouse=%v: message never showed", byMouse)
		}

		m = openConsole(t, m, byMouse)
		if !m.outputOnTop() {
			t.Fatalf("byMouse=%v: console did not open", byMouse)
		}
		if _, kind := m.sb.Message(); kind != statusbar.MessageNone {
			t.Errorf("byMouse=%v: stale message left under the console", byMouse)
		}
	}
}

// Closing must not clear, or a notice the console itself raised — the path
// "w" reports after an export — is wiped on the way out by a user who
// clicked the badge to leave.
func TestClosingTheConsoleKeepsANoticeItRaised(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Deploy"})
	// The badge only exists once there is something to look at, so give the
	// buffer a record before trying to click it.
	m = send(t, m, StatusInfoMsg{Text: "deployment triggered"})

	m = openConsole(t, m, true)
	if !m.outputOnTop() {
		t.Fatal("badge click did not open the console")
	}

	m = send(t, m, output.Notice{Text: "wrote /tmp/output-20260812-142301.log"})
	if _, kind := m.sb.Message(); kind == statusbar.MessageNone {
		t.Fatal("export notice never showed")
	}

	m = openConsole(t, m, true) // same affordance, now closes
	if m.outputOnTop() {
		t.Fatal("badge click did not close the console")
	}
	if msg, kind := m.sb.Message(); kind == statusbar.MessageNone {
		t.Error("export path wiped on the way out; the user never got to read it")
	} else if !strings.Contains(msg, "output-20260812") {
		t.Errorf("statusbar shows %q, want the export path", msg)
	}
}

// middleSlot places the shell's chrome and returns the statusbar's center
// slot. Placing first is required: View sizes the bars on a copy of the
// Model (its receiver is by value), so the real rects come only from
// placeChrome.
func middleSlot(t *testing.T, m *Model) geom.Rect {
	t.Helper()
	m.placeChrome()
	r := m.sb.MiddleContentRect()
	if r.W <= 0 {
		t.Fatalf("center slot has no width to click: %+v", r)
	}
	return r
}

// pressChrome sends a press straight at clickChrome and reports whether the
// shell consumed it. Going through the shell's own handler rather than
// Update is what makes "the click landed" observable — asserting only on
// what changed afterwards passes just as well when the click missed.
func pressChrome(t *testing.T, m *Model, x, y int) (tea.Cmd, bool) {
	t.Helper()
	return m.clickChrome(mouse.Msg{
		MouseMsg: tea.MouseMsg{
			X: x, Y: y,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonLeft,
		},
		Clicks: 1,
	})
}

// The message is a truncated echo of something the log holds in full, so
// clicking it is the obvious way to ask for the rest.
func TestClickingTheStatusMessageOpensTheConsole(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Deploy"})
	m = send(t, m, StatusErrorMsg{Text: "deploy failed", Body: "the full stderr"})

	slot := middleSlot(t, &m)
	cmd, handled := pressChrome(t, &m, slot.X+slot.W/2, slot.Y)
	if !handled {
		t.Fatal("shell did not consume a click on the status message")
	}
	if cmd != nil {
		m = send(t, m, cmd())
	}

	if !m.outputOnTop() {
		t.Error("clicking the status message did not open the console")
	}
}

// Only while a message is up. A neutral center slot looks exactly like bar
// padding, and clicking blank footer space must not navigate anywhere.
func TestClickingABlankCenterSlotDoesNothing(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Deploy"})
	m = send(t, m, StatusInfoMsg{Text: "hello"}) // gives the buffer a record
	m = send(t, m, ClearStatus()())

	slot := middleSlot(t, &m)
	if _, handled := pressChrome(t, &m, slot.X+slot.W/2, slot.Y); handled {
		t.Error("shell claimed a click on an empty center slot")
	}
	if m.outputOnTop() {
		t.Error("clicking an empty center slot opened the console")
	}
}

// The console raises messages of its own — the path "w" reports after an
// export. Clicking one of those to close the view that produced it would be
// backwards, so the message opens and never toggles.
func TestClickingAMessageRaisedByTheConsoleDoesNotCloseIt(t *testing.T) {
	m := newOutputApp(t, &stubScreen{name: "Deploy"})
	m = send(t, m, typeKey("o"))
	if !m.outputOnTop() {
		t.Fatal("console did not open")
	}

	m = send(t, m, output.Notice{Text: "wrote /tmp/output-20260812-142301.log"})

	slot := middleSlot(t, &m)
	cmd, handled := pressChrome(t, &m, slot.X+slot.W/2, slot.Y)
	if !handled {
		t.Fatal("shell did not consume the click; the rest of this proves nothing")
	}
	if cmd != nil {
		m = send(t, m, cmd())
	}

	if !m.outputOnTop() {
		t.Error("clicking the export notice closed the console that produced it")
	}
}

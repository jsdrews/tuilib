package integration

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"

	"github.com/jsdrews/tuilib/demoapi"
	exactivity "github.com/jsdrews/tuilib/examples/patterns/activity"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// The whole stack, end to end, against a real server.
//
// Everything below is covered somewhere already — the shell's broadcasts in
// pkg/app, the component's rendering in internal/componenttest, the layering
// in pkg/activity — and all of it against hand-built rows and hand-written
// message sequences. What none of them cover is the *join*: a user pressing a
// key, the menu resolving an action, runner.Go executing it, a POST reaching a
// server, that server's log streaming back into the console, and the row the
// verb was about saying so throughout.
//
// This is the test that answers "can we exercise actions and activity status
// against demoapi", and the reason it uses examples/patterns/activity rather
// than a purpose-built screen is that the example is the thing people copy.

// busyLabel is what a working row says. It is the server's vocabulary, and the
// example's verbs use the same strings for Busy — so this one constant covers
// the local indicator and the derived one that replaces it.
const busyLabel = demoapi.SyncSyncing

// syncReceipt is what the example's Sync reports when its Run returns. It is
// deliberately "requested" rather than "completed": the function returns once
// the server has accepted the operation, and the row goes on saying Syncing
// afterwards.
const syncReceipt = "Sync requested"

func newActionApp(t *testing.T) (*harness, *httptest.Server) {
	t.Helper()

	// A real server over a real socket, and the example reaches it through
	// demoapi.From — so this also covers the TUILIB_DEMO_API path that
	// `task demo` uses.
	srv := httptest.NewServer(demoapi.New(demoapi.Options{Seed: 31, Apps: 5}))
	t.Setenv(demoapi.EnvBase, srv.URL)

	m := app.New(app.Options{
		Root:       exactivity.New(theme.Dark()),
		Themes:     []theme.Theme{theme.Dark()},
		SkipConfig: true,
		Mouse:      app.MouseClick,
		ActionsKey: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "actions")),
		OutputKey:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "output")),
	})
	return newAppHarness(t, m), srv
}

// pick opens the action menu and chooses the row with this label.
func (h *harness) pick(label string) {
	h.t.Helper()
	h.key("a")
	if !strings.Contains(h.render(), label) {
		h.t.Fatalf("the menu does not offer %q:\n%s", label, h.render())
	}
	for i := 0; i < 8; i++ {
		if strings.Contains(h.render(), "▸ "+label) {
			h.key("enter")
			return
		}
		h.key("j")
	}
	h.t.Fatalf("could not put the cursor on %q:\n%s", label, h.render())
}

func TestSyncFromTheMenuDrivesRowIndicatorAndConsole(t *testing.T) {
	h, srv := newActionApp(t)
	defer srv.Close()

	// Wait for the first poll to bring rows in; the menu has nothing to act on
	// until then.
	if !h.pumpUntil(5*time.Second, func() bool {
		return strings.Contains(h.render(), "Synced") || strings.Contains(h.render(), "OutOfSync")
	}) {
		t.Fatalf("no rows arrived from the server:\n%s", h.render())
	}

	h.pick("Sync")

	// 1. The row says so before the server has been asked anything — the shell
	//    broadcasting Set.Targets, labelled with the verb's Busy text.
	if !h.pumpUntil(3*time.Second, func() bool {
		return strings.Contains(h.render(), busyLabel)
	}) {
		t.Fatalf("the target row never showed the action running:\n%s", h.render())
	}

	// 2. And it says the *same* word for the whole run. The verb's Busy text is
	//    the server's own vocabulary, so the handoff from a local indicator to
	//    a derived one is not a change of wording — and no per-target phase
	//    churns the label in between. A row whose text changes three times in
	//    two seconds is harder to read than one that does not change at all,
	//    and it reads differently depending on how many rows you marked.
	for i := 0; i < 24; i++ {
		v := h.render()
		if spinnerGlyph(v) == "" {
			break // the run is over
		}
		if !strings.Contains(v, busyLabel) {
			t.Fatalf("a working row stopped saying %q:\n%s", busyLabel, v)
		}
		h.pumpFor(150 * time.Millisecond)
	}

	// 3. The server's log reached the console, under the action's label. The
	//    line is one only the server can produce, so this cannot pass on
	//    anything the client made up.
	if !h.pumpUntil(6*time.Second, func() bool {
		h.key("o")
		out := h.render()
		h.key("o")
		return strings.Contains(out, "successfully rolled out")
	}) {
		h.key("o")
		t.Errorf("the server's log never reached the console:\n%s", h.render())
	}

	// 4. And it settles: the handoff ends at an observation, and the row goes
	//    back to showing whatever the data says.
	if !h.pumpUntil(12*time.Second, func() bool {
		v := h.render()
		return !strings.Contains(v, busyLabel)
	}) {
		t.Errorf("the indicator never settled:\n%s", h.render())
	}
}

// Opening the console over a working row is what froze the spinner: the stack
// forwards to the top screen only, so the ticks went to the console and the
// chain died with nothing to restart it.
func TestConsoleDoesNotFreezeARowIndicator(t *testing.T) {
	h, srv := newActionApp(t)
	defer srv.Close()

	if !h.pumpUntil(5*time.Second, func() bool {
		return strings.Contains(h.render(), "Synced") || strings.Contains(h.render(), "OutOfSync")
	}) {
		t.Fatalf("no rows arrived:\n%s", h.render())
	}

	h.pick("Sync")
	if !h.pumpUntil(3*time.Second, func() bool {
		return strings.Contains(h.render(), busyLabel)
	}) {
		t.Fatalf("the row never started:\n%s", h.render())
	}

	// Go and read the log for a while, which starves the screen underneath.
	h.key("o")
	h.pumpFor(600 * time.Millisecond)
	h.key("esc")

	// Back on the screen, the animation has to resume. Two distinct frames is
	// the evidence: one frame proves only that something is still drawn, and a
	// frozen spinner draws forever.
	first := spinnerGlyph(h.render())
	if first == "" {
		t.Fatalf("the indicator vanished while the console was open:\n%s", h.render())
	}
	moved := h.pumpUntil(4*time.Second, func() bool {
		return spinnerGlyph(h.render()) != first
	})
	if !moved {
		t.Errorf("the spinner never advanced after the console was closed; "+
			"it is frozen on %q:\n%s", first, h.render())
	}
}

// spinnerGlyph returns whichever braille frame is on screen, or "" for none.
// The frames are spinner.Dot's, which pane and activity both default to.
func spinnerGlyph(view string) string {
	for _, g := range []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"} {
		if strings.Contains(view, g) {
			return g
		}
	}
	return ""
}

// The report this exists for, in the order it happened: run a sync, let it
// finish, go and read the log, come back — and the row must both keep spinning
// and eventually settle, with no keypress anywhere.
//
// Top-only routing broke it twice over. The spinner's tick chain died, so the
// glyph froze; and pkg/poll's chain died with it, so no observation ever
// arrived to end the handoff. The row sat on a stale phase forever, and the
// first keypress was what appeared to fix it.
func TestConsoleVisitLeavesRowLiveWithoutAKeypress(t *testing.T) {
	h, srv := newActionApp(t)
	defer srv.Close()

	if !h.pumpUntil(5*time.Second, func() bool {
		return strings.Contains(h.render(), "Synced") || strings.Contains(h.render(), "OutOfSync")
	}) {
		t.Fatalf("no rows arrived:\n%s", h.render())
	}

	h.pick("Sync")

	// Let the action run all the way out, so nothing of its own is left to
	// deliver messages on the way back.
	if !h.pumpUntil(8*time.Second, func() bool {
		return strings.Contains(h.render(), syncReceipt)
	}) {
		t.Fatalf("the action never finished:\n%s", h.render())
	}

	h.key("o")
	h.pumpFor(700 * time.Millisecond)
	h.key("esc")

	// From here on, no keys. Only time and whatever the program schedules for
	// itself — which is the whole question.
	first := spinnerGlyph(h.render())
	if first == "" {
		t.Fatalf("the row stopped showing the handoff:\n%s", h.render())
	}
	if !h.pumpUntil(4*time.Second, func() bool { return spinnerGlyph(h.render()) != first }) {
		t.Errorf("the spinner is frozen on %q and only a keypress would move it:\n%s",
			first, h.render())
	}

	// And the poll survived the visit, so an observation ends the handoff on
	// its own. Before the fix this waited forever on a stale phase.
	if !h.pumpUntil(12*time.Second, func() bool {
		v := h.render()
		return !strings.Contains(v, busyLabel)
	}) {
		t.Errorf("no observation ever arrived to settle the row:\n%s", h.render())
	}
}

// "Can these statuses eventually true up from a read from the server?"
//
// They must, and this is the case that proved they did not: a failed sync left
// demoapi reporting Syncing with nothing to clear it, so the row spun forever.
// The client was faithfully showing the server; the server was wrong. Which is
// the point of asserting convergence against a real one rather than trusting
// the layering in isolation.
//
// What converges, and how fast: the indicator set is replaced wholesale by
// every observation, so it is never anything but the last read. A read
// therefore trues the row up within one poll interval, always.
//
// (This used to gate on a ✗ appearing first. Outcome glyphs belonged to the
// action-driven path; with the data as the only source there is nothing to
// report but what the server says, and the failure shows as the status the
// server settles on.)
func TestRowConvergesOnTheServerAfterAFailedSync(t *testing.T) {
	h, srv := newActionApp(t)
	defer srv.Close()

	if !h.pumpUntil(5*time.Second, func() bool {
		return strings.Contains(h.render(), "Synced") || strings.Contains(h.render(), "OutOfSync")
	}) {
		t.Fatalf("no rows arrived:\n%s", h.render())
	}

	h.pick("Fail a sync")

	// The server picks the request up and reports it, which is the only way
	// the row moves at all now.
	if !h.pumpUntil(8*time.Second, func() bool {
		return strings.Contains(h.render(), demoapi.SyncSyncing)
	}) {
		t.Fatalf("the server never reported the sync as running:\n%s", h.render())
	}

	// No keys from here. The row must end up showing the server's resting
	// status with nothing left over — no spinner for work that is over.
	settled := h.pumpUntil(20*time.Second, func() bool {
		v := h.render()
		return strings.Contains(v, demoapi.SyncOutOfSync) && spinnerGlyph(v) == ""
	})
	if !settled {
		t.Errorf("the row never trued up to the server after a failed sync:\n%s", h.render())
	}
}

// One run, many rows.
//
// Every marked row spins, and none of it goes through the run: the verb asks
// the server to sync two applications, and the next poll reports both of them
// Syncing. So this asserts the derived path at multi-row arity — which is also
// why the rows settle as the server finishes them rather than together when
// the run returns.
func TestOneRunSpinsEveryMarkedRow(t *testing.T) {
	h, srv := newActionApp(t)
	defer srv.Close()

	if !h.pumpUntil(5*time.Second, func() bool {
		return strings.Contains(h.render(), "Synced") || strings.Contains(h.render(), "OutOfSync")
	}) {
		t.Fatalf("no rows arrived:\n%s", h.render())
	}

	// Mark two rows: x on the cursor, down, x again.
	h.key("x")
	h.key("j")
	h.key("x")
	if got := strings.Count(h.render(), glyph.Default().Mark); got < 2 {
		t.Fatalf("marked %d rows, want 2:\n%s", got, h.render())
	}

	h.pick("Sync")

	if !h.pumpUntil(4*time.Second, func() bool {
		return countIndicators(h.render()) >= 2
	}) {
		t.Errorf("one run did not spin both marked rows:\n%s", h.render())
	}

	// And they settle, one observation at a time, when the server says so.
	if !h.pumpUntil(20*time.Second, func() bool {
		return countIndicators(h.render()) == 0
	}) {
		t.Errorf("the rows never settled:\n%s", h.render())
	}
}

// countIndicators counts rows currently showing an activity label.
//
// One label to count, at either arity: the example reports no per-target
// phases, so a run over three rows shows the same word on each rather than a
// run-scoped counter drawn as though it were per-row progress.
func countIndicators(view string) int { return strings.Count(view, busyLabel) }

// The derived path with no action anywhere near it: the server starts work of
// its own and the row says so. This is the whole feature, and the case a
// viewer is most likely to miss because nothing on screen prompted it — so the
// example asks demoapi for a short schedule, and the pane title counts what
// the last poll found working.
func TestServerStartedWorkShowsWithoutAnyAction(t *testing.T) {
	h, srv := newActionApp(t)
	defer srv.Close()

	if !h.pumpUntil(5*time.Second, func() bool {
		return strings.Contains(h.render(), demoapi.SyncSynced) ||
			strings.Contains(h.render(), demoapi.SyncOutOfSync)
	}) {
		t.Fatalf("no rows arrived:\n%s", h.render())
	}

	// No keys from here. Anything that appears is the server's doing, reported
	// by ActivityWhen off a poll result.
	if !h.pumpUntil(15*time.Second, func() bool {
		return strings.Contains(h.render(), "working")
	}) {
		t.Errorf("the server never started work the screen noticed:\n%s", h.render())
	}
	if got := h.render(); !strings.Contains(got, busyLabel) {
		t.Errorf("the title counts work but no row shows it:\n%s", got)
	}
}

// The receipt has to be true of a dispatch. Run returning means the request was
// accepted, not that the server is done — and it plainly is not, because the
// row is still saying so when the receipt appears.
func TestDispatchReceiptDoesNotClaimCompletion(t *testing.T) {
	h, srv := newActionApp(t)
	defer srv.Close()

	if !h.pumpUntil(5*time.Second, func() bool {
		return strings.Contains(h.render(), demoapi.SyncSynced) ||
			strings.Contains(h.render(), demoapi.SyncOutOfSync)
	}) {
		t.Fatalf("no rows arrived:\n%s", h.render())
	}

	h.pick("Sync")

	if !h.pumpUntil(8*time.Second, func() bool {
		return strings.Contains(h.render(), syncReceipt)
	}) {
		t.Fatalf("no receipt appeared:\n%s", h.render())
	}
	if v := h.render(); strings.Contains(v, "Sync completed") {
		t.Errorf("the receipt claims the work is done while the row still says otherwise:\n%s", v)
	}
}

// What happens when you send a command to something that just received one on
// its own — the conflict, end to end.
//
// Three layers, and all three are needed. The server refuses (409), because it
// is the only thing that knows. The action surfaces the refusal, so the row
// shows ✗ and the statusbar says why. And the menu dims the verb when the last
// poll said the row was busy, so the ordinary case never gets that far.
//
// The dimming is best-effort by construction: it reads an observation that can
// be a poll interval old, and a schedule can fire inside that window. Which is
// exactly why the other two layers are not optional.
//
// Driven by making the server busy directly rather than by waiting for its
// schedule to land somewhere useful. An earlier version hunted for a
// server-started job under the cursor and skipped when it could not find one in
// time, which is a test that would never catch the regression it was written
// for.
func TestCommandOnServerBusyRowIsRefusedAndExplained(t *testing.T) {
	h, srv := newActionApp(t)
	defer srv.Close()

	if !h.pumpUntil(5*time.Second, func() bool {
		return strings.Contains(h.render(), demoapi.SyncSynced) ||
			strings.Contains(h.render(), demoapi.SyncOutOfSync)
	}) {
		t.Fatalf("no rows arrived:\n%s", h.render())
	}

	// The cursor starts on the first row, which is app-00000. Start work on it
	// from outside the TUI entirely: this session did not launch it, so nothing
	// but a poll can tell the screen about it.
	resp, err := srv.Client().Post(srv.URL+"/apps/app-00000/sync", "", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("could not make the server busy: status %d", resp.StatusCode)
	}

	// The poll observes it and the row starts spinning.
	if !h.pumpUntil(6*time.Second, func() bool {
		return strings.Contains(h.render(), "working")
	}) {
		t.Fatalf("the screen never noticed the server's work:\n%s", h.render())
	}

	// Now the menu must refuse, with a reason that names the state.
	h.key("a")
	v := h.render()
	h.key("esc")

	if !strings.Contains(v, "already") {
		t.Errorf("the menu offered a verb for a row the server is working on:\n%s", v)
	}
	if !strings.Contains(strings.ToLower(v), "syncing") {
		t.Errorf("the reason does not say what it is already doing:\n%s", v)
	}

	// And the server refuses the command even if the client asks anyway —
	// which it will, whenever a schedule fires inside the poll window.
	second, err := srv.Client().Post(srv.URL+"/apps/app-00000/sync", "", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer second.Body.Close()
	if second.StatusCode != http.StatusConflict {
		t.Errorf("second command = %d, want 409", second.StatusCode)
	}
}

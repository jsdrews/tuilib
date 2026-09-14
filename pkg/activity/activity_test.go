package activity

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/glyph"
)

func newSet(t *testing.T, hold time.Duration) Set {
	t.Helper()
	return New(Options{Hold: hold})
}

// runCmd executes a command and returns the message it produced. Nil in, nil
// out, so a caller can chain without branching.
func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestStartRendersSpinnerAndLabel(t *testing.T) {
	s := newSet(t, time.Millisecond)
	if cmd := s.Start("a", "syncing"); cmd == nil {
		t.Fatal("Start returned no tick command")
	}
	got, ok := s.Render("a", 20)
	if !ok {
		t.Fatal("Render reported no entry for a started key")
	}
	if !strings.HasSuffix(got, " syncing") {
		t.Errorf("Render = %q, want it to end in the label", got)
	}
	if !s.Active() || s.Count() != 1 {
		t.Errorf("Active=%v Count=%d, want true/1", s.Active(), s.Count())
	}
}

func TestRenderUnknownKey(t *testing.T) {
	s := newSet(t, time.Millisecond)
	if _, ok := s.Render("missing", 20); ok {
		t.Error("Render reported an entry for a key that was never started")
	}
}

func TestRenderNarrowDropsLabelNotGlyph(t *testing.T) {
	s := newSet(t, time.Millisecond)
	s.Start("a", "syncing")

	// "⠋ syncing" is 9 cells; one less than that must not truncate the word.
	got, _ := s.Render("a", 8)
	if strings.Contains(got, "sync") {
		t.Errorf("Render(width=8) = %q, want the glyph alone rather than a cut label", got)
	}
	if got == "" {
		t.Error("Render(width=8) dropped the glyph too")
	}

	if wide, _ := s.Render("a", 9); !strings.Contains(wide, "syncing") {
		t.Errorf("Render(width=9) = %q, want the full label to fit", wide)
	}
	if _, ok := s.Render("a", 0); ok {
		t.Error("Render(width=0) reported an entry")
	}
}

func TestFinishShowsOutcomeGlyphs(t *testing.T) {
	g := glyph.Default()
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"success", nil, g.ActivityOK},
		{"failure", errors.New("boom"), g.ActivityFail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSet(t, time.Minute) // long hold: the outcome stays put
			s.Start("a", "syncing")
			s.Finish("a", tc.err)

			got, ok := s.Render("a", 20)
			if !ok {
				t.Fatal("entry vanished on Finish rather than holding its outcome")
			}
			if !strings.HasPrefix(got, tc.want) {
				t.Errorf("Render = %q, want it to start with %q", got, tc.want)
			}
			st, _ := s.State("a")
			if !st.Done {
				t.Error("State.Done is false after Finish")
			}
			if st.Failed() != (tc.err != nil) {
				t.Errorf("State.Failed() = %v, want %v", st.Failed(), tc.err != nil)
			}
			if s.Count() != 0 {
				t.Errorf("Count = %d, want 0 — a held outcome is not running", s.Count())
			}
		})
	}
}

func TestHoldClearsTheEntry(t *testing.T) {
	s := newSet(t, time.Millisecond)
	s.Start("a", "syncing")

	msg := runCmd(s.Finish("a", nil))
	if msg == nil {
		t.Fatal("Finish returned no hold command")
	}
	if _, ok := s.Render("a", 20); !ok {
		t.Fatal("entry cleared before its hold was delivered")
	}

	s.Handle(msg, nil)
	if _, ok := s.Render("a", 20); ok {
		t.Error("entry survived its hold")
	}
}

func TestNegativeHoldKeepsTheOutcome(t *testing.T) {
	s := newSet(t, -1)
	s.Start("a", "syncing")
	if cmd := s.Finish("a", nil); cmd != nil {
		t.Error("Finish armed a hold timer despite a negative Hold")
	}
	if _, ok := s.Render("a", 20); !ok {
		t.Error("outcome cleared under a negative Hold")
	}
	s.Clear("a")
	if _, ok := s.Render("a", 20); ok {
		t.Error("Clear left the entry in place")
	}
}

// A key restarted during its hold must not be cleared by the timer the
// previous run armed — the generation is what makes that safe.
func TestStaleHoldDoesNotClearARestartedKey(t *testing.T) {
	s := newSet(t, time.Millisecond)
	s.Start("a", "syncing")
	stale := runCmd(s.Finish("a", nil))

	s.Start("a", "syncing again")
	s.Handle(stale, nil)

	st, ok := s.State("a")
	if !ok {
		t.Fatal("the stale hold cleared a restarted key")
	}
	if st.Label != "syncing again" || st.Done {
		t.Errorf("State = %+v, want the live restart", st)
	}
}

func TestHandleStartAppliesOnlyHeldKeys(t *testing.T) {
	s := newSet(t, time.Millisecond)
	held := map[string]bool{"a": true, "b": true}

	s.Handle(StartMsg{Keys: []string{"a", "c"}, Label: "syncing", RunID: 7},
		func(k string) bool { return held[k] })

	if _, ok := s.State("a"); !ok {
		t.Error("a held key was not started")
	}
	if _, ok := s.State("c"); ok {
		t.Error("a key the component does not hold was started")
	}
}

func TestHandleStartWithNilHoldsTakesEverything(t *testing.T) {
	s := newSet(t, time.Millisecond)
	s.Handle(StartMsg{Keys: []string{"a", "b"}, Label: "syncing", RunID: 7}, nil)
	if s.Count() != 2 {
		t.Errorf("Count = %d, want 2", s.Count())
	}
}

func TestRunScopedUpdateAndEnd(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Handle(StartMsg{Keys: []string{"a", "b"}, Label: "syncing", RunID: 7}, nil)
	s.Handle(StartMsg{Keys: []string{"c"}, Label: "deleting", RunID: 9}, nil)

	s.Handle(UpdateMsg{RunID: 7, Label: "syncing 1/2"}, nil)
	for _, k := range []string{"a", "b"} {
		if st, _ := s.State(k); st.Label != "syncing 1/2" {
			t.Errorf("%s label = %q, want the relabelled run", k, st.Label)
		}
	}
	if st, _ := s.State("c"); st.Label != "deleting" {
		t.Errorf("c label = %q, want another run left alone", st.Label)
	}

	s.Handle(EndMsg{RunID: 7, Err: errors.New("nope")}, nil)
	for _, k := range []string{"a", "b"} {
		if st, _ := s.State(k); !st.Failed() {
			t.Errorf("%s did not finish with the run", k)
		}
	}
	if st, _ := s.State("c"); st.Done {
		t.Error("c finished with another run's EndMsg")
	}
}

func TestRelabelDoesNotResetSince(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Start("a", "syncing")
	before, _ := s.State("a")

	s.Relabel("a", "syncing 3/7")
	after, _ := s.State("a")

	if !after.Since.Equal(before.Since) {
		t.Error("Relabel restarted the clock; Since should measure the work")
	}
	if after.Label != "syncing 3/7" {
		t.Errorf("Label = %q, want the new one", after.Label)
	}
}

func TestRelabelIgnoresFinishedKeys(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Start("a", "syncing")
	s.Finish("a", nil)
	s.Relabel("a", "syncing again")
	if st, _ := s.State("a"); st.Label != "syncing" {
		t.Errorf("Label = %q, want a held outcome left alone", st.Label)
	}
}

func TestTickStopsWhenNothingIsRunning(t *testing.T) {
	s := newSet(t, time.Minute)
	tick := runCmd(s.Start("a", "syncing"))
	if tick == nil {
		t.Fatal("Start armed no tick")
	}
	if next := s.Handle(tick, nil); next == nil {
		t.Fatal("a running set stopped animating")
	}

	s.Finish("a", nil) // held, not running
	if next := s.Handle(tick, nil); next != nil {
		t.Error("the set kept animating with nothing running")
	}
}

func TestSecondStartDoesNotStartASecondTickChain(t *testing.T) {
	s := newSet(t, time.Minute)
	if cmd := s.Start("a", "syncing"); cmd == nil {
		t.Fatal("first Start armed no tick")
	}
	if cmd := s.Start("b", "syncing"); cmd != nil {
		t.Error("second Start armed a second tick chain; the spinner would run double speed")
	}
}

func TestAdoptCarriesEntriesAndRearms(t *testing.T) {
	old := newSet(t, time.Millisecond)
	old.Start("a", "syncing")
	old.Start("b", "syncing")
	old.Finish("b", nil)

	fresh := newSet(t, time.Millisecond)
	if cmd := fresh.Adopt(old); cmd == nil {
		t.Fatal("Adopt returned no command; the spinner and hold are both stranded")
	}

	if st, ok := fresh.State("a"); !ok || st.Done {
		t.Error("a running entry did not survive the rebuild")
	}
	if st, ok := fresh.State("b"); !ok || !st.Done {
		t.Error("a held outcome did not survive the rebuild")
	}
}

func TestAdoptedEntriesAreIndependent(t *testing.T) {
	old := newSet(t, time.Minute)
	old.Start("a", "syncing")

	fresh := newSet(t, time.Minute)
	fresh.Adopt(old)
	fresh.Relabel("a", "changed")

	if st, _ := old.State("a"); st.Label != "syncing" {
		t.Error("Adopt aliased the old Set's entries rather than copying them")
	}
}

func TestClearAll(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Start("a", "x")
	s.Start("b", "y")
	s.ClearAll()
	if s.Active() {
		t.Error("ClearAll left entries behind")
	}
}

func TestStyleIsAppliedAndOptional(t *testing.T) {
	plain := New(Options{Hold: time.Minute})
	plain.Start("a", "syncing")
	got, _ := plain.Render("a", 20)
	if strings.Contains(got, "\x1b") {
		t.Errorf("Render = %q, want no escapes without a Style", got)
	}

	styled := New(Options{Hold: time.Minute, Style: func(st State, text string) string {
		if st.Failed() {
			return "!" + text
		}
		return "<" + text + ">"
	}})
	styled.Start("a", "syncing")
	if got, _ := styled.Render("a", 20); !strings.HasPrefix(got, "<") {
		t.Errorf("Render = %q, want the Style applied", got)
	}
	styled.Finish("a", errors.New("boom"))
	if got, _ := styled.Render("a", 20); !strings.HasPrefix(got, "!") {
		t.Errorf("Render = %q, want Style to see the failed state", got)
	}
}

type progressWriter struct{ got []string }

func (w *progressWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *progressWriter) Progress(text string)        { w.got = append(w.got, text) }

type plainWriter struct{}

func (plainWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestProgress(t *testing.T) {
	w := &progressWriter{}
	Progress(w, "syncing 3/7")
	if len(w.got) != 1 || w.got[0] != "syncing 3/7" {
		t.Errorf("got %v, want one report", w.got)
	}

	// The whole point of the optional interface: an action written against a
	// plain io.Writer must not care.
	Progress(plainWriter{}, "ignored")
}

func TestZeroHoldIsTheDefault(t *testing.T) {
	if got := New(Options{}).hold; got != DefaultHold {
		t.Errorf("hold = %v, want DefaultHold", got)
	}
}

// spinner.Dot's frames carry a trailing space. Composing on top of that would
// double the gap and cost a cell the outcome glyphs do not pay.
func TestSpinnerFramePaddingIsTrimmed(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Start("a", "syncing")
	got, _ := s.Render("a", 40)
	if strings.Contains(got, "  ") {
		t.Errorf("Render = %q, want a single space between glyph and label", got)
	}

	s.Finish("a", nil)
	done, _ := s.Render("a", 40)
	if strings.Contains(done, "  ") {
		t.Errorf("finished Render = %q, want a single space", done)
	}
}

func TestBadgeRightAligns(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Start("a", "syncing")

	got := s.Badge("a", "worker-pool", 40)
	if xansi.StringWidth(got) != 40 {
		t.Errorf("width = %d, want 40 (%q)", xansi.StringWidth(got), got)
	}
	if !strings.HasPrefix(got, "worker-pool") {
		t.Errorf("got %q, want the row text first", got)
	}
	if !strings.HasSuffix(got, "syncing") {
		t.Errorf("got %q, want the badge last", got)
	}
}

func TestBadgeTruncatesTheRowNotTheBadge(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Start("a", "syncing")

	got := s.Badge("a", strings.Repeat("x", 100), 20)
	if xansi.StringWidth(got) != 20 {
		t.Errorf("width = %d, want 20 (%q)", xansi.StringWidth(got), got)
	}
	if !strings.HasSuffix(got, "syncing") {
		t.Errorf("got %q, want the badge to survive", got)
	}
}

func TestBadgeLeavesUnknownKeysAlone(t *testing.T) {
	s := newSet(t, time.Minute)
	if got := s.Badge("missing", "row", 40); got != "row" {
		t.Errorf("got %q, want the row untouched", got)
	}
}

func TestBadgeIsCappedAtHalfTheRow(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Start("a", "a very long status label indeed")
	got := s.Badge("a", "row", 20)
	if xansi.StringWidth(got) != 20 {
		t.Errorf("width = %d, want 20 (%q)", xansi.StringWidth(got), got)
	}
	if !strings.Contains(got, "row") {
		t.Errorf("got %q, want the row still visible", got)
	}
}

// --- decisions 18-20 ------------------------------------------------------

func TestDeriveDrivesActivityWithNoLocalCall(t *testing.T) {
	s := newSet(t, time.Minute)
	if cmd := s.Derive(map[string]string{"a": "running"}); cmd == nil {
		t.Fatal("Derive armed no tick")
	}
	st, ok := s.State("a")
	if !ok || st.Label != "running" || st.Done {
		t.Errorf("State = %+v, %v; want a running derived entry", st, ok)
	}
	if s.Count() != 1 {
		t.Errorf("Count = %d, want 1", s.Count())
	}
}

func TestDeriveIsWholesale(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Derive(map[string]string{"a": "running", "b": "pending"})
	s.Derive(map[string]string{"a": "running"})

	if _, ok := s.State("b"); ok {
		t.Error("a key absent from the new observation survived it")
	}
	if _, ok := s.State("a"); !ok {
		t.Error("a key still present was dropped")
	}
}

// Elapsed time should measure the work, not the poll that last saw it.
func TestDerivePreservesSince(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Derive(map[string]string{"a": "running"})
	first, _ := s.State("a")

	s.Derive(map[string]string{"a": "running"})
	second, _ := s.State("a")

	if !second.Since.Equal(first.Since) {
		t.Error("Since restarted on the second observation of the same work")
	}
}

func TestDerivedEntryClearsWithNoOutcomeGlyph(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Derive(map[string]string{"a": "running"})
	s.Derive(map[string]string{})

	if _, ok := s.Render("a", 20); ok {
		t.Error("a derived entry left an outcome behind; the cell's own value says how it ended")
	}
}

func TestLocalWinsOverDerived(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Derive(map[string]string{"a": "running"})
	s.Start("a", "launching")

	if st, _ := s.State("a"); st.Label != "launching" {
		t.Errorf("Label = %q, want the local entry to win", st.Label)
	}
}

// The stale-poll flicker: a request already in flight when the user acted
// comes back carrying the pre-click value. It must not wipe the spinner.
func TestDerivedRestingDoesNotRetireALiveLocalEntry(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Start("a", "launching")
	s.Derive(map[string]string{}) // the stale page: nothing is busy

	st, ok := s.State("a")
	if !ok || st.Done {
		t.Fatalf("State = %+v, %v; the local entry was retired by a stale observation", st, ok)
	}
	if st.Label != "launching" {
		t.Errorf("Label = %q, want the local entry untouched", st.Label)
	}
}

// Decision 19: the scenario this exists for. A dispatch that returns in 200ms
// must not clear the row before the data has said anything about the work.
func TestSuccessfulFinishWaitsForTheNextObservation(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Derive(map[string]string{}) // a source of truth exists
	s.Start("a", "launching")

	if cmd := s.Finish("a", nil); cmd == nil {
		t.Fatal("Finish armed no confirmation timer")
	}
	st, ok := s.State("a")
	if !ok || st.Done {
		t.Fatalf("State = %+v; want the row still moving while it waits", st)
	}
	if s.Count() != 1 {
		t.Errorf("Count = %d, want the entry to still read as running", s.Count())
	}

	s.Derive(map[string]string{}) // the observation: still not busy
	if _, ok := s.State("a"); ok {
		t.Error("the entry survived the observation that should have retired it")
	}
}

func TestConfirmingObservationCanHandOffToDerived(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Derive(map[string]string{})
	s.Start("a", "launching")
	s.Finish("a", nil)

	s.Derive(map[string]string{"a": "running"}) // the server picked it up

	st, ok := s.State("a")
	if !ok {
		t.Fatal("the handoff left an idle frame with no entry at all")
	}
	if st.Label != "running" || st.Done {
		t.Errorf("State = %+v, want the derived entry to have taken over", st)
	}
}

// A failed dispatch started nothing, so there is nothing to confirm.
func TestFailedFinishReportsAtOnce(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Derive(map[string]string{})
	s.Start("a", "launching")
	s.Finish("a", errors.New("connection refused"))

	st, _ := s.State("a")
	if !st.Failed() {
		t.Errorf("State = %+v, want the error reported without waiting", st)
	}
}

// With no source of truth, nothing will ever call Derive, so the handoff has
// to switch itself off or the row spins until Confirm expires.
func TestNoDeriveMeansNoHandoff(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Start("a", "syncing")
	s.Finish("a", nil)

	if st, _ := s.State("a"); !st.Done {
		t.Error("a Set that has never derived deferred its outcome anyway")
	}
}

func TestConfirmExpiryFallsBackToTheOutcome(t *testing.T) {
	s := New(Options{Hold: time.Minute, Confirm: time.Millisecond})
	s.Derive(map[string]string{})
	s.Start("a", "launching")

	msg := runCmd(s.Finish("a", nil))
	if msg == nil {
		t.Fatal("no confirmation timer")
	}
	s.Handle(msg, nil)

	st, ok := s.State("a")
	if !ok || !st.Done || st.Err != nil {
		t.Errorf("State = %+v, %v; want the action's own outcome after expiry", st, ok)
	}
}

func TestConfirmExpiryIgnoresARestartedKey(t *testing.T) {
	s := New(Options{Hold: time.Minute, Confirm: time.Millisecond})
	s.Derive(map[string]string{})
	s.Start("a", "launching")
	stale := runCmd(s.Finish("a", nil))

	s.Start("a", "launching again")
	s.Handle(stale, nil)

	if st, _ := s.State("a"); st.Done || st.Label != "launching again" {
		t.Errorf("State = %+v, want the stale expiry ignored", st)
	}
}

func TestBusyPredicate(t *testing.T) {
	pred := Busy("running", "pending")
	for _, tc := range []struct {
		in    string
		label string
		busy  bool
	}{
		{"running", "running", true},
		{"  Running ", "Running", true},
		{"\x1b[32mpending\x1b[0m", "pending", true},
		{"successful", "", false},
		{"", "", false},
	} {
		label, busy := pred(tc.in)
		if busy != tc.busy || label != tc.label {
			t.Errorf("Busy(%q) = (%q, %v), want (%q, %v)", tc.in, label, busy, tc.label, tc.busy)
		}
	}
}

func TestReviseFlashesAnUnseenChange(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Revise(map[string]Change{"a": {Rev: "t1", Label: "successful"}})
	if _, ok := s.State("a"); ok {
		t.Fatal("a first sighting flashed; every row would flash on load")
	}

	s.Revise(map[string]Change{"a": {Rev: "t2", Label: "failed"}})
	st, ok := s.State("a")
	if !ok || !st.Changed || !st.Done {
		t.Fatalf("State = %+v, %v; want a change flash", st, ok)
	}
	if st.Label != "failed" {
		t.Errorf("Label = %q, want the row's new value — a bare glyph would hide it", st.Label)
	}
	if got, _ := s.Render("a", 20); !strings.HasPrefix(got, glyph.Default().ActivityChanged) {
		t.Errorf("Render = %q, want the changed glyph, not an outcome", got)
	}
}

func TestReviseIsQuietWhenTheRowIsAlreadyBusy(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Revise(map[string]Change{"a": {Rev: "t1"}})
	s.Derive(map[string]string{"a": "running"})
	s.Revise(map[string]Change{"a": {Rev: "t2"}})

	if st, _ := s.State("a"); st.Changed {
		t.Error("flashed a row whose spinner already says it is working")
	}
}

func TestReviseDefersToALocalEntry(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Revise(map[string]Change{"a": {Rev: "t1"}})
	s.Start("a", "launching")
	s.Revise(map[string]Change{"a": {Rev: "t2"}})

	if st, _ := s.State("a"); st.Changed || st.Label != "launching" {
		t.Errorf("State = %+v, want the local entry to own the row", st)
	}
}

func TestReviseUnchangedIsQuiet(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Revise(map[string]Change{"a": {Rev: "t1"}})
	s.Revise(map[string]Change{"a": {Rev: "t1"}})
	if _, ok := s.State("a"); ok {
		t.Error("an unchanged revision flashed")
	}
}

func TestAdoptCarriesBothLayers(t *testing.T) {
	old := newSet(t, time.Minute)
	old.Derive(map[string]string{"a": "running"})
	old.Start("b", "launching")
	old.Finish("b", nil) // awaiting confirmation

	fresh := newSet(t, time.Minute)
	fresh.Adopt(old)

	if st, ok := fresh.State("a"); !ok || st.Label != "running" {
		t.Error("the derived layer did not survive the rebuild")
	}
	if st, ok := fresh.State("b"); !ok || st.Done {
		t.Error("an awaiting local entry did not survive the rebuild as running")
	}
	fresh.Derive(map[string]string{})
	if _, ok := fresh.State("b"); ok {
		t.Error("the adopted entry lost its pending handoff")
	}
}

// --- a starved tick chain ------------------------------------------------

// screen.Stack forwards messages to the top screen only, so pushing anything
// over a screen with a spinner sends its ticks somewhere that drops them. The
// chain ends, and before reviveTick existed it never came back: ticking stayed
// true, armTick refused, and the row was frozen mid-spin for the rest of the
// session. Reported from the output console, reachable from any child screen.
func TestTickChainRevivesAfterBeingStarved(t *testing.T) {
	fast := spinner.Spinner{Frames: []string{"1", "2"}, FPS: time.Millisecond}
	s := New(Options{Hold: time.Minute, Spinner: &fast})

	if cmd := s.Start("a", "syncing"); cmd == nil {
		t.Fatal("Start armed no tick")
	}
	// The chain is nominally in flight; never deliver its tick, which is what
	// a hidden screen does to it.
	time.Sleep(20 * time.Millisecond)

	if got := s.Handle(struct{}{}, nil); got == nil {
		t.Error("a starved animation was never revived")
	}
}

// The revival must not fire on a healthy chain, or every message would arm
// another one.
func TestHealthyTickChainIsNotRearmed(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Start("a", "syncing")
	if got := s.Handle(struct{}{}, nil); got != nil {
		t.Error("re-armed a chain that had only just started")
	}
}

func TestIdleSetIsNotRevived(t *testing.T) {
	fast := spinner.Spinner{Frames: []string{"1", "2"}, FPS: time.Millisecond}
	s := New(Options{Hold: time.Minute, Spinner: &fast})
	time.Sleep(20 * time.Millisecond)

	if got := s.Handle(struct{}{}, nil); got != nil {
		t.Error("an idle Set scheduled a tick")
	}

	// And a held outcome is not running either.
	s.Start("a", "syncing")
	s.Finish("a", nil)
	time.Sleep(20 * time.Millisecond)
	if got := s.Handle(struct{}{}, nil); got != nil {
		t.Error("a held outcome kept the animation alive")
	}
}

// A derived entry animates too, so it must be revivable on the same terms.
func TestDerivedEntryIsRevived(t *testing.T) {
	fast := spinner.Spinner{Frames: []string{"1", "2"}, FPS: time.Millisecond}
	s := New(Options{Hold: time.Minute, Spinner: &fast})

	s.Derive(map[string]string{"a": "running"})
	time.Sleep(20 * time.Millisecond)

	if got := s.Handle(struct{}{}, nil); got == nil {
		t.Error("a starved derived entry was never revived")
	}
}

// --- a progress phase must not outlive the work ---------------------------

// Progress describes a step in flight. Carried into an outcome it misreports
// the row: a failed sync read "✗ applying", naming a step that had finished,
// and one waiting for confirmation sat on "applying" after the action had
// stopped applying anything.
func TestOutcomeRevertsToTheBaseLabel(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Start("a", "syncing")
	s.Relabel("a", "applying")
	if st, _ := s.State("a"); st.Label != "applying" {
		t.Fatalf("Label = %q, want the progress phase while running", st.Label)
	}

	s.Finish("a", errors.New("boom"))
	st, _ := s.State("a")
	if st.Label != "syncing" {
		t.Errorf("Label = %q after failing, want the base label", st.Label)
	}
	if !st.Failed() {
		t.Error("the outcome was lost with the label")
	}
}

func TestHandoffRevertsToTheBaseLabel(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Derive(map[string]string{}) // a source of truth exists
	s.Start("a", "refreshing")
	s.Relabel("a", "submitting")

	s.Finish("a", nil) // awaiting confirmation, still moving
	st, ok := s.State("a")
	if !ok || st.Done {
		t.Fatalf("state = %+v, %v; want the handoff still running", st, ok)
	}
	if st.Label != "refreshing" {
		t.Errorf("Label = %q while awaiting, want the base label", st.Label)
	}
}

// A restart re-bases, so a phase from the previous run cannot survive into the
// next one's outcome.
func TestRestartRebasesTheLabel(t *testing.T) {
	s := newSet(t, time.Minute)
	s.Start("a", "syncing")
	s.Relabel("a", "applying")
	s.Start("a", "deleting")
	s.Finish("a", errors.New("nope"))

	if st, _ := s.State("a"); st.Label != "deleting" {
		t.Errorf("Label = %q, want the new run's base", st.Label)
	}
}

func TestSettledPredicate(t *testing.T) {
	pred := Settled("Synced", "OutOfSync")
	for _, tc := range []struct {
		in    string
		label string
		busy  bool
	}{
		{"Synced", "", false},
		{"OutOfSync", "", false},
		{"  synced ", "", false},     // case- and space-insensitive
		{"Syncing", "Syncing", true}, // labelled with the server's word
		{"Refreshing", "Refreshing", true},
		{"Terminating", "Terminating", true}, // a status it has never heard of
		{"", "", false},                      // a blank cell is not work
		{"\x1b[32mSynced\x1b[0m", "", false},
	} {
		label, busy := pred(tc.in)
		if busy != tc.busy || label != tc.label {
			t.Errorf("Settled(%q) = (%q, %v), want (%q, %v)", tc.in, label, busy, tc.label, tc.busy)
		}
	}
}

// The reason to prefer Settled: a server that learns a new in-progress status
// keeps spinning, where a list of busy values would treat it as done.
func TestSettledAndBusyDisagreeOnAnUnknownStatus(t *testing.T) {
	const novel = "Terminating"
	if _, busy := Busy("Syncing")(novel); busy {
		t.Error("Busy claimed to recognise a status it was not given")
	}
	if _, busy := Settled("Synced", "OutOfSync")(novel); !busy {
		t.Error("Settled treated an unknown status as done")
	}
}

package activity

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"
)

func newSet() Set { return New(Options{}) }

// runCmd executes a command and returns the message it produced. Nil in, nil
// out, so a caller can chain without branching.
func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// --- observing ------------------------------------------------------------

func TestDeriveMakesAKeyBusy(t *testing.T) {
	s := newSet()
	if cmd := s.Derive(map[string]string{"a": "running"}); cmd == nil {
		t.Fatal("Derive armed no tick, so the spinner would never animate")
	}
	st, ok := s.State("a")
	if !ok || st.Label != "running" {
		t.Errorf("State = %+v, %v; want a busy entry labelled from the data", st, ok)
	}
	if !s.Active() || s.Count() != 1 {
		t.Errorf("Active=%v Count=%d, want true/1", s.Active(), s.Count())
	}
}

// The central property: an observation is the whole truth as of that moment,
// so a key absent from it is not busy — the caller never has to report that a
// row stopped.
func TestDeriveIsWholesale(t *testing.T) {
	s := newSet()
	s.Derive(map[string]string{"a": "running", "b": "pending"})
	s.Derive(map[string]string{"a": "running"})

	if _, ok := s.State("b"); ok {
		t.Error("a key absent from the new observation survived it")
	}
	if _, ok := s.State("a"); !ok {
		t.Error("a key still present was dropped")
	}
}

func TestAnEmptyObservationClearsEverything(t *testing.T) {
	s := newSet()
	s.Derive(map[string]string{"a": "running"})
	s.Derive(map[string]string{})

	if s.Active() || s.Count() != 0 {
		t.Errorf("Active=%v Count=%d after an empty observation", s.Active(), s.Count())
	}
	if _, ok := s.Render("a", 20); ok {
		t.Error("an entry left something behind; the cell's own value says how it ended")
	}
}

// Elapsed time should measure the work, not the poll that last saw it.
func TestSincePersistsAcrossObservations(t *testing.T) {
	s := newSet()
	s.Derive(map[string]string{"a": "running"})
	first, _ := s.State("a")

	s.Derive(map[string]string{"a": "running"})
	second, _ := s.State("a")

	if !second.Since.Equal(first.Since) {
		t.Error("Since restarted on the second observation of the same work")
	}
}

// A row that goes busy, settles, and goes busy again is new work.
func TestSinceRestartsAfterAGap(t *testing.T) {
	s := newSet()
	s.Derive(map[string]string{"a": "running"})
	first, _ := s.State("a")
	s.Derive(map[string]string{})
	time.Sleep(2 * time.Millisecond)
	s.Derive(map[string]string{"a": "running"})

	if second, _ := s.State("a"); !second.Since.After(first.Since) {
		t.Error("Since survived a settled observation; that is a second piece of work")
	}
}

func TestRelabellingFollowsTheData(t *testing.T) {
	s := newSet()
	s.Derive(map[string]string{"a": "Pending"})
	s.Derive(map[string]string{"a": "Syncing"})

	if st, _ := s.State("a"); st.Label != "Syncing" {
		t.Errorf("Label = %q, want the newest observation's word", st.Label)
	}
}

// --- rendering ------------------------------------------------------------

func TestRenderIsGlyphThenLabel(t *testing.T) {
	s := newSet()
	s.Derive(map[string]string{"a": "syncing"})

	got, ok := s.Render("a", 20)
	if !ok {
		t.Fatal("Render reported no entry for a busy key")
	}
	if !strings.HasSuffix(got, " syncing") {
		t.Errorf("Render = %q, want it to end in the label", got)
	}
}

func TestRenderUnknownKey(t *testing.T) {
	s := newSet()
	if _, ok := s.Render("missing", 20); ok {
		t.Error("Render reported an entry for a key the data never reported busy")
	}
}

// A cell of eight showing "⣾ syncin" is worse than one showing "⣾".
func TestNarrowRenderDropsTheLabelNotTheGlyph(t *testing.T) {
	s := newSet()
	s.Derive(map[string]string{"a": "syncing"})

	got, _ := s.Render("a", 8)
	if strings.Contains(got, "sync") {
		t.Errorf("Render = %q, want the label dropped rather than cut", got)
	}
	if xansi.StringWidth(got) == 0 {
		t.Error("the glyph went too")
	}
	if w := xansi.StringWidth(got); w > 8 {
		t.Errorf("width = %d, want it to fit in 8", w)
	}
}

func TestRenderNeverExceedsItsWidth(t *testing.T) {
	s := newSet()
	s.Derive(map[string]string{"a": "a-very-long-status-indeed"})
	for w := 1; w <= 40; w++ {
		got, ok := s.Render("a", w)
		if !ok {
			t.Fatalf("width %d: no render", w)
		}
		if n := xansi.StringWidth(got); n > w {
			t.Errorf("width %d produced %d cells: %q", w, n, got)
		}
	}
}

func TestZeroWidthRendersNothing(t *testing.T) {
	s := newSet()
	s.Derive(map[string]string{"a": "running"})
	if _, ok := s.Render("a", 0); ok {
		t.Error("a zero-width cell still rendered an indicator")
	}
}

// spinner.Dot's frames carry a trailing space. Composing on top of that would
// double the gap between glyph and label.
func TestSpinnerFramePaddingIsTrimmed(t *testing.T) {
	s := newSet()
	s.Derive(map[string]string{"a": "x"})
	got, _ := s.Render("a", 20)
	if strings.Contains(got, "  x") {
		t.Errorf("Render = %q, want a single space before the label", got)
	}
}

func TestStyleIsAppliedAndOptional(t *testing.T) {
	plain := newSet()
	plain.Derive(map[string]string{"a": "running"})
	bare, _ := plain.Render("a", 20)
	if strings.Contains(bare, "\x1b") {
		t.Errorf("a nil Style emitted escapes: %q", bare)
	}

	styled := New(Options{Style: func(st State, text string) string {
		return "<" + st.Label + ">" + text
	}})
	styled.Derive(map[string]string{"a": "running"})
	got, _ := styled.Render("a", 20)
	if !strings.HasPrefix(got, "<running>") {
		t.Errorf("Render = %q, want Style applied with the state", got)
	}
}

// --- the badge ------------------------------------------------------------

func TestBadgeRightAligns(t *testing.T) {
	s := newSet()
	s.Derive(map[string]string{"a": "running"})

	got := s.Badge("a", "worker", 30)
	if xansi.StringWidth(got) != 30 {
		t.Errorf("width = %d, want the full row width", xansi.StringWidth(got))
	}
	if !strings.HasPrefix(got, "worker") || !strings.HasSuffix(got, "running") {
		t.Errorf("Badge = %q, want the row then the indicator", got)
	}
}

// The badge is the news; the row is what gives way.
func TestBadgeTruncatesTheRowNotTheBadge(t *testing.T) {
	s := newSet()
	s.Derive(map[string]string{"a": "running"})

	got := s.Badge("a", strings.Repeat("x", 60), 24)
	if !strings.Contains(got, "running") {
		t.Errorf("Badge = %q, want the indicator kept", got)
	}
	if xansi.StringWidth(got) != 24 {
		t.Errorf("width = %d, want 24", xansi.StringWidth(got))
	}
}

func TestBadgeLeavesSettledRowsAlone(t *testing.T) {
	s := newSet()
	if got := s.Badge("a", "worker", 30); got != "worker" {
		t.Errorf("Badge = %q, want the row untouched", got)
	}
}

// --- the tick chain -------------------------------------------------------

func TestAnIdleSetSchedulesNothing(t *testing.T) {
	s := newSet()
	if cmd := s.Derive(map[string]string{}); cmd != nil {
		t.Error("an observation with nothing busy armed a tick")
	}
	if cmd := s.Handle(struct{}{}); cmd != nil {
		t.Error("an idle Set armed a tick from an unrelated message")
	}
}

func TestASecondObservationDoesNotStartASecondChain(t *testing.T) {
	s := newSet()
	if cmd := s.Derive(map[string]string{"a": "running"}); cmd == nil {
		t.Fatal("the first observation armed nothing")
	}
	if cmd := s.Derive(map[string]string{"a": "running", "b": "running"}); cmd != nil {
		t.Error("a second observation armed a second chain")
	}
}

func TestTheChainStopsWhenTheLastRowSettles(t *testing.T) {
	s := newSet()
	s.Derive(map[string]string{"a": "running"})
	tick, ok := runCmd(s.spin.Tick).(spinner.TickMsg)
	if !ok {
		t.Fatal("no tick")
	}
	if cmd := s.Handle(tick); cmd == nil {
		t.Error("the chain stopped while a row was still busy")
	}

	s.Derive(map[string]string{})
	if cmd := s.Handle(tick); cmd != nil {
		t.Error("the chain kept going after the last row settled")
	}
}

// The chain lives in commands in flight, so anything that stops delivering
// messages ends it — and ticking stays true, which would leave armTick
// refusing to start another. A frozen spinner on a row that is still working
// is the symptom, and it does not recover on its own.
func TestAStarvedChainIsRevived(t *testing.T) {
	s := newSet()
	s.Derive(map[string]string{"a": "running"})

	// Pretend the last tick was long enough ago that the chain must be dead.
	s.lastTick = time.Now().Add(-time.Second)

	if cmd := s.Handle(struct{}{}); cmd == nil {
		t.Error("a starved chain was not revived, so the row is frozen for good")
	}
}

func TestRecentTicksAreNotRevived(t *testing.T) {
	s := newSet()
	s.Derive(map[string]string{"a": "running"})
	if cmd := s.Handle(struct{}{}); cmd != nil {
		t.Error("a healthy chain was duplicated by an unrelated message")
	}
}

// --- carrying across a rebuild --------------------------------------------

func TestAdoptCarriesTheObservationAndRearms(t *testing.T) {
	old := newSet()
	old.Derive(map[string]string{"a": "running"})

	fresh := newSet()
	cmd := fresh.Adopt(old)

	if st, ok := fresh.State("a"); !ok || st.Label != "running" {
		t.Errorf("State = %+v, %v; the rebuilt Set lost the observation", st, ok)
	}
	if cmd == nil {
		t.Error("Adopt re-armed no tick, so the carried row would sit frozen")
	}
}

func TestAdoptKeepsTheNewSetsOwnStyle(t *testing.T) {
	old := New(Options{Style: func(State, string) string { return "OLD" }})
	old.Derive(map[string]string{"a": "running"})

	fresh := New(Options{Style: func(State, string) string { return "NEW" }})
	fresh.Adopt(old)

	if got, _ := fresh.Render("a", 20); got != "NEW" {
		t.Errorf("Render = %q, want the rebuilt Set's own palette", got)
	}
}

// --- the predicates -------------------------------------------------------

func TestBusyMatchesAndLabels(t *testing.T) {
	pred := Busy("running", "pending")
	for _, tc := range []struct {
		in    string
		label string
		busy  bool
	}{
		{"running", "running", true},
		{"RUNNING", "RUNNING", true}, // matched case-insensitively, labelled as it appeared
		{"  pending  ", "pending", true},
		{"succeeded", "", false},
		{"", "", false},
	} {
		label, busy := pred(tc.in)
		if busy != tc.busy || label != tc.label {
			t.Errorf("Busy(%q) = %q, %v; want %q, %v", tc.in, label, busy, tc.label, tc.busy)
		}
	}
}

// The predicate reads data, not presentation.
func TestPredicatesSeeThroughStyling(t *testing.T) {
	styled := "\x1b[32mrunning\x1b[0m"
	if _, busy := Busy("running")(styled); !busy {
		t.Error("Busy did not match a coloured cell")
	}
	if _, busy := Settled("ok")(styled); !busy {
		t.Error("Settled did not treat a coloured non-terminal value as work")
	}
}

// Settled is the safer default: a status the server invents later is treated
// as work rather than quietly stopping the spinner.
func TestSettledTreatsTheUnknownAsWork(t *testing.T) {
	pred := Settled("succeeded", "failed")
	for _, tc := range []struct {
		in    string
		label string
		busy  bool
	}{
		{"succeeded", "", false},
		{"failed", "", false},
		{"", "", false}, // a blank cell is not work in progress
		{"running", "running", true},
		{"some-new-status", "some-new-status", true},
	} {
		label, busy := pred(tc.in)
		if busy != tc.busy || label != tc.label {
			t.Errorf("Settled(%q) = %q, %v; want %q, %v", tc.in, label, busy, tc.label, tc.busy)
		}
	}
}

// Adopt copies rather than aliases, so the Set being replaced cannot write
// into the live one afterwards through a shared map.
func TestAdoptDoesNotAliasTheOtherSet(t *testing.T) {
	old := newSet()
	old.Derive(map[string]string{"a": "running"})

	fresh := newSet()
	fresh.Adopt(old)
	old.Derive(map[string]string{"a": "running", "b": "running"})

	if fresh.Count() != 1 {
		t.Errorf("the adopted Set sees %d entries; it shares a map with the old one", fresh.Count())
	}
}

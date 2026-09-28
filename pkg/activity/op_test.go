package activity

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The v2 operation surface: Dispatch / Done / BeginRead / Accept. Each test
// names the API shape it stands for, because the modes exist for those shapes
// and nothing else.

// unobserved runs cmd and returns every UnobservedMsg it produces.
func unobserved(cmd tea.Cmd) []UnobservedMsg {
	if cmd == nil {
		return nil
	}
	var out []UnobservedMsg
	switch m := cmd().(type) {
	case UnobservedMsg:
		out = append(out, m)
	case tea.BatchMsg:
		for _, c := range m {
			out = append(out, unobserved(c)...)
		}
	}
	return out
}

func claimedBy(s Set, key string, op Op) bool {
	c, ok := s.expected[key]
	return ok && c.op == op.id
}

// --- Observed: Argo sync -------------------------------------------------

// A poll issued before the POST answers carries the pre-write status. It must
// not end the claim — the v1 `writing > 0` rule, now enforced here — but its
// rows still apply, so the rest of the table is not frozen for the POST's
// latency.
func TestAReadTakenAcrossTheWriteCannotEndTheClaim(t *testing.T) {
	s := New(Options{})
	op, _ := s.Dispatch([]string{"a"}, "Syncing", Observed)

	rd := s.BeginRead() // issued while the POST is in flight
	s.Done(op, nil)     // POST answers
	if !s.Accept(rd, nil) {
		t.Fatal("a read in order was refused; its rows should still apply")
	}
	s.observe(nil, map[string]string{"a": "Succeeded"})

	if !claimedBy(s, "a", op) {
		t.Error("a read issued before the acknowledgement ended the claim")
	}
}

// And one taken after the acknowledgement is entitled to.
func TestAReadTakenAfterTheAckEndsTheClaim(t *testing.T) {
	s := New(Options{})
	op, _ := s.Dispatch([]string{"a"}, "Syncing", Observed)
	s.Done(op, nil)

	rd := s.BeginRead()
	s.Accept(rd, nil)
	s.observe(map[string]string{"a": "Running"}, map[string]string{"a": "Running"})

	if claimedBy(s, "a", op) {
		t.Error("a busy read did not confirm the claim")
	}
	if st, _ := s.State("a"); st.Label != "Running" {
		t.Errorf("label = %q, want the server's own %q", st.Label, "Running")
	}
}

// The case that looked like nothing happened: a sync shorter than a poll, on
// an app whose phase reads the same before and after. It still ends — but it
// says so.
func TestAnUnobservedSyncIsReported(t *testing.T) {
	s := New(Options{Settle: 1})
	op, _ := s.dispatchAt([]string{"a", "b"}, "Syncing", Observed,
		map[string]string{"a": "Succeeded", "b": "Succeeded"})
	s.Done(op, nil)

	var got []UnobservedMsg
	for i := 0; i < 2; i++ {
		rd := s.BeginRead()
		s.Accept(rd, nil)
		got = append(got, unobserved(s.observe(nil, map[string]string{"a": "Succeeded", "b": "Succeeded"}))...)
	}

	if len(got) != 1 {
		t.Fatalf("reports = %+v, want one for the operation", got)
	}
	m := got[0]
	if m.Op != op.ID() || len(m.Keys) != 2 || m.Label != "Syncing" || m.Changed {
		t.Errorf("report = %+v", m)
	}
}

// A status that moved while nobody looked is the work having happened.
func TestAChangedStatusIsReportedAsChanged(t *testing.T) {
	s := New(Options{})
	op, _ := s.dispatchAt([]string{"a"}, "Syncing", Observed, map[string]string{"a": ""})
	s.Done(op, nil)
	rd := s.BeginRead()
	s.Accept(rd, nil)
	got := unobserved(s.observe(nil, map[string]string{"a": "Succeeded"}))
	if len(got) != 1 || !got[0].Changed {
		t.Errorf("reports = %+v, want one with Changed", got)
	}
}

// A confirmed claim is the ordinary ending and says nothing.
func TestAConfirmedClaimIsNotReported(t *testing.T) {
	s := New(Options{})
	op, _ := s.dispatchAt([]string{"a"}, "Syncing", Observed, nil)
	s.Done(op, nil)
	rd := s.BeginRead()
	s.Accept(rd, nil)
	if got := unobserved(s.observe(map[string]string{"a": "Running"}, nil)); len(got) != 0 {
		t.Errorf("reports = %+v, want none", got)
	}
}

// A refused request withdraws its claim at once.
func TestARefusedRequestWithdraws(t *testing.T) {
	s := New(Options{})
	op, _ := s.dispatchAt([]string{"a"}, "Syncing", Observed, nil)
	s.Done(op, errors.New("409"))
	if _, ok := s.State("a"); ok {
		t.Error("a refused request left its claim")
	}
}

// --- ordering ------------------------------------------------------------

func TestAnOvertakenReadIsRefused(t *testing.T) {
	s := New(Options{})
	older, newer := s.BeginRead(), s.BeginRead()
	if !s.Accept(newer, nil) {
		t.Fatal("newest read refused")
	}
	if s.Accept(older, nil) {
		t.Error("a read overtaken by a newer one was accepted")
	}
}

// A failed read ends acknowledged Observed claims — nothing is coming to — and
// leaves alone the ones whose own request will end them.
func TestAFailedReadWithdrawsOnlyWhatWasWaitingOnIt(t *testing.T) {
	s := New(Options{})
	acked, _ := s.dispatchAt([]string{"a"}, "Syncing", Observed, nil)
	s.Done(acked, nil)
	inflight, _ := s.dispatchAt([]string{"b"}, "Syncing", Observed, nil)
	held, _ := s.dispatchAt([]string{"c"}, "Refreshing", Held, nil)

	if s.Accept(s.BeginRead(), errors.New("503")) {
		t.Fatal("a failed read was accepted")
	}
	if claimedBy(s, "a", acked) {
		t.Error("an acknowledged Observed claim survived a failed read")
	}
	if !claimedBy(s, "b", inflight) {
		t.Error("a claim whose request is still in flight was withdrawn")
	}
	if !claimedBy(s, "c", held) {
		t.Error("a Held claim was withdrawn by a failed read")
	}
}

// --- Held: Argo refresh, job handles --------------------------------------

// The refresh GET holds the connection; every poll during it reads the app
// exactly as before. None of them may end the claim.
func TestObservationsCannotEndAHeldClaim(t *testing.T) {
	s := New(Options{})
	op, _ := s.dispatchAt([]string{"a"}, "Refreshing", Held, map[string]string{"a": "Succeeded"})

	for i := 0; i < 5; i++ {
		rd := s.BeginRead()
		s.Accept(rd, nil)
		if got := unobserved(s.observe(nil, map[string]string{"a": "Succeeded"})); len(got) != 0 {
			t.Fatalf("a Held claim was reported unobserved: %+v", got)
		}
	}
	if st, ok := s.State("a"); !ok || st.Label != "Refreshing" {
		t.Fatalf("state = %+v %v, want still Refreshing", st, ok)
	}

	s.Done(op, nil)
	if _, ok := s.State("a"); ok {
		t.Error("Done did not end a Held claim")
	}
}

// If the server reports the key busy mid-hold — a refresh that kicked off an
// auto-sync — its label shows, and the busy state outlives the hold.
func TestAHeldClaimDefersToTheServersLabel(t *testing.T) {
	s := New(Options{})
	op, _ := s.dispatchAt([]string{"a"}, "Refreshing", Held, nil)
	s.observe(map[string]string{"a": "Running"}, nil)

	if st, _ := s.State("a"); st.Label != "Running" {
		t.Errorf("label = %q, want the server's %q", st.Label, "Running")
	}
	s.Done(op, nil)
	if st, ok := s.State("a"); !ok || st.Label != "Running" {
		t.Errorf("after Done = %+v %v, want the observed Running to remain", st, ok)
	}
}

// Done for a superseded operation leaves the newer claim alone.
func TestDoneTouchesOnlyItsOwnClaims(t *testing.T) {
	s := New(Options{})
	first, _ := s.dispatchAt([]string{"a"}, "Refreshing", Held, nil)
	second, _ := s.dispatchAt([]string{"a"}, "Syncing", Held, nil)
	s.Done(first, nil)
	if !claimedBy(s, "a", second) {
		t.Error("Done on a superseded operation ended the newer claim")
	}
}

// --- streams ---------------------------------------------------------------

// A watch event is taken as it arrives. Before the acknowledgement it cannot
// speak to the claim; after, it can — with no token at all.
func TestStreamObservationsNeedNoToken(t *testing.T) {
	s := New(Options{})
	op, _ := s.Dispatch([]string{"a"}, "Syncing", Observed)

	s.observe(nil, map[string]string{"a": "Succeeded"}) // event before the POST answered
	if !claimedBy(s, "a", op) {
		t.Fatal("an event from before the acknowledgement ended the claim")
	}
	s.Done(op, nil)
	s.observe(nil, map[string]string{"a": "Succeeded"})
	if claimedBy(s, "a", op) {
		t.Error("an event after the acknowledgement did not end the claim")
	}
}

// --- reads failing ----------------------------------------------------------

// A row busy only because the last good read said so turns Unknown while reads
// fail: kept, since clearing it would invent a read, but not animated.
func TestAFailedReadMakesObservedWorkUnknown(t *testing.T) {
	s := New(Options{})
	s.Accept(s.BeginRead(), nil)
	s.observe(map[string]string{"a": "Running"}, nil)

	s.Accept(s.BeginRead(), errors.New("503"))
	st, ok := s.State("a")
	if !ok || !st.Unknown {
		t.Fatalf("state = %+v %v, want kept and Unknown", st, ok)
	}
	if !s.ReadsFailing() {
		t.Error("ReadsFailing is false after a failed read")
	}
	if text, _ := s.Render("a", 20); !strings.HasPrefix(text, "?") {
		t.Errorf("render = %q, want a static ? in place of the spinner", text)
	}

	s.Accept(s.BeginRead(), nil)
	s.observe(map[string]string{"a": "Running"}, nil)
	if st, _ := s.State("a"); st.Unknown || s.ReadsFailing() {
		t.Error("a good read did not clear Unknown")
	}
}

// A request still open is live whatever the reads are doing.
func TestAnOpenRequestIsNeverUnknown(t *testing.T) {
	s := New(Options{})
	s.dispatchAt([]string{"a"}, "Refreshing", Held, nil)
	s.observe(map[string]string{"a": "Running"}, nil)
	s.Accept(s.BeginRead(), errors.New("503"))
	if st, _ := s.State("a"); st.Unknown {
		t.Error("a key with an open Held request was drawn Unknown")
	}
}

// A failure overtaken by a newer good read says nothing.
func TestAnOvertakenFailureIsIgnored(t *testing.T) {
	s := New(Options{})
	older, newer := s.BeginRead(), s.BeginRead()
	s.Accept(newer, nil)
	s.observe(map[string]string{"a": "Running"}, nil)
	s.Accept(older, errors.New("503"))
	if s.ReadsFailing() {
		t.Error("a failure older than the newest good read marked reads failing")
	}
}

// Done says what the reply means, so a screen with both kinds of verb cannot
// report an Observed request's 202 as the work being finished.
func TestDoneReportsWhatTheReplyMeans(t *testing.T) {
	for _, tc := range []struct {
		mode Mode
		err  error
		want Outcome
	}{
		{Observed, nil, Acknowledged},
		{Held, nil, Ended},
		{Observed, errors.New("409"), Withdrawn},
		{Held, errors.New("503"), Withdrawn},
	} {
		s := New(Options{})
		op, _ := s.Dispatch([]string{"a"}, "Running", tc.mode)
		if got := s.Done(op, tc.err); got != tc.want {
			t.Errorf("Done(%v, %v) = %v, want %v", tc.mode, tc.err, got, tc.want)
		}
	}
}

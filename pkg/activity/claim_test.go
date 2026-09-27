package activity

import (
	"errors"
	"testing"
	"time"
)

func claimed(settle int) Set { return New(Options{Settle: settle}) }

// --- the allowance --------------------------------------------------------

// The three endings, side by side, because the difference between them is the
// whole of Settle: only the uninformative one costs anything.
func TestWhatEachKindOfObservationDoesToAClaim(t *testing.T) {
	for _, tc := range []struct {
		name   string
		busy   map[string]string
		values map[string]string
		kept   bool
	}{
		{
			name: "confirmed",
			busy: map[string]string{"a": "Syncing"},
			kept: false,
		},
		{
			name:   "the value changed, so the server acted",
			values: map[string]string{"a": "Synced"},
			kept:   false,
		},
		{
			name:   "the value is what it was, so nothing was said",
			values: map[string]string{"a": "OutOfSync"},
			kept:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := claimed(3)
			s.expect([]string{"a"}, "Syncing", map[string]string{"a": "OutOfSync"})
			s.observe(tc.busy, tc.values)

			_, stillClaimed := s.expected["a"]
			if stillClaimed != tc.kept {
				t.Errorf("claim kept = %v, want %v", stillClaimed, tc.kept)
			}
		})
	}
}

// Zero is decision 14's original rule: the next observation retires the claim
// whatever it says, which is right for any API that sets status inline.
func TestZeroSettleRetiresOnTheNextObservation(t *testing.T) {
	s := claimed(0)
	s.expect([]string{"a"}, "Syncing", map[string]string{"a": "OutOfSync"})
	s.observe(nil, map[string]string{"a": "OutOfSync"})

	if _, ok := s.State("a"); ok {
		t.Error("a claim survived an observation with a zero allowance")
	}
}

// A ceiling rather than a floor: every uninformative read spends one, so a
// claim nothing ever speaks to still ends. A request the server accepted and
// silently dropped is what this bound exists for.
func TestAnUnconfirmedClaimAlwaysEnds(t *testing.T) {
	s := claimed(2)
	s.expect([]string{"a"}, "Syncing", map[string]string{"a": "OutOfSync"})
	for i := range 2 {
		s.observe(nil, map[string]string{"a": "OutOfSync"})
		if _, ok := s.State("a"); !ok {
			t.Fatalf("claim died on observation %d of an allowance of 2", i+1)
		}
	}
	s.observe(nil, map[string]string{"a": "OutOfSync"})
	if _, ok := s.State("a"); ok {
		t.Error("claim outlived its allowance")
	}
}

// Values are how an observation proves the server acted, so without them every
// observation is uninformative — which is the honest reading for a component
// with no predicate, whose reads can only count claims down.
func TestWithoutValuesEveryObservationIsUninformative(t *testing.T) {
	s := claimed(1)
	s.expect([]string{"a"}, "Syncing", nil)
	s.observe(nil, nil)
	if _, ok := s.State("a"); !ok {
		t.Fatal("the allowance was spent by an observation that could say nothing")
	}
	s.observe(nil, nil)
	if _, ok := s.State("a"); ok {
		t.Error("the allowance was never spent, so the claim cannot end")
	}
}

// --- the union ------------------------------------------------------------

// Observed beats claimed, because that label is the server's own word rather
// than the screen's guess.
func TestAnObservedLabelSupersedesAClaimedOne(t *testing.T) {
	s := claimed(5)
	s.expect([]string{"a"}, "Syncing", nil)
	s.observe(map[string]string{"a": "Reconciling"}, nil)

	st, ok := s.State("a")
	if !ok {
		t.Fatal("the key stopped working across the handoff")
	}
	if st.Label != "Reconciling" {
		t.Errorf("label = %q, want the observed %q", st.Label, "Reconciling")
	}
}

// Elapsed time measures the work, and for work the user started the work began
// at the keypress — not at the poll that first caught up with it.
func TestSinceMeasuresFromTheClaim(t *testing.T) {
	s := claimed(5)
	s.expect([]string{"a"}, "Syncing", nil)
	at, _ := s.State("a")

	time.Sleep(2 * time.Millisecond)
	s.observe(map[string]string{"a": "Syncing"}, nil)

	st, _ := s.State("a")
	if !st.Since.Equal(at.Since) {
		t.Errorf("Since = %v, want the claim's %v — the work started at the keypress", st.Since, at.Since)
	}
}

// --- taking it back -------------------------------------------------------

// A refused request withdraws its own claims and nobody else's.
func TestARefusalWithdrawsOnlyItsOwnClaims(t *testing.T) {
	s := claimed(5)
	refused, _ := s.Dispatch([]string{"a"}, "Syncing", Observed)
	s.Dispatch([]string{"b"}, "Syncing", Observed)
	s.Done(refused, errors.New("409"))

	if _, ok := s.State("a"); ok {
		t.Error("the refused claim is still there")
	}
	if _, ok := s.State("b"); !ok {
		t.Error("refusing one operation took another's claim")
	}
}

// A failed read withdraws claims waiting on reads, and leaves observations —
// those turn Unknown instead (see op_test.go).
func TestAFailedReadDropsClaimsAndKeepsObservations(t *testing.T) {
	s := claimed(5)
	s.derive(map[string]string{"observed": "running"})
	s.expect([]string{"claimed"}, "Syncing", nil)
	s.Accept(s.BeginRead(), errors.New("503"))

	if _, ok := s.State("claimed"); ok {
		t.Error("a claim survived the reads stopping")
	}
	if _, ok := s.State("observed"); !ok {
		t.Error("a failed read took an observation with it")
	}
}

// --- scope ----------------------------------------------------------------

// A Set that has never been told what the component holds scopes nothing: that
// is not the same as one told it holds nothing.
func TestAnUnscopedSetShowsEverything(t *testing.T) {
	s := claimed(0)
	s.derive(map[string]string{"a": "running"})
	if _, ok := s.State("a"); !ok {
		t.Error("an unscoped Set hid a key")
	}

	s.setScope(nil)
	if _, ok := s.State("a"); ok {
		t.Error("a Set told it holds no keys still showed one")
	}
}

// Kept, not discarded — rows and busy-ness arrive on separate cadences.
func TestAnOutOfScopeEntryIsKeptAndComesBack(t *testing.T) {
	s := claimed(0)
	s.setScope([]string{"a"})
	s.derive(map[string]string{"a": "running", "b": "running"})

	if got := s.Count(); got != 1 {
		t.Errorf("Count = %d, want 1 — only one of the two is held", got)
	}
	s.setScope([]string{"a", "b"})
	if _, ok := s.State("b"); !ok {
		t.Error("the out-of-scope entry was discarded rather than kept")
	}
}

// Nothing visible, nothing animated: a spinner nobody can see still costs a
// tick every frame.
func TestAnEntryNoOneCanSeeArmsNoTickChain(t *testing.T) {
	s := claimed(0)
	s.setScope([]string{"a"})
	if cmd := s.derive(map[string]string{"elsewhere": "running"}); cmd != nil {
		t.Error("a tick chain started for a key the component does not hold")
	}
}

// --- the theme-swap carry -------------------------------------------------

func TestAdoptCarriesClaimsAndScope(t *testing.T) {
	old := claimed(4)
	old.setScope([]string{"a"})
	old.expect([]string{"a"}, "Syncing", nil)
	old.derive(map[string]string{"elsewhere": "running"})

	fresh := claimed(4)
	fresh.Adopt(old)

	if _, ok := fresh.State("a"); !ok {
		t.Error("the claim did not survive the rebuild")
	}
	if _, ok := fresh.State("elsewhere"); ok {
		t.Error("the scope did not survive the rebuild, so an unheld key is showing")
	}
}

func TestAdoptDoesNotAliasTheOtherSetsClaims(t *testing.T) {
	old := claimed(4)
	old.expect([]string{"a"}, "Syncing", nil)

	fresh := claimed(4)
	fresh.Adopt(old)
	delete(old.expected, "a")

	if _, ok := fresh.State("a"); !ok {
		t.Error("a change to the discarded Set reached into the live one")
	}
}

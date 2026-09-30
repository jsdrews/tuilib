package resume

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func at(sec int, ms int) time.Time {
	return t0.Add(time.Duration(sec)*time.Second + time.Duration(ms)*time.Millisecond)
}

func TestResumeSkipsExactlyTheLinesAlreadyHeld(t *testing.T) {
	tr := New(Options{})
	for _, ts := range []time.Time{at(0, 100), at(1, 0), at(1, 300), at(1, 600)} {
		tr.Observe(ts)
	}
	since, skip, ok := tr.Resume(at(5, 0))
	if !ok || !since.Equal(at(1, 0)) || skip != 3 {
		t.Fatalf("Resume = %v, %d, %v; want second 1, skip 3", since, skip, ok)
	}
	// The source replays second 1 from its start, then continues.
	replay := []time.Time{at(1, 0), at(1, 300), at(1, 600), at(1, 900), at(2, 0)}
	var kept []time.Time
	for _, ts := range replay {
		if !tr.Drop(ts) {
			kept = append(kept, ts)
			tr.Observe(ts)
		}
	}
	if len(kept) != 2 || !kept[0].Equal(at(1, 900)) {
		t.Errorf("kept %v, want the line at 1.9s and the one at 2s", kept)
	}
}

func TestDropStopsAtTheFirstNewLine(t *testing.T) {
	tr := New(Options{})
	tr.Observe(at(3, 0))
	tr.Resume(at(4, 0))
	if tr.Drop(at(3, 500)) != true {
		t.Fatal("the held line should be skipped")
	}
	if tr.Drop(at(3, 800)) {
		t.Fatal("a second line in the same second is new")
	}
	if tr.Drop(at(1, 0)) {
		t.Error("after the first new line nothing more is dropped")
	}
}

func TestNotResumingDropsNothing(t *testing.T) {
	tr := New(Options{})
	tr.Observe(at(1, 0))
	if tr.Drop(at(1, 0)) {
		t.Error("Drop outside a resume must keep every line")
	}
}

func TestSinceBeforeAnything(t *testing.T) {
	tr := New(Options{})
	if _, _, ok := tr.Since(); ok {
		t.Error("no line observed: start from the tail")
	}
}

func TestReconnectedReportsTheGap(t *testing.T) {
	tr := New(Options{})
	tr.Observe(at(0, 0))
	tr.Lost(at(10, 0))
	tr.Resume(at(52, 0))
	if tr.Reconnected() != 42*time.Second {
		t.Errorf("gap = %v, want 42s", tr.Reconnected())
	}
	tr.Resume(at(60, 0))
	if tr.Reconnected() != 0 {
		t.Error("a resume without a loss has no gap")
	}
}

func TestOlderCutKeepsOnlyOlderLines(t *testing.T) {
	tr := New(Options{})
	// Held: the last two lines of second 5, then second 6.
	for _, ts := range []time.Time{at(5, 500), at(5, 900), at(6, 0)} {
		tr.Observe(ts)
	}
	// A longer tail: second 4, all four lines of second 5, then second 6.
	longer := []time.Time{at(4, 0), at(4, 1), at(5, 0), at(5, 200), at(5, 500), at(5, 900), at(6, 0)}
	if n := tr.OlderCut(longer); n != 4 {
		t.Fatalf("OlderCut = %d, want the 2 lines of second 4 and the first 2 of second 5", n)
	}
	tr.ObserveOlder(longer[:4])
	older := []time.Time{at(3, 0), at(4, 0), at(4, 1), at(5, 0)}
	if n := tr.OlderCut(older); n != 1 {
		t.Errorf("after prepending, OlderCut = %d, want only second 3", n)
	}
}

func TestGranularity(t *testing.T) {
	tr := New(Options{Granularity: time.Millisecond})
	tr.Observe(at(1, 5))
	if since, skip, _ := tr.Since(); !since.Equal(at(1, 5)) || skip != 1 {
		t.Errorf("Since = %v, %d", since, skip)
	}
}

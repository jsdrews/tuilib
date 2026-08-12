package output

import "testing"

// event is a head plus n body lines, the shape everything in the buffer
// actually takes.
func event(b *Buffer, source, text string, lvl Level, body int) {
	b.Append(Record{Level: lvl, Source: source, Text: text, Head: true})
	for i := 0; i < body; i++ {
		b.Append(Record{Level: LevelInfo, Source: source, Text: "body"})
	}
}

// Trimming must never leave a body line whose head is gone. The per-line
// prefix is what makes a line self-describing under filter mode, and a
// stranded body line has a source but no command.
func TestTrimCutsOnEventBoundaries(t *testing.T) {
	b := NewBuffer(30)
	for i := 0; i < 40; i++ {
		event(b, "Deploy", "run", LevelInfo, 4)
	}

	if got := b.Len(); got > 30 {
		t.Fatalf("buffer over cap: %d > 30", got)
	}
	recs := b.Records()
	if len(recs) == 0 {
		t.Fatal("buffer empty after trim")
	}
	if !recs[0].Head {
		t.Errorf("first surviving record is a body line — trim cut mid-event")
	}
}

// Trimming to a low-water mark rather than to exactly max is what keeps the
// screen's mirror cheap: an epoch change forces a full rebuild, so a stream
// sitting at the cap must not bump it on every single line.
func TestTrimIsAmortized(t *testing.T) {
	b := NewBuffer(100)
	for i := 0; i < 100; i++ {
		b.Append(Record{Text: "x", Head: true})
	}

	before := b.Epoch()
	bumps := 0
	for i := 0; i < 30; i++ {
		b.Append(Record{Text: "x", Head: true})
		if b.Epoch() != before {
			bumps++
			before = b.Epoch()
		}
	}
	if bumps > 4 {
		t.Errorf("epoch moved %d times over 30 appends at cap — trim is not amortized", bumps)
	}
	if bumps == 0 {
		t.Error("epoch never moved; the buffer is not trimming at all")
	}
}

// A single build can be longer than the whole ring. Dropping the event
// wholesale would wipe a run that is still streaming, so its head is kept
// and the oldest body lines go instead.
func TestOversizedEventKeepsItsHead(t *testing.T) {
	b := NewBuffer(20)
	b.Append(Record{Source: "go", Text: "$ go build ./...", Head: true})
	for i := 0; i < 200; i++ {
		b.Append(Record{Source: "go", Text: "compiling"})
	}

	recs := b.Records()
	if len(recs) > 20 {
		t.Fatalf("buffer over cap: %d", len(recs))
	}
	if !recs[0].Head || recs[0].Text != "$ go build ./..." {
		t.Errorf("head not retained; first record = %+v", recs[0])
	}
}

func TestUnreadCountsEventsNotLines(t *testing.T) {
	b := NewBuffer(0)
	event(b, "Deploy", "one", LevelInfo, 50)
	event(b, "Deploy", "two", LevelInfo, 50)

	if got := b.Unread(); got != 2 {
		t.Errorf("Unread() = %d, want 2 (a 50-line dump is one event)", got)
	}
	b.MarkRead()
	if got := b.Unread(); got != 0 {
		t.Errorf("Unread() after MarkRead = %d, want 0", got)
	}
}

// Once the ring wraps, the badge must stop advertising events that are no
// longer in it — otherwise it counts up forever and means nothing.
func TestUnreadNeverExceedsWhatSurvives(t *testing.T) {
	b := NewBuffer(20)
	for i := 0; i < 100; i++ {
		event(b, "Deploy", "run", LevelInfo, 2)
	}

	heads := 0
	for _, r := range b.Records() {
		if r.Head {
			heads++
		}
	}
	if got := b.Unread(); got > heads {
		t.Errorf("Unread() = %d but only %d events survive in the ring", got, heads)
	}
}

// The tint comes from a run's exit status, which arrives as a continuation
// line rather than a head — so unread-error accounting cannot look at heads
// alone.
func TestUnreadErrorFromContinuationLine(t *testing.T) {
	b := NewBuffer(0)
	b.Append(Record{Level: LevelInfo, Source: "kubectl", Text: "$ kubectl apply", Head: true})
	b.Append(Record{Level: LevelInfo, Source: "kubectl", Text: "applying"})
	if b.UnreadError() {
		t.Fatal("tinted before anything failed")
	}

	b.Append(Record{Level: LevelError, Source: "kubectl", Text: "kubectl failed: exit status 1"})
	if !b.UnreadError() {
		t.Error("a failing completion line did not tint the badge")
	}
	b.MarkRead()
	if b.UnreadError() {
		t.Error("tint survived MarkRead")
	}
}

func TestUnreadErrorClearsWhenTheErrorIsTrimmedAway(t *testing.T) {
	b := NewBuffer(20)
	b.Append(Record{Level: LevelError, Source: "old", Text: "boom", Head: true})
	if !b.UnreadError() {
		t.Fatal("error not registered")
	}
	for i := 0; i < 200; i++ {
		event(b, "Deploy", "run", LevelInfo, 2)
	}
	if b.UnreadError() {
		t.Error("badge still tinted for an error the ring has dropped")
	}
}

func TestClearMarksEverythingRead(t *testing.T) {
	b := NewBuffer(0)
	event(b, "Deploy", "boom", LevelError, 3)
	b.Clear()

	if b.Len() != 0 {
		t.Errorf("Len() = %d after Clear", b.Len())
	}
	if b.Unread() != 0 || b.UnreadError() {
		t.Errorf("Clear left unread state behind: n=%d err=%v", b.Unread(), b.UnreadError())
	}
}

func TestRunLifecycleAndKill(t *testing.T) {
	b := NewBuffer(0)
	killed := 0
	b.StartRun(1, "kubectl", func() error { killed++; return nil })
	b.StartRun(2, "terraform", func() error { killed++; return nil })

	if got := b.InFlight(); got != 2 {
		t.Fatalf("InFlight() = %d, want 2", got)
	}
	runs := b.Runs()
	if len(runs) != 2 || runs[0].Label != "kubectl" || runs[1].Label != "terraform" {
		t.Errorf("Runs() not in start order: %+v", runs)
	}

	if err := b.Kill(1); err != nil {
		t.Errorf("Kill(1) = %v", err)
	}
	if killed != 1 {
		t.Errorf("kill func called %d times, want 1", killed)
	}
	if err := b.Kill(99); err != nil {
		t.Errorf("Kill of an unknown run should be a no-op, got %v", err)
	}

	b.EndRun(1)
	if got := b.InFlight(); got != 1 {
		t.Errorf("InFlight() after EndRun = %d, want 1", got)
	}
}

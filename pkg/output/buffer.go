package output

import "time"

// Run is a capture that is currently streaming into the buffer.
//
// The kill func is a closure rather than an *exec.Cmd so this package stays
// out of os/exec: the shell holds the process and hands the buffer only the
// ability to end it, which also makes the kill path testable without
// spawning anything.
type Run struct {
	ID      int64
	Label   string
	Started time.Time

	kill func() error
}

// Buffer is the app-wide ring of records plus the accounting the statusbar
// badge reads: how many events have arrived since the log was last read,
// whether any of them was an error, and what is still streaming.
//
// The zero value is not usable; construct with NewBuffer.
type Buffer struct {
	records []Record
	max     int

	// Counts are cumulative and monotonic so unread is a difference between
	// totals rather than a scan of the ring.
	//
	// The *Dropped counters are what keep that honest once the ring wraps:
	// without them the badge would go on advertising events that have
	// already been trimmed away, and a red tint would outlive the error
	// that caused it. Both are maintained inside the drop loop, which is
	// already walking exactly those records.
	heads       int64
	readHeads   int64
	headsDrop   int64
	errs        int64
	readErrs    int64
	errsDropped int64

	// epoch changes whenever records are dropped from the front. A reader
	// mirroring the buffer (the screen's logview) can append the tail while
	// the epoch holds and rebuild only when it moves.
	epoch int

	runs  map[int64]*Run
	order []int64
}

// NewBuffer returns an empty buffer capped at max records. max <= 0 applies
// DefaultMaxRecords.
func NewBuffer(max int) *Buffer {
	if max <= 0 {
		max = DefaultMaxRecords
	}
	return &Buffer{max: max, runs: map[int64]*Run{}}
}

// Append adds one record, stamping Time when the caller left it zero, and
// trims if the ring is over cap.
func (b *Buffer) Append(r Record) {
	if r.Time.IsZero() {
		r.Time = time.Now()
	}
	b.records = append(b.records, r)
	b.count(r)
	b.trim()
}

func (b *Buffer) count(r Record) {
	if r.Head {
		b.heads++
	}
	if r.Level == LevelError {
		b.errs++
	}
}

// AppendAll adds records in order. Cheaper than a loop for a burst, and it
// trims once at the end rather than per line.
func (b *Buffer) AppendAll(rs []Record) {
	for _, r := range rs {
		if r.Time.IsZero() {
			r.Time = time.Now()
		}
		b.records = append(b.records, r)
		b.count(r)
	}
	b.trim()
}

// Records returns the buffered records, oldest first. The slice aliases the
// internal ring — copy it if you intend to retain it across appends.
func (b *Buffer) Records() []Record { return b.records }

// Len is the number of buffered records.
func (b *Buffer) Len() int { return len(b.records) }

// Epoch changes whenever records are dropped from the front (trim or Clear).
// A mirror that is appending the tail should compare epochs first and rebuild
// from scratch when it has moved.
func (b *Buffer) Epoch() int { return b.epoch }

// Clear empties the ring and marks everything read — there is nothing left
// for the badge to be counting.
func (b *Buffer) Clear() {
	b.records = nil
	b.epoch++
	b.readHeads = b.heads
	b.readErrs = b.errs
}

// MarkRead resets the unread count and the error tint. The shell calls this
// when the output screen pops, rather than when it opens, so records arriving
// while the user sits on the screen don't come back unread as they leave.
func (b *Buffer) MarkRead() {
	b.readHeads = b.heads
	b.readErrs = b.errs
}

// Unread is the number of events — head records — since the last MarkRead,
// never counting events the ring has already dropped.
//
// A capture contributes one regardless of how many lines it emits: a
// 200-line dump is one failure with a lot of evidence, not 200 pieces of
// news.
func (b *Buffer) Unread() int { return int(b.heads - max(b.readHeads, b.headsDrop)) }

// UnreadError reports whether any record since the last MarkRead was an
// error and is still buffered.
//
// It considers continuation lines too, so a capture that streams clean
// output and then fails on its completion line still tints the badge.
func (b *Buffer) UnreadError() bool { return b.errs > max(b.readErrs, b.errsDropped) }

// StartRun registers a capture as in flight. kill may be nil for a run that
// cannot be signalled.
func (b *Buffer) StartRun(id int64, label string, kill func() error) {
	if id == 0 {
		return
	}
	if _, dup := b.runs[id]; dup {
		return
	}
	b.runs[id] = &Run{ID: id, Label: label, Started: time.Now(), kill: kill}
	b.order = append(b.order, id)
}

// EndRun deregisters a capture. Unknown ids are ignored.
func (b *Buffer) EndRun(id int64) {
	if _, ok := b.runs[id]; !ok {
		return
	}
	delete(b.runs, id)
	for i, v := range b.order {
		if v == id {
			b.order = append(b.order[:i], b.order[i+1:]...)
			break
		}
	}
}

// Runs returns the in-flight captures in the order they started.
func (b *Buffer) Runs() []Run {
	out := make([]Run, 0, len(b.order))
	for _, id := range b.order {
		if r, ok := b.runs[id]; ok {
			out = append(out, *r)
		}
	}
	return out
}

// InFlight is the number of captures currently streaming. The badge renders
// a static marker while this is non-zero.
func (b *Buffer) InFlight() int { return len(b.runs) }

// Kill signals the run with the given id. Returns nil for an unknown id or
// a run registered without a kill func — both mean "nothing left to stop."
func (b *Buffer) Kill(id int64) error {
	r, ok := b.runs[id]
	if !ok || r.kill == nil {
		return nil
	}
	return r.kill()
}

// trim drops whole events off the front once the ring is over cap.
//
// It cuts to a low-water mark rather than to exactly max so the cost is
// amortized: a stream at cap would otherwise force a re-sync on every single
// line, since dropping from the front invalidates the mirror.
func (b *Buffer) trim() {
	if b.max <= 0 || len(b.records) <= b.max {
		return
	}
	target := b.max - b.max/10
	if target < 1 {
		target = 1
	}

	drop := len(b.records) - target
	// Advance to the next event boundary. Cutting mid-event would leave
	// body lines with no head naming the command they came from — the one
	// thing the per-line prefix was supposed to guarantee.
	for drop < len(b.records) && !b.records[drop].Head {
		drop++
	}

	if drop >= len(b.records) {
		// A single event is larger than the entire cap (a long build). Drop
		// its oldest body lines instead of the event itself, which would
		// wipe a run that is still streaming.
		b.dropWithinEvent(target)
		return
	}

	b.dropped(b.records[:drop])
	b.records = append(b.records[:0:0], b.records[drop:]...)
	b.epoch++
}

// dropped folds a discarded span into the *Dropped counters, so unread
// accounting stops advertising records nobody can reach any more.
func (b *Buffer) dropped(rs []Record) {
	for _, r := range rs {
		if r.Head {
			b.headsDrop++
		}
		if r.Level == LevelError {
			b.errsDropped++
		}
	}
}

// dropWithinEvent trims the oldest body lines of an oversized event while
// keeping its head, so the surviving lines still say what produced them.
func (b *Buffer) dropWithinEvent(target int) {
	if len(b.records) == 0 {
		return
	}
	if !b.records[0].Head {
		cut := len(b.records) - target
		b.dropped(b.records[:cut])
		b.records = append(b.records[:0:0], b.records[cut:]...)
		b.epoch++
		return
	}
	keep := target - 1
	if keep < 0 {
		keep = 0
	}
	cut := len(b.records) - keep
	b.dropped(b.records[1:cut])
	out := make([]Record, 0, keep+1)
	out = append(out, b.records[0])
	out = append(out, b.records[cut:]...)
	b.records = out
	b.epoch++
}

// Package resume keeps a line stream resumable when its source can only be
// asked for "lines since time T", and T is coarser than the lines.
//
// Kubernetes pod logs are the case it exists for: the API server truncates
// sinceTime to the second, so a reconnect that asks for the last line's
// second repeats every line already seen in it, and one that asks for the
// next second loses whatever else arrived in it. The fix — stern's — is to
// always request timestamps, remember the last second and how many lines it
// held, resume at that second and skip that many. Tracker is that algorithm,
// with no I/O and nothing from Kubernetes: the screen builds its own request
// from Since, and feeds each line's timestamp through Drop and Observe.
//
// It tracks the oldest end too, for "load older": a source that cannot page
// backwards is asked for a longer tail, which comes back as older lines
// followed by the ones already held. OlderCut says where the older lines
// stop.
//
// It cannot detect loss. A resume that lands after the source rotated its
// file returns nothing for what went into the rotated one, and a quiet
// stream looks identical. Reconnected reports the gap so the screen can
// mark it rather than claim there was or wasn't loss.
package resume

import "time"

// DefaultGranularity is the resolution the source resumes at, when
// Options.Granularity is zero — a second, as the Kubernetes API server
// truncates sinceTime to.
const DefaultGranularity = time.Second

// Options configures a Tracker.
type Options struct {
	// Granularity is the resolution the source's "since" parameter honours.
	// Zero means DefaultGranularity.
	Granularity time.Duration
}

// Tracker remembers enough about a stream's two ends to resume it exactly.
// The zero value is not ready; use New.
type Tracker struct {
	gran time.Duration

	seen bool
	// newest is the newest line's timestamp truncated to the granularity,
	// and newestN how many lines have been observed at it.
	newest  time.Time
	newestN int
	// oldest and oldestN are the same at the other end.
	oldest  time.Time
	oldestN int

	// While resuming, lines up to the skip point are repeats.
	resuming bool
	skip     int

	lostAt time.Time
	gap    time.Duration
}

// New returns a Tracker.
func New(opts Options) Tracker {
	if opts.Granularity <= 0 {
		opts.Granularity = DefaultGranularity
	}
	return Tracker{gran: opts.Granularity}
}

// Observe records a line the view kept, at timestamp ts. Call it for every
// line, in order, after Drop has declined it.
func (t *Tracker) Observe(ts time.Time) {
	s := ts.Truncate(t.gran)
	if !t.seen {
		t.seen = true
		t.newest, t.newestN = s, 1
		t.oldest, t.oldestN = s, 1
		return
	}
	switch {
	case s.Equal(t.newest):
		t.newestN++
	case s.After(t.newest):
		t.newest, t.newestN = s, 1
	}
	// The oldest second only gains lines while the stream is still in it.
	if t.newest.Equal(t.oldest) {
		t.oldestN = t.newestN
	}
}

// Seen reports whether any line has been observed.
func (t Tracker) Seen() bool { return t.seen }

// Since reports where to resume: ask the source for lines since this time,
// and the first skip lines at it are ones already held. ok is false before
// any line was observed — start from the tail instead.
func (t Tracker) Since() (since time.Time, skip int, ok bool) {
	return t.newest, t.newestN, t.seen
}

// Lost marks the stream as ended at now — a dropped connection, or a
// follow that stopped with its container.
func (t *Tracker) Lost(now time.Time) {
	if t.lostAt.IsZero() {
		t.lostAt = now
	}
}

// Resume starts a reconnect at now: the next lines through Drop are checked
// against what is already held. It returns the resume point, as Since does.
func (t *Tracker) Resume(now time.Time) (since time.Time, skip int, ok bool) {
	t.gap = 0
	if !t.lostAt.IsZero() {
		t.gap = now.Sub(t.lostAt)
		t.lostAt = time.Time{}
	}
	t.resuming = t.seen
	t.skip = t.newestN
	return t.Since()
}

// Reconnected reports how long the stream was down before the last Resume,
// or zero when it was never lost. Mark it in the view: lines that arrived
// while it was down may be missing, and nothing says whether they are.
func (t Tracker) Reconnected() time.Duration { return t.gap }

// Drop reports whether a line at ts, arriving after Resume, repeats one
// already held. The first line that is not a repeat ends the check.
func (t *Tracker) Drop(ts time.Time) bool {
	if !t.resuming {
		return false
	}
	s := ts.Truncate(t.gran)
	switch {
	case s.Before(t.newest):
		return true
	case s.Equal(t.newest) && t.skip > 0:
		t.skip--
		return true
	}
	t.resuming = false
	return false
}

// OlderCut reports how many of stamps — the timestamps of a longer tail,
// oldest first — are older than the lines already held. The rest repeat
// the held lines and belong nowhere. Prepend that many lines, then ObserveOlder
// their stamps.
func (t Tracker) OlderCut(stamps []time.Time) int {
	if !t.seen {
		return len(stamps)
	}
	atOldest := 0
	for _, ts := range stamps {
		if ts.Truncate(t.gran).Equal(t.oldest) {
			atOldest++
		}
	}
	// The held lines at the oldest second are the last oldestN of them.
	keepAtOldest := max(0, atOldest-t.oldestN)
	n := 0
	for _, ts := range stamps {
		s := ts.Truncate(t.gran)
		switch {
		case s.Before(t.oldest):
			n++
		case s.Equal(t.oldest) && keepAtOldest > 0:
			keepAtOldest--
			n++
		default:
			return n
		}
	}
	return n
}

// ObserveOlder records lines prepended at the oldest end, oldest first.
func (t *Tracker) ObserveOlder(stamps []time.Time) {
	if len(stamps) == 0 {
		return
	}
	if !t.seen {
		for _, ts := range stamps {
			t.Observe(ts)
		}
		return
	}
	first := stamps[0].Truncate(t.gran)
	n := 0
	for _, ts := range stamps {
		if ts.Truncate(t.gran).Equal(first) {
			n++
		}
	}
	if first.Equal(t.oldest) {
		t.oldestN += n
		return
	}
	t.oldest, t.oldestN = first, n
}

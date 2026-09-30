package remoteview

// Range is the items a component holds of a remote set: a contiguous run,
// each with a key, either at logical offsets of a Seekable set (with a
// total) or as a Span of Anchored data (no offsets, more-or-not at each
// edge). It grows by merging pages, trims the end furthest from the
// viewport past a cap, and answers "is logical index i resident".
type Range[T any] struct {
	items []T
	keys  []string

	start int
	total int // -1 unknown
	span  bool

	moreOlder, moreNewer bool
}

// NewRange returns an empty Seekable range.
func NewRange[T any]() Range[T] { return Range[T]{total: -1} }

// NewSpan returns an empty span, with more beyond both edges until pages
// say otherwise.
func NewSpan[T any]() Range[T] {
	return Range[T]{total: -1, span: true, moreOlder: true, moreNewer: true}
}

// IsSpan reports whether the range is a span of Anchored data.
func (r Range[T]) IsSpan() bool { return r.span }

// Len is how many items are resident.
func (r Range[T]) Len() int { return len(r.items) }

// Count is the logical length: the total when known, else what is held
// through its end. For a span it is the resident length.
func (r Range[T]) Count() int {
	if r.span {
		return len(r.items)
	}
	if r.total >= 0 {
		return max(r.total, r.start+len(r.items))
	}
	return r.start + len(r.items)
}

// Total is the reported total, or -1.
func (r Range[T]) Total() int {
	if r.span {
		return -1
	}
	return r.total
}

// Held reports the logical start and length of what is resident.
func (r Range[T]) Held() (start, count int) { return r.start, len(r.items) }

// At returns logical item i, and false when it isn't resident.
func (r Range[T]) At(i int) (T, bool) {
	var zero T
	j := i - r.start
	if j < 0 || j >= len(r.items) {
		return zero, false
	}
	return r.items[j], true
}

// KeyAt returns logical item i's key, or "".
func (r Range[T]) KeyAt(i int) string {
	j := i - r.start
	if j < 0 || j >= len(r.keys) {
		return ""
	}
	return r.keys[j]
}

// IndexOf returns the logical index of the item keyed k, or -1.
func (r Range[T]) IndexOf(k string) int {
	for j, key := range r.keys {
		if key == k && k != "" {
			return r.start + j
		}
	}
	return -1
}

// Edges returns the keys of the oldest and newest resident items — the
// cursors an Anchored source extends from.
func (r Range[T]) Edges() (older, newer string) {
	if len(r.keys) == 0 {
		return "", ""
	}
	return r.keys[0], r.keys[len(r.keys)-1]
}

// More reports whether each edge of a span has more beyond it.
func (r Range[T]) More() (older, newer bool) { return r.moreOlder, r.moreNewer }

// SetMore sets whether each edge of a span has more beyond it.
func (r *Range[T]) SetMore(older, newer bool) { r.moreOlder, r.moreNewer = older, newer }

// Clear drops everything held.
func (r *Range[T]) Clear() {
	r.items, r.keys, r.start = nil, nil, 0
	if r.span {
		r.moreOlder, r.moreNewer = true, true
	}
}

// Replace installs items as the whole range, at offset of a set total long
// (Seekable; ignored for a span).
func (r *Range[T]) Replace(items []T, keys []string, offset, total int) {
	r.items = append([]T(nil), items...)
	r.keys = fitKeys(keys, len(items))
	if !r.span {
		r.start, r.total = max(0, offset), total
	}
}

// Merge installs a Seekable page: one that touches or overlaps what is held
// extends it (the page wins where they overlap), one that doesn't replaces
// it.
func (r *Range[T]) Merge(items []T, keys []string, offset, total int) {
	keys = fitKeys(keys, len(items))
	end := offset + len(items)
	held := r.start + len(r.items)
	if len(r.items) == 0 || len(items) == 0 || offset > held || end < r.start {
		r.Replace(items, keys, offset, total)
		return
	}
	start := min(offset, r.start)
	n := max(end, held) - start
	nextItems := make([]T, n)
	nextKeys := make([]string, n)
	copy(nextItems[r.start-start:], r.items)
	copy(nextKeys[r.start-start:], r.keys)
	copy(nextItems[offset-start:], items)
	copy(nextKeys[offset-start:], keys)
	r.items, r.keys, r.start, r.total = nextItems, nextKeys, start, total
}

// Append adds items at the newer edge of a span (or the end of a range),
// oldest first. Keys already held are skipped; an item the page places
// before a held key — a late arrival — goes before it, so the result keeps
// the page's own order without comparing cursors. It reports how many
// items were inserted before index at, so a host can keep its cursor.
func (r *Range[T]) Append(items []T, keys []string, at int) (before int) {
	return r.mergeKeyed(items, fitKeys(keys, len(items)), false, at)
}

// Prepend adds items at the older edge, oldest first, merged the same way.
func (r *Range[T]) Prepend(items []T, keys []string, at int) (before int) {
	return r.mergeKeyed(items, fitKeys(keys, len(items)), true, at)
}

func (r *Range[T]) mergeKeyed(items []T, keys []string, front bool, at int) (before int) {
	known := make(map[string]int, len(r.keys))
	for j, k := range r.keys {
		if k != "" {
			known[k] = j
		}
	}
	// Where unknown items go until a held key tells us otherwise: before
	// the first held key the page mentions, or else at the edge.
	pos := len(r.items)
	if front {
		pos = 0
	}
	for _, k := range keys {
		if j, ok := known[k]; ok {
			pos = j
			break
		}
	}
	atJ := at - r.start
	for i, k := range keys {
		if j, ok := known[k]; ok {
			pos = j + 1
			continue
		}
		r.items = insert(r.items, pos, items[i])
		r.keys = insert(r.keys, pos, k)
		for kk, jj := range known {
			if jj >= pos {
				known[kk] = jj + 1
			}
		}
		if k != "" {
			known[k] = pos
		}
		if pos <= atJ {
			before++
			atJ++
		}
		pos++
	}
	return before
}

// Trim drops items past max from the end furthest from the viewport
// [first, last] (logical indices), and reports how many went from the
// front. A trimmed span edge has more beyond it again.
func (r *Range[T]) Trim(max, first, last int) (front int) {
	over := len(r.items) - max
	if max <= 0 || over <= 0 {
		return 0
	}
	above := first - r.start
	below := r.start + len(r.items) - 1 - last
	if above >= below {
		r.items, r.keys = r.items[over:], r.keys[over:]
		if r.span {
			r.moreOlder = true
		} else {
			r.start += over
		}
		return over
	}
	r.items, r.keys = r.items[:len(r.items)-over], r.keys[:len(r.keys)-over]
	if r.span {
		r.moreNewer = true
	}
	return 0
}

func insert[E any](s []E, i int, v E) []E {
	s = append(s, v)
	copy(s[i+1:], s[i:])
	s[i] = v
	return s
}

func fitKeys(keys []string, n int) []string {
	out := make([]string, n)
	copy(out, keys)
	return out
}

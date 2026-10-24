package grouping

import (
	"reflect"
	"strings"
	"testing"
)

// lines renders a layout as "# header" and item indices, which is the whole
// of what the menus draw from it.
func lines(l Layout) string {
	var out []string
	for _, r := range l.Rows() {
		if r.Header {
			out = append(out, "# "+r.Group)
			continue
		}
		out = append(out, string(rune('0'+r.Item)))
	}
	return strings.Join(out, " ")
}

func TestOneGroupIsFlat(t *testing.T) {
	for _, groups := range [][]string{
		{"", "", ""},
		{"db", "db"},
		{"other", "", "other"},
	} {
		l := New(groups, nil)
		if l.Grouped() {
			t.Errorf("%q: grouped, want flat", groups)
		}
		if got := lines(l); got != strings.Join(digits(len(groups)), " ") {
			t.Errorf("%q: %s, want items in source order", groups, got)
		}
	}
}

func digits(n int) []string {
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, string(rune('0'+i)))
	}
	return out
}

func TestGroupsInFirstSeenOrderWithOtherLast(t *testing.T) {
	groups := []string{"net", "", "db", "net", "other", "db"}
	got := lines(New(groups, nil))
	want := "# net 0 3 # db 2 5 # other 1 4"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestHeaderTextAsWritten(t *testing.T) {
	got := lines(New([]string{"Read", "WRITE"}, nil))
	if want := "# Read 0 # WRITE 1"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestKeepHidesEmptyGroupsButStaysGrouped(t *testing.T) {
	groups := []string{"db", "db", "net"}
	l := New(groups, func(i int) bool { return i != 2 })
	if got, want := lines(l), "# db 0 1"; got != want {
		t.Errorf("got %s, want %s: a filter down to one group keeps its header", got, want)
	}
	if l.Kept(2) || l.RowOf(2) != -1 {
		t.Error("dropped item reported as kept")
	}
}

func TestStepSkipsHeadersAndRejected(t *testing.T) {
	// # a 0 1 # b 2 3
	l := New([]string{"a", "a", "b", "b"}, nil)
	not2 := func(i int) bool { return i != 2 }
	if got := l.Step(1, 1, not2); got != 3 {
		t.Errorf("down from 1 skipping 2: %d, want 3", got)
	}
	if got := l.Step(3, 1, nil); got != 3 {
		t.Errorf("down past the end: %d, want 3 (no wrap)", got)
	}
	if got := l.Step(0, 10, nil); got != 3 {
		t.Errorf("half-page past the end: %d, want 3", got)
	}
	if got := l.Step(-1, 1, nil); got != 0 {
		t.Errorf("first step from nothing: %d, want 0", got)
	}
	if got := l.Step(-1, -1, nil); got != 3 {
		t.Errorf("first step up from nothing: %d, want 3", got)
	}
}

func TestStepFollowsDisplayOrder(t *testing.T) {
	// # a 0 2 # b 1
	l := New([]string{"a", "b", "a"}, nil)
	if got := l.Step(0, 1, nil); got != 2 {
		t.Errorf("down from 0: %d, want 2 (next in display order)", got)
	}
	if got := l.Last(nil); got != 1 {
		t.Errorf("last: %d, want 1", got)
	}
}

func TestJumpGroup(t *testing.T) {
	// # a 0 1 # b 2 3 # c 4
	l := New([]string{"a", "a", "b", "b", "c"}, nil)
	cases := []struct {
		from, dir, want int
		ok              func(int) bool
	}{
		{0, 1, 2, nil},
		{1, 1, 2, nil},
		{3, 1, 4, nil},
		{4, 1, 4, nil}, // no next group
		{3, -1, 0, nil},
		{0, -1, 0, nil},
		{4, -1, 2, nil},
		{0, 1, 3, func(i int) bool { return i != 2 }},         // first accepted entry
		{0, 1, 4, func(i int) bool { return i < 2 || i > 3 }}, // a group with nothing accepted is skipped
	}
	for _, c := range cases {
		if got := l.JumpGroup(c.from, c.dir, c.ok); got != c.want {
			t.Errorf("JumpGroup(%d, %d) = %d, want %d", c.from, c.dir, got, c.want)
		}
	}
	flat := New([]string{"", ""}, nil)
	if got := flat.JumpGroup(0, 1, nil); got != 0 {
		t.Errorf("flat layout jumped to %d", got)
	}
}

func TestItemNear(t *testing.T) {
	// rows: #a 0 #b 1
	l := New([]string{"a", "b"}, nil)
	got := []int{l.ItemNear(0), l.ItemNear(1), l.ItemNear(2), l.ItemNear(3), l.ItemNear(4)}
	if want := []int{0, 0, 1, 1, -1}; !reflect.DeepEqual(got, want) {
		t.Errorf("ItemNear rows 0-4 = %v, want %v", got, want)
	}
	if !l.HeaderAbove(1) || New([]string{"", ""}, nil).HeaderAbove(1) {
		t.Error("HeaderAbove")
	}
}

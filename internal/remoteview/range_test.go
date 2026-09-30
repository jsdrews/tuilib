package remoteview

import (
	"fmt"
	"strings"
	"testing"
)

func keyed(prefix string, from, to int) ([]string, []string) {
	var items, keys []string
	for i := from; i < to; i++ {
		k := fmt.Sprintf("%s%d", prefix, i)
		items, keys = append(items, k), append(keys, k)
	}
	return items, keys
}

func joined(r Range[string]) string {
	return strings.Join(r.items, ",")
}

func TestMergeExtendsAdjacentAndReplacesFar(t *testing.T) {
	r := NewRange[string]()
	it, k := keyed("", 0, 3)
	r.Merge(it, k, 0, 10)
	it, k = keyed("", 3, 5)
	r.Merge(it, k, 3, 10)
	if s, n := r.Held(); s != 0 || n != 5 {
		t.Fatalf("Held = %d,%d want 0,5", s, n)
	}
	it, k = keyed("", 8, 10)
	r.Merge(it, k, 8, 10)
	if s, n := r.Held(); s != 8 || n != 2 {
		t.Errorf("a far page should replace: %d,%d", s, n)
	}
	if _, ok := r.At(3); ok {
		t.Error("index 3 is no longer resident")
	}
	if r.Count() != 10 {
		t.Errorf("Count = %d", r.Count())
	}
}

func TestAppendSkipsKnownAndPlacesLateItems(t *testing.T) {
	r := NewSpan[string]()
	r.Append([]string{"a", "b", "d"}, []string{"a", "b", "d"}, 0)
	// A poll with overlap: b and d are known, c arrived late between them.
	before := r.Append([]string{"b", "c", "d", "e"}, []string{"b", "c", "d", "e"}, 2)
	if got := joined(r); got != "a,b,c,d,e" {
		t.Fatalf("items = %s", got)
	}
	if before != 1 {
		t.Errorf("before = %d, want 1 — c landed above the cursor on d", before)
	}
}

func TestPrependKeepsOrder(t *testing.T) {
	r := NewSpan[string]()
	r.Append([]string{"c", "d"}, []string{"c", "d"}, 0)
	before := r.Prepend([]string{"a", "b", "c"}, []string{"a", "b", "c"}, 0)
	if got := joined(r); got != "a,b,c,d" {
		t.Fatalf("items = %s", got)
	}
	if before != 2 {
		t.Errorf("before = %d, want 2", before)
	}
	if o, n := r.Edges(); o != "a" || n != "d" {
		t.Errorf("Edges = %s,%s", o, n)
	}
}

func TestTrimDropsTheFarEnd(t *testing.T) {
	r := NewSpan[string]()
	it, k := keyed("", 0, 10)
	r.Append(it, k, 0)
	r.SetMore(false, false)
	if front := r.Trim(6, 7, 9); front != 4 {
		t.Fatalf("viewing the newest end should trim the front, got %d", front)
	}
	if o, _ := r.More(); !o {
		t.Error("a trimmed older edge has more again")
	}
	if got := joined(r); got != "4,5,6,7,8,9" {
		t.Errorf("items = %s", got)
	}
	if front := r.Trim(3, 0, 1); front != 0 || joined(r) != "4,5,6" {
		t.Errorf("viewing the oldest end should trim the back: %s", joined(r))
	}
}

func TestSeekableTrimMovesStart(t *testing.T) {
	r := NewRange[string]()
	it, k := keyed("", 100, 110)
	r.Merge(it, k, 100, 1000)
	r.Trim(5, 108, 109)
	if s, n := r.Held(); s != 105 || n != 5 {
		t.Errorf("Held = %d,%d", s, n)
	}
	if r.IndexOf("107") != 107 {
		t.Errorf("IndexOf = %d", r.IndexOf("107"))
	}
}

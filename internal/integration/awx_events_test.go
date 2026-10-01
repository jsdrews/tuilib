//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/jsdrews/tuilib/examples/patterns/eventlog"
)

// The eventlog example against demoapi's AWX-shaped endpoints, over HTTP:
// what pkg/remote's typed fetch functions look like against a real wire
// format — counter ranges, page mode under a filter, find past a counter —
// checked through the app shell.

func TestAWXEventsFinishedSearchAndFilter(t *testing.T) {
	h := bApp(t, eventlog.NewFinished)
	h.pumpFor(1500 * time.Millisecond)
	_, cur, out := bRows(h)
	if cur != 1 {
		t.Fatalf("should open at event 1, got %d:\n%s", cur, out)
	}
	h.key("/")
	for _, c := range "fatal" {
		h.key(string(c))
	}
	h.key("enter")
	var seen []int
	for i := 0; i < 5; i++ {
		h.key("n")
		h.pumpFor(1500 * time.Millisecond)
		_, c, _ := bRows(h)
		seen = append(seen, c)
	}
	want := []int{37, 74, 111, 148, 222} // 185 is a task header
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("fatal walk = %v, want %v", seen, want)
			break
		}
	}
	// Filter to failed events, then search inside the filter past the page.
	h.key("f")
	for _, c := range "failed" {
		h.key(string(c))
	}
	h.key("enter")
	h.pumpFor(1500 * time.Millisecond)
	_, _, out = bRows(h)
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "ok: [") || strings.Contains(l, "changed: [") {
			t.Errorf("filter failed shows a non-failed event: %s", l)
			break
		}
	}
}

func TestAWXEventsLiveHolesFill(t *testing.T) {
	h := bApp(t, eventlog.New)
	h.pumpFor(1500 * time.Millisecond)
	_, _, out := bRows(h)
	if !strings.Contains(out, "FOLLOWING") {
		t.Error("a running job should follow")
	}
	h.pumpFor(4 * time.Second)
	rs, _, _ := bRows(h)
	holes := 0
	for n, txt := range rs {
		if strings.TrimSpace(txt) == "·" && n < len(rs)-5 {
			holes++
		}
	}
	if holes > 0 {
		t.Errorf("%d holes stayed unfilled behind the tail", holes)
	}
}

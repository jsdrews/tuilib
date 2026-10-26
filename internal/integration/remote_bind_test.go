//go:build integration

package integration

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jsdrews/tuilib/examples/remotedata"
	"github.com/jsdrews/tuilib/examples/remotedata/anchored"
	"github.com/jsdrews/tuilib/examples/remotedata/events"
	"github.com/jsdrews/tuilib/examples/remotedata/records"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

var bEsc = regexp.MustCompile(`\x1b\[[0-9;]*m`)
var bRow = regexp.MustCompile(`│ *(\d+)(▌| )│ (.*)`)

func bRows(h *harness) (map[int]string, int, string) {
	out := bEsc.ReplaceAllString(h.render(), "")
	got, cur := map[int]string{}, -1
	for _, l := range strings.Split(out, "\n") {
		if m := bRow.FindStringSubmatch(l); m != nil {
			n, _ := strconv.Atoi(m[1])
			got[n] = m[3]
			if m[2] == "▌" {
				cur = n
			}
		}
	}
	return got, cur, out
}

// The examples bound with pkg/remote, driven through the real app shell:
// what the binding owns — paging, following, landing hits, the filter's
// round trip — checked end to end rather than through its own fakes.

func bApp(t *testing.T, mk func(theme.Theme) screen.Screen) *harness {
	th := theme.New(palette.Dark())
	return newAppHarness(t, app.New(app.Options{Root: mk(th), Palettes: []palette.Palette{th.Palette}, SkipConfig: true}))
}

func TestRemoteBindFollowsAfterG(t *testing.T) {
	h := bApp(t, events.New)
	h.pumpFor(1500 * time.Millisecond)
	for i := 0; i < 30; i++ {
		h.key("k")
	}
	h.pumpFor(3 * time.Second)
	h.key("G")
	h.pumpFor(2500 * time.Millisecond)
	rows, cur, out := bRows(h)
	// The bug this guards against was a tail that never loaded: the newest
	// rows stayed placeholders while the loading range chased the end. A
	// "·" further up is not it — the job saves some events late, and a
	// hole is a real event not saved yet, filled on a later poll.
	if strings.TrimSpace(rows[cur]) == "·" {
		t.Errorf("after G the newest event should be loaded:\n%s", out)
	}
	if !strings.Contains(out, "FOLLOWING") || cur < 50 {
		t.Errorf("G should follow the newest (cursor %d):\n%s", cur, out)
	}
}

func TestRemoteBindAnchoredReanchorsOnAHit(t *testing.T) {
	h := bApp(t, anchored.New)
	h.pumpFor(1200 * time.Millisecond)
	out := bEsc.ReplaceAllString(h.render(), "")
	if !strings.Contains(out, "FOLLOWING") || !strings.Contains(out, "request 1999") {
		t.Fatalf("should open at the newest documents:\n%s", out)
	}
	h.key("/")
	for _, c := range "request 1234 " {
		h.key(string(c))
	}
	h.key("enter")
	h.key("N")
	h.pumpFor(2500 * time.Millisecond)
	out = bEsc.ReplaceAllString(h.render(), "")
	if !strings.Contains(out, "request 1234 ") {
		t.Errorf("a find past the span should re-anchor on the hit:\n%s", out)
	}
}

func TestRemoteBindTableFilterLands(t *testing.T) {
	h := bApp(t, records.New)
	h.pumpFor(800 * time.Millisecond)
	h.key("/")
	for _, c := range "region:eu-west" {
		h.key(string(c))
	}
	h.key("enter")
	h.pumpFor(1200 * time.Millisecond)
	out := bEsc.ReplaceAllString(h.render(), "")
	if strings.Contains(out, "loading") || !strings.Contains(out, "eu-west") {
		t.Errorf("filtered page should land:\n%s", out)
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "│ us-") || strings.Contains(l, "│ ap-") {
			t.Errorf("row outside the filter: %s", l)
		}
	}
}

// The showcase holds four of these bindings behind one tab strip, and the
// strip hands every message that is not a key or a click to every tab. Three
// are eventlogs, so the viewport and filter one reports reach the others; each
// must act on its own alone. Searching the Anchored tab and then visiting the
// live Events tab is the path where they would cross.
func TestRemoteDataTabsKeepTheirOwnStreams(t *testing.T) {
	h := bApp(t, remotedata.New)
	h.pumpFor(800 * time.Millisecond)
	if out := bEsc.ReplaceAllString(h.render(), ""); !strings.Contains(out, "eu-") && !strings.Contains(out, "us-") {
		t.Fatalf("the Records tab never loaded:\n%s", out)
	}

	h.key("4")
	h.pumpFor(1200 * time.Millisecond)
	h.key("/")
	for _, c := range "request 1234 " {
		h.key(string(c))
	}
	h.key("enter")
	h.key("N")
	h.pumpFor(2500 * time.Millisecond)
	if out := bEsc.ReplaceAllString(h.render(), ""); !strings.Contains(out, "request 1234 ") {
		t.Fatalf("the Anchored tab's search did not land:\n%s", out)
	}

	h.key("2")
	h.pumpFor(2 * time.Second)
	rows, _, out := bRows(h)
	if len(rows) == 0 || strings.Contains(out, "request 1234") {
		t.Errorf("the Events tab shows another tab's data, or none of its own:\n%s", out)
	}
}

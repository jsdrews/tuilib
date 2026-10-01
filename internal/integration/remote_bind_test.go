//go:build integration

package integration

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jsdrews/tuilib/examples/patterns/anchored"
	"github.com/jsdrews/tuilib/examples/patterns/eventlog"
	"github.com/jsdrews/tuilib/examples/patterns/remote"
	"github.com/jsdrews/tuilib/pkg/app"
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
	th := theme.Dark()
	return newAppHarness(t, app.New(app.Options{Root: mk(th), Themes: []theme.Theme{th}, SkipConfig: true}))
}

func TestRemoteBindFollowsAfterG(t *testing.T) {
	h := bApp(t, eventlog.New)
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
	h := bApp(t, remote.New)
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

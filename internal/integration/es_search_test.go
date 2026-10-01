//go:build integration

package integration

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jsdrews/tuilib/examples/patterns/anchored"
)

// The anchored example against demoapi's Elasticsearch-shaped search, over
// HTTP. Following a live index for long enough that late-ingested documents
// arrive: every document between the first and last on screen must be
// there. A tail without overlap would lose the late ones, and the rows'
// own request numbers would show the gap.
func TestESSearchFollowLosesNoLateDocuments(t *testing.T) {
	h := bApp(t, anchored.New)
	h.pumpFor(1200 * time.Millisecond)
	h.pumpFor(9 * time.Second)
	if out := bEsc.ReplaceAllString(h.render(), ""); !strings.Contains(out, "FOLLOWING") {
		t.Fatalf("should be following:\n%s", out)
	}
	// A late document lands a few seconds behind the newest, so look
	// there: the screen's last rows are always newer than any lag.
	for i := 0; i < 25; i++ {
		h.key("k")
	}
	h.pumpFor(200 * time.Millisecond)
	out := bEsc.ReplaceAllString(h.render(), "")
	req := regexp.MustCompile(`request (\d+)`)
	type seen struct{ row, seq int }
	var got []seen
	row := 0
	for _, l := range strings.Split(out, "\n") {
		if !strings.Contains(l, "│ ") || !regexp.MustCompile(`\d\d:\d\d:\d\d\.\d{3}`).MatchString(l) {
			continue
		}
		if m := req.FindStringSubmatch(l); m != nil {
			n, _ := strconv.Atoi(m[1])
			got = append(got, seen{row, n})
		}
		row++
	}
	if len(got) < 5 {
		t.Fatalf("too few rows to judge:\n%s", out)
	}
	if got[len(got)-1].seq <= 20000 {
		t.Fatalf("the index should have grown past its seeded documents:\n%s", out)
	}
	for i := 1; i < len(got); i++ {
		if d := got[i].seq - got[i-1].seq; d != got[i].row-got[i-1].row {
			t.Errorf("rows %d→%d hold requests %d→%d: a document is missing\n%s",
				got[i-1].row, got[i].row, got[i-1].seq, got[i].seq, out)
			break
		}
	}
}

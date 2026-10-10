package source

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func newAnch(opts AnchoredOptions) Anchored {
	if opts.PageSize == 0 {
		opts.PageSize = 100
	}
	if opts.ViewportDelay == 0 {
		opts.ViewportDelay = -1
	}
	return NewAnchored(opts)
}

func TestAnchoredNewestLoadsOlderFromTheTail(t *testing.T) {
	m := newAnch(AnchoredOptions{})
	q := req(m.Init())
	if q == nil || q.Dir != Older || q.Cursor != "" || q.Limit != 100 || !q.FromAnchor {
		t.Fatalf("q = %+v, want older from the tail", q)
	}
	older, newer := m.More()
	if !older || newer {
		t.Errorf("More = %v,%v — at the newest end nothing is newer", older, newer)
	}
}

func TestAnchoredStartsAtOptionsAnchor(t *testing.T) {
	m := newAnch(AnchoredOptions{Anchor: Oldest()})
	q := req(m.Init())
	if q == nil || q.Dir != Newer || q.Cursor != "" || !q.FromAnchor {
		t.Fatalf("q = %+v, want newer from the oldest item", q)
	}
	m = newAnch(AnchoredOptions{Anchor: At("c-7")})
	if q := req(m.Init()); q == nil || q.Cursor != "c-7" || !q.Inclusive {
		t.Fatalf("q = %+v, want older from the anchor, inclusive", q)
	}
}

func TestAnchoredAtIsInclusiveThenExtendsNewer(t *testing.T) {
	m := newAnch(AnchoredOptions{})
	q := req(m.SetAnchor(At("hit-42")))
	if q.Dir != Older || q.Cursor != "hit-42" || !q.Inclusive {
		t.Fatalf("q = %+v, want older from the anchor, inclusive", q)
	}
	m.Viewport(95, 5) // the anchor sits near the newer edge
	_, cmd := m.Deliver(Page{Gen: q.Gen, Count: 100, More: true})
	next := req(cmd)
	if next == nil || next.Dir != Newer || next.Inclusive {
		t.Fatalf("next = %+v, want the newer side of the anchor", next)
	}
}

func TestAnchoredNoEdgeRequestsBeforeTheFirstAnswer(t *testing.T) {
	m := newAnch(AnchoredOptions{})
	m.Init()
	if req(m.Viewport(0, 0)) != nil {
		t.Error("stale items must not page")
	}
}

func TestAnchoredExtendsTheNearerEdgeAndStopsWhenClosed(t *testing.T) {
	m := newAnch(AnchoredOptions{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Count: 100, More: true})
	if got := req(m.Viewport(500, 0)); got != nil {
		t.Fatalf("far from the older edge, asked %+v", *got)
	}
	o := req(m.Viewport(20, 80))
	if o == nil || o.Dir != Older {
		t.Fatalf("near the older edge = %+v", o)
	}
	if again := req(m.Viewport(19, 81)); again != nil {
		t.Error("the same edge was asked for twice")
	}
	m.Deliver(Page{Gen: o.Gen, Count: 30, More: false})
	if got := req(m.Viewport(0, 130)); got != nil {
		t.Errorf("a closed edge was asked for: %+v", *got)
	}
}

func TestAnchoredOtherEdgeCancelsTheOutstandingOne(t *testing.T) {
	m := newAnch(AnchoredOptions{})
	q := req(m.SetAnchor(At("x")))
	m.Deliver(Page{Gen: q.Gen, Count: 100, More: true})
	o := req(m.Viewport(10, 400))
	n := req(m.Viewport(400, 10))
	if o == nil || n == nil || n.Dir != Newer {
		t.Fatalf("o=%+v n=%+v", o, n)
	}
	if o.Ctx.Err() == nil {
		t.Error("moving to the other edge should cancel the first request")
	}
}

func TestAnchoredFollowFetchesOrProbes(t *testing.T) {
	m := newAnch(AnchoredOptions{Follow: time.Millisecond})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Count: 100, More: true})
	m.Viewport(900, 0)
	p := pollOf2(&m, m.SetGrowing(true))
	if p == nil || !p.Poll || p.Probe || p.Dir != Newer {
		t.Fatalf("following poll = %+v", p)
	}
	_, cmd := m.Deliver(Page{Gen: p.Gen, Count: 3})
	m.Viewport(500, 400)
	p = pollOf2(&m, cmd)
	if p == nil || !p.Probe {
		t.Fatalf("a poll while scrolled back should be a probe: %+v", p)
	}
}

// pollOf2 fires the poll timer a command armed on an Anchored source.
func pollOf2(m *Anchored, cmd tea.Cmd) *Query {
	for _, msg := range msgs(cmd) {
		if p, ok := msg.(pollMsg); ok {
			return req(m.Update(p))
		}
	}
	return nil
}

func TestAnchoredFindAndHistory(t *testing.T) {
	m := newAnch(AnchoredOptions{})
	q := req(m.Init())
	_, cmd := m.Deliver(Page{Gen: q.Gen, Count: 100, More: true})
	if count[QueryAnsweredMsg](msgs(cmd)) != 1 {
		t.Error("the anchor's first answer should be reported")
	}
	f := req(m.Find("timeout", Older))
	if f == nil || !f.Find || f.Limit != 1 || f.Dir != Older {
		t.Fatalf("find = %+v", f)
	}
	ok, _ := m.Deliver(Page{Gen: f.Gen, Found: true, Cursor: "c-9"})
	if !ok {
		t.Fatal("find answer refused")
	}
	next := req(m.SetAnchor(At("c-9")))
	if next == nil || next.Cursor != "c-9" {
		t.Errorf("re-anchor = %+v", next)
	}
	_, cmd = m.Deliver(Page{Gen: next.Gen, Err: errors.New("x")})
	if count[QueryFailedMsg](msgs(cmd)) != 1 {
		t.Error("a failed anchor should be reported")
	}
}

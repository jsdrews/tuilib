package source

import (
	"errors"
	"testing"
)

func TestRevertRestoresTheAnsweredQueryWithoutARequest(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	deliver(&m, Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 500})
	m.Viewport(0, 9)

	q = req(m.SetQuery("", nil, "Owner", false))
	deliver(&m, Page{Gen: q.Gen, Err: errors.New("refused")})

	ok, cmd := m.Revert()
	if !ok {
		t.Fatal("Revert refused with an answered query to go back to")
	}
	if r := req(cmd); r != nil {
		t.Errorf("Revert requested %+v; the held window covers the screen", *r)
	}
	if start, count := m.Held(); start != 0 || count != 100 || m.Total() != 500 {
		t.Errorf("held (%d,%d) total %d, want the answered window back", start, count, m.Total())
	}
	// The next request is under the answered query, not the refused one.
	if r := req(m.Viewport(300, 309)); r == nil || r.Sort != "" {
		t.Errorf("scrolling after Revert asked %+v, want the unsorted query", r)
	}
}

func TestRevertNeedsAFailedQueryAndAnEarlierAnswer(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	if ok, _ := m.Revert(); ok {
		t.Error("Revert with nothing answered")
	}
	deliver(&m, Page{Gen: q.Gen, Err: errors.New("down")})
	if ok, _ := m.Revert(); ok {
		t.Error("Revert after a first load failed: there is no answer to go back to")
	}

	m = newSrc(t, Options{})
	q = req(m.Init())
	deliver(&m, Page{Gen: q.Gen, Count: 10, Total: 10})
	q = req(m.SetQuery("x", nil, "", false))
	deliver(&m, Page{Gen: q.Gen, Count: 3, Total: 3})
	if ok, _ := m.Revert(); ok {
		t.Error("Revert after the new query was answered")
	}
}

// Two queries in flight before either answers: Revert goes back to the one
// that was answered, not the one in between.
func TestRevertSkipsUnansweredQueries(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	deliver(&m, Page{Gen: q.Gen, Count: 10, Total: 10})
	m.SetQuery("", nil, "Title", false)
	q = req(m.SetQuery("", nil, "Owner", false))
	deliver(&m, Page{Gen: q.Gen, Err: errors.New("refused")})
	m.Revert()
	if r := req(m.Refresh()); r == nil || r.Sort != "" {
		t.Errorf("after Revert the query is %+v, want the answered unsorted one", r)
	}
}

func TestAnchoredRevert(t *testing.T) {
	m := NewAnchored(AnchoredOptions{PageSize: 10, ViewportDelay: -1})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Count: 10, More: true})
	q = req(m.SetQuery("", nil, "Owner", true))
	m.Deliver(Page{Gen: q.Gen, Err: errors.New("refused")})
	if ok, _ := m.Revert(); !ok {
		t.Fatal("Anchored Revert refused")
	}
	if older, _ := m.More(); !older {
		t.Error("Revert lost the span's edge state")
	}
	if r := req(m.Refresh()); r == nil || r.Sort != "" {
		t.Errorf("after Revert the query is %+v", r)
	}
}

package breadcrumb

import (
	"testing"

	"github.com/jsdrews/tuilib/pkg/geom"
)

func newCrumbs(t *testing.T, width int, crumbs ...string) Model {
	t.Helper()
	m := New(Options{Crumbs: crumbs})
	m.SetRect(geom.New(0, 0, width, 1))
	return m
}

// crumbStart returns the x of the first cell of crumb i when everything
// fits: one cell of bar padding, then each earlier crumb plus a separator.
func crumbStart(m Model, i int) int {
	x := m.rect.X + m.barStyle.GetPaddingLeft()
	for j := 0; j < i; j++ {
		x += len(m.crumbs[j]) + len(m.separator)
	}
	return x
}

func TestCrumbAtResolvesEachCrumb(t *testing.T) {
	m := newCrumbs(t, 80, "Home", "Cities", "Detail")

	for i := range m.crumbs {
		got, ok := m.CrumbAt(crumbStart(m, i), 0)
		if !ok {
			t.Errorf("crumb %d: CrumbAt reported no hit", i)
			continue
		}
		if got != i {
			t.Errorf("CrumbAt at crumb %d resolved to %d", i, got)
		}
	}
}

// The separator is dead space — clicking it should not navigate.
func TestCrumbAtSeparatorIsNotAHit(t *testing.T) {
	m := newCrumbs(t, 80, "Home", "Cities")

	x := m.rect.X + m.barStyle.GetPaddingLeft() + len("Home")
	if _, ok := m.CrumbAt(x, 0); ok {
		t.Errorf("a click on the separator resolved to a crumb")
	}
}

func TestCrumbAtPastEndIsNotAHit(t *testing.T) {
	m := newCrumbs(t, 80, "Home", "Cities")

	if _, ok := m.CrumbAt(70, 0); ok {
		t.Errorf("a click on empty bar background resolved to a crumb")
	}
}

func TestCrumbAtOffTheBarIsNotAHit(t *testing.T) {
	m := newCrumbs(t, 80, "Home", "Cities")

	if _, ok := m.CrumbAt(2, 5); ok {
		t.Errorf("a click on another row resolved to a crumb")
	}
}

func TestCrumbAtEmptyTrailIsNotAHit(t *testing.T) {
	m := newCrumbs(t, 80)

	if _, ok := m.CrumbAt(2, 0); ok {
		t.Errorf("an empty breadcrumb reported a hit")
	}
}

// When the trail is elided, the visible crumbs must still map back to their
// original indices — otherwise clicking "Detail" would navigate somewhere
// else entirely.
func TestCrumbAtMapsThroughElision(t *testing.T) {
	crumbs := []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo"}
	m := newCrumbs(t, 24, crumbs...) // too narrow for the whole trail

	// Walk every cell of the bar; any hit must name a crumb whose label is
	// actually drawn, and the last crumb must be reachable.
	sawLast := false
	for x := range 24 {
		i, ok := m.CrumbAt(x, 0)
		if !ok {
			continue
		}
		if i < 0 || i >= len(crumbs) {
			t.Fatalf("CrumbAt(%d) resolved to out-of-range index %d", x, i)
		}
		if i == len(crumbs)-1 {
			sawLast = true
		}
	}
	if !sawLast {
		t.Errorf("the current crumb was not reachable in an elided trail")
	}
}

// The ellipsis stands for crumbs that aren't drawn; it has no single target.
func TestCrumbAtEllipsisIsNotAHit(t *testing.T) {
	m := newCrumbs(t, 24, "Alpha", "Bravo", "Charlie", "Delta", "Echo")

	x := m.rect.X + m.barStyle.GetPaddingLeft()
	if _, ok := m.CrumbAt(x, 0); ok {
		t.Errorf("a click on the ellipsis placeholder resolved to a crumb")
	}
}

func TestStaleRectDeclinesCrumbClicks(t *testing.T) {
	m := newCrumbs(t, 80, "Home", "Cities")
	geom.NextGen()

	if _, ok := m.CrumbAt(crumbStart(m, 0), 0); ok {
		t.Errorf("a breadcrumb with a stale rect resolved a click")
	}
}

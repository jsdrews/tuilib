package geom

import "testing"

func TestContainsBounds(t *testing.T) {
	r := Rect{X: 5, Y: 2, W: 4, H: 3}
	cases := []struct {
		x, y int
		want bool
	}{
		{5, 2, true},  // top-left corner is inside
		{8, 4, true},  // bottom-right corner is inside
		{9, 4, false}, // one past the right edge
		{8, 5, false}, // one past the bottom edge
		{4, 2, false}, // one before the left edge
		{5, 1, false}, // one before the top edge
	}
	for _, c := range cases {
		if got := r.Contains(c.x, c.y); got != c.want {
			t.Errorf("Contains(%d, %d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestHitRejectsStaleGeneration(t *testing.T) {
	r := New(0, 0, 10, 10)
	if !r.Hit(5, 5) {
		t.Fatalf("freshly stamped rect should accept a contained point")
	}
	NextGen()
	if r.Hit(5, 5) {
		t.Errorf("rect from a previous generation should decline the point")
	}
}

func TestInsetClampsAtZero(t *testing.T) {
	r := Rect{X: 0, Y: 0, W: 3, H: 3}.Inset(2)
	if r.W != 0 || r.H != 0 {
		t.Errorf("Inset(2) on a 3x3 = %dx%d, want 0x0", r.W, r.H)
	}
	if !r.Empty() {
		t.Errorf("over-inset rect should report Empty")
	}
}

func TestInsetSkipsBorder(t *testing.T) {
	r := Rect{X: 10, Y: 4, W: 20, H: 8, Gen: 7}.Inset(1)
	want := Rect{X: 11, Y: 5, W: 18, H: 6, Gen: 7}
	if r != want {
		t.Errorf("Inset(1) = %+v, want %+v", r, want)
	}
}

func TestAnchorInPlacesAtThePoint(t *testing.T) {
	outer := Rect{X: 0, Y: 0, W: 80, H: 24}
	got := AnchorIn(outer, 10, 4, 20, 6)
	if got.X != 10 || got.Y != 4 || got.W != 20 || got.H != 6 {
		t.Errorf("AnchorIn = %+v, want a 20x6 box at (10,4)", got)
	}
}

// A menu opened near the bottom-right must flip back inside rather than
// hanging off the edge.
func TestAnchorInPushesBackInsideTheFarEdge(t *testing.T) {
	outer := Rect{X: 0, Y: 0, W: 80, H: 24}
	got := AnchorIn(outer, 78, 23, 20, 6)
	if got.X+got.W > 80 || got.Y+got.H > 24 {
		t.Errorf("AnchorIn = %+v, which overflows an 80x24 outer", got)
	}
	if got.X != 60 || got.Y != 18 {
		t.Errorf("AnchorIn = %+v, want (60,18)", got)
	}
}

func TestAnchorInRespectsANonZeroOrigin(t *testing.T) {
	outer := Rect{X: 5, Y: 3, W: 40, H: 10}
	if got := AnchorIn(outer, 0, 0, 8, 4); got.X != 5 || got.Y != 3 {
		t.Errorf("AnchorIn = %+v, want clamping to the outer origin (5,3)", got)
	}
	if got := AnchorIn(outer, 100, 100, 8, 4); got.X != 37 || got.Y != 9 {
		t.Errorf("AnchorIn = %+v, want (37,9)", got)
	}
}

// A child bigger than its bounds clamps to the origin, so it is clipped from
// the far edge rather than sliding out of view at the near one.
func TestAnchorInClampsAnOversizeChildToTheOrigin(t *testing.T) {
	outer := Rect{X: 2, Y: 2, W: 10, H: 5}
	got := AnchorIn(outer, 6, 4, 40, 20)
	if got.X != 2 || got.Y != 2 {
		t.Errorf("AnchorIn = %+v, want the outer origin (2,2)", got)
	}
}

func TestAnchorInCarriesTheGeneration(t *testing.T) {
	outer := Rect{X: 0, Y: 0, W: 80, H: 24, Gen: 99}
	if got := AnchorIn(outer, 3, 3, 10, 4); got.Gen != 99 {
		t.Errorf("Gen = %d, want 99 — a placed rect must stay hit-testable", got.Gen)
	}
}

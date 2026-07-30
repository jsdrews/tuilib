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

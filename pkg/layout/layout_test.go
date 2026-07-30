package layout

import (
	"strings"
	"testing"

	"github.com/jsdrews/tuilib/pkg/geom"
)

// recorder is a Node that remembers the rect it was rendered into, so tests
// can assert on what the engine handed down rather than on pixels.
type recorder struct {
	got  geom.Rect
	body string
}

func (rec *recorder) node() Node {
	return RenderFunc(func(r geom.Rect) string {
		rec.got = r
		return rec.body
	})
}

func assertRect(t *testing.T, label string, got, want geom.Rect) {
	t.Helper()
	if got.X != want.X || got.Y != want.Y || got.W != want.W || got.H != want.H {
		t.Errorf("%s rect = (x%d y%d %dx%d), want (x%d y%d %dx%d)",
			label, got.X, got.Y, got.W, got.H, want.X, want.Y, want.W, want.H)
	}
}

func TestVStackPropagatesRowOffsets(t *testing.T) {
	top, mid, bot := &recorder{}, &recorder{}, &recorder{}
	VStack(
		Fixed(1, top.node()),
		Flex(1, mid.node()),
		Fixed(1, bot.node()),
	).Render(geom.Rect{X: 0, Y: 0, W: 40, H: 10})

	assertRect(t, "top", top.got, geom.Rect{X: 0, Y: 0, W: 40, H: 1})
	assertRect(t, "mid", mid.got, geom.Rect{X: 0, Y: 1, W: 40, H: 8})
	assertRect(t, "bot", bot.got, geom.Rect{X: 0, Y: 9, W: 40, H: 1})
}

func TestHStackPropagatesColumnOffsets(t *testing.T) {
	side, body := &recorder{}, &recorder{}
	HStack(
		Fixed(24, side.node()),
		Flex(1, body.node()),
	).Render(geom.Rect{X: 0, Y: 0, W: 80, H: 20})

	assertRect(t, "side", side.got, geom.Rect{X: 0, Y: 0, W: 24, H: 20})
	assertRect(t, "body", body.got, geom.Rect{X: 24, Y: 0, W: 56, H: 20})
}

// Nested stacks must accumulate, not reset — a component deep in the tree
// needs absolute terminal coordinates to hit-test a mouse event.
func TestNestedStacksAccumulateOrigin(t *testing.T) {
	deep := &recorder{}
	VStack(
		Fixed(1, RenderFunc(func(geom.Rect) string { return "" })),
		Flex(1, HStack(
			Fixed(10, RenderFunc(func(geom.Rect) string { return "" })),
			Flex(1, deep.node()),
		)),
	).Render(geom.Rect{X: 0, Y: 0, W: 50, H: 10})

	assertRect(t, "deep", deep.got, geom.Rect{X: 10, Y: 1, W: 40, H: 9})
}

func TestRenderGenerationPropagatesToChildren(t *testing.T) {
	child := &recorder{}
	root := geom.New(0, 0, 20, 5)
	VStack(Flex(1, child.node())).Render(root)

	if child.got.Gen != root.Gen {
		t.Errorf("child generation = %d, want %d (inherited from root)", child.got.Gen, root.Gen)
	}
}

// ZStack draws both layers over the same area, so both must be told so.
func TestZStackGivesBothLayersTheSameRect(t *testing.T) {
	base, overlay := &recorder{}, &recorder{}
	r := geom.Rect{X: 3, Y: 2, W: 30, H: 8}
	ZStack(base.node(), overlay.node()).Render(r)

	assertRect(t, "base", base.got, r)
	assertRect(t, "overlay", overlay.got, r)
}

// The rect Center hands its child must match where lipgloss.Place actually
// draws it. Asserting against the rendered output rather than against a
// duplicate of the formula is the point: if lipgloss changes how it splits
// an odd gap, this fails instead of silently mis-routing every modal click.
func TestCenterRectMatchesRenderedPosition(t *testing.T) {
	for _, tc := range []struct {
		name               string
		outerW, outerH     int
		naturalW, naturalH int
	}{
		{"even gaps", 40, 10, 20, 4},
		{"odd horizontal gap", 41, 10, 20, 4},
		{"odd vertical gap", 40, 11, 20, 4},
		{"both odd", 41, 11, 20, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			child := &recorder{
				body: strings.TrimRight(strings.Repeat(strings.Repeat("X", tc.naturalW)+"\n", tc.naturalH), "\n"),
			}
			out := Center(tc.naturalW, tc.naturalH, child.node()).
				Render(geom.Rect{X: 0, Y: 0, W: tc.outerW, H: tc.outerH})

			gotY, gotX := -1, -1
			for y, line := range strings.Split(out, "\n") {
				if x := strings.IndexByte(line, 'X'); x >= 0 {
					gotY, gotX = y, x
					break
				}
			}
			if gotY < 0 {
				t.Fatalf("child content not found in placed output")
			}
			if child.got.X != gotX || child.got.Y != gotY {
				t.Errorf("Center handed child (x%d y%d) but drew it at (x%d y%d)",
					child.got.X, child.got.Y, gotX, gotY)
			}
		})
	}
}

// A modal nested inside a stack must land at its absolute position, not at
// an offset relative to the pane it overlays.
func TestCenterInsideStackIsAbsolute(t *testing.T) {
	modal := &recorder{body: "MM\nMM"}
	VStack(
		Fixed(2, RenderFunc(func(geom.Rect) string { return "\n" })),
		Flex(1, Center(2, 2, modal.node())),
	).Render(geom.Rect{X: 0, Y: 0, W: 10, H: 8})

	// Body rect is (x0 y2 10x6); a 2x2 child centers to x=(10-2)/2=4, y=2+(6-2)/2=4.
	assertRect(t, "modal", modal.got, geom.Rect{X: 4, Y: 4, W: 2, H: 2})
}

func TestDistributeAllFixed(t *testing.T) {
	got := distribute([]Item{Fixed(3, nil), Fixed(5, nil)}, 20)
	want := []int{3, 5}
	assertSlice(t, got, want)
}

func TestDistributeAllFlexEvenSplit(t *testing.T) {
	got := distribute([]Item{Flex(1, nil), Flex(1, nil), Flex(1, nil)}, 10)
	want := []int{3, 3, 4} // remainder goes to the last flex child
	assertSlice(t, got, want)
}

func TestDistributeWeightedFlex(t *testing.T) {
	got := distribute([]Item{Flex(2, nil), Flex(1, nil)}, 12)
	want := []int{8, 4}
	assertSlice(t, got, want)
}

func TestDistributeFixedPlusFlex(t *testing.T) {
	got := distribute([]Item{Fixed(1, nil), Flex(1, nil), Fixed(1, nil)}, 10)
	want := []int{1, 8, 1}
	assertSlice(t, got, want)
}

func TestDistributeOversubscribed(t *testing.T) {
	got := distribute([]Item{Fixed(8, nil), Fixed(8, nil), Flex(1, nil)}, 10)
	// Fixed items exceed the total; remaining is clamped to 0 and the flex
	// child gets 0.
	want := []int{8, 8, 0}
	assertSlice(t, got, want)
}

func TestDistributeRemainderToLastFlex(t *testing.T) {
	got := distribute([]Item{Flex(1, nil), Flex(1, nil)}, 11)
	// 11 / 2 = 5 each; remainder 1 goes to the last flex child.
	want := []int{5, 6}
	assertSlice(t, got, want)
}

func assertSlice(t *testing.T, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d (got=%v want=%v)", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("[%d]=%d want=%d (got=%v want=%v)", i, got[i], want[i], got, want)
		}
	}
}

package layout

import "testing"

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

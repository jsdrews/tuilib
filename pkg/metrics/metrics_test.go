package metrics

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestBadgeEmpty(t *testing.T) {
	if got := Badge(0, 0, 0); got != "" {
		t.Errorf("Badge(0,0,0) = %q, want empty", got)
	}
}

func TestBadgeBuckets(t *testing.T) {
	got := ansi.Strip(Badge(12, 3, 1))
	want := "12✓ 3⚠ 1✗"
	if got != want {
		t.Errorf("Badge(12,3,1) stripped = %q, want %q", got, want)
	}
}

func TestBadgeOmitsZero(t *testing.T) {
	got := ansi.Strip(Badge(5, 0, 2))
	want := "5✓ 2✗"
	if got != want {
		t.Errorf("Badge(5,0,2) stripped = %q, want %q", got, want)
	}
}

func TestRatioFull(t *testing.T) {
	got := Ratio(10, 10)
	if !strings.Contains(got, "\x1b[32m") {
		t.Errorf("Ratio(10,10) should be green; got %q", got)
	}
	if stripped := ansi.Strip(got); stripped != "10/10" {
		t.Errorf("Ratio(10,10) stripped = %q, want 10/10", stripped)
	}
}

func TestRatioZero(t *testing.T) {
	got := Ratio(0, 10)
	if !strings.Contains(got, "\x1b[31m") {
		t.Errorf("Ratio(0,10) should be red; got %q", got)
	}
}

func TestRatioPartial(t *testing.T) {
	got := Ratio(5, 10)
	if !strings.Contains(got, "\x1b[33m") {
		t.Errorf("Ratio(5,10) should be yellow; got %q", got)
	}
}

func TestRatioOverflow(t *testing.T) {
	// done > total should still color green (treat as "full enough").
	got := Ratio(12, 10)
	if !strings.Contains(got, "\x1b[32m") {
		t.Errorf("Ratio(12,10) should be green; got %q", got)
	}
}

func TestBarWidthExact(t *testing.T) {
	got := Bar(0.5, 1.0, 10)
	if w := ansi.StringWidth(got); w != 10 {
		t.Errorf("Bar visible width = %d, want 10", w)
	}
}

func TestBarFull(t *testing.T) {
	got := Bar(1.0, 1.0, 8)
	stripped := ansi.Strip(got)
	if stripped != strings.Repeat(barFilled, 8) {
		t.Errorf("Bar(1,1,8) stripped = %q, want all filled", stripped)
	}
}

func TestBarEmpty(t *testing.T) {
	got := Bar(0, 1.0, 8)
	stripped := ansi.Strip(got)
	if stripped != strings.Repeat(barEmpty, 8) {
		t.Errorf("Bar(0,1,8) stripped = %q, want all empty", stripped)
	}
}

func TestBarZeroMax(t *testing.T) {
	got := Bar(5, 0, 6)
	if w := ansi.StringWidth(got); w != 6 {
		t.Errorf("Bar with max=0 width = %d, want 6", w)
	}
}

func TestBarZeroWidth(t *testing.T) {
	if got := Bar(0.5, 1.0, 0); got != "" {
		t.Errorf("Bar(width=0) = %q, want empty", got)
	}
}

func TestSparkEmpty(t *testing.T) {
	if got := Spark(nil, 8); got != "" {
		t.Errorf("Spark(nil) = %q, want empty", got)
	}
	if got := Spark([]float64{}, 8); got != "" {
		t.Errorf("Spark([]) = %q, want empty", got)
	}
}

func TestSparkWidthExact(t *testing.T) {
	got := Spark([]float64{1, 2, 3, 4, 5, 6, 7, 8}, 8)
	if w := ansi.StringWidth(got); w != 8 {
		t.Errorf("Spark width = %d, want 8", w)
	}
}

func TestSparkResampleDown(t *testing.T) {
	// 16 values, render to 4 — should bucket-average.
	got := Spark([]float64{1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4}, 4)
	if w := ansi.StringWidth(got); w != 4 {
		t.Errorf("Spark resample-down width = %d, want 4", w)
	}
	stripped := []rune(ansi.Strip(got))
	if len(stripped) != 4 {
		t.Fatalf("expected 4 glyphs, got %d in %q", len(stripped), string(stripped))
	}
	// Sequence is monotonically increasing → first glyph low, last glyph high.
	if stripped[0] != sparkGlyphs[0] {
		t.Errorf("first bucket should map to lowest glyph, got %q", string(stripped[0]))
	}
	if stripped[3] != sparkGlyphs[len(sparkGlyphs)-1] {
		t.Errorf("last bucket should map to highest glyph, got %q", string(stripped[3]))
	}
}

func TestSparkResampleUp(t *testing.T) {
	// 3 values, render to 8 — should repeat samples.
	got := Spark([]float64{1, 5, 9}, 8)
	if w := ansi.StringWidth(got); w != 8 {
		t.Errorf("Spark resample-up width = %d, want 8", w)
	}
}

func TestSparkFlat(t *testing.T) {
	// All values equal → flat sparkline (lowest glyph for everything).
	got := Spark([]float64{4, 4, 4, 4}, 4)
	stripped := ansi.Strip(got)
	want := strings.Repeat(string(sparkGlyphs[0]), 4)
	if stripped != want {
		t.Errorf("flat Spark = %q, want %q", stripped, want)
	}
}

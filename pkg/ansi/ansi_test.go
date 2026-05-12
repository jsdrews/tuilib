package ansi

import (
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestHyperlinkVisibleWidth(t *testing.T) {
	got := Hyperlink("https://example.com/long/path", "click")
	if w := xansi.StringWidth(got); w != 5 {
		t.Errorf("visible width = %d, want 5", w)
	}
}

func TestHyperlinkSurvivesCut(t *testing.T) {
	url := "https://example.com/very/long/path/that/would/be/truncated"
	full := Hyperlink(url, "Click here for example.com")
	cut := xansi.Cut(full, 0, 10)
	if w := xansi.StringWidth(cut); w != 10 {
		t.Errorf("cut visible width = %d, want 10", w)
	}
	gotURL, _, ok := ExtractHyperlink(cut)
	if !ok {
		t.Fatalf("ExtractHyperlink failed on cut string %q", cut)
	}
	if gotURL != url {
		t.Errorf("URL after cut = %q, want %q", gotURL, url)
	}
}

func TestExtractHyperlinkRoundTrip(t *testing.T) {
	url, text, ok := ExtractHyperlink(Hyperlink("https://a.test/x", "label"))
	if !ok || url != "https://a.test/x" || text != "label" {
		t.Errorf("round-trip = (%q, %q, %v)", url, text, ok)
	}
}

func TestExtractHyperlinkBareStringFalse(t *testing.T) {
	if _, _, ok := ExtractHyperlink("plain text"); ok {
		t.Errorf("ExtractHyperlink should be false for bare string")
	}
	if _, _, ok := ExtractHyperlink(""); ok {
		t.Errorf("ExtractHyperlink should be false for empty string")
	}
	if _, _, ok := ExtractHyperlink(CellColor(2, "green")); ok {
		t.Errorf("ExtractHyperlink should be false for CellColor output")
	}
}

func TestExtractHyperlinkEmptyText(t *testing.T) {
	url, text, ok := ExtractHyperlink(Hyperlink("https://a.test", ""))
	if !ok || url != "https://a.test" || text != "" {
		t.Errorf("empty-text round-trip = (%q, %q, %v)", url, text, ok)
	}
}

package query

import (
	"reflect"
	"testing"
)

var cols = []string{"Name", "Region", "Population", "Status"}

func TestParseBareTerm(t *testing.T) {
	got := Parse("oslo", cols)
	if len(got) != 1 {
		t.Fatalf("Parse returned %d terms, want 1", len(got))
	}
	if got[0].Column != -1 || got[0].Title != "" {
		t.Errorf("bare term scoped to column %d (%q)", got[0].Column, got[0].Title)
	}
	if got[0].Value != "oslo" || got[0].Raw != "oslo" {
		t.Errorf("Value=%q Raw=%q", got[0].Value, got[0].Raw)
	}
}

func TestParseLowercasesLiteralValue(t *testing.T) {
	got := Parse("OSLO", cols)
	if got[0].Value != "oslo" {
		t.Errorf("Value = %q, want lowercased", got[0].Value)
	}
	if got[0].Raw != "OSLO" {
		t.Errorf("Raw = %q, want the text as typed", got[0].Raw)
	}
}

func TestParseSplitsOnWhitespace(t *testing.T) {
	got := Parse("  oslo   region:europe\tbig ", cols)
	if len(got) != 3 {
		t.Fatalf("Parse returned %d terms, want 3", len(got))
	}
	if got[1].Column != 1 || got[1].Value != "europe" {
		t.Errorf("second term = %+v", got[1])
	}
}

func TestParseResolvesColumnByPrefix(t *testing.T) {
	got := Parse("reg:europe", cols)
	if got[0].Column != 1 {
		t.Fatalf("Column = %d, want 1", got[0].Column)
	}
	if got[0].Title != "Region" {
		t.Errorf("Title = %q, want the canonical column title", got[0].Title)
	}
	if got[0].Value != "europe" {
		t.Errorf("Value = %q", got[0].Value)
	}
	if got[0].Raw != "reg:europe" {
		t.Errorf("Raw = %q, want the whole clause as typed", got[0].Raw)
	}
}

func TestParseAmbiguousKeyFallsBackToBare(t *testing.T) {
	// "s" prefixes nothing here, but with these columns "P" is unique and
	// "R" is unique; use a genuinely ambiguous set instead.
	amb := []string{"Status", "State"}
	got := Parse("st:up", amb)
	if got[0].Column != -1 {
		t.Fatalf("Column = %d, want -1 for an ambiguous key", got[0].Column)
	}
	if got[0].Value != "st:up" {
		t.Errorf("Value = %q, want the whole clause matched literally", got[0].Value)
	}
}

func TestParseUnknownKeyFallsBackToBare(t *testing.T) {
	got := Parse("zzz:up", cols)
	if got[0].Column != -1 || got[0].Value != "zzz:up" {
		t.Errorf("unknown key not treated as a literal: %+v", got[0])
	}
}

func TestParseTrailingColonIsLiteral(t *testing.T) {
	got := Parse("region:", cols)
	if got[0].Column != -1 || got[0].Value != "region:" {
		t.Errorf("trailing colon not literal: %+v", got[0])
	}
}

func TestParseKeepsExtraColonsInValue(t *testing.T) {
	got := Parse("region:eu:west", cols)
	if got[0].Column != 1 || got[0].Value != "eu:west" {
		t.Errorf("only the first colon should split: %+v", got[0])
	}
}

func TestParseRegexTerm(t *testing.T) {
	got := Parse("~^os", cols)
	if got[0].Regex == nil {
		t.Fatal("~ prefix did not compile to a regex")
	}
	if got[0].Value != "" {
		t.Errorf("Value = %q, want empty for a regex term", got[0].Value)
	}
	if !got[0].Match("OSLO") {
		t.Error("regex should be case-insensitive")
	}
}

func TestParseScopedRegexTerm(t *testing.T) {
	got := Parse("region:~^euro", cols)
	if got[0].Column != 1 || got[0].Regex == nil {
		t.Fatalf("scoped regex not parsed: %+v", got[0])
	}
}

func TestParseInvalidRegexFallsBackToLiteral(t *testing.T) {
	got := Parse("~[", cols)
	if got[0].Regex != nil {
		t.Fatal("uncompilable pattern should not yield a regex")
	}
	if got[0].Value != "~[" {
		t.Errorf("Value = %q, want the tilde kept in the literal", got[0].Value)
	}
}

func TestParseLoneTildeIsLiteral(t *testing.T) {
	got := Parse("~", cols)
	if got[0].Regex != nil || got[0].Value != "~" {
		t.Errorf("lone tilde not literal: %+v", got[0])
	}
}

func TestParseEmptyInput(t *testing.T) {
	if got := Parse("   ", cols); len(got) != 0 {
		t.Errorf("Parse(whitespace) = %d terms, want 0", len(got))
	}
}

func TestColumnByPrefix(t *testing.T) {
	cases := []struct {
		key  string
		want int
	}{
		{"Region", 1},
		{"region", 1},
		{"REG", 1},
		{"r", 1},
		{"n", 0},
		{"p", 2},
		{"s", 3},
		{"zzz", -1},
	}
	for _, c := range cases {
		if got := ColumnByPrefix(c.key, cols); got != c.want {
			t.Errorf("ColumnByPrefix(%q) = %d, want %d", c.key, got, c.want)
		}
	}
}

func TestColumnByPrefixAmbiguous(t *testing.T) {
	if got := ColumnByPrefix("s", []string{"Status", "State"}); got != -1 {
		t.Errorf("ambiguous prefix = %d, want -1", got)
	}
}

func TestMatchStripsANSI(t *testing.T) {
	term := Parse("healthy", cols)[0]
	if !term.Match("\x1b[32mHealthy\x1b[39m") {
		t.Error("match should see through foreground escapes")
	}
}

func TestMatchRegexStripsANSI(t *testing.T) {
	term := Parse("~^healthy$", cols)[0]
	if !term.Match("\x1b[32mHealthy\x1b[39m") {
		t.Error("regex should anchor against the stripped text")
	}
}

func TestMatchAllAndsTerms(t *testing.T) {
	row := []string{"Oslo", "Europe", "700k", "Healthy"}
	if !MatchAll(row, Parse("oslo region:europe", cols)) {
		t.Error("both terms match; row should pass")
	}
	if MatchAll(row, Parse("oslo region:asia", cols)) {
		t.Error("one failing term should reject the row")
	}
}

func TestMatchAllScopedTermIgnoresOtherColumns(t *testing.T) {
	row := []string{"Europe", "Asia", "1", "ok"}
	if MatchAll(row, Parse("region:europe", cols)) {
		t.Error("scoped term matched a different column's cell")
	}
}

func TestMatchAllShortRow(t *testing.T) {
	row := []string{"Oslo"}
	if MatchAll(row, Parse("status:up", cols)) {
		t.Error("a missing cell should not match a scoped term")
	}
	if MatchAll(row, Parse("status:", cols)) {
		t.Error(`"status:" is a literal and "Oslo" does not contain it`)
	}
}

func TestMatchAllEmptyTermsMatchesEverything(t *testing.T) {
	if !MatchAll([]string{"anything"}, nil) {
		t.Error("no terms should match every row")
	}
}

func TestMatchAllBareTermScansEveryCell(t *testing.T) {
	row := []string{"Oslo", "Europe", "700k", "Healthy"}
	if !MatchAll(row, Parse("700", cols)) {
		t.Error("bare term should match against any cell")
	}
	if MatchAll(row, Parse("tokyo", cols)) {
		t.Error("bare term matched a row it shouldn't")
	}
}

func TestParseTermsAreIndependent(t *testing.T) {
	got := Parse("a b", cols)
	want := []string{"a", "b"}
	var vals []string
	for _, tm := range got {
		vals = append(vals, tm.Value)
	}
	if !reflect.DeepEqual(vals, want) {
		t.Errorf("values = %v, want %v", vals, want)
	}
}

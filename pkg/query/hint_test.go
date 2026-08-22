package query

import (
	"reflect"
	"testing"
)

var hintCols = []string{"Name", "Region", "Status"}

var hintRows = [][]string{
	{"Oslo", "Europe", "\x1b[32mHealthy\x1b[39m"},
	{"Osaka", "Asia", "Healthy"},
	{"Ostrava", "Europe", "Degraded"},
	{"Lima", "", "healthy"},
}

func TestDistinctDedupesSortsAndNormalizes(t *testing.T) {
	got := Distinct(hintRows, len(hintCols))
	if len(got) != 3 {
		t.Fatalf("Distinct returned %d columns, want 3", len(got))
	}
	if want := []string{"lima", "osaka", "oslo", "ostrava"}; !reflect.DeepEqual(got[0], want) {
		t.Errorf("names = %v, want %v", got[0], want)
	}
	if want := []string{"asia", "europe"}; !reflect.DeepEqual(got[1], want) {
		t.Errorf("regions = %v, want %v (empty cell skipped, deduped)", got[1], want)
	}
	if want := []string{"degraded", "healthy"}; !reflect.DeepEqual(got[2], want) {
		t.Errorf("statuses = %v, want %v (ANSI stripped, case folded)", got[2], want)
	}
}

func TestDistinctShortRow(t *testing.T) {
	got := Distinct([][]string{{"a"}}, 3)
	if len(got) != 3 {
		t.Fatalf("want a slot per column, got %d", len(got))
	}
	if len(got[1]) != 0 || len(got[2]) != 0 {
		t.Errorf("a short row should contribute nothing to missing columns: %v", got)
	}
}

func TestActiveTermMidTyping(t *testing.T) {
	act, ok := ActiveTerm("reg:eu", hintCols)
	if !ok {
		t.Fatal("expected an active term")
	}
	if act.Column != 1 || act.Title != "Region" {
		t.Errorf("resolved to %d/%q", act.Column, act.Title)
	}
	if act.Key != "reg" {
		t.Errorf("Key = %q, want the prefix as typed", act.Key)
	}
	if act.Value != "eu" || act.Start != 0 {
		t.Errorf("Value=%q Start=%d", act.Value, act.Start)
	}
}

func TestActiveTermUsesTrailingToken(t *testing.T) {
	act, ok := ActiveTerm("oslo status:heal", hintCols)
	if !ok {
		t.Fatal("expected an active term")
	}
	if act.Column != 2 || act.Value != "heal" {
		t.Errorf("act = %+v", act)
	}
	if act.Start != 5 {
		t.Errorf("Start = %d, want the offset of the trailing token", act.Start)
	}
}

func TestActiveTermEmptyValue(t *testing.T) {
	act, ok := ActiveTerm("region:", hintCols)
	if !ok {
		t.Fatal("a bare key: should still be an active term (hint every value)")
	}
	if act.Value != "" {
		t.Errorf("Value = %q, want empty", act.Value)
	}
}

func TestActiveTermRejects(t *testing.T) {
	cases := []struct {
		in  string
		why string
	}{
		{"", "empty input"},
		{"region:eu ", "trailing space ends the term"},
		{"region:eu\t", "trailing tab ends the term"},
		{"oslo", "no colon"},
		{":eu", "no key before the colon"},
		{"region:~eu", "regex values get no hint"},
		{"zzz:eu", "unresolvable key"},
	}
	for _, c := range cases {
		if _, ok := ActiveTerm(c.in, hintCols); ok {
			t.Errorf("ActiveTerm(%q) = ok, want false (%s)", c.in, c.why)
		}
	}
}

func TestCandidates(t *testing.T) {
	d := Distinct(hintRows, len(hintCols))
	got := Candidates(d, 0, "os")
	want := []string{"osaka", "oslo", "ostrava"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Candidates = %v, want %v", got, want)
	}
	if got := Candidates(d, 0, "OS"); !reflect.DeepEqual(got, want) {
		t.Errorf("prefix should be case-insensitive, got %v", got)
	}
	if got := Candidates(d, 0, ""); len(got) != 4 {
		t.Errorf("empty prefix should return every value, got %v", got)
	}
}

func TestCandidatesOutOfRangeColumn(t *testing.T) {
	d := Distinct(hintRows, len(hintCols))
	if got := Candidates(d, -1, "os"); got != nil {
		t.Errorf("col -1 = %v, want nil", got)
	}
	if got := Candidates(d, 99, "os"); got != nil {
		t.Errorf("col 99 = %v, want nil", got)
	}
}

func TestCompleteExtendsToCommonPrefix(t *testing.T) {
	d := Distinct(hintRows, len(hintCols))
	// osaka / oslo / ostrava share "os" — already typed, nothing to add.
	if _, ok := Complete("name:os", hintCols, d); ok {
		t.Error("common prefix adds nothing; Complete should decline")
	}
	// oslo / ostrava share "os"; narrowing to "osl" resolves to one value.
	got, ok := Complete("name:osl", hintCols, d)
	if !ok {
		t.Fatal("expected a completion")
	}
	if got != "name:oslo" {
		t.Errorf("Complete = %q, want %q", got, "name:oslo")
	}
}

func TestCompletePreservesTypedKey(t *testing.T) {
	d := Distinct(hintRows, len(hintCols))
	got, ok := Complete("NaM:osl", hintCols, d)
	if !ok {
		t.Fatal("expected a completion")
	}
	if got != "NaM:oslo" {
		t.Errorf("Complete = %q — the key must not be rewritten to the canonical title", got)
	}
}

func TestCompleteSplicesTrailingToken(t *testing.T) {
	d := Distinct(hintRows, len(hintCols))
	got, ok := Complete("europe name:osl", hintCols, d)
	if !ok {
		t.Fatal("expected a completion")
	}
	if got != "europe name:oslo" {
		t.Errorf("Complete = %q, want the earlier terms left alone", got)
	}
}

func TestCompleteDeclines(t *testing.T) {
	d := Distinct(hintRows, len(hintCols))
	cases := []struct {
		in  string
		why string
	}{
		{"oslo", "no active term"},
		{"name:zzz", "no candidates match"},
		{"name:oslo", "already complete"},
		{"name:~os", "regex terms are not completed"},
	}
	for _, c := range cases {
		got, ok := Complete(c.in, hintCols, d)
		if ok {
			t.Errorf("Complete(%q) = ok, want false (%s)", c.in, c.why)
		}
		if got != c.in {
			t.Errorf("Complete(%q) returned %q; a declined completion must return the input unchanged", c.in, got)
		}
	}
}

func TestLongestCommonPrefix(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"oslo"}, "oslo"},
		{[]string{"oslo", "osaka"}, "os"},
		{[]string{"oslo", "lima"}, ""},
		{[]string{"os", "oslo", "ostrava"}, "os"},
	}
	for _, c := range cases {
		if got := LongestCommonPrefix(c.in); got != c.want {
			t.Errorf("LongestCommonPrefix(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

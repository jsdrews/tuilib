package query

import "testing"

var bookCols = []string{"Name", "Year", "Status"}

func TestParseOperators(t *testing.T) {
	for _, tc := range []struct {
		in    string
		col   int
		op    Op
		value string
		text  string
	}{
		{"name:Tolk", 0, OpContains, "tolk", "Tolk"},
		{"name:~^Tol", 0, OpRegex, "", "^Tol"},
		{"name=Tolkien", 0, OpEq, "tolkien", "Tolkien"},
		{"status!=done", 2, OpNe, "done", "done"},
		{"year>1950", 1, OpGt, "1950", "1950"},
		{"year>=1950", 1, OpGe, "1950", "1950"},
		{"y<2000", 1, OpLt, "2000", "2000"},
		{"year<=2000", 1, OpLe, "2000", "2000"},
		{"name:a=b", 0, OpContains, "a=b", "a=b"}, // the first operator wins
	} {
		ts := Parse(tc.in, bookCols)
		if len(ts) != 1 {
			t.Fatalf("%q: %d terms", tc.in, len(ts))
		}
		got := ts[0]
		if got.Column != tc.col || got.Op != tc.op || got.Value != tc.value || got.Text != tc.text || got.Raw != tc.in {
			t.Errorf("%q: got col %d op %v value %q text %q raw %q", tc.in, got.Column, got.Op, got.Value, got.Text, got.Raw)
		}
	}
}

// Parse never fails: anything it can't read as a scoped comparison is the
// literal text, matched as a substring anywhere.
func TestParseFallsBackToABareLiteral(t *testing.T) {
	for _, in := range []string{">=5", "year>=", "nope>=5", "status!", "=x"} {
		ts := Parse(in, bookCols)
		if len(ts) != 1 || ts[0].Column != -1 || ts[0].Op != OpContains || ts[0].Value != in || ts[0].Text != in {
			t.Errorf("%q parsed to %+v, want a bare literal", in, ts)
		}
	}
}

func TestOpString(t *testing.T) {
	want := map[Op]string{OpContains: ":", OpRegex: "~", OpEq: "=", OpNe: "!=", OpGt: ">", OpGe: ">=", OpLt: "<", OpLe: "<="}
	for op, s := range want {
		if op.String() != s {
			t.Errorf("%d.String() = %q, want %q", op, op.String(), s)
		}
	}
}

func TestMatchAllAppliesOperators(t *testing.T) {
	row := []string{"Tolkien", "1954", "\x1b[32mdone\x1b[39m"}
	for _, tc := range []struct {
		filter string
		want   bool
	}{
		{"name=tolkien", true},
		{"name=tolk", false},
		{"status!=done", false},
		{"status!=open", true},
		{"year>1950", true},
		{"year>=1954", true},
		{"year>1954", false},
		{"year<2000", true},
		{"year<=1953", false},
		{"year>950", true}, // numeric, not "1954" < "950" as strings
		{"name>m", true},   // string order when either side isn't a number
		{"name<m", false},
		{"year>=1950 name:tol", true},
	} {
		if got := MatchAll(row, Parse(tc.filter, bookCols)); got != tc.want {
			t.Errorf("%q: %v, want %v", tc.filter, got, tc.want)
		}
	}
}

func TestCompletionRecognisesOperators(t *testing.T) {
	act, ok := ActiveTerm("year>=19", bookCols)
	if !ok || act.Column != 1 || act.Sep != ">=" || act.Op != OpGe || act.Value != "19" {
		t.Fatalf("ActiveTerm = %+v, %v", act, ok)
	}
	distinct := [][]string{nil, {"1954", "1955"}, {"done", "open"}}
	if got, ok := Complete("status!=o", bookCols, distinct); !ok || got != "status!=open" {
		t.Errorf("Complete = %q, %v; want the operator kept as typed", got, ok)
	}
	if got, ok := Complete("y>19", bookCols, distinct); !ok || got != "y>195" {
		t.Errorf("Complete = %q, %v", got, ok)
	}
	if _, ok := ActiveTerm("status!", bookCols); ok {
		t.Error("a half-typed != is not a term yet")
	}
}

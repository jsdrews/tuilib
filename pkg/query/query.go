// Package query parses the filter grammar shared by tuilib's filterable
// components — the "bare substring / key:value column scope / ~regex"
// syntax pkg/table exposes in its filter bar.
//
// It is a leaf: it imports nothing from tuilib, for the same reason
// pkg/geom doesn't. The component that applies a filter to rows it holds
// and a coordinator that translates the same filter into a remote request
// (query params, a WHERE clause) need the identical parse, and neither
// should have to import the other to get it.
//
// The grammar: input is split on whitespace into AND-ed Terms. A bare term
// matches any cell as a case-insensitive substring. A term shaped
// "key:value" scopes the match to the single column whose title
// case-insensitively starts with key ("region:europe"); an ambiguous or
// unresolvable key falls through as a literal bare term, which is also how
// to search for a literal colon. A value prefixed with "~" compiles as a
// case-insensitive regex ("~^new", "region:~^euro"); a compile error falls
// back to a literal substring including the tilde, so Parse never refuses
// input and never returns an error.
//
// A scoped term can compare instead of contain, by putting an operator
// where the colon goes: "name=tolkien", "status!=done", "year>1950",
// "year>=1950", "year<2000", "year<=2000". The key resolves the same way,
// and an unresolvable key or a missing value is a literal bare term, as
// with ":". Bare terms take no operator: ">=5" is the text ">=5". Term.Op
// says which comparison a term makes; its zero value is OpContains, so code
// written before operators existed reads every term as it always did.
//
// Matching is ANSI-aware: cells are stripped of escape sequences before
// comparison, so colored content matches on its visible text.
//
// Distinct / ActiveTerm / Candidates / Complete are the completion half of
// the grammar — what the filter bar needs to hint at a column's values
// while a "key:" term is being typed. They are pure functions over a
// candidate set, so a caller backed by a remote source can feed them facet
// values instead of values scraped from resident rows.
package query

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Op is the comparison a term makes against a cell.
type Op int

const (
	// OpContains is a case-insensitive substring match — "key:value", and
	// every bare term. The zero value.
	OpContains Op = iota
	// OpRegex is a case-insensitive regex match — "key:~pattern" or a bare
	// "~pattern".
	OpRegex
	// OpEq is case-insensitive equality — "key=value".
	OpEq
	// OpNe is its negation — "key!=value".
	OpNe
	// OpGt, OpGe, OpLt and OpLe order the cell against the value —
	// "key>v", "key>=v", "key<v", "key<=v" — numerically when both parse
	// as numbers, and as case-insensitive strings otherwise.
	OpGt
	OpGe
	OpLt
	OpLe
)

// ops is every operator as typed, two-character forms first so ">=" is
// never read as ">" followed by "=".
var ops = []struct {
	sep string
	op  Op
}{
	{"!=", OpNe}, {">=", OpGe}, {"<=", OpLe},
	{":", OpContains}, {"=", OpEq}, {">", OpGt}, {"<", OpLt},
}

// String is the operator as typed: ":", "~", "=", "!=", ">", ">=", "<", "<=".
func (o Op) String() string {
	switch o {
	case OpRegex:
		return "~"
	case OpEq:
		return "="
	case OpNe:
		return "!="
	case OpGt:
		return ">"
	case OpGe:
		return ">="
	case OpLt:
		return "<"
	case OpLe:
		return "<="
	}
	return ":"
}

// split finds the first operator in tok and returns the key before it, the
// operator as typed, its Op, and the value after it. ok is false when there
// is no operator with text on both sides.
func split(tok string) (key, sep string, op Op, val string, ok bool) {
	for i := 1; i < len(tok); i++ {
		for _, o := range ops {
			if strings.HasPrefix(tok[i:], o.sep) {
				if i+len(o.sep) >= len(tok) {
					return "", "", 0, "", false
				}
				return tok[:i], o.sep, o.op, tok[i+len(o.sep):], true
			}
		}
	}
	return "", "", 0, "", false
}

// Term is one AND-ed clause from a parsed filter string.
type Term struct {
	// Column is the index of the column this term is scoped to, or -1
	// when the term is bare and matches against any cell.
	Column int
	// Title is the resolved column title for a scoped term — the full
	// title, not the prefix the user typed, so a caller building a remote
	// request can use it as a field name directly. Empty for bare terms.
	Title string
	// Op is the comparison: OpContains (the zero value), OpRegex, or one of
	// the comparison operators, which only scoped terms carry.
	Op Op
	// Value is the lowercased literal the cell is compared against. Empty
	// when Regex is set.
	Value string
	// Text is the value exactly as typed after the operator — the pattern
	// without its "~" for a regex, the whole clause for a bare term. Use it
	// when building a request that is case-sensitive, where Value's
	// lowercasing would change the answer.
	Text string
	// Regex is the compiled pattern for a "~"-prefixed value, already
	// case-insensitive. Nil for literal terms.
	Regex *regexp.Regexp
	// Raw is the clause exactly as typed, including any "key:" prefix and
	// "~" marker, so a query can be round-tripped or echoed back.
	Raw string
}

// Parse splits input on whitespace into AND-ed terms, resolving "key:value"
// clauses against columns (a slice of column titles, in column order).
// It never fails: unresolvable keys and uncompilable regexes degrade to
// literal substring terms.
func Parse(input string, columns []string) []Term {
	parts := strings.Fields(input)
	out := make([]Term, 0, len(parts))
	for _, p := range parts {
		if key, _, op, val, ok := split(p); ok {
			if col := ColumnByPrefix(key, columns); col >= 0 {
				out = append(out, compile(col, columns[col], op, val, p))
				continue
			}
		}
		out = append(out, compile(-1, "", OpContains, p, p))
	}
	return out
}

// compile builds a Term from a clause. A leading "~" requests regex
// matching; on compile failure the raw value (including the tilde) is kept
// as a literal substring so the user always sees results for what they
// typed.
func compile(col int, title string, op Op, val, raw string) Term {
	t := Term{Column: col, Title: title, Op: op, Raw: raw, Text: val}
	if op == OpContains && strings.HasPrefix(val, "~") && len(val) > 1 {
		if re, err := regexp.Compile("(?i)" + val[1:]); err == nil {
			t.Op, t.Regex, t.Text = OpRegex, re, val[1:]
			return t
		}
	}
	t.Value = strings.ToLower(val)
	return t
}

// ColumnByPrefix returns the index of the unique column whose title starts
// with key (case-insensitive), or -1 when there is no match or when the
// prefix is ambiguous across several columns.
func ColumnByPrefix(key string, columns []string) int {
	key = strings.ToLower(key)
	match := -1
	for i, title := range columns {
		if strings.HasPrefix(strings.ToLower(title), key) {
			if match >= 0 {
				return -1
			}
			match = i
		}
	}
	return match
}

// Match reports whether cell satisfies the term. The cell is ANSI-stripped
// first; literal terms compare lowercased, regex terms rely on the (?i)
// flag applied at compile time, and the ordering operators compare as
// numbers when cell and value both parse as one.
func (t Term) Match(cell string) bool {
	s := ansi.Strip(cell)
	if t.Regex != nil {
		return t.Regex.MatchString(s)
	}
	s = strings.ToLower(strings.TrimSpace(s))
	switch t.Op {
	case OpEq:
		return s == t.Value
	case OpNe:
		return s != t.Value
	case OpGt, OpGe, OpLt, OpLe:
		c := compare(s, t.Value)
		switch t.Op {
		case OpGt:
			return c > 0
		case OpGe:
			return c >= 0
		case OpLt:
			return c < 0
		}
		return c <= 0
	}
	return strings.Contains(s, t.Value)
}

// compare orders a against b: numerically when both parse as numbers,
// otherwise as strings.
func compare(a, b string) int {
	x, errA := strconv.ParseFloat(a, 64)
	y, errB := strconv.ParseFloat(b, 64)
	if errA == nil && errB == nil {
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
		return 0
	}
	return strings.Compare(a, b)
}

// MatchAll reports whether every term matches cells. Scoped terms test only
// their column (a short row is treated as having an empty cell there); bare
// terms match when any cell matches. An empty term slice matches everything.
func MatchAll(cells []string, terms []Term) bool {
	for _, t := range terms {
		if t.Column >= 0 {
			cell := ""
			if t.Column < len(cells) {
				cell = cells[t.Column]
			}
			if !t.Match(cell) {
				return false
			}
			continue
		}
		hit := false
		for _, cell := range cells {
			if t.Match(cell) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

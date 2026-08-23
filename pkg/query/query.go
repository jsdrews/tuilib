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
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Term is one AND-ed clause from a parsed filter string.
type Term struct {
	// Column is the index of the column this term is scoped to, or -1
	// when the term is bare and matches against any cell.
	Column int
	// Title is the resolved column title for a scoped term — the full
	// title, not the prefix the user typed, so a caller building a remote
	// request can use it as a field name directly. Empty for bare terms.
	Title string
	// Value is the lowercased literal matched as a substring. Empty when
	// Regex is set.
	Value string
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
		if i := strings.Index(p, ":"); i > 0 && i < len(p)-1 {
			key, val := p[:i], p[i+1:]
			if col := ColumnByPrefix(key, columns); col >= 0 {
				out = append(out, compile(col, columns[col], val, p))
				continue
			}
		}
		out = append(out, compile(-1, "", p, p))
	}
	return out
}

// compile builds a Term from a clause. A leading "~" requests regex
// matching; on compile failure the raw value (including the tilde) is kept
// as a literal substring so the user always sees results for what they
// typed.
func compile(col int, title, val, raw string) Term {
	t := Term{Column: col, Title: title, Raw: raw}
	if strings.HasPrefix(val, "~") && len(val) > 1 {
		if re, err := regexp.Compile("(?i)" + val[1:]); err == nil {
			t.Regex = re
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
// flag applied at compile time.
func (t Term) Match(cell string) bool {
	s := ansi.Strip(cell)
	if t.Regex != nil {
		return t.Regex.MatchString(s)
	}
	return strings.Contains(strings.ToLower(s), t.Value)
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

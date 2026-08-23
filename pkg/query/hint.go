package query

import (
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Distinct returns, for each of the ncols columns, the sorted unique
// lowercased ANSI-stripped values appearing in that column across rows.
// Empty cells are skipped, and a row shorter than ncols contributes
// nothing to the columns it lacks.
//
// This is the candidate set behind Candidates and Complete. Rebuild it when
// the row set changes, not per keystroke — it is linear in the data. A
// caller whose rows are one page of a larger remote set should feed
// server-known facet values instead, since values scraped from a single
// page complete to answers that are wrong rather than merely incomplete.
func Distinct(rows [][]string, ncols int) [][]string {
	out := make([][]string, ncols)
	for i := range out {
		vals := make([]string, 0, len(rows))
		for _, r := range rows {
			if i < len(r) {
				vals = append(vals, r[i])
			}
		}
		out[i] = NormalizeValues(vals)
	}
	return out
}

// NormalizeValues puts a candidate set into the form Candidates and
// Complete expect: ANSI-stripped, lowercased, empties dropped, deduped,
// sorted. Distinct applies it per column; call it directly when feeding
// candidates from somewhere other than resident rows — a facet endpoint,
// an enum, a schema — so server-supplied values compare the same way
// scraped ones do.
func NormalizeValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.ToLower(ansi.Strip(v))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// Active describes the "key:value" term at the end of a filter input that
// the user is still typing — the term completion hints apply to.
type Active struct {
	// Column is the resolved column index for Key.
	Column int
	// Title is that column's full title.
	Title string
	// Key is the key exactly as typed, which may be a prefix of Title.
	// Completion writes this back verbatim so finishing a value never
	// also rewrites "reg" into "Region" under the user's cursor.
	Key string
	// Value is the partial value typed after the colon, possibly empty.
	Value string
	// Start is the byte offset of the term's first character in the input,
	// so a caller can splice a completion in without re-scanning.
	Start int
}

// ActiveTerm reports the trailing key:value term the user is mid-typing.
// ok is false when the input is empty, ends in whitespace (the term is
// finished), has no resolvable key before a colon, or carries a "~" regex
// value — enumerating a regex's matches isn't a useful hint.
func ActiveTerm(input string, columns []string) (Active, bool) {
	if input == "" || strings.HasSuffix(input, " ") || strings.HasSuffix(input, "\t") {
		return Active{}, false
	}
	start := 0
	if i := strings.LastIndexAny(input, " \t"); i >= 0 {
		start = i + 1
	}
	tok := input[start:]
	i := strings.Index(tok, ":")
	if i <= 0 {
		return Active{}, false
	}
	key, val := tok[:i], tok[i+1:]
	if strings.HasPrefix(val, "~") {
		return Active{}, false
	}
	col := ColumnByPrefix(key, columns)
	if col < 0 {
		return Active{}, false
	}
	return Active{Column: col, Title: columns[col], Key: key, Value: val, Start: start}, true
}

// Candidates returns the values in distinct[col] whose lowercased form
// starts with prefix. Returns nil when col is out of range, so a caller can
// pass an unresolved column index without guarding first.
func Candidates(distinct [][]string, col int, prefix string) []string {
	if col < 0 || col >= len(distinct) {
		return nil
	}
	prefix = strings.ToLower(prefix)
	var out []string
	for _, v := range distinct[col] {
		if strings.HasPrefix(v, prefix) {
			out = append(out, v)
		}
	}
	return out
}

// Complete extends the in-progress key:value term to the longest common
// prefix of its remaining candidates and returns the rewritten input. ok is
// false when there is no active term, when nothing matches the partial
// value, or when the common prefix adds nothing to what is already typed —
// in which case input is returned unchanged.
func Complete(input string, columns []string, distinct [][]string) (string, bool) {
	act, ok := ActiveTerm(input, columns)
	if !ok {
		return input, false
	}
	cands := Candidates(distinct, act.Column, act.Value)
	if len(cands) == 0 {
		return input, false
	}
	common := LongestCommonPrefix(cands)
	if len(common) <= len(act.Value) {
		return input, false
	}
	next := input[:act.Start] + act.Key + ":" + common
	if next == input {
		return input, false
	}
	return next, true
}

// LongestCommonPrefix returns the longest byte-wise prefix shared by every
// string in strs, or "" when strs is empty or they share nothing.
func LongestCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	prefix := strs[0]
	for _, s := range strs[1:] {
		n := 0
		for n < len(prefix) && n < len(s) && prefix[n] == s[n] {
			n++
		}
		prefix = prefix[:n]
		if prefix == "" {
			break
		}
	}
	return prefix
}

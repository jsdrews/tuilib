// Package changelog reads and rotates CHANGELOG.md, the record a consumer
// upgrades from. The release job asks it three things: the next version the
// Unreleased entries call for, the notes to publish for it, and the file with
// those entries moved under that version's heading. Check holds Unreleased to
// the shape the rest depends on, so a malformed entry fails the PR that wrote
// it rather than the release that tries to publish it.
package changelog

import (
	"fmt"
	"strconv"
	"strings"
)

// The sections an Unreleased entry may sit under, in the order they appear.
const (
	Breaking   = "Breaking"
	Deprecated = "Deprecated"
	Added      = "Added"
	Changed    = "Changed"
	Fixed      = "Fixed"
)

var known = []string{Breaking, Deprecated, Added, Changed, Fixed}

// MigrationMarker opens the before/after a Breaking entry must carry.
const MigrationMarker = "**Migration:**"

const unreleasedHeading = "## Unreleased"

// Log is CHANGELOG.md split around its Unreleased section.
type Log struct {
	head       []string
	unreleased []string
	tail       []string
}

// Parse splits a changelog. It fails only when there is no single Unreleased
// heading to split on.
func Parse(text string) (Log, error) {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	at := -1
	var f fence
	for i, l := range lines {
		if f.inside(l) || strings.TrimSpace(l) != unreleasedHeading {
			continue
		}
		if at >= 0 {
			return Log{}, fmt.Errorf("line %d: second %q heading", i+1, unreleasedHeading)
		}
		at = i
	}
	if at < 0 {
		return Log{}, fmt.Errorf("no %q heading", unreleasedHeading)
	}
	end := len(lines)
	f = fence{}
	for i := at + 1; i < len(lines); i++ {
		if !f.inside(lines[i]) && strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	return Log{head: lines[:at], unreleased: lines[at+1 : end], tail: lines[end:]}, nil
}

// Notes is the Unreleased body as it will be published, without its heading.
func (l Log) Notes() string {
	return strings.TrimSpace(strings.Join(l.unreleased, "\n"))
}

// Empty reports whether Unreleased records nothing.
func (l Log) Empty() bool { return len(l.Sections()) == 0 }

// Sections lists the Unreleased sections that hold anything, in file order.
func (l Log) Sections() []string {
	var out []string
	for _, s := range l.sections() {
		if s.filled {
			out = append(out, s.name)
		}
	}
	return out
}

type section struct {
	name   string
	line   int
	body   []string
	filled bool
}

func (l Log) sections() []section {
	var out []section
	var f fence
	for i, ln := range l.unreleased {
		if !f.inside(ln) && strings.HasPrefix(ln, "### ") {
			out = append(out, section{name: strings.TrimSpace(ln[4:]), line: i})
			continue
		}
		if len(out) == 0 {
			continue
		}
		cur := &out[len(out)-1]
		cur.body = append(cur.body, ln)
		if strings.TrimSpace(ln) != "" {
			cur.filled = true
		}
	}
	return out
}

// Check reports every way Unreleased departs from the format: text outside a
// section, a section name nobody reads or one used twice, and a Breaking
// change without a migration. Line numbers count from the top of the file.
func (l Log) Check() []error {
	var errs []error
	base := len(l.head) + 2
	for i, ln := range l.unreleased {
		if strings.HasPrefix(ln, "### ") {
			break
		}
		if strings.TrimSpace(ln) != "" {
			errs = append(errs, fmt.Errorf("line %d: text before the first ### section", base+i))
			break
		}
	}
	seen := map[string]bool{}
	for _, s := range l.sections() {
		if seen[s.name] {
			errs = append(errs, fmt.Errorf("line %d: second %q section", base+s.line, s.name))
		}
		seen[s.name] = true
		if !isKnown(s.name) {
			errs = append(errs, fmt.Errorf("line %d: unknown section %q (want one of %s)",
				base+s.line, s.name, strings.Join(known, ", ")))
		}
		if s.name == Breaking {
			errs = append(errs, checkBreaking(s, base)...)
		}
	}
	return errs
}

// checkBreaking holds each Breaking entry to a #### heading of its own with a
// migration under it, because the migration is what a consumer acts on.
func checkBreaking(s section, base int) []error {
	var errs []error
	title, at, migrated := "", 0, false
	flush := func() {
		if title != "" && !migrated {
			errs = append(errs, fmt.Errorf("line %d: breaking change %q has no %s", at, title, MigrationMarker))
		}
	}
	var f fence
	for i, ln := range s.body {
		n := base + s.line + 1 + i
		fenced := f.inside(ln)
		switch {
		case !fenced && strings.HasPrefix(ln, "#### "):
			flush()
			title, at, migrated = strings.TrimSpace(ln[5:]), n, false
		case title == "" && strings.TrimSpace(ln) != "":
			errs = append(errs, fmt.Errorf("line %d: a breaking change needs its own #### heading", n))
			return errs
		case !fenced && strings.HasPrefix(strings.TrimSpace(ln), MigrationMarker):
			migrated = true
		}
	}
	flush()
	return errs
}

// Next is the version the Unreleased entries call for after latest, or ""
// when Unreleased is empty. A Breaking entry is a major bump from v1 on and a
// minor one before it, as semver allows of v0; any other entry but Fixed is a
// minor bump; Fixed alone is a patch.
func (l Log) Next(latest string) (string, error) {
	major, minor, patch, err := parseVersion(latest)
	if err != nil {
		return "", err
	}
	secs := l.Sections()
	if len(secs) == 0 {
		return "", nil
	}
	has := func(name string) bool {
		for _, s := range secs {
			if s == name {
				return true
			}
		}
		return false
	}
	onlyFixed := true
	for _, s := range secs {
		onlyFixed = onlyFixed && s == Fixed
	}
	switch {
	case has(Breaking) && major > 0:
		major, minor, patch = major+1, 0, 0
	case onlyFixed:
		patch++
	default:
		minor, patch = minor+1, 0
	}
	return fmt.Sprintf("v%d.%d.%d", major, minor, patch), nil
}

// Release returns the changelog with Unreleased emptied and its entries filed
// under version, dated date, as the newest release.
func (l Log) Release(version, date string) string {
	var b strings.Builder
	for _, ln := range l.head {
		b.WriteString(ln + "\n")
	}
	b.WriteString(unreleasedHeading + "\n\n")
	fmt.Fprintf(&b, "## %s - %s\n\n", version, date)
	b.WriteString(l.Notes() + "\n")
	if len(l.tail) > 0 {
		b.WriteString("\n" + strings.Join(l.tail, "\n") + "\n")
	}
	return b.String()
}

func parseVersion(v string) (major, minor, patch int, err error) {
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("version %q is not vMAJOR.MINOR.PATCH", v)
	}
	var n [3]int
	for i, p := range parts {
		if n[i], err = strconv.Atoi(p); err != nil || n[i] < 0 {
			return 0, 0, 0, fmt.Errorf("version %q is not vMAJOR.MINOR.PATCH", v)
		}
	}
	return n[0], n[1], n[2], nil
}

func isKnown(name string) bool {
	for _, k := range known {
		if k == name {
			return true
		}
	}
	return false
}

// fence tracks fenced code blocks, in which a # line is code, not a heading.
// A fence closes only on a run of its own character at least as long as the
// one that opened it, so a ```go example can sit inside a ````markdown one.
type fence struct {
	char  byte
	width int
}

// inside reports whether line is part of a fenced block, its fences included.
func (f *fence) inside(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" || (t[0] != '`' && t[0] != '~') {
		return f.width > 0
	}
	n := 0
	for n < len(t) && t[n] == t[0] {
		n++
	}
	switch {
	case n < 3:
		return f.width > 0
	case f.width == 0:
		f.char, f.width = t[0], n
	case t[0] == f.char && n >= f.width && strings.TrimSpace(t[n:]) == "":
		f.width = 0
	}
	return true
}

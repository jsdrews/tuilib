package changelog

import (
	"os"
	"strings"
	"testing"
)

const head = "# Changelog\n\nHow to read this.\n\n"

const older = "## v0.1.0 - 2026-01-01\n\n### Added\n\n- First.\n"

func parse(t *testing.T, unreleased string) Log {
	t.Helper()
	l, err := Parse(head + "## Unreleased\n\n" + unreleased + "\n" + older)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestNext(t *testing.T) {
	cases := []struct {
		name, unreleased, latest, want string
	}{
		{"empty", "", "v0.4.2", ""},
		{"headings only", "### Added\n\n### Fixed\n", "v0.4.2", ""},
		{"fixed is a patch", "### Fixed\n\n- A bug.\n", "v0.4.2", "v0.4.3"},
		{"added is a minor", "### Added\n\n- A thing.\n", "v0.4.2", "v0.5.0"},
		{"fixed plus added is a minor", "### Added\n\n- A.\n\n### Fixed\n\n- B.\n", "v0.4.2", "v0.5.0"},
		{"deprecated is a minor", "### Deprecated\n\n- Old.\n", "v0.4.2", "v0.5.0"},
		{"breaking before v1 is a minor", "### Breaking\n\n#### X\n\n**Migration:** y\n", "v0.4.2", "v0.5.0"},
		{"breaking from v1 is a major", "### Breaking\n\n#### X\n\n**Migration:** y\n", "v1.4.2", "v2.0.0"},
		{"fixed twice is still a patch", "### Fixed\n\n- A.\n\n### Fixed\n\n- B.\n", "v0.4.2", "v0.4.3"},
		{"bare semver tag", "### Fixed\n\n- A bug.\n", "0.4.2", "v0.4.3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parse(t, c.unreleased).Next(c.latest)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("Next(%s) = %q, want %q", c.latest, got, c.want)
			}
		})
	}
}

func TestNextRejectsBadVersion(t *testing.T) {
	if _, err := parse(t, "### Fixed\n\n- A.\n").Next("v1.2"); err == nil {
		t.Fatal("want an error for v1.2")
	}
}

func TestCheck(t *testing.T) {
	cases := []struct {
		name, unreleased, want string
	}{
		{"empty is fine", "", ""},
		{"well formed", "### Breaking\n\n#### `a.B` renamed\n\nWhy.\n\n**Migration:**\n\n```go\nx\n```\n\n### Added\n\n- Y.\n", ""},
		{"text before a section", "- loose\n\n### Added\n\n- Y.\n", "before the first ###"},
		{"section twice", "### Added\n\n- A.\n\n### Added\n\n- B.\n", `second "Added" section`},
		{"unknown section", "### Removed\n\n- Y.\n", `unknown section "Removed"`},
		{"breaking without migration", "### Breaking\n\n#### `a.B` renamed\n\nWhy.\n", "has no **Migration:**"},
		{"second breaking without migration", "### Breaking\n\n#### one\n\n**Migration:** x\n\n#### two\n\nWhy.\n", `"two" has no`},
		{"breaking as a bullet", "### Breaking\n\n- `a.B` renamed\n", "its own #### heading"},
		{"marker inside a fence does not count", "### Breaking\n\n#### one\n\n```\n**Migration:**\n```\n", "has no"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := parse(t, c.unreleased).Check()
			var msgs []string
			for _, e := range errs {
				msgs = append(msgs, e.Error())
			}
			got := strings.Join(msgs, "; ")
			if c.want == "" && got != "" {
				t.Fatalf("Check() = %s, want no problems", got)
			}
			if !strings.Contains(got, c.want) {
				t.Fatalf("Check() = %q, want it to mention %q", got, c.want)
			}
		})
	}
}

func TestFencedHashesAreNotHeadings(t *testing.T) {
	l := parse(t, "### Breaking\n\n#### x\n\n**Migration:**\n\n```sh\n## not a release\n### not a section\n```\n")
	if errs := l.Check(); len(errs) > 0 {
		t.Fatalf("Check() = %v", errs)
	}
	if !strings.Contains(l.Notes(), "## not a release") {
		t.Fatalf("a fenced ## line ended Unreleased early:\n%s", l.Notes())
	}
}

func TestNestedFences(t *testing.T) {
	l := parse(t, "### Added\n\n- Y.\n\n````markdown\n```go\nx\n```\n## not a release\n````\n")
	if !strings.Contains(l.Notes(), "## not a release") {
		t.Fatalf("an inner ``` closed the outer ```` fence:\n%s", l.Notes())
	}
}

func TestRelease(t *testing.T) {
	got := parse(t, "### Fixed\n\n- A bug.\n").Release("v0.1.1", "2026-02-02")
	want := head + "## Unreleased\n\n## v0.1.1 - 2026-02-02\n\n### Fixed\n\n- A bug.\n\n" + older
	if got != want {
		t.Fatalf("Release():\n%s\nwant:\n%s", got, want)
	}
	again, err := Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Empty() {
		t.Fatalf("Unreleased after a release = %q, want empty", again.Notes())
	}
}

func TestParseNeedsOneUnreleased(t *testing.T) {
	if _, err := Parse(head + older); err == nil {
		t.Fatal("want an error without Unreleased")
	}
	if _, err := Parse(head + "## Unreleased\n\n## Unreleased\n"); err == nil {
		t.Fatal("want an error for two Unreleased headings")
	}
}

// The repository's own changelog is held to the format on every test run, so
// a malformed entry fails locally before CI or a release finds it.
func TestRepositoryChangelog(t *testing.T) {
	raw, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	l, err := Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range l.Check() {
		t.Error(e)
	}
}

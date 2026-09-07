package componenttest

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/alert"
	"github.com/jsdrews/tuilib/pkg/confirm"
	"github.com/jsdrews/tuilib/pkg/form"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/input"
	"github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/toggle"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// Every interactive component groups its bindings for the key overlay, and
// every one derives Help from those groups. Asserting it here rather than in
// each package is the point: the last time shared behaviour was written per
// component and tested where it was written, five of them shipped broken
// (see the "Don't test shared behaviour in one component's package"
// anti-pattern in CLAUDE.md).

type sectioned interface {
	Help() []key.Binding
	HelpSections() []help.Section
}

func components(t *testing.T) map[string]sectioned {
	t.Helper()
	th := theme.Dark()

	lo := th.List()
	lo.Filterable, lo.Markable = true, true
	l := list.New(lo)

	to := th.Table()
	to.Filterable, to.Markable = true, true
	to.Columns = []table.Column{{Title: "Name", Width: 10, Sortable: true}}
	tb := table.New(to)

	tro := th.Tree()
	tro.Searchable, tro.Markable = true, true
	tr := tree.New(tro)

	io := th.Inspector()
	io.Filterable = true
	ins := inspector.New(io)

	lvo := th.Logview()
	lvo.Searchable = true
	lv := logview.New(lvo)

	tvo := th.TextView()
	tvo.Searchable = true
	tv := textview.New(tvo)

	am := action.New(th.Actions())
	cf := confirm.New(th.Confirm())
	al := alert.New(th.Alert())
	in := input.New(th.Input())
	tg := toggle.New(th.Toggle())
	fm := form.New(th.Form())

	return map[string]sectioned{
		"list": &l, "table": &tb, "tree": &tr, "inspector": &ins,
		"logview": &lv, "textview": &tv, "input": &in, "toggle": &tg,
		"form": &fm, "confirm": &cf, "alert": &al, "action": &am,
	}
}

// Help is derived from the groups, so the flat list and the grouped one can
// never disagree — the failure mode otherwise is a binding advertised in the
// footer and missing from the overlay, or the reverse.
func TestHelpMatchesItsSections(t *testing.T) {
	for name, c := range components(t) {
		t.Run(name, func(t *testing.T) {
			flat := c.Help()
			grouped := help.Flatten(c.HelpSections())
			if len(flat) != len(grouped) {
				t.Fatalf("Help has %d bindings, sections hold %d", len(flat), len(grouped))
			}
			for i := range flat {
				if a, b := flat[i].Help(), grouped[i].Help(); a != b {
					t.Errorf("binding %d: Help says %v, sections say %v", i, a, b)
				}
			}
		})
	}
}

// A group is named by what its keys do. Components draw those names from one
// vocabulary so "Navigate" means the same thing in a list, a table and a
// tree — an ad-hoc name (the component's own, say) is how a heading ends up
// over bindings it doesn't describe.
func TestSectionTitlesComeFromTheVocabulary(t *testing.T) {
	known := map[string]bool{
		help.SectionNavigate: true, help.SectionScroll: true,
		help.SectionFilter: true, help.SectionSearch: true,
		help.SectionSelect: true, help.SectionSort: true,
		help.SectionExpand: true, help.SectionView: true,
		help.SectionEdit: true, help.SectionSubmit: true,
		help.SectionTabs: true,
	}
	for name, c := range components(t) {
		t.Run(name, func(t *testing.T) {
			secs := c.HelpSections()
			if len(secs) == 0 {
				return // input reports nothing while blurred
			}
			for _, s := range secs {
				if !known[s.Title] {
					t.Errorf("group titled %q is not in the vocabulary", s.Title)
				}
				if len(s.Bindings) == 0 {
					t.Errorf("group %q is a heading over nothing", s.Title)
				}
			}
		})
	}
}

// Marking is the case that surfaced this: its keys belong under Select, not
// under whatever the screen holding the component happens to be called.
func TestMarkingKeysAreGroupedUnderSelect(t *testing.T) {
	for name, c := range components(t) {
		if name != "list" && name != "table" && name != "tree" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			var found []string
			for _, s := range c.HelpSections() {
				for _, b := range s.Bindings {
					if strings.Contains(b.Help().Desc, "mark") {
						found = append(found, s.Title)
					}
				}
			}
			if len(found) == 0 {
				t.Fatalf("markable component advertises no marking keys")
			}
			for _, title := range found {
				if title != help.SectionSelect {
					t.Errorf("a marking key is grouped under %q, want %q", title, help.SectionSelect)
				}
			}
		})
	}
}

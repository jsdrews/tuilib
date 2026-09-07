package focus

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/help"
)

// sectioned is a Focusable that names itself and groups its bindings, the
// way every interactive component in the library does.
type sectioned struct {
	name    string
	focused bool
	verb    string
}

func (c *sectioned) Focus() tea.Cmd { c.focused = true; return nil }
func (c *sectioned) Blur()          { c.focused = false }
func (c *sectioned) Focused() bool  { return c.focused }
func (c *sectioned) Title() string  { return c.name }
func (c *sectioned) HelpSections() []help.Section {
	return help.Sections(
		help.Group(help.SectionNavigate, key.NewBinding(key.WithKeys("j"), key.WithHelp("j", "down"))),
		help.Group(help.SectionSelect, key.NewBinding(key.WithKeys("x"), key.WithHelp("x", c.verb))),
	)
}

// flat only reports a binding list, the fallback path for a component that
// hasn't been grouped.
type flat struct{ focused bool }

func (c *flat) Focus() tea.Cmd { c.focused = true; return nil }
func (c *flat) Blur()          { c.focused = false }
func (c *flat) Focused() bool  { return c.focused }
func (c *flat) Help() []key.Binding {
	return []key.Binding{key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh"))}
}

// Help answers "what can I press right now" and covers the focused pane
// alone; HelpSections answers "what can I press at all" and has to cover
// every pane, grouped by what the keys do.
func TestHelpSectionsCoversEveryPaneByFunction(t *testing.T) {
	g := NewGroup(&sectioned{name: "query", verb: "run"}, &sectioned{name: "results", verb: "mark row"})
	g.Init()

	secs := g.HelpSections()
	if len(secs) != 5 {
		t.Fatalf("got %d sections, want cycling + two groups per pane: %+v", len(secs), secs)
	}
	want := []string{"Focus", "query · Navigate", "query · Select", "results · Navigate", "results · Select"}
	for i, w := range want {
		if secs[i].Title != w {
			t.Errorf("section %d titled %q, want %q", i, secs[i].Title, w)
		}
	}
	if got := secs[4].Bindings[0].Help().Desc; got != "mark row" {
		t.Errorf("unfocused pane contributed %q, want its own binding", got)
	}
}

// With one pane the owner is not in question, so repeating it on every
// heading would be noise — the functional names stand alone.
func TestHelpSectionsDoesNotQualifyASinglePane(t *testing.T) {
	g := NewGroup(&sectioned{name: "results", verb: "mark row"})
	g.Init()

	secs := g.HelpSections()
	if len(secs) != 2 {
		t.Fatalf("got %d sections, want the pane's two groups with no cycling pair: %+v", len(secs), secs)
	}
	for i, w := range []string{help.SectionNavigate, help.SectionSelect} {
		if secs[i].Title != w {
			t.Errorf("section %d titled %q, want the bare %q", i, secs[i].Title, w)
		}
	}
}

// A component that only reports a flat Help() still contributes; with
// siblings it is qualified by its pane so the bindings are attributable.
func TestHelpSectionsCarriesUngroupedComponents(t *testing.T) {
	g := NewGroup(&sectioned{name: "results", verb: "mark row"}, &flat{})
	g.Init()

	secs := g.HelpSections()
	last := secs[len(secs)-1]
	if last.Title != "pane 2" {
		t.Errorf("untitled ungrouped pane titled %q, want a fallback name", last.Title)
	}
	if got := last.Bindings[0].Help().Desc; got != "refresh" {
		t.Errorf("ungrouped pane contributed %q, want its binding", got)
	}
}

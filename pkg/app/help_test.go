package app

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// sectionedScreen describes its bindings as groups, the way a screen holding
// a focus.Group does.
type sectionedScreen struct{ stubScreen }

func (s *sectionedScreen) HelpSections() []help.Section {
	return []help.Section{
		{Title: "Query", Bindings: []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "run query")),
		}},
		{Title: "Results", Bindings: []key.Binding{
			key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "mark row")),
		}},
	}
}

func helpApp(t *testing.T, opts Options) Model {
	t.Helper()
	if opts.Root == nil {
		opts.Root = &stubScreen{name: "root"}
	}
	opts.Themes = []theme.Theme{theme.Dark()}
	opts.SkipConfig = true
	m := New(opts)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: termW, Height: termH})
	m = tm.(Model)
	_ = m.View()
	return m
}

func TestHelpKeyOpensAndClosesOverlay(t *testing.T) {
	m := helpApp(t, Options{})

	m = send(t, m, typeKey("?"))
	if !m.helpUp {
		t.Fatalf("? did not open the key overlay")
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Global") || !strings.Contains(view, "alpha") {
		t.Errorf("overlay missing the shell's globals or the screen's bindings:\n%s", view)
	}

	// The same key closes it, which the overlay can't do for itself: the
	// key is the app's choice.
	m = send(t, m, typeKey("?"))
	if m.helpUp {
		t.Errorf("? did not close the overlay it opened")
	}
}

func TestEscClosesHelpOverlay(t *testing.T) {
	m := helpApp(t, Options{})
	m = send(t, m, typeKey("?"))
	if !m.helpUp {
		t.Fatalf("setup: overlay did not open")
	}

	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = tm.(Model)
	if cmd == nil {
		t.Fatalf("esc produced no command")
	}
	m = send(t, m, cmd())
	if m.helpUp {
		t.Errorf("esc did not close the overlay")
	}
}

// While the overlay is up it owns the keyboard, exactly as the action menu
// does — a stray key must not reach the screen or quit the program.
func TestHelpOverlaySwallowsGlobalKeys(t *testing.T) {
	m := helpApp(t, Options{})
	m = send(t, m, typeKey("?"))

	tm, cmd := m.Update(typeKey("q"))
	m = tm.(Model)
	if cmd == nil {
		t.Fatalf("q did nothing while the overlay was up")
	}
	msg := cmd()
	if _, quit := msg.(tea.QuitMsg); quit {
		t.Fatalf("q quit the program while the overlay was up")
	}
	// q is the overlay's own close key, so it dismissed rather than quit.
	if m = send(t, m, msg); m.helpUp {
		t.Errorf("q neither quit nor closed the overlay")
	}
}

// The screen's own grouping wins when it has one; a screen without falls
// back to a single section named after it.
func TestScreenSectionsAreUsed(t *testing.T) {
	m := helpApp(t, Options{Root: &sectionedScreen{stubScreen{name: "Search"}}})
	m = send(t, m, typeKey("?"))

	view := stripANSI(m.View())
	for _, want := range []string{"Query", "run query", "Results", "mark row"} {
		if !strings.Contains(view, want) {
			t.Errorf("overlay missing %q from the screen's sections:\n%s", want, view)
		}
	}
}

func TestFlatScreenGetsSectionNamedAfterIt(t *testing.T) {
	m := helpApp(t, Options{Root: &stubScreen{name: "Cities"}})
	m = send(t, m, typeKey("?"))

	if view := stripANSI(m.View()); !strings.Contains(view, "Cities") {
		t.Errorf("a screen without sections should be titled with its own name:\n%s", view)
	}
}

// The Global section is the shell's to write: the opt-in keys especially,
// since a screen author copying an existing screen has no reason to know
// they exist (rule 14).
func TestGlobalSectionCarriesShellKeys(t *testing.T) {
	m := helpApp(t, Options{
		ThemeKey:   key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		OutputKey:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "output")),
		ActionsKey: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "actions")),
		Root:       &actionScreen{stubScreen{name: "root"}},
	})
	m = send(t, m, typeKey("?"))

	view := stripANSI(m.View())
	for _, want := range []string{`\bo\s+output\b`, `\ba\s+actions\b`, `\bt\s+theme\b`, `\bq\s+quit\b`} {
		if !regexp.MustCompile(want).MatchString(view) {
			t.Errorf("Global section missing /%s/:\n%s", want, view)
		}
	}
}

// Quit only works at the root and esc only above it, so the overlay
// advertises whichever one the user can actually press.
func TestGlobalSectionTracksStackDepth(t *testing.T) {
	m := helpApp(t, Options{})
	m = send(t, m, screen.PushMsg{Screen: &stubScreen{name: "child"}})
	m = send(t, m, typeKey("?"))

	view := stripANSI(m.View())
	if !regexp.MustCompile(`\besc\s+back\b`).MatchString(view) {
		t.Errorf("no esc-back hint below the root screen:\n%s", view)
	}
	if regexp.MustCompile(`\bq\s+quit\b`).MatchString(view) {
		t.Errorf("advertised quit on a screen where q does not quit:\n%s", view)
	}
}

// actionScreen has verbs, so the actions key is live on it.
type actionScreen struct{ stubScreen }

func (s *actionScreen) Actions() action.Set {
	return action.Set{Actions: []action.Action{{Label: "restart", Do: func() tea.Cmd { return nil }}}}
}

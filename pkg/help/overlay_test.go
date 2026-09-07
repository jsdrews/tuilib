package help

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
)

func bind(k, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(k), key.WithHelp(k, desc))
}

func newOverlay(t *testing.T, secs []Section, searchable bool) Overlay {
	t.Helper()
	o := NewOverlay(OverlayOptions{Searchable: searchable})
	o.SetSections(secs)
	o.SetRect(geom.New(0, 0, 80, 30))
	return o
}

func plain(o Overlay) string { return ansi.Strip(o.View()) }

// Grouping is the reason the overlay exists: a binding is only legible when
// the list says which component it belongs to.
func TestOverlayRendersSectionHeadings(t *testing.T) {
	o := newOverlay(t, []Section{
		{Title: "Global", Bindings: []key.Binding{bind("q", "quit")}},
		{Title: "Results", Bindings: []key.Binding{bind("x", "mark row")}},
	}, false)

	view := plain(o)
	for _, want := range []string{"Global", "quit", "Results", "mark row"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

// Search is what makes a thirty-binding list usable: type the verb, get the
// keys for it, with everything else gone rather than merely dimmed.
func TestOverlaySearchReducesToMatches(t *testing.T) {
	o := newOverlay(t, []Section{
		{Title: "Global", Bindings: []key.Binding{bind("q", "quit")}},
		{Title: "Results", Bindings: []key.Binding{
			bind("x", "mark row"),
			bind("A", "mark all"),
			bind("j", "down"),
		}},
	}, true)

	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	for _, r := range "mark" {
		o, _ = o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	view := plain(o)
	for _, want := range []string{"mark row", "mark all"} {
		if !strings.Contains(view, want) {
			t.Errorf("search dropped a matching binding %q:\n%s", want, view)
		}
	}
	for _, gone := range []string{"quit", "down"} {
		if strings.Contains(view, gone) {
			t.Errorf("search kept non-matching binding %q:\n%s", gone, view)
		}
	}
	// A section left with nothing goes with it, rather than standing as an
	// empty heading.
	if strings.Contains(view, "Global") {
		t.Errorf("empty section still rendered:\n%s", view)
	}
}

// The query matches what a key does as well as what it is called: users
// look for "ctrl" about as often as for "half page".
func TestOverlaySearchMatchesKeysAndDescriptions(t *testing.T) {
	o := newOverlay(t, []Section{{Title: "Scroll", Bindings: []key.Binding{
		key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "half page down")),
		bind("j", "down"),
	}}}, true)

	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	for _, r := range "ctrl" {
		o, _ = o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	view := plain(o)
	if !strings.Contains(view, "half page down") {
		t.Errorf("query against the dispatch keys found nothing:\n%s", view)
	}
	if strings.Contains(view, "j  down") {
		t.Errorf("query matched a binding it shouldn't:\n%s", view)
	}
}

// A screen that lists q and t in its own Help() — most do — must not have
// them printed twice once a host prepends a Global group it didn't ask for.
func TestSuppressDropsWhatTheHostAlreadyClaims(t *testing.T) {
	globals := []key.Binding{bind("q", "quit"), bind("t", "theme")}
	secs := Suppress(globals, []Section{
		{Title: "Screen", Bindings: []key.Binding{bind("q", "quit"), bind("d", "delete")}},
	})

	if len(secs) != 1 {
		t.Fatalf("got %d sections, want 1", len(secs))
	}
	if got := len(secs[0].Bindings); got != 1 {
		t.Fatalf("screen section holds %d bindings, want 1 after suppression", got)
	}
	if got := secs[0].Bindings[0].Help().Desc; got != "delete" {
		t.Errorf("kept %q, want the binding the host didn't claim", got)
	}
}

// A group emptied by suppression is dropped rather than left as a heading
// over nothing.
func TestSuppressDropsEmptiedGroups(t *testing.T) {
	secs := Suppress([]key.Binding{bind("q", "quit")}, []Section{
		{Title: "Screen", Bindings: []key.Binding{bind("q", "quit")}},
	})
	if len(secs) != 0 {
		t.Fatalf("got %d sections, want none: %+v", len(secs), secs)
	}
}

// Two panes binding ↑/k to "up" are not duplicates — they are the same verb
// aimed at different components, and dropping the second leaves a pane
// looking as though it cannot be scrolled.
func TestCompileSectionsKeepsRepeatsAcrossGroups(t *testing.T) {
	secs := CompileSections([]Section{
		{Title: "files · Navigate", Bindings: []key.Binding{bind("k", "up")}},
		{Title: "results · Navigate", Bindings: []key.Binding{bind("k", "up")}},
	})
	if len(secs) != 2 {
		t.Fatalf("got %d sections, want both panes: %+v", len(secs), secs)
	}
}

// A repeat *within* one group is a mistake, and an empty group is a heading
// over nothing.
func TestCompileSectionsDedupesWithinAGroupAndDropsEmpties(t *testing.T) {
	secs := CompileSections([]Section{
		{Title: "Navigate", Bindings: []key.Binding{bind("k", "up"), bind("k", "up")}},
		{Title: "Empty"},
	})
	if len(secs) != 1 {
		t.Fatalf("got %d sections, want 1: %+v", len(secs), secs)
	}
	if got := len(secs[0].Bindings); got != 1 {
		t.Errorf("group holds %d bindings, want 1 after dedupe", got)
	}
}

func TestOverlayCloseKeyEmitsClosedMsg(t *testing.T) {
	o := newOverlay(t, []Section{{Title: "Global", Bindings: []key.Binding{bind("q", "quit")}}}, true)

	_, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("esc produced no command")
	}
	if _, ok := cmd().(ClosedMsg); !ok {
		t.Errorf("esc did not ask to close")
	}
}

// Esc steps out of a half-typed search before it closes the overlay —
// otherwise a mistyped query takes the whole reference with it.
func TestEscInSearchDoesNotCloseOverlay(t *testing.T) {
	o := newOverlay(t, []Section{{Title: "Global", Bindings: []key.Binding{bind("q", "quit")}}}, true)

	o, _ = o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !o.IsCapturingKeys() {
		t.Fatalf("setup: / did not focus the search field")
	}
	_, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		if _, ok := cmd().(ClosedMsg); ok {
			t.Errorf("esc closed the overlay instead of leaving the search field")
		}
	}
}

// Clicking off the modal dismisses it — the mouse spelling of esc, and what
// makes the shell's own "? close" affordance work without a special case.
func TestPressOutsideBoundsCloses(t *testing.T) {
	o := newOverlay(t, []Section{{Title: "Global", Bindings: []key.Binding{bind("q", "quit")}}}, false)
	_ = o.View()

	b := o.Bounds()
	_, cmd := o.Update(mouse.Msg{
		MouseMsg: tea.MouseMsg{X: b.X - 2, Y: b.Y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		Clicks:   1,
	})
	if cmd == nil {
		t.Fatalf("a press outside the modal produced no command")
	}
	if _, ok := cmd().(ClosedMsg); !ok {
		t.Errorf("a press outside the modal did not close it")
	}

	if _, cmd := o.Update(mouse.Msg{
		MouseMsg: tea.MouseMsg{X: b.X + 2, Y: b.Y + 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		Clicks:   1,
	}); cmd != nil {
		if _, ok := cmd().(ClosedMsg); ok {
			t.Errorf("a press inside the modal closed it")
		}
	}
}

// The modal sizes itself to its content and centers in the bounds it was
// given — long lists scroll inside it rather than growing past the screen.
func TestOverlayStaysWithinBounds(t *testing.T) {
	var many []key.Binding
	for i := 0; i < 60; i++ {
		many = append(many, bind(string(rune('a'+i%26)), strings.Repeat("wordy ", 8)))
	}
	o := NewOverlay(OverlayOptions{Searchable: true})
	o.SetSections([]Section{{Title: "Lots", Bindings: many}})

	bounds := geom.New(0, 1, 60, 20)
	o.SetRect(bounds)

	b := o.Bounds()
	if b.W > bounds.W || b.H > bounds.H {
		t.Errorf("modal %dx%d exceeds its bounds %dx%d", b.W, b.H, bounds.W, bounds.H)
	}
	if b.X < bounds.X || b.Y < bounds.Y {
		t.Errorf("modal at (%d,%d) sits outside bounds origin (%d,%d)", b.X, b.Y, bounds.X, bounds.Y)
	}
	lines := strings.Split(o.View(), "\n")
	if len(lines) != bounds.H {
		t.Errorf("rendered %d rows, want the %d its rect promises", len(lines), bounds.H)
	}
}

// A qualifier repeats on every one of that owner's headings, and the
// natural source for it is a pane title — which in this library is often
// "name · hint". Carrying the hint into every heading is what turns a
// qualifier into a paragraph.
func TestQualifyTrimsThePaneHint(t *testing.T) {
	secs := Qualify("files · / to filter", []Section{{Title: SectionNavigate}})
	if got, want := secs[0].Title, "files · Navigate"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
}

func TestQualifyCapsLongOwners(t *testing.T) {
	long := strings.Repeat("deployment", 4)
	secs := Qualify(long, []Section{{Title: SectionSort}})
	prefix := strings.SplitN(secs[0].Title, " · ", 2)[0]
	if w := lipgloss.Width(prefix); w > OwnerWidth {
		t.Errorf("owner %q is %d cells, want at most %d", prefix, w, OwnerWidth)
	}
}

// The list runs top to bottom in one column and scrolls when it overflows —
// a group keeps a fixed place rather than moving between columns as the
// terminal changes width.
func TestOverlayRunsTopToBottom(t *testing.T) {
	var secs []Section
	for i := 0; i < 6; i++ {
		secs = append(secs, Section{
			Title: strings.Repeat("x", 6) + string(rune('A'+i)),
			Bindings: []key.Binding{
				bind("a", "first"), bind("b", "second"), bind("c", "third"),
			},
		})
	}
	o := NewOverlay(OverlayOptions{})
	o.SetSections(secs)

	bounds := geom.New(0, 0, 120, 30)
	o.SetRect(bounds)

	for _, ln := range strings.Split(plain(o), "\n") {
		if strings.Count(ln, "first") > 1 {
			t.Fatalf("two groups landed on one row:\n%s", ln)
		}
	}
	if b := o.Bounds(); b.H > bounds.H {
		t.Errorf("modal is %d rows tall, past its %d-row bounds", b.H, bounds.H)
	}
}

// A long list stays inside its bounds; the pane scrolls the rest.
func TestOverlayStaysWithinNarrowBounds(t *testing.T) {
	var many []key.Binding
	for i := 0; i < 30; i++ {
		many = append(many, bind(string(rune('a'+i%26)), "does a thing"))
	}
	o := NewOverlay(OverlayOptions{})
	o.SetSections([]Section{{Title: "Lots", Bindings: many}})

	bounds := geom.New(0, 0, 44, 20)
	o.SetRect(bounds)

	if b := o.Bounds(); b.W > bounds.W || b.H > bounds.H {
		t.Errorf("modal %dx%d exceeds its bounds %dx%d", b.W, b.H, bounds.W, bounds.H)
	}
}

// Package multiselect demonstrates marking — the multi-selection a user
// builds with space — on a table.Model, and the action.Set that acts on it.
//
// Press space to mark the cursor row (it toggles both ways), X to extend the
// selection from the last-marked row down to the cursor, A to mark every
// row the filter currently shows, D to drop the selection, or click the ✓
// gutter. Then press "a" or right-click to open the menu: it is titled with
// what it will act on ("3 items"), and every verb that did not declare Multi
// is dimmed with a reason.
//
// X spans either direction from the anchor, which stays put — so ranging again
// from the same anchor grows or shrinks one span rather than walking it along.
// Shift+click does the same thing with the mouse. Both are additive, so a range
// extends a selection rather than replacing it.
//
// # Marking is keyed, or it is wrong
//
// Marks are held by the row's Key, not its index, which is why this screen
// populates the table with SetKeyedRows. On anonymous rows (SetRows) every
// mark operation is a deliberate no-op: an inert feature is recoverable, and a
// selection that has silently drifted onto its neighbours is not. That drift
// is not hypothetical — it is what an index-held mark does the moment a polled
// refresh reorders the set between the user marking rows and picking a verb.
//
// # Two things worth doing to this screen
//
// Mark a row, then filter it out of sight. The count in the title does not
// move, because a key does not care whether its row is currently visible — so
// the marked row is still in the selection, and the menu still says "2 items"
// while one of them is off screen. Correct, and a genuine surprise, which is
// why the menu's title is the disclosure rather than a decoration.
//
// Then cycle the theme with "t". The marks survive, because SetTheme carries
// them across the rebuild with SetMarks the same way it carries the cursor and
// the filter (rule 4). Forgetting that line is invisible until a user loses a
// twelve-row selection to a keypress that was supposed to change the colours.
//
// # Selection() is the accessor to reach for
//
// Not Marks(). Selection() is "the marked keys, or the cursor's key when
// nothing is marked", which is the branch every caller would otherwise write
// by hand and some would forget — and whose failure mode is a verb quietly
// acting on one row when the user had marked six.
package multiselect

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/ansi"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

type deployment struct {
	id       string
	name     string
	env      string
	health   string
	replicas int
	want     int
	age      string
	// terminating supplies an author-written Disabled reason, distinct from
	// the two the menu fills in on its own (arity and exclusivity).
	terminating bool
}

func seed() []deployment {
	return []deployment{
		{"api-server", "api-server", "prod", "Healthy", 3, 3, "4d", false},
		{"web-frontend", "web-frontend", "prod", "Healthy", 2, 2, "6h", false},
		{"worker-pool", "worker-pool", "prod", "Degraded", 1, 4, "12m", false},
		{"cache-redis", "cache-redis", "prod", "Healthy", 1, 1, "3d", false},
		{"metrics-agent", "metrics-agent", "prod", "Down", 0, 1, "9d", true},
		{"api-server-stg", "api-server-stg", "staging", "Healthy", 1, 1, "2d", false},
		{"web-frontend-stg", "web-frontend-stg", "staging", "Degraded", 1, 2, "5h", false},
		{"batch-runner-stg", "batch-runner", "staging", "Healthy", 2, 2, "1d", false},
	}
}

// Screen is an ordinary table screen. Marking is one Options field; acting on
// the marks is the Actions method at the bottom.
type Screen struct {
	t    theme.Theme
	tab  table.Model
	deps []deployment
}

// New returns the multi-select demo's root screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{deps: seed()}
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string         { return "Multi-select" }
func (s *Screen) Init() tea.Cmd         { return nil }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.tab.IsCapturingKeys() }
func (s *Screen) Layout() layout.Node   { return layout.Sized(&s.tab) }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.tab, cmd = s.tab.Update(msg)
	s.refreshTitle()
	return s, cmd
}

// Help leans on the table, which already advertises x, X, A and D because
// Markable is set — the bindings and the hints come from the same Keys struct
// (rule 26), so nothing here restates them. The verbs are absent on purpose:
// moving discovery off the footer and into the menu is most of the point.
func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

// HelpSections passes the table's own groups through — Navigate, Scroll,
// Sort, Filter, Select — and adds only what belongs to this screen.
//
// The heading over the marking keys is "Select", not "Multi-select": groups
// are named by what the keys do, and a heading named after the screen ends
// up over every binding on it, scroll keys included.
func (s *Screen) HelpSections() []help.Section {
	return help.SectionsOf(&s.tab, help.Group("Multi-select",
		key.NewBinding(key.WithKeys("mouse:right"), key.WithHelp("right-click", "actions")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	))
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	cursor, value := s.tab.Cursor(), s.tab.Value()
	sortCol, sortDesc := s.tab.SortColumn(), s.tab.SortDescending()
	marks := s.tab.Marks()

	opts := t.Table()
	opts.Title = "deployments"
	opts.Filterable = true
	opts.Markable = true
	opts.Filter.Placeholder = "filter, env:prod, health:~degraded…"
	opts.Columns = []table.Column{
		{Title: "Deployment", Flex: 1, Width: 18, Sortable: true},
		{Title: "Env", Width: 9, Sortable: true},
		{Title: "Health", Width: 14, Sortable: true, Less: healthLess},
		{Title: "Replicas", Width: 10, Align: lipgloss.Right},
		{Title: "Age", Width: 6, Sortable: true},
	}
	s.tab = table.New(opts)
	s.applyRows()

	if value != "" {
		s.tab.SetValue(value)
	}
	s.tab.SetCursor(cursor)
	s.tab.SetSort(sortCol, sortDesc)
	s.tab.SetMarks(marks)
	s.refreshTitle()
}

// applyRows keys every row by deployment id. That is the precondition for
// marking, and the same primitive pkg/poll leans on to keep the cursor in
// place across a refresh.
func (s *Screen) applyRows() {
	rows := make([]table.KeyedRow, len(s.deps))
	for i, d := range s.deps {
		rows[i] = table.KeyedRow{
			Key: d.id,
			Cells: []string{
				d.name,
				d.env,
				colorHealth(d.health),
				fmt.Sprintf("%d/%d", d.replicas, d.want),
				d.age,
			},
		}
	}
	s.tab.SetKeyedRows(rows)
}

// refreshTitle puts the mark count on the pane's border, which is what makes
// "marks survive filtering" observable: filter a marked row out of sight and
// this number stays where it was.
func (s *Screen) refreshTitle() {
	if n := s.tab.MarkCount(); n > 0 {
		s.tab.SetTitle("deployments · " + strconv.Itoa(n) + " marked")
		return
	}
	s.tab.SetTitle("deployments")
}

// Actions satisfies action.Provider. It is rebuilt per call, so it reports on
// whatever is selected right now.
//
// Every verb reads Selection() identically whether the user marked six rows or
// marked nothing and left the cursor on one. The arity difference lives in the
// Multi field, and the menu — not this code — is what enforces it.
func (s *Screen) Actions() action.Set {
	targets := s.tab.Selection()
	if len(targets) == 0 {
		return action.Set{}
	}
	label := s.tab.SelectionLabel()

	return action.Set{
		Target: label,
		Count:  len(targets),
		Actions: []action.Action{
			{
				Label:     "Restart",
				Desc:      "roll the pods",
				Key:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "restart")),
				Multi:     true,
				Exclusive: true,
				Run:       rollout(targets),
			},
			{
				Label: "Scale to zero",
				Desc:  "drain and stop",
				Multi: true,
				Run:   scale(targets),
			},
			{
				// No Multi, deliberately. Mark a second row and the menu dims
				// this itself with a reason — the zero value runs the safe way
				// round, so forgetting to think about arity costs a disabled
				// row rather than a verb that picks one of three targets
				// arbitrarily.
				Label: "Describe",
				Desc:  "one deployment only",
				Run:   describe(targets),
			},
			{
				Label:    "Delete",
				Multi:    true,
				Disabled: s.deleteGuard(targets),
				Confirm:  "Delete " + label + "? This cannot be undone.",
				Run:      remove(targets),
			},
			{
				Label: "Clear marks",
				Desc:  "drop the selection",
				Multi: true,
				Do: func() tea.Cmd {
					s.tab.ClearMarks()
					s.refreshTitle()
					return nil
				},
			},
		},
	}
}

func (s *Screen) deleteGuard(targets []string) string {
	for _, id := range targets {
		for _, d := range s.deps {
			if d.id == id && d.terminating {
				return d.name + " is already terminating"
			}
		}
	}
	return ""
}

// The verbs. Each is background work: a context to watch, a writer whose lines
// become one console event, and an error that decides the outcome. A verb over
// a marked set is a loop and nothing more — which is the point of Selection()
// returning a slice even when it holds one key.

func rollout(targets []string) action.Func {
	return func(ctx context.Context, out io.Writer) error {
		for _, id := range targets {
			fmt.Fprintf(out, "restarting %s\n", id)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(700 * time.Millisecond):
			}
			fmt.Fprintf(out, "%s rolled out\n", id)
		}
		return nil
	}
}

func scale(targets []string) action.Func {
	return func(_ context.Context, out io.Writer) error {
		for _, id := range targets {
			fmt.Fprintf(out, "scaled %s to 0 replicas\n", id)
		}
		return nil
	}
}

func describe(targets []string) action.Func {
	return func(_ context.Context, out io.Writer) error {
		fmt.Fprintf(out, "Name:     %s\nStatus:   Running\nStrategy: RollingUpdate\n", targets[0])
		return nil
	}
}

func remove(targets []string) action.Func {
	return func(_ context.Context, out io.Writer) error {
		for _, id := range targets {
			fmt.Fprintf(out, "deleted deployment/%s\n", id)
		}
		return fmt.Errorf("refusing to delete in a demo")
	}
}

func colorHealth(h string) string {
	switch h {
	case "Healthy":
		return ansi.CellColor(2, "✓ Healthy")
	case "Degraded":
		return ansi.CellColor(3, "● Degraded")
	case "Down":
		return ansi.CellColor(1, "✗ Down")
	}
	return h
}

func healthLess(a, b string) bool { return healthRank(a) < healthRank(b) }

// healthRank ranks the rendered cell, which carries the ansi.CellColor escape
// around the word — strings.Contains reads through it without a strip.
func healthRank(s string) int {
	switch {
	case strings.Contains(s, "Down"):
		return 0
	case strings.Contains(s, "Degraded"):
		return 1
	}
	return 2
}

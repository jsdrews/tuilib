// Package actions demonstrates pkg/action end to end: a screen declares what
// can be done to the current selection, and the app shell does everything
// else.
//
// Press "a" or right-click a row. The menu opens beside the rows, titled with
// what it will act on. Pick a verb and it runs in the background: the TUI
// stays yours, the output streams into the console under the action's label as
// a single event, the badge carries ⟳ while it works, and "o" then "x"
// cancels it.
//
// This screen is deliberately single-target. Marking (list.Options.Markable)
// and the Multi arity gate are real and tested, but demonstrating them here
// made the screen teach two things at once and neither read clearly — a reader
// met it as a menu demo and never discovered the other half. They belong in an
// example whose subject they are.
//
// # What this screen does not contain
//
// No menu. No confirm modal. No goroutine, context, buffer or result message.
// No mouse handling. No logging. All of that is app.Options.ActionsKey plus
// the shell — this screen's entire contribution is Actions() below, which is a
// list of labels and functions.
//
// Exclusive is the field worth understanding here: it says the verb must not
// overlap a run of itself on the same target, and the menu renders it as
// "already running" rather than silently dropping the press. Start a Restart
// and reopen the menu while it works.
package actions

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/theme"
)

type deployment struct {
	name     string
	ready    string
	age      string
	stopping bool
}

var deployments = []deployment{
	{"api-server", "3/3", "4d", false},
	{"web-frontend", "2/2", "6h", false},
	{"worker-pool", "0/4", "12m", false},
	{"cache-redis", "1/1", "3d", false},
	{"metrics-agent", "1/1", "9d", true},
}

// New returns the actions demo's root screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{deps: deployments}
	s.SetTheme(t)
	return s
}

// Screen is an ordinary screen with an ordinary list. The only thing that
// makes it actionable is the Actions method.
type Screen struct {
	t    theme.Theme
	deps []deployment
	list list.Model
}

func (s *Screen) Title() string         { return "Actions" }
func (s *Screen) Init() tea.Cmd         { return nil }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.list.Filtering() }
func (s *Screen) Layout() layout.Node   { return layout.Sized(&s.list) }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

// Help does not mention the actions key: the shell advertises that itself, and
// only on screens where the menu would actually open. Nor does it mention the
// individual verbs — moving those off the footer is the entire point.
func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

// HelpSections passes the list's own groups through and adds this screen's.
func (s *Screen) HelpSections() []help.Section {
	return help.SectionsOf(&s.list, help.Group("Deployments",
		key.NewBinding(key.WithKeys("mouse:right"), key.WithHelp("right-click", "actions")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	))
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	cursor, value := s.list.Cursor(), s.list.Value()
	lo := t.List()
	lo.Title = "deployments"
	lo.Filterable = true
	lo.Filter.Placeholder = "filter…"
	s.list = list.New(lo)
	s.list.SetKeyedItems(s.items())
	if value != "" {
		s.list.SetValue(value)
	}
	s.list.SetCursor(cursor)
}

// items are keyed so the cursor stays on its row across a swap rather than
// drifting onto a neighbour — and so Selection() has an identity to report.
func (s *Screen) items() []list.KeyedItem {
	out := make([]list.KeyedItem, len(s.deps))
	for i, d := range s.deps {
		out[i] = list.KeyedItem{
			Key:     d.name,
			Display: fmt.Sprintf("%-16s %-6s %s", d.name, d.ready, d.age),
		}
	}
	return out
}

// Actions satisfies action.Provider — the whole of this screen's contribution.
//
// It is rebuilt per call, so it always reports on whatever is selected right
// now. Selection() is the cursor row here; on a markable list it would be the
// marked set instead, which is why this reads the same either way.
func (s *Screen) Actions() action.Set {
	targets := s.list.Selection()
	if len(targets) == 0 {
		return action.Set{}
	}
	label := s.list.SelectionLabel()

	return action.Set{
		Target: label,
		Count:  len(targets),
		Actions: []action.Action{
			{
				Label:     "Restart",
				Desc:      "roll the pods",
				Key:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "restart")),
				Exclusive: true,
				Run:       rollout(targets),
			},
			{
				Label: "Scale to zero",
				Desc:  "drain and stop",
				Run:   scale(targets),
			},
			{
				Label: "View logs",
				Desc:  "open the log viewer",
				Key:   key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "logs")),
				Do: func() tea.Cmd {
					return screen.Push(newLogScreen(s.t, targets[0]))
				},
			},
			{
				Label:    "Delete",
				Disabled: s.deleteGuard(targets),
				Confirm:  "Delete " + label + "? This cannot be undone.",
				Run:      remove(targets),
			},
		},
	}
}

// deleteGuard is an author-supplied Disabled reason, distinct from the two the
// menu fills in on its own.
func (s *Screen) deleteGuard(targets []string) string {
	for _, name := range targets {
		for _, d := range s.deps {
			if d.name == name && d.stopping {
				return d.name + " is already terminating"
			}
		}
	}
	return ""
}

// The verbs themselves. Each is background work: a context it should watch, a
// writer whose lines become the console event, and an error that decides the
// outcome. Nothing here knows about the TUI.

func rollout(targets []string) action.Func {
	return func(ctx context.Context, out io.Writer) error {
		for _, name := range targets {
			fmt.Fprintf(out, "restarting %s\n", name)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(900 * time.Millisecond):
			}
			fmt.Fprintf(out, "%s rolled out\n", name)
		}
		return nil
	}
}

func scale(targets []string) action.Func {
	return func(_ context.Context, out io.Writer) error {
		for _, name := range targets {
			fmt.Fprintf(out, "scaled %s to 0 replicas\n", name)
		}
		return nil
	}
}

func remove(targets []string) action.Func {
	return func(_ context.Context, out io.Writer) error {
		for _, name := range targets {
			fmt.Fprintf(out, "deleted deployment/%s\n", name)
		}
		return fmt.Errorf("refusing to delete in a demo")
	}
}

// logScreen is what the "View logs" Do action pushes — the case that is not
// background work, and so has none of Run's reporting.
type logScreen struct {
	name string
	tv   textview.Model
}

func newLogScreen(t theme.Theme, name string) screen.Screen {
	s := &logScreen{name: name}
	s.SetTheme(t)
	return s
}

func (s *logScreen) Title() string       { return s.name + " logs" }
func (s *logScreen) Init() tea.Cmd       { return nil }
func (s *logScreen) OnEnter(any) tea.Cmd { return nil }
func (s *logScreen) Layout() layout.Node { return layout.Sized(&s.tv) }
func (s *logScreen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

func (s *logScreen) HelpSections() []help.Section {
	return help.SectionsOf(&s.tv, help.Group("Log",
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))))
}
func (s *logScreen) IsCapturingKeys() bool { return s.tv.IsCapturingKeys() }

func (s *logScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.tv, cmd = s.tv.Update(msg)
	return s, cmd
}

func (s *logScreen) SetTheme(t theme.Theme) {
	query := s.tv.Query()
	o := t.TextView()
	o.Title = s.name
	o.Searchable = true
	o.Content = fakeLog(s.name)
	s.tv = textview.New(o)
	if query != "" {
		s.tv.SetQuery(query)
	}
}

func fakeLog(name string) string {
	var b strings.Builder
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&b, "2026-08-26T10:%02d:12Z  %s  handled request id=%d in %dms\n",
			i%60, name, 4000+i, 3+i%17)
	}
	return b.String()
}

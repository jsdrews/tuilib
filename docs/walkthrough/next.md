# What next

## What you built, and what it taught

Every tuilib app is put together from the same handful of ideas, and the
walkthrough used each of them once:

| Idea | Where you met it |
|---|---|
| A screen is a struct of components plus eight methods, most of them one line of delegation | [1](01-shell.md) |
| Components start from a theme builder; `SetTheme` rebuilds them and `State`/`Restore` keeps the user's place | [1](01-shell.md), [2](02-table.md) |
| Rows from changing data are keyed, so the cursor and the marks follow the data, not the index | [2](02-table.md) |
| Layout is declared with stacks of `Fixed` and `Flex`; nothing computes a size | [3](03-two-panes.md) |
| With several components, a `focus.Group` owns focus; keys go to the focused one, everything else to all | [3](03-two-panes.md) |
| Data goes down the stack through constructors and comes back up through `screen.Pop` and `OnEnter` | [4](04-drill-down.md), [6](06-form.md) |
| Anything that takes time is a command that returns a message; nothing outside `Update` touches the screen | [4](04-drill-down.md), [7](07-live-data.md) |
| Covered screens keep receiving timers, so they stay current | [4](04-drill-down.md), [7](07-live-data.md) |
| Verbs are declared as data in an action menu, not bound to letters | [5](05-verbs.md), [6](06-form.md) |
| Feedback comes in three weights: the statusbar, the console, a modal | [5](05-verbs.md) |
| Forms validate themselves and only report valid submissions | [6](06-form.md) |
| A poll says when to read; activity says which rows are busy; the server is the source of truth | [7](07-live-data.md) |

## Testing a screen

A screen is an ordinary Go value, so it can be tested without a terminal:
build it, give it a theme, send it messages, and render its layout to a
string. Save this as `services_test.go`:

```go
package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/theme"
)

func newTestServices(t *testing.T) *services {
	t.Helper()
	s := newServices(NewFleet(FleetOptions{}))
	s.SetTheme(theme.New(palette.Dark()))
	s.Update(s.fetch()()) // run the read, and deliver what it returns
	return s
}

// render draws the screen the way the shell would, colours stripped.
func render(s *services) string {
	geom.NextGen()
	return ansi.Strip(s.Layout().Render(geom.New(0, 0, 100, 20)))
}

// typ sends each rune as a key press, then enter.
func typ(s *services, text string) {
	for _, r := range text {
		s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		render(s)
	}
	s.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

func TestFilterNarrowsTheTable(t *testing.T) {
	s := newTestServices(t)
	render(s)
	typ(s, "/env:stag")

	out := render(s)
	if strings.Contains(out, "legacy-billing") {
		t.Errorf("a prod service survived env:staging:\n%s", out)
	}
	if !strings.Contains(out, "recommendations") {
		t.Errorf("a staging service was filtered out:\n%s", out)
	}
}

func TestDetailFollowsTheCursor(t *testing.T) {
	s := newTestServices(t)
	render(s)
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	if out := render(s); !strings.Contains(out, "checkout · prod") {
		t.Errorf("the detail didn't follow the cursor to checkout:\n%s", out)
	}
}

func TestThemeSwapKeepsTheFilter(t *testing.T) {
	s := newTestServices(t)
	render(s)
	typ(s, "/check")
	s.SetTheme(theme.New(palette.Nord()))

	if out := render(s); !strings.Contains(out, "/ check") || !strings.Contains(out, "2 / 2") {
		t.Errorf("the filter or its rows were lost on a theme swap:\n%s", out)
	}
}
```

The pieces:

- `theme.New(palette.Dark())` is a theme without the shell: a palette drawn
  with the default surface.
- `s.fetch()()` calls the command to get its message and hands that to
  `Update`, which is what Bubble Tea would do. Since the backend in the test
  has no latency, it returns at once.
- `render` does what the shell does each frame: start a new frame with
  `geom.NextGen()`, then render the layout into a rectangle. Components learn
  their size from that render, so render once before sending keys.
- `ansi.Strip` removes the colours, so the assertions read the text.

```sh
go test ./...
```

Each test asserts something the walkthrough promised. The last one is the
most useful kind: delete the `s.table.Restore(st)` line from `SetTheme` and
it fails.

## Where the rest of the library fits

The walkthrough used about half the components. The rest slot into the same
screen shape, and you now know enough to read their pages directly.

| When you need | Use | Read |
|---|---|---|
| A list of names rather than a table of records | `pkg/list` | [List](../components/list.md) |
| A home screen of places to go, in groups | `pkg/menu` | [Menu](../components/menu.md) |
| Hierarchical data: namespaces, folders | `pkg/tree` | [Tree](../components/tree.md) |
| Several views of one thing behind a tab strip | `pkg/tab` | [Tabs](../components/tabs.md) |
| A yes/no or an acknowledgement you host yourself | `pkg/confirm`, `pkg/alert` | [Confirm](../components/confirm.md), [Alert](../components/alert.md) |
| More rows than one request can return | `pkg/remote` over `table` or `eventlog` | [Remote data](../guide/remote.md) |
| Running a command: an editor, a build | `pkg/runner` | [Running subprocesses](../guide/subprocesses.md) |
| Status counts, ratios, bars, sparklines in a cell | `pkg/metrics` | [Metrics](../components/metrics.md) |
| Your own colours or surface | `palette.Palette`, `surface.Surface` | [Theming and state](../guide/theming.md), [Surfaces](../surfaces.md) |
| Different keys for a component | `Options.Keys` | [Keys, focus and mouse](../guide/input.md) |

Two more places worth knowing:

- [Library rules](../reference/rules.md) states each convention the
  walkthrough followed, with the reason for it. When you're unsure how
  something should be done, the answer is usually there.
- The repository's examples launcher (`go run ./examples/launcher` in a
  clone) has larger screens that put many features together, and each one
  names the directory its code lives in. See [Examples](../examples.md).

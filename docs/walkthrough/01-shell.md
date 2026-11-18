# 1. The shell and a screen

The first step is the smallest program that is recognisably a tuilib app: one
screen, showing a page of text, inside the app shell. It's short, but it has
the shape every later step keeps.

Create `main.go`.

## Imports

```go
import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/theme"
)
```

Every tuilib package lives under `pkg/`, and you import the ones you use.
These six are the ones nearly every app imports: `app` (the shell), `screen`
(the interface you implement), `layout` (where things go), `theme` (how they
look), `help` (key hints), and one component, here `textview`. `key` and
`tea` come from Bubbles and Bubble Tea, which tuilib is built on.

## A screen is a struct

```go
type home struct {
	text textview.Model
}
```

A screen holds its components as fields. This one holds a single textview, a
scrollable, searchable page of static text.

Components are values (`textview.Model`, not `*textview.Model`), and their
`Update` returns a new value, the way Bubble Tea models do. The screen itself
is used through a pointer, so its methods can replace those fields.

## The one-line methods

`screen.Screen` is an interface of eight methods. Five of them are one line
here:

```go
func (s *home) Title() string         { return "Fleet" }
func (s *home) Init() tea.Cmd         { return nil }
func (s *home) OnEnter(any) tea.Cmd   { return nil }
func (s *home) IsCapturingKeys() bool { return s.text.IsCapturingKeys() }
func (s *home) Layout() layout.Node   { return layout.Sized(&s.text) }
```

`Title` is the screen's crumb in the breadcrumb across the top. It's called on
every render, so it can change, which step 4 makes use of.

`Init` runs once, when the screen is first put on the stack, and returns a
command to start anything the screen needs: a fetch, a timer. Nothing yet.

`OnEnter` runs every time the screen becomes the top of the stack, including
the first. Its argument is a value handed back by a screen that just closed;
step 6 uses it.

`IsCapturingKeys` answers one question for the shell: is a text field taking
the keyboard right now? The shell owns some keys of its own (`q` quits, `?`
opens help), and while this returns true it leaves them alone, so a `q`
typed into a search box is a letter. The textview has a search row and knows
whether it's focused, so the screen just asks it.

`Layout` says where the screen's components go. `layout.Sized(&s.text)`
means "this component, filling whatever space the screen is given". Pass a
pointer: the layout engine hands the component its size and position on
every render, by calling its `SetRect`. The screen never learns the terminal
size and never needs to.

## Key hints

```go
func (s *home) Help() []key.Binding          { return s.text.Help() }
func (s *home) HelpSections() []help.Section { return s.text.HelpSections() }
```

`Help` returns the keys the screen answers to, for the statusbar's hint strip
and the `?` overlay. Every component reports its own, and they change with its
state: while the search row is focused, the textview reports typing keys
rather than scrolling keys. So the screen forwards rather than lists.

`HelpSections` is optional. It returns the same keys grouped by what they do
(Navigate, Search, View), which is how the `?` overlay lays them out. Without
it, the overlay shows one group named after the screen. Forward it whenever
you host a component.

## Update forwards everything

```go
func (s *home) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.text, cmd = s.text.Update(msg)
	return s, cmd
}
```

`Update` receives every message the shell routes to this screen: keys,
mouse events, timers, the results of commands. The screen hands each one to
its component, keeps the updated component, and returns any command it asked
for.

Forward everything, not just keys. A component decides for itself what it
acts on, and some of what it needs isn't a key: the tick that animates a
spinner, the blink of a cursor. A screen that filters messages before its
components see them breaks them in ways that are hard to trace.

## SetTheme builds the components

```go
func (s *home) SetTheme(t theme.Theme) {
	st := s.text.State()
	opts := t.TextView()
	opts.Title = "welcome"
	opts.Searchable = true
	opts.Content = welcome
	s.text = textview.New(opts)
	s.text.Restore(st)
}
```

This is where the screen builds its components, and it's the method that
looks least like a constructor while being one. Line by line:

1. `s.text.State()` takes a snapshot of everything the user has done to the
   textview: scroll position, search query, current match, wrap mode.
2. `t.TextView()` returns `textview.Options` with every colour, border, glyph
   and key binding already filled in from the theme. Every component has a
   builder like this on `theme.Theme`: `t.List()`, `t.Table()`,
   `t.Inspector()`, and so on. Always start from it.
3. You set only what's specific to this instance: its title (drawn on the
   border), whether it's searchable, its content. Don't set colours here, and
   don't set a width or height: the layout engine sizes the component.
4. `textview.New(opts)` builds it.
5. `s.text.Restore(st)` puts the snapshot back.

The shell calls `SetTheme` once before the first frame, and again every time
the user switches palette or surface (step 2 turns that on). Each time, the
screen rebuilds its components from the new theme, and `State`/`Restore`
carries the user's place across, so a theme switch never loses a scroll
position or a half-typed search.

On the first call there's nothing to carry: `s.text` is a zero value, its
`State()` is empty, and restoring an empty state does nothing. That's why
the same five lines serve as both the constructor and the rebuild, and why
`home` needs no `newHome` function.

## The content

```go
const welcome = `This is a tuilib app.

The row above is the breadcrumb: one crumb per screen on the stack. The row
below is the statusbar: "? help" on the left, the version on the right. You
didn't write either; the app shell draws them around your screen.

This pane is a textview. It owns its border, its title, its scrollbar and its
search row. Try:

  /      search this text, enter to keep the query, esc to clear it
  n  N   next and previous match
  w      toggle word wrap
  ?      every key this screen answers to, grouped by what it does
  q      quit`
```

## main hands the screen to the shell

```go
func main() {
	m := app.New(app.Options{
		Root:    &home{},
		Version: "step 1",
	})
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

`app.New` builds the shell around a root screen and returns a `tea.Model`,
which you run like any Bubble Tea program. `tea.WithAltScreen()` takes over
the whole terminal and gives it back on exit.

`Root` is the bottom of the screen stack. `Version` is drawn at the right of
the statusbar. Everything else in `app.Options` is optional, and later steps
turn on what they need.

## Run it

```sh
go mod tidy
go run .
```

```
 Fleet
┌ welcome ─────────────────────────────────────────────────────────────────────────────┐
│/                                                                                     │
│───────────────────────────────────────────────────────────────────────────────────── │
│This is a tuilib app.                                                                 │
│                                                                                      │
│The row above is the breadcrumb: one crumb per screen on the stack. The row           │
│below is the statusbar: "? help" on the left, the version on the right. You           │
│didn't write either; the app shell draws them around your screen.                     │
│                                                                                      │
│This pane is a textview. It owns its border, its title, its scrollbar and its         │
│search row. Try:                                                                      │
│                                                                                      │
│  /      search this text, enter to keep the query, esc to clear it                   │
│  n  N   next and previous match                                                      │
│  w      toggle word wrap                                                             │
│  ?      every key this screen answers to, grouped by what it does                    │
│  q      quit                                                                         │
└──────────────────────────────────────────────────────────────────────────────────────┘
 ? help                                                                          step 1
```

Try the keys the text lists. Notice that the search row lives *inside* the
pane, under the title, rather than as a separate box above it. Every tuilib
component owns its pane: the border, the title, the scrollbar, any filter or
search row. You never wrap a component in another box, and you never render a
label above one; you set its `Title`.

Search for something, then press `q` while the search row is focused. It
types a `q`. That's `IsCapturingKeys` at work. Press `esc` to leave the field
and `q` quits.

Resize the terminal. The pane follows, and you wrote nothing to make it.

## What you have

A screen is a struct of components plus eight methods. Of those, most are
one line that delegates to a component: help, key capture, layout, update.
The only one with any substance is `SetTheme`, where components are built
from the theme and the user's place is carried across with `State` and
`Restore`. That proportion holds for the whole walkthrough: the library does
the work, and the screen says what it wants.

## The files so far

The files this step changed, as they should stand now.

??? example "main.go"

    ```go
    // Command fleet is a console for a handful of services.
    package main

    import (
    	"fmt"
    	"os"

    	"github.com/charmbracelet/bubbles/key"
    	tea "github.com/charmbracelet/bubbletea"

    	"github.com/jsdrews/tuilib/pkg/app"
    	"github.com/jsdrews/tuilib/pkg/help"
    	"github.com/jsdrews/tuilib/pkg/layout"
    	"github.com/jsdrews/tuilib/pkg/screen"
    	"github.com/jsdrews/tuilib/pkg/textview"
    	"github.com/jsdrews/tuilib/pkg/theme"
    )

    type home struct {
    	text textview.Model
    }

    func (s *home) Title() string         { return "Fleet" }
    func (s *home) Init() tea.Cmd         { return nil }
    func (s *home) OnEnter(any) tea.Cmd   { return nil }
    func (s *home) IsCapturingKeys() bool { return s.text.IsCapturingKeys() }
    func (s *home) Layout() layout.Node   { return layout.Sized(&s.text) }

    func (s *home) Help() []key.Binding          { return s.text.Help() }
    func (s *home) HelpSections() []help.Section { return s.text.HelpSections() }

    func (s *home) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    	var cmd tea.Cmd
    	s.text, cmd = s.text.Update(msg)
    	return s, cmd
    }

    func (s *home) SetTheme(t theme.Theme) {
    	st := s.text.State()
    	opts := t.TextView()
    	opts.Title = "welcome"
    	opts.Searchable = true
    	opts.Content = welcome
    	s.text = textview.New(opts)
    	s.text.Restore(st)
    }

    const welcome = `This is a tuilib app.

    The row above is the breadcrumb: one crumb per screen on the stack. The row
    below is the statusbar: "? help" on the left, the version on the right. You
    didn't write either; the app shell draws them around your screen.

    This pane is a textview. It owns its border, its title, its scrollbar and its
    search row. Try:

      /      search this text, enter to keep the query, esc to clear it
      n  N   next and previous match
      w      toggle word wrap
      ?      every key this screen answers to, grouped by what it does
      q      quit`

    func main() {
    	m := app.New(app.Options{
    		Root:    &home{},
    		Version: "step 1",
    	})
    	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
    		fmt.Fprintln(os.Stderr, err)
    		os.Exit(1)
    	}
    }
    ```

Next, [a table](02-table.md).

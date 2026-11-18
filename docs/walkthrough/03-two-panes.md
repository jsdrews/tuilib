# 3. Two panes

A table says which services exist. This step adds a detail pane beside it
that shows everything about the one under the cursor. Two interactive
components on one screen raise the question this step is about: which of them
gets the keys?

All the changes are in `services.go`.

## New imports

Add these to the import block:

```go
	"strconv"
```

```go
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/inspector"
```

## The struct

```go
type services struct {
	api  *Fleet
	svcs []Service

	table  table.Model
	detail inspector.Model
	focus  focus.Group
	shown  string // the key the detail is showing
}
```

An `inspector` is a two-column label/value viewer for one record, with
nested fields that expand and collapse. It's the natural partner for a
table: the table compares many records, and the inspector reads one.

A `focus.Group` holds the screen's interactive components in tab order and
gives the keyboard to exactly one of them. `shown` remembers which service
the detail is showing, so it's updated only when the cursor moves to a
different one.

## The methods that delegate to the group

```go
func (s *services) Title() string         { return "Services" }
func (s *services) Init() tea.Cmd         { return tea.Batch(s.focus.Init(), s.reload()) }
func (s *services) OnEnter(any) tea.Cmd   { return nil }
func (s *services) IsCapturingKeys() bool { return s.focus.IsCapturingKeys() }

func (s *services) Help() []key.Binding          { return s.focus.Help() }
func (s *services) HelpSections() []help.Section { return s.focus.HelpSections() }
```

With more than one component, these ask the group instead of a component.
The group answers from whichever component has focus: `IsCapturingKeys` is
true while the focused pane's filter is taking input, and `Help` lists
`tab`/`shift+tab` plus the focused pane's keys. `HelpSections` returns every
pane's groups, each qualified with its pane's title ("services · Navigate",
"detail · Navigate") so the `?` overlay can tell them apart.

`Init` batches the group's `Init` with the load. `tea.Batch` runs several
commands together.

## Layout: Flex and Fixed

```go
func (s *services) Layout() layout.Node {
	return layout.HStack(
		layout.Flex(1, layout.Sized(&s.table)),
		layout.Fixed(34, layout.Sized(&s.detail)),
	)
}
```

`HStack` lays its items out left to right (`VStack` is top to bottom).
Each item is either:

- `Fixed(n, node)`, exactly `n` cells along the stack's direction: here 34
  columns for the detail; or
- `Flex(weight, node)`, a share of whatever the `Fixed` items leave. With
  one `Flex`, the table takes all of it.

Two `Flex(1, …)` items would split the width evenly; `Flex(2, …)` beside
`Flex(1, …)` would take two thirds. Stacks nest, so any layout is a tree of
these. Notice there's no arithmetic: you never compute `width - 34`, and you
never need the terminal size. The layout engine divides the space on every
frame and hands each component its rectangle.

## Update routes keys to the focused pane

```go
func (s *services) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd
	s.focus, cmd = s.focus.Update(msg)
	cmds = append(cmds, cmd)

	if _, isKey := msg.(tea.KeyMsg); isKey {
		switch {
		case s.focus.Is(&s.table):
			s.table, cmd = s.table.Update(msg)
		case s.focus.Is(&s.detail):
			s.detail, cmd = s.detail.Update(msg)
		}
		cmds = append(cmds, cmd)
	} else {
		s.table, cmd = s.table.Update(msg)
		cmds = append(cmds, cmd)
		s.detail, cmd = s.detail.Update(msg)
		cmds = append(cmds, cmd)
	}

	s.showDetail()
	return s, tea.Batch(cmds...)
}
```

This is the routing every multi-pane screen uses, and it's worth reading
slowly.

First, **the group sees every message**, not just `tab`. It cycles on
`tab`/`shift+tab`, but it also has to see the request a component sends when
it's clicked. A click lands on the component, which can't focus itself
(only the group knows the others need blurring), so the component replies
with a message asking the group for focus. A group fed only keys never sees
that request, and the clicked pane lights up while the keys keep going to
the old one.

Then **keys go to the focused component only**. Otherwise `j` would move the
cursor in both panes, and typing into one pane's filter would drive the other.

**Everything else goes to every component**: mouse events, timers, command
results. A mouse event especially: each component checks whether the
position falls inside its own rectangle and ignores it if not, so sending
every click to every component is how the one under the pointer gets it. It
is tempting to route the mouse like the keys, to the focused pane, and that
is the single easiest mistake to make here: the pane you click on never hears
about it, so it can never take focus.

Last, `showDetail` makes the detail agree with the table.

## Following the cursor

```go
// showDetail puts the table's current service in the detail pane, when the
// cursor has moved to a different one.
func (s *services) showDetail() {
	k, ok := s.table.SelectedKey()
	if !ok || k == s.shown {
		return
	}
	svc, ok := s.service(k)
	if !ok {
		return
	}
	s.shown = k
	s.detail.SetTitle(svc.Name + " · " + svc.Env)
	s.detail.SetFields(fieldsOf(svc))
}

func (s *services) service(k string) (Service, bool) {
	for _, svc := range s.svcs {
		if svc.Key() == k {
			return svc, true
		}
	}
	return Service{}, false
}
```

`SelectedKey` is the key of the row under the cursor, whatever the filter
and sort have done to the order. The screen looks the service up in its own
data, rather than reading cells back out of the table, and hands the
inspector a new title and new fields.

Running this after every message is cheap, because it returns at once unless
the cursor has moved.

```go
func fieldsOf(svc Service) []inspector.Field {
	return []inspector.Field{
		{Label: "name", Value: svc.Name},
		{Label: "environment", Value: svc.Env},
		{Label: "owner", Value: svc.Owner},
		{Label: "version", Value: svc.Version},
		{Label: "status", Value: svc.Status},
		{Label: "replicas", Children: []inspector.Field{
			{Label: "desired", Value: strconv.Itoa(svc.Replicas)},
			{Label: "ready", Value: strconv.Itoa(svc.Ready)},
		}},
	}
}
```

A field with `Children` is a branch the user can fold with `space`. If your
record arrives as decoded JSON, `inspector.FromMap(m)` builds these fields
for you, nested to any depth.

## SetTheme builds three things

```go
func (s *services) SetTheme(t theme.Theme) {
	st := s.table.State()
	opts := t.Table()
	opts.Title = "services"
	opts.Filterable = true
	opts.Filter.Placeholder = "filter: checkout, env:prod, status:~^de"
	opts.Columns = []table.Column{
		{Title: "Service", Width: 16, Flex: 1, Sortable: true},
		{Title: "Env", Width: 8, Sortable: true},
		{Title: "Status", Width: 12, Sortable: true},
		{Title: "Ready", Width: 5, Align: lipgloss.Right},
	}
	s.table = table.New(opts)
	s.table.Restore(st)

	dst := s.detail.State()
	do := t.Inspector()
	do.Title = "detail"
	if s.shown != "" {
		do.Title = s.detail.Title()
	}
	do.InitialDepth = 1
	s.detail = inspector.New(do)
	s.detail.Restore(dst)

	i := s.focus.Index()
	s.focus = focus.NewGroup(&s.table, &s.detail).WithKeys(focus.Keys{
		Next: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next pane")),
		Prev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev pane")),
	})
	s.focus.SetIndex(i)
}
```

The table is unchanged. The inspector follows the same five lines, with two
details: `InitialDepth: 1` opens the first level of branches, and the title
is carried over by hand once the detail is showing something, because a
title is an option rather than state.

The group is rebuilt last, over the **addresses** of the two fields. The
components behind `&s.table` and `&s.detail` were just replaced, but the
addresses weren't, so a group built over them points at the new ones.
Rebuilding it this way also reads the index before and restores it after, so
a theme switch leaves focus on the pane that had it.

`WithKeys` only relabels the cycling keys: the default help text says
"field", which suits a form better than a pair of panes.

## Load and show

One line changes in `reload`:

```go
	s.svcs = svcs
	s.table.SetKeyedRows(rowsOf(svcs))
	s.shown = "" // the record may have changed under the same key
	s.showDetail()
	return nil
```

`showDetail` runs from `Update`, but nothing has called `Update` when `Init`
loads the first rows, so `reload` shows the detail itself. Clearing `shown`
first means a reload refreshes the detail even when the cursor is still on
the same key and only the record behind it changed.

## Run it

```
 Services
┌ services ──────────────────────────────────────────────────┐┌ api-gateway · prod ────────────┐
│/ filter: checkout, env:prod, status:~^de                   ││  name       : api-gateway      │
│─────────────────────────────────────────────────────────── ││  environment: prod             │
│Service               │ Env        │ Status         │ Ready ││  owner      : platform         │
│─────────────────────────────────────────────────────────── ││  version    : 2.14.0           │
│api-gateway           │ prod       │ ● Healthy      │   3/3 ││  status     : Healthy          │
│checkout              │ prod       │ ● Degraded     │   3/4 ││▾ replicas                      │
│legacy-billing        │ prod       │ ● Down         │   0/1 ││    desired: 3                  │
│notifications         │ prod       │ ● Healthy      │   2/2 ││    ready  : 3                  │
│search                │ prod       │ ● Healthy      │   2/2 ││                                │
│api-gateway           │ staging    │ ● Healthy      │   1/1 ││                                │
│checkout              │ staging    │ ● Healthy      │   1/1 ││                                │
│recommendations       │ staging    │ ● Degraded     │   1/2 ││                                │
│                                                            ││                                │
│                                                            ││                                │
└ 8 / 8 ────────────────────────────────────────────── 1 / 8 ┘└───────────────────────── 1 / 8 ┘
 ? help                                                                                  step 3
```

| Do | See |
|---|---|
| `j`/`k` | the detail follows the cursor |
| `tab` | focus moves to the detail; its border lights up; `j`/`k` now move its cursor |
| `space` on `replicas` | folds the branch |
| click either pane | it takes focus |
| scroll the wheel over the pane that *isn't* focused | it scrolls, and focus stays where it was |
| `/` in the table, then `tab` | nothing cycles: the filter is taking input. `enter` or `esc` leaves it, then `tab` cycles |
| `?` | both panes' keys, each under its pane's name |

The wheel row is deliberate: scrolling is looking, not choosing, so it acts
on whatever is under the pointer and never takes the keyboard.

## The files so far

The files this step changed, as they should stand now. Anything not listed is unchanged from the step before.

??? example "services.go"

    ```go
    package main

    import (
    	"fmt"
    	"strconv"

    	"github.com/charmbracelet/bubbles/key"
    	tea "github.com/charmbracelet/bubbletea"
    	"github.com/charmbracelet/lipgloss"

    	"github.com/jsdrews/tuilib/pkg/ansi"
    	"github.com/jsdrews/tuilib/pkg/app"
    	"github.com/jsdrews/tuilib/pkg/focus"
    	"github.com/jsdrews/tuilib/pkg/help"
    	"github.com/jsdrews/tuilib/pkg/inspector"
    	"github.com/jsdrews/tuilib/pkg/layout"
    	"github.com/jsdrews/tuilib/pkg/screen"
    	"github.com/jsdrews/tuilib/pkg/table"
    	"github.com/jsdrews/tuilib/pkg/theme"
    )

    type services struct {
    	api  *Fleet
    	svcs []Service

    	table  table.Model
    	detail inspector.Model
    	focus  focus.Group
    	shown  string // the key the detail is showing
    }

    func newServices(api *Fleet) *services {
    	return &services{api: api}
    }

    func (s *services) Title() string         { return "Services" }
    func (s *services) Init() tea.Cmd         { return tea.Batch(s.focus.Init(), s.reload()) }
    func (s *services) OnEnter(any) tea.Cmd   { return nil }
    func (s *services) IsCapturingKeys() bool { return s.focus.IsCapturingKeys() }

    func (s *services) Help() []key.Binding          { return s.focus.Help() }
    func (s *services) HelpSections() []help.Section { return s.focus.HelpSections() }

    func (s *services) Layout() layout.Node {
    	return layout.HStack(
    		layout.Flex(1, layout.Sized(&s.table)),
    		layout.Fixed(34, layout.Sized(&s.detail)),
    	)
    }

    func (s *services) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    	var cmds []tea.Cmd
    	var cmd tea.Cmd
    	s.focus, cmd = s.focus.Update(msg)
    	cmds = append(cmds, cmd)

    	if _, isKey := msg.(tea.KeyMsg); isKey {
    		switch {
    		case s.focus.Is(&s.table):
    			s.table, cmd = s.table.Update(msg)
    		case s.focus.Is(&s.detail):
    			s.detail, cmd = s.detail.Update(msg)
    		}
    		cmds = append(cmds, cmd)
    	} else {
    		s.table, cmd = s.table.Update(msg)
    		cmds = append(cmds, cmd)
    		s.detail, cmd = s.detail.Update(msg)
    		cmds = append(cmds, cmd)
    	}

    	s.showDetail()
    	return s, tea.Batch(cmds...)
    }

    // showDetail puts the table's current service in the detail pane, when the
    // cursor has moved to a different one.
    func (s *services) showDetail() {
    	k, ok := s.table.SelectedKey()
    	if !ok || k == s.shown {
    		return
    	}
    	svc, ok := s.service(k)
    	if !ok {
    		return
    	}
    	s.shown = k
    	s.detail.SetTitle(svc.Name + " · " + svc.Env)
    	s.detail.SetFields(fieldsOf(svc))
    }

    func (s *services) service(k string) (Service, bool) {
    	for _, svc := range s.svcs {
    		if svc.Key() == k {
    			return svc, true
    		}
    	}
    	return Service{}, false
    }

    func fieldsOf(svc Service) []inspector.Field {
    	return []inspector.Field{
    		{Label: "name", Value: svc.Name},
    		{Label: "environment", Value: svc.Env},
    		{Label: "owner", Value: svc.Owner},
    		{Label: "version", Value: svc.Version},
    		{Label: "status", Value: svc.Status},
    		{Label: "replicas", Children: []inspector.Field{
    			{Label: "desired", Value: strconv.Itoa(svc.Replicas)},
    			{Label: "ready", Value: strconv.Itoa(svc.Ready)},
    		}},
    	}
    }

    func (s *services) SetTheme(t theme.Theme) {
    	st := s.table.State()
    	opts := t.Table()
    	opts.Title = "services"
    	opts.Filterable = true
    	opts.Filter.Placeholder = "filter: checkout, env:prod, status:~^de"
    	opts.Columns = []table.Column{
    		{Title: "Service", Width: 16, Flex: 1, Sortable: true},
    		{Title: "Env", Width: 8, Sortable: true},
    		{Title: "Status", Width: 12, Sortable: true},
    		{Title: "Ready", Width: 5, Align: lipgloss.Right},
    	}
    	s.table = table.New(opts)
    	s.table.Restore(st)

    	dst := s.detail.State()
    	do := t.Inspector()
    	do.Title = "detail"
    	if s.shown != "" {
    		do.Title = s.detail.Title()
    	}
    	do.InitialDepth = 1
    	s.detail = inspector.New(do)
    	s.detail.Restore(dst)

    	i := s.focus.Index()
    	s.focus = focus.NewGroup(&s.table, &s.detail).WithKeys(focus.Keys{
    		Next: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next pane")),
    		Prev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev pane")),
    	})
    	s.focus.SetIndex(i)
    }

    // reload reads the fleet and hands it to the table.
    func (s *services) reload() tea.Cmd {
    	svcs, err := s.api.List()
    	if err != nil {
    		return app.ErrorOf(err)
    	}
    	s.svcs = svcs
    	s.table.SetKeyedRows(rowsOf(svcs))
    	s.shown = "" // the record may have changed under the same key
    	s.showDetail()
    	return nil
    }

    func rowsOf(svcs []Service) []table.KeyedRow {
    	rows := make([]table.KeyedRow, len(svcs))
    	for i, svc := range svcs {
    		rows[i] = table.KeyedRow{
    			Key: svc.Key(),
    			Cells: []string{
    				svc.Name,
    				svc.Env,
    				statusCell(svc.Status),
    				fmt.Sprintf("%d/%d", svc.Ready, svc.Replicas),
    			},
    			Data: svc,
    		}
    	}
    	return rows
    }

    // statusCell colours a status with a foreground-only escape, so the selected
    // row's background runs unbroken across the cell.
    func statusCell(status string) string {
    	switch status {
    	case Healthy:
    		return ansi.CellColor(2, "● "+status)
    	case Down:
    		return ansi.CellColor(1, "● "+status)
    	}
    	return ansi.CellColor(3, "● "+status)
    }
    ```

Next, [drilling down](04-drill-down.md).

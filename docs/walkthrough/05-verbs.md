# 5. Verbs

The app can show the fleet but can't change it. This step adds verbs:
restart and delete, acting on one service or on several the user has marked,
with a confirmation before the destructive one and a log of what happened.

You might expect to bind them to letters, `r` for restart and `d` for delete.
tuilib steers you away from that. A screen soon has more verbs than there are
sensible letters, they collide with keys components already use, and nobody
can discover them without reading the help. Instead, a screen *declares* its
verbs and the shell shows them in a menu.

## Turn on the menu and the console

In `main.go`, add two options:

```go
		ActionsKey: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "actions")),
		OutputKey:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "output")),
```

`ActionsKey` opens the action menu (a right click opens it too, at the
pointer). `OutputKey` opens the output console, the app's log, where the
verbs' output will go. Both are off unless you give them a key.

## Let the user mark rows

In `services.go`'s `SetTheme`, one line on the table's options:

```go
	opts.Markable = true
```

The table now has a mark gutter and the keys to fill it: `x` toggles the
cursor's row, `X` marks a range from the last mark to the cursor, `A` marks
everything the filter shows, `D` clears every mark. Clicking the gutter
toggles a row.

Marks are held by key, which is the other reason step 2 keyed its rows: if
the rows are reloaded or reordered between the user marking six of them and
choosing a verb, the marks stay on the same six services rather than sliding
onto their neighbours.

## Declare the verbs

Add the imports (`fmt` is already there):

```go
	"context"
	"io"
```

```go
	"github.com/jsdrews/tuilib/pkg/action"
```

Then give the screen an `Actions` method. That's all it takes for the shell
to treat it as having verbs (it implements `action.Provider`):

```go
// Actions lists the verbs for whatever is selected in the table: the marked
// rows, or the cursor's row when nothing is marked.
func (s *services) Actions() action.Set {
	if !s.focus.Is(&s.table) {
		return action.Set{}
	}
	keys := s.table.Selection()
	if len(keys) == 0 {
		return action.Set{}
	}
	label := s.table.SelectionLabel()
	return action.Set{
		Target:  label,
		Targets: keys,
		Actions: []action.Action{
			{
				Label: "View logs", Group: "read",
				Do: func() tea.Cmd { return screen.Push(newLogs(s.api, keys[0], s.t)) },
			},
			{
				Label: "Restart", Desc: "roll every replica", Group: "write",
				Multi: true, Exclusive: true,
				Run: s.restart(keys),
			},
			{
				Label: "Delete", Group: "danger", Multi: true,
				Confirm: "Delete " + label + "? This can't be undone.",
				Run:     s.delete(keys),
			},
		},
	}
}
```

The shell calls `Actions` each time the menu opens, so it always describes
the selection as it is right then. Keep it cheap: no I/O.

`s.table.Selection()` is "the marked keys, or the cursor's key if nothing is
marked". Every verb wants exactly that, and writing the branch by hand is how
a verb ends up acting on the cursor's row while six others are marked. Use it
rather than `Marks()`. `SelectionLabel()` names it for a title:
`prod/checkout`, or `3 items`.

Each `action.Action` is a verb, described as data:

- `Group` files it under a header. Headers appear once a set uses two or
  more groups.
- `Multi` says the verb can act on more than one target. It's off by default,
  so "View logs", which can only open one screen, shows as disabled, with a
  reason, while two rows are marked. The safe default means forgetting to
  think about it leaves a visibly disabled verb rather than one that quietly
  picks one of the marked rows.
- `Confirm` puts a yes/no dialog between choosing the verb and running it.
  No modal to build: the shell owns it.
- `Exclusive` refuses a second restart of the same target while one is
  running, and the menu says so.
- A verb has either `Do` or `Run`.

**`Do`** returns a command, run as-is. Use it for verbs that aren't
background work: pushing a screen, as "View logs" does.

**`Run`** is background work, and it's what most verbs should be:

```go
func (s *services) restart(keys []string) action.Func {
	api := s.api
	return func(ctx context.Context, out io.Writer) error {
		for _, k := range keys {
			fmt.Fprintf(out, "restarting %s\n", k)
			if err := api.Restart(ctx, k); err != nil {
				return err
			}
			fmt.Fprintf(out, "%s is healthy\n", k)
		}
		return nil
	}
}

func (s *services) delete(keys []string) action.Func {
	api := s.api
	return func(_ context.Context, out io.Writer) error {
		for _, k := range keys {
			if err := api.Delete(k); err != nil {
				return err
			}
			fmt.Fprintf(out, "deleted %s\n", k)
		}
		return nil
	}
}
```

An `action.Func` is `func(ctx context.Context, out io.Writer) error`. The
shell runs it on its own goroutine, so the UI stays live while it works, and
looks after the rest:

- whatever it writes to `out` goes to the output console, as one event under
  the verb's name;
- `ctx` is cancelled if the user kills the run from the console;
- when it returns, the statusbar says `Restart completed`, or shows the error.

Since it runs off the UI goroutine, it mustn't touch the screen. It reads
`keys` and `api`, captured when the menu was opened, and talks only to the
backend, which is safe to call from any goroutine.

## Reload when a run finishes

The verbs change the fleet, so the table should show the result. In
`Update`, after the group's update, add:

```go
	if _, done := msg.(runner.Captured); done {
		cmds = append(cmds, s.reload())
	}
```

and import `github.com/jsdrews/tuilib/pkg/runner`. When a `Run` finishes, the
shell delivers a `runner.Captured` message to the screens as well as handling
it itself. The screen doesn't need to know which run it was: any of them may
have changed the fleet.

## Run it

Mark `checkout` and `notifications` with `x`, then press `a`:

```
 Services
┌ services ──────────────────────────────────────────────────┐┌ notifications · prod ──────────┐
│/ filter: checkout, env:prod, status:~^de                   ││  name       : notifications    │
│─────────────────────────────────────────────────────────── ││  environment: prod             │
│  Service             │ Env        │ Status         │ Ready ││  owner      : growth           │
│──────────────────────────────┏ Actions · 2 items ━━━━━━━━━━━━━┓version    : 0.9.7            │
│  api-gateway         │ prod  ┃── read ─────────────────────── ┃status     : Healthy          │
│✓ checkout            │ prod  ┃  View logs  one item at a time ┃replicas                      │
│  legacy-billing      │ prod  ┃── write ────────────────────── ┃  desired: 2                  │
│✓ notifications       │ prod  ┃▸ Restart    roll every replica ┃  ready  : 2                  │
│  search              │ prod  ┃── danger ───────────────────── ┃                              │
│  api-gateway         │ stagin┃  Delete                        ┃                              │
│  checkout            │ stagin┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛                              │
│  recommendations     │ staging    │ ● Degraded     │   1/2 ││                                │
│                                                            ││                                │
│                                                            ││                                │
└ 8 / 8 ────────────────────────────────────────────── 4 / 8 ┘└───────────────────────── 1 / 8 ┘
 ? help                                                                                  step 5
```

"View logs" is dimmed because two rows are marked. Choose Restart. Two
seconds per service later:

```
<!-- frame: step5/after restart -->
```

The statusbar says it finished, and the badge says there's `1 output` you
haven't read. Press `o`, or click the message:

```
 Services › Output
┌ Output ──────────────────────────────────────────────────────────────────────────────────────┐
│/                                                                                             │
│───────────────────────────────────────────────────────────────────────────────────────────── │
│14:46:16 INF Restart    › Restart · 2 items                                                   │
│14:46:16 INF Restart    │ restarting prod/checkout                                            │
│14:46:18 INF Restart    │ prod/checkout is healthy                                            │
│14:46:18 INF Restart    │ restarting prod/notifications                                       │
│14:46:20 INF Restart    │ prod/notifications is healthy                                       │
│14:46:20 INF Restart    │ Restart completed                                                   │
│                                                                                              │
│                                                                                              │
│                                                                                              │
│                                                                                              │
│                                                                                              │
│                                                                                              │
└ FOLLOWING ───────────────────────────────────────────────────────────────────────────────────┘
 ? help                                                                        1 output  step 5
```

Everything the verb wrote, under its name and timestamped. `esc` or `o` closes
it.

Now try the failures:

| Do | See |
|---|---|
| restart `legacy-billing` | the statusbar shows the start of the error; the console has all of it |
| start a restart, `o`, then `x` | kills the run; `ctx` is cancelled and the restart stops |
| choose Restart again on a row still restarting | dimmed: "already running" |
| choose Delete | the confirmation, with `No` highlighted; `y`/`n`/`esc` or the buttons answer it |
| press `a` with the detail pane focused | nothing: the screen returns no verbs there |

## Three weights of feedback

This step used all three ways an app tells the user something, and each has
its place:

| Weight | For | How |
|---|---|---|
| passing | "Restart completed" | `app.Info`, `app.Error`, `app.ErrorOf`: one line on the statusbar, gone at the next keypress |
| readable later | a run's output, an error's whole chain | the output console; every statusbar message is also logged there |
| blocking | "are you sure?" | a confirm dialog: `Confirm` on a verb here, or `pkg/confirm` for one you host yourself; `pkg/alert` for something to acknowledge |

Pick the lightest one that's honest. A modal for "saved" is an interruption,
and a stack trace on the statusbar is a sliver nobody can read.

## The files so far

The files this step changed, as they should stand now. Anything not listed is unchanged from the step before.

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
    	"github.com/jsdrews/tuilib/pkg/palette"
    	"github.com/jsdrews/tuilib/pkg/surface"
    )

    func main() {
    	m := app.New(app.Options{
    		Root:       newServices(NewFleet(FleetOptions{})),
    		Palettes:   palette.All(),
    		Surfaces:   surface.Builtins(),
    		ThemeKey:   key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "palette")),
    		SurfaceKey: key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "surface")),
    		ActionsKey: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "actions")),
    		OutputKey:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "output")),
    		Mouse:      app.MouseClick,
    		Version:    "step 5",
    	})
    	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
    		fmt.Fprintln(os.Stderr, err)
    		os.Exit(1)
    	}
    }
    ```

??? example "services.go"

    ```go
    package main

    import (
    	"context"
    	"fmt"
    	"io"
    	"strconv"

    	"github.com/charmbracelet/bubbles/key"
    	tea "github.com/charmbracelet/bubbletea"
    	"github.com/charmbracelet/lipgloss"

    	"github.com/jsdrews/tuilib/pkg/action"
    	"github.com/jsdrews/tuilib/pkg/ansi"
    	"github.com/jsdrews/tuilib/pkg/app"
    	"github.com/jsdrews/tuilib/pkg/focus"
    	"github.com/jsdrews/tuilib/pkg/help"
    	"github.com/jsdrews/tuilib/pkg/inspector"
    	"github.com/jsdrews/tuilib/pkg/layout"
    	"github.com/jsdrews/tuilib/pkg/runner"
    	"github.com/jsdrews/tuilib/pkg/screen"
    	"github.com/jsdrews/tuilib/pkg/table"
    	"github.com/jsdrews/tuilib/pkg/theme"
    )

    type services struct {
    	t    theme.Theme
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

    	if _, done := msg.(runner.Captured); done {
    		cmds = append(cmds, s.reload())
    	}

    	if s.opens(msg) {
    		if k, ok := s.table.SelectedKey(); ok {
    			return s, tea.Batch(append(cmds, screen.Push(newLogs(s.api, k, s.t)))...)
    		}
    	}

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

    // opens reports enter typed into the table, or a double click on it.
    func (s *services) opens(msg tea.Msg) bool {
    	_, isKey := msg.(tea.KeyMsg)
    	return (!isKey || s.focus.Is(&s.table)) && s.table.IsActivate(msg)
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
    	s.t = t

    	st := s.table.State()
    	opts := t.Table()
    	opts.Title = "services"
    	opts.Filterable = true
    	opts.Markable = true
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

    // Actions lists the verbs for whatever is selected in the table: the marked
    // rows, or the cursor's row when nothing is marked.
    func (s *services) Actions() action.Set {
    	if !s.focus.Is(&s.table) {
    		return action.Set{}
    	}
    	keys := s.table.Selection()
    	if len(keys) == 0 {
    		return action.Set{}
    	}
    	label := s.table.SelectionLabel()
    	return action.Set{
    		Target:  label,
    		Targets: keys,
    		Actions: []action.Action{
    			{
    				Label: "View logs", Group: "read",
    				Do: func() tea.Cmd { return screen.Push(newLogs(s.api, keys[0], s.t)) },
    			},
    			{
    				Label: "Restart", Desc: "roll every replica", Group: "write",
    				Multi: true, Exclusive: true,
    				Run: s.restart(keys),
    			},
    			{
    				Label: "Delete", Group: "danger", Multi: true,
    				Confirm: "Delete " + label + "? This can't be undone.",
    				Run:     s.delete(keys),
    			},
    		},
    	}
    }

    func (s *services) restart(keys []string) action.Func {
    	api := s.api
    	return func(ctx context.Context, out io.Writer) error {
    		for _, k := range keys {
    			fmt.Fprintf(out, "restarting %s\n", k)
    			if err := api.Restart(ctx, k); err != nil {
    				return err
    			}
    			fmt.Fprintf(out, "%s is healthy\n", k)
    		}
    		return nil
    	}
    }

    func (s *services) delete(keys []string) action.Func {
    	api := s.api
    	return func(_ context.Context, out io.Writer) error {
    		for _, k := range keys {
    			if err := api.Delete(k); err != nil {
    				return err
    			}
    			fmt.Fprintf(out, "deleted %s\n", k)
    		}
    		return nil
    	}
    }
    ```

Next, [a form](06-form.md).

# 7. Live data

Up to now the backend answered at once and never changed by itself, which is
not what remote systems do. This step makes it behave like one: every call
takes 400ms, and services change on their own, losing replicas and rolling
out new versions. Then it makes the app cope:

- reads move off the UI goroutine, so a slow backend never freezes the
  screen;
- the table shows a spinner until the first read lands;
- the fleet is re-read every two seconds, without the cursor, the filter or
  the marks noticing;
- rows the server is busy with show a spinner and a status, and so do rows
  the user just restarted, from the moment they ask.

## A slower backend

In `main.go`, construct the backend with:

```go
		Root:       newServices(NewFleet(FleetOptions{Latency: 400 * time.Millisecond, Drift: true})),
```

and add `"time"` to the imports. Run the app now, before changing anything
else, and it already feels wrong: the first frame waits for the read, and
every restart, delete and create freezes the UI for 400ms. That's the cost of
calling the backend from `Update`, which runs on the UI goroutine. The rest of
this step removes it.

## Reads become commands

In `services.go`, add the imports:

```go
	"time"
```

```go
	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/poll"
```

and replace `reload` with a command and the message it produces:

```go
// fetchedMsg is one read of the fleet, stamped with the read it answers.
type fetchedMsg struct {
	read activity.Read
	svcs []Service
	err  error
}

// fetch reads the fleet off the UI goroutine.
func (s *services) fetch() tea.Cmd {
	api, rd := s.api, s.table.BeginRead()
	return func() tea.Msg {
		svcs, err := api.List()
		return fetchedMsg{read: rd, svcs: svcs, err: err}
	}
}
```

A `tea.Cmd` is a function Bubble Tea runs on a goroutine of its own. Whatever
it returns is delivered to `Update` as a message. So `fetch` returns at once,
the read happens elsewhere, and the result arrives later as a `fetchedMsg`.
As in step 4, the command copies what it needs (`api`) and touches nothing
else on the screen.

`BeginRead` stamps the read as it's issued. Reads can come back in a
different order from the one they were sent in, and the stamp is how the
table tells a stale answer from a current one when it arrives.

```go
// applied lands a read: the rows when it arrived in order and succeeded, the
// error once when reads start failing.
func (s *services) applied(m fetchedMsg) tea.Cmd {
	wasFailing := s.table.ReadsFailing()
	if !s.table.ApplyRead(m.read, rowsOf(m.svcs), m.err) {
		if m.err != nil && !wasFailing {
			return app.ErrorOf(m.err)
		}
		return nil
	}
	s.svcs = m.svcs
	s.shown = ""
	s.poll.MarkRefreshed()
	if !s.loaded {
		s.loaded = true
		return s.table.SetLoading(false)
	}
	return nil
}
```

`ApplyRead` is `SetKeyedRows` with that ordering applied. It returns false,
and changes nothing, when the answer has been overtaken by a newer one or the
read failed. When reads start failing it reports the error once rather than
on every poll; the pretend backend never fails, but a real one will, and an
outage shouldn't put a fresh error on the statusbar every two seconds.

When the read did land, the screen keeps the data, clears `shown` so the
detail pane picks up any change to the current record, tells the poll it
refreshed, and ends the loading spinner after the first read.

## Loading and polling

Add two fields to the struct, and build the poll in the constructor:

```go
	poll   poll.Model
	loaded bool // the first read has landed
}

func newServices(api *Fleet) *services {
	return &services{api: api, poll: poll.New(poll.Options{Interval: 2 * time.Second})}
}
```

Replace `Init`:

```go
func (s *services) Init() tea.Cmd {
	return tea.Batch(s.focus.Init(), s.poll.Init(), s.table.SetLoading(true), s.fetch())
}
```

`SetLoading(true)` swaps the table's body for a spinner. It returns a command
because a spinner is animated by ticks: the command is its first tick, and
the component asks for each next one itself, as long as it's forwarded
messages.

`poll.Model` is a timer and nothing else. It never reads data; every
`Interval` it produces a `poll.RefreshMsg`, and the screen decides what to
do about it. `Init` starts it.

Then, at the top of `Update`, forward every message to the poll and handle
the two new ones. The `runner.Captured` case from step 5 changes too, because
`reload` is gone:

```go
	var cmds []tea.Cmd
	var cmd tea.Cmd
	cmds = append(cmds, s.poll.Update(msg))
	s.focus, cmd = s.focus.Update(msg)
	cmds = append(cmds, cmd)

	switch m := msg.(type) {
	case poll.RefreshMsg:
		cmds = append(cmds, s.fetch())
	case fetchedMsg:
		cmds = append(cmds, s.applied(m))
	case restartedMsg:
		switch s.table.Done(m.op, m.err) {
		case activity.Withdrawn:
			cmds = append(cmds, app.ErrorOf(m.err))
		case activity.Ended:
			cmds = append(cmds, app.Info("Restarted "+m.key), s.poll.Refresh())
		}
	case runner.Captured:
		cmds = append(cmds, s.poll.Refresh())
	}
```

(`restartedMsg` is below.) `s.poll.Update` returns the next tick each time it
sees its own, which is what keeps the poll running. `s.poll.Refresh()` asks
for a `RefreshMsg` now, without disturbing the schedule: the way to say "read
again, something just changed".

`OnEnter` uses it the same way, in place of `reload`:

```go
	return tea.Batch(s.poll.Refresh(), app.Info("Created "+created.Key()))
```

The rows are replaced every two seconds now, and that's where step 2's keys
pay off. The cursor stays on the same service, the filter and sort are
reapplied to the new rows, and the marks stay on the services that were
marked, even when a service appears or disappears above them.

The poll also keeps running while the logs screen is pushed over this one,
for the reason in step 4: covered screens still receive timers. When the user
comes back, the table is already current.

## Spinners on busy rows

A loading spinner says the whole table is waiting. A row spinner says "these
two services are busy", which is what the user wants to know about a fleet.
`pkg/activity`, built into the table, draws them. Its model is two
sentences: **the server is the source of truth, and when the user starts
work, the row is told at once, until the truth can take over.**

First, tell the table how to read the truth. In `SetTheme`:

```go
	opts.ActivityColumn = "Status"
	opts.BusyWhen = func(r table.KeyedRow) (string, bool) {
		return working(r.Data.(Service).Status)
	}
```

and beside it, at the top level of the file:

```go
// working says which statuses mean the server is busy with a service.
var working = activity.Busy(Deploying, Restarting)
```

`BusyWhen` is asked about each row on every read, and answers with the
status to show and whether it means "busy". It reads the record in `Data`
(this is what step 2 put it there for), so the field it looks at doesn't have
to be a column. `activity.Busy` builds the common predicate from a list of
busy values. `ActivityColumn` is where the spinner draws. In a list or a tree
it draws as a badge at the end of the row instead.

The backend now starts rollouts on its own, so wait a few seconds and a row
will show `⣽ Deploying` for three polls and settle back to `● Healthy` with a
new version, without the app doing anything.

## Restart, with a spinner from the keypress

Step 5's Restart ran in the background and reported in the console. The row
itself said nothing until the restart was over and the table reloaded. Now
that rows can show activity, Restart can tell the row at once. Change the
verb to:

```go
		action.Action{
			Label: "Restart", Desc: "roll every replica", Group: "write", Multi: true,
			Do: func() tea.Cmd {
				return s.table.DispatchEach(keys, Restarting, activity.Held, s.restart)
			},
		},
```

and replace `restart` with a per-row request:

```go
// restartedMsg is one row's restart answering.
type restartedMsg struct {
	op  activity.Op
	key string
	err error
}

// restart is one row's request: it blocks until the restart is over, which
// is why the verb is dispatched Held.
func (s *services) restart(op activity.Op, key string) tea.Cmd {
	api := s.api
	return func() tea.Msg {
		return restartedMsg{op: op, key: key, err: api.Restart(context.Background(), key)}
	}
}
```

`DispatchEach` does three things for each key: it puts a claim on the row (a
spinner labelled `Restarting`, straight away), it calls `restart` for the
command that does the work, and it batches those commands with the
spinner's first tick. The `activity.Op` it hands `restart` identifies that
row's claim, and comes back in `restartedMsg`.

One request per row, not one for the whole selection, so that one row failing
stops one spinner rather than all of them.

`activity.Held` is the decision every verb needs: **who knows when the work
ends?** Here, the request does. `Restart` blocks until the restart is over, so
when it answers the work is finished. The other mode, `activity.Observed`, is
for a request that only *starts* work (an HTTP 202 Accepted): then the server's
status, read by polling, says when it ends.

Back in `Update`, the `restartedMsg` case hands the answer to `Done`, which
ends the claim and says what the answer meant:

- `activity.Withdrawn`: the request failed; show the error.
- `activity.Ended`: a `Held` request answered, so the work is over; say so,
  and read again now rather than waiting up to two seconds for the poll.
- (`activity.Acknowledged` is what an `Observed` request returns: accepted,
  still running. Say "requested", never "completed".)

Switch on what `Done` returns rather than assuming: a handler that says
"completed" for every answer is wrong as soon as an `Observed` verb shares it.

Restart is a `Do` verb now, because the screen handles the answer itself.
That costs it the console output and the kill from step 5. Delete stays a
`Run`.

## The table's title

One last touch: say on the border how many rows are busy. At the end of
`Update`:

```go
	s.showDetail()
	s.table.SetTitle(s.tableTitle())
	return s, tea.Batch(cmds...)
}

func (s *services) tableTitle() string {
	if n := s.table.ActivityCount(); n > 0 {
		return fmt.Sprintf("services · %d working", n)
	}
	return "services"
}
```

The border is where a component says anything about its own state: the row
counter, `FOLLOWING`, a filter's match count. A title set from `Update` keeps
it current.

## Create without freezing

The form calls the backend from `Update` too. In `create.go`, add a field and
a message:

```go
type create struct {
	api    *Fleet
	form   form.Model
	saving bool
}

// createdMsg is the backend answering a Create.
type createdMsg struct {
	svc Service
	err error
}
```

and handle the submit as a command:

```go
	switch m := msg.(type) {
	case form.SubmittedMsg:
		if s.saving {
			return s, nil
		}
		s.saving = true
		api, svc := s.api, s.service()
		return s, func() tea.Msg { return createdMsg{svc: svc, err: api.Create(svc)} }
	case createdMsg:
		s.saving = false
		if m.err != nil {
			return s, app.ErrorOf(m.err)
		}
		return s, screen.Pop(m.svc)
```

The same shape as `fetch`: issue the request as a command, act on its message.
`saving` stops a second `enter` sending a second create while the first is on
its way.

## Run it

The first frame shows the loading spinner while the first read is under way:

```
 Services
┌ services ──────────────────────────────────────────────────┐┌ detail ────────────────────────┐
│/ filter: checkout, env:prod, status:~^de                   ││                                │
│─────────────────────────────────────────────────────────── ││                                │
│                                                            ││                                │
│                                                            ││                                │
│                                                            ││                                │
│                                                            ││                                │
│                                                            ││                                │
│                             ⣽                              ││                                │
│                                                            ││                                │
│                                                            ││                                │
│                                                            ││                                │
│                                                            ││                                │
│                                                            ││                                │
│                                                            ││                                │
└ 0 / 0 ─────────────────────────────────────────────────────┘└────────────────────────────────┘
 ? help                                                                                  step 7
```

Then the rows. Mark two and restart them:

```
 Services
┌ services · 2 working ──────────────────────────────────────┐┌ notifications · prod ──────────┐
│/ filter: checkout, env:prod, status:~^de                   ││  name       : notifications    │
│─────────────────────────────────────────────────────────── ││  environment: prod             │
│  Service             │ Env        │ Status         │ Ready ││  owner      : growth           │
│─────────────────────────────────────────────────────────── ││  version    : 0.9.7            │
│  api-gateway         │ prod       │ ● Healthy      │   3/3 ││  status     : Healthy          │
│✓ checkout            │ prod       │ ⣽ Restarting   │   3/4 ││▾ replicas                      │
│  legacy-billing      │ prod       │ ● Down         │   0/1 ││    desired: 2                  │
│✓ notifications       │ prod       │ ⣽ Restarting   │   2/2 ││    ready  : 2                  │
│  search              │ prod       │ ● Healthy      │   2/2 ││                                │
│  api-gateway         │ staging    │ ● Healthy      │   1/1 ││                                │
│  checkout            │ staging    │ ● Healthy      │   1/1 ││                                │
│  recommendations     │ staging    │ ● Degraded     │   1/2 ││                                │
│                                                            ││                                │
│                                                            ││                                │
└ 8 / 8 ────────────────────────────────────────────── 4 / 8 ┘└───────────────────────── 1 / 8 ┘
 ? help                                                                        1 output  step 7
```

| Do | See |
|---|---|
| wait | now and then a row goes `⣽ Deploying` and comes back with a new version; a healthy one drops to Degraded |
| move the cursor, filter, mark rows, then wait | the rows refresh under you; nothing you did moves |
| restart two rows | both spin from the keypress; each stops when its own restart answers |
| restart `legacy-billing` | spins, then the error; the row goes back to `Down` |
| open the logs, wait, come back | the table is current: it kept polling underneath |
| `t` mid-restart | the new palette, still spinning: activity is part of `State` |

## The files so far

The files this step changed, as they should stand now. Anything not listed is unchanged from the step before.

??? example "main.go"

    ```go
    // Command fleet is a console for a handful of services.
    package main

    import (
    	"fmt"
    	"os"
    	"time"

    	"github.com/charmbracelet/bubbles/key"
    	tea "github.com/charmbracelet/bubbletea"

    	"github.com/jsdrews/tuilib/pkg/app"
    	"github.com/jsdrews/tuilib/pkg/palette"
    	"github.com/jsdrews/tuilib/pkg/surface"
    )

    func main() {
    	m := app.New(app.Options{
    		Root:       newServices(NewFleet(FleetOptions{Latency: 400 * time.Millisecond, Drift: true})),
    		Palettes:   palette.All(),
    		Surfaces:   surface.Builtins(),
    		ThemeKey:   key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "palette")),
    		SurfaceKey: key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "surface")),
    		ActionsKey: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "actions")),
    		OutputKey:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "output")),
    		Mouse:      app.MouseClick,
    		Version:    "step 7",
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
    	"time"

    	"github.com/charmbracelet/bubbles/key"
    	tea "github.com/charmbracelet/bubbletea"
    	"github.com/charmbracelet/lipgloss"

    	"github.com/jsdrews/tuilib/pkg/action"
    	"github.com/jsdrews/tuilib/pkg/activity"
    	"github.com/jsdrews/tuilib/pkg/ansi"
    	"github.com/jsdrews/tuilib/pkg/app"
    	"github.com/jsdrews/tuilib/pkg/focus"
    	"github.com/jsdrews/tuilib/pkg/help"
    	"github.com/jsdrews/tuilib/pkg/inspector"
    	"github.com/jsdrews/tuilib/pkg/layout"
    	"github.com/jsdrews/tuilib/pkg/poll"
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

    	poll   poll.Model
    	loaded bool // the first read has landed
    }

    func newServices(api *Fleet) *services {
    	return &services{api: api, poll: poll.New(poll.Options{Interval: 2 * time.Second})}
    }

    func (s *services) Title() string         { return "Services" }
    func (s *services) IsCapturingKeys() bool { return s.focus.IsCapturingKeys() }

    func (s *services) Init() tea.Cmd {
    	return tea.Batch(s.focus.Init(), s.poll.Init(), s.table.SetLoading(true), s.fetch())
    }

    func (s *services) Help() []key.Binding          { return s.focus.Help() }
    func (s *services) HelpSections() []help.Section { return s.focus.HelpSections() }

    // OnEnter runs whenever this screen becomes the top of the stack. A child
    // that popped with a value hands it over here.
    func (s *services) OnEnter(result any) tea.Cmd {
    	created, ok := result.(Service)
    	if !ok {
    		return nil
    	}
    	return tea.Batch(s.poll.Refresh(), app.Info("Created "+created.Key()))
    }

    func (s *services) Layout() layout.Node {
    	return layout.HStack(
    		layout.Flex(1, layout.Sized(&s.table)),
    		layout.Fixed(34, layout.Sized(&s.detail)),
    	)
    }

    func (s *services) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    	var cmds []tea.Cmd
    	var cmd tea.Cmd
    	cmds = append(cmds, s.poll.Update(msg))
    	s.focus, cmd = s.focus.Update(msg)
    	cmds = append(cmds, cmd)

    	switch m := msg.(type) {
    	case poll.RefreshMsg:
    		cmds = append(cmds, s.fetch())
    	case fetchedMsg:
    		cmds = append(cmds, s.applied(m))
    	case restartedMsg:
    		switch s.table.Done(m.op, m.err) {
    		case activity.Withdrawn:
    			cmds = append(cmds, app.ErrorOf(m.err))
    		case activity.Ended:
    			cmds = append(cmds, app.Info("Restarted "+m.key), s.poll.Refresh())
    		}
    	case runner.Captured:
    		cmds = append(cmds, s.poll.Refresh())
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
    	s.table.SetTitle(s.tableTitle())
    	return s, tea.Batch(cmds...)
    }

    func (s *services) tableTitle() string {
    	if n := s.table.ActivityCount(); n > 0 {
    		return fmt.Sprintf("services · %d working", n)
    	}
    	return "services"
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
    	opts.ActivityColumn = "Status"
    	opts.BusyWhen = func(r table.KeyedRow) (string, bool) {
    		return working(r.Data.(Service).Status)
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

    // working says which statuses mean the server is busy with a service.
    var working = activity.Busy(Deploying, Restarting)

    // fetchedMsg is one read of the fleet, stamped with the read it answers.
    type fetchedMsg struct {
    	read activity.Read
    	svcs []Service
    	err  error
    }

    // fetch reads the fleet off the UI goroutine.
    func (s *services) fetch() tea.Cmd {
    	api, rd := s.api, s.table.BeginRead()
    	return func() tea.Msg {
    		svcs, err := api.List()
    		return fetchedMsg{read: rd, svcs: svcs, err: err}
    	}
    }

    // applied lands a read: the rows when it arrived in order and succeeded, the
    // error once when reads start failing.
    func (s *services) applied(m fetchedMsg) tea.Cmd {
    	wasFailing := s.table.ReadsFailing()
    	if !s.table.ApplyRead(m.read, rowsOf(m.svcs), m.err) {
    		if m.err != nil && !wasFailing {
    			return app.ErrorOf(m.err)
    		}
    		return nil
    	}
    	s.svcs = m.svcs
    	s.shown = ""
    	s.poll.MarkRefreshed()
    	if !s.loaded {
    		s.loaded = true
    		return s.table.SetLoading(false)
    	}
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

    // Actions lists the verbs: creating a service always, and for whatever is
    // selected in the table (the marked rows, or the cursor's row when nothing is
    // marked) viewing, restarting and deleting it.
    func (s *services) Actions() action.Set {
    	set := action.Set{
    		Target: "fleet",
    		Actions: []action.Action{{
    			Label: "New service", Group: "fleet",
    			Multi: true, // it ignores the selection, so any number of marks is fine
    			Do:    func() tea.Cmd { return screen.Push(newCreate(s.api, s.t)) },
    		}},
    	}
    	keys := s.table.Selection()
    	if !s.focus.Is(&s.table) || len(keys) == 0 {
    		return set
    	}
    	label := s.table.SelectionLabel()
    	set.Target, set.Targets = label, keys
    	set.Actions = append(set.Actions,
    		action.Action{
    			Label: "View logs", Group: "read",
    			Do: func() tea.Cmd { return screen.Push(newLogs(s.api, keys[0], s.t)) },
    		},
    		action.Action{
    			Label: "Restart", Desc: "roll every replica", Group: "write", Multi: true,
    			Do: func() tea.Cmd {
    				return s.table.DispatchEach(keys, Restarting, activity.Held, s.restart)
    			},
    		},
    		action.Action{
    			Label: "Delete", Group: "danger", Multi: true,
    			Confirm: "Delete " + label + "? This can't be undone.",
    			Run:     s.delete(keys),
    		},
    	)
    	return set
    }

    // restartedMsg is one row's restart answering.
    type restartedMsg struct {
    	op  activity.Op
    	key string
    	err error
    }

    // restart is one row's request: it blocks until the restart is over, which
    // is why the verb is dispatched Held.
    func (s *services) restart(op activity.Op, key string) tea.Cmd {
    	api := s.api
    	return func() tea.Msg {
    		return restartedMsg{op: op, key: key, err: api.Restart(context.Background(), key)}
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

??? example "create.go"

    ```go
    package main

    import (
    	"errors"
    	"regexp"
    	"strconv"

    	"github.com/charmbracelet/bubbles/key"
    	tea "github.com/charmbracelet/bubbletea"

    	"github.com/jsdrews/tuilib/pkg/app"
    	"github.com/jsdrews/tuilib/pkg/form"
    	"github.com/jsdrews/tuilib/pkg/help"
    	"github.com/jsdrews/tuilib/pkg/layout"
    	"github.com/jsdrews/tuilib/pkg/screen"
    	"github.com/jsdrews/tuilib/pkg/theme"
    )

    type create struct {
    	api    *Fleet
    	form   form.Model
    	saving bool
    }

    // createdMsg is the backend answering a Create.
    type createdMsg struct {
    	svc Service
    	err error
    }

    func newCreate(api *Fleet, t theme.Theme) *create {
    	s := &create{api: api}
    	s.SetTheme(t)
    	return s
    }

    func (s *create) Title() string         { return "New service" }
    func (s *create) Init() tea.Cmd         { return s.form.Init() }
    func (s *create) OnEnter(any) tea.Cmd   { return nil }
    func (s *create) IsCapturingKeys() bool { return s.form.IsCapturingKeys() }

    func (s *create) Layout() layout.Node {
    	return layout.Center(56, 16, layout.Sized(&s.form))
    }

    func (s *create) Help() []key.Binding          { return s.form.Help() }
    func (s *create) HelpSections() []help.Section { return s.form.HelpSections() }

    func (s *create) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    	switch m := msg.(type) {
    	case form.SubmittedMsg:
    		if s.saving {
    			return s, nil
    		}
    		s.saving = true
    		api, svc := s.api, s.service()
    		return s, func() tea.Msg { return createdMsg{svc: svc, err: api.Create(svc)} }
    	case createdMsg:
    		s.saving = false
    		if m.err != nil {
    			return s, app.ErrorOf(m.err)
    		}
    		return s, screen.Pop(m.svc)
    	case form.CancelledMsg:
    		return s, screen.Pop(nil)
    	}
    	var cmd tea.Cmd
    	s.form, cmd = s.form.Update(msg)
    	return s, cmd
    }

    // service is what the form describes. Every value has already passed the
    // form's validation by the time SubmittedMsg arrives.
    func (s *create) service() Service {
    	replicas, _ := strconv.Atoi(s.form.String("replicas"))
    	return Service{
    		Name:     s.form.String("name"),
    		Env:      s.form.String("env"),
    		Owner:    s.form.String("owner"),
    		Version:  "0.1.0",
    		Replicas: replicas,
    	}
    }

    func (s *create) SetTheme(t theme.Theme) {
    	opts := t.Form()
    	opts.Title = "new service"
    	opts.SubmitText = "Create"
    	opts.Fields = []form.Field{
    		form.Text(form.TextOptions{
    			Key: "name", Label: "Name", Placeholder: "lowercase-with-dashes",
    			Required: true, Validate: validName,
    		}),
    		form.Select(form.SelectOptions{
    			Key: "env", Label: "Environment", Options: []string{"prod", "staging"},
    		}),
    		form.Text(form.TextOptions{
    			Key: "owner", Label: "Owning team", Required: true,
    		}),
    		form.Text(form.TextOptions{
    			Key: "replicas", Label: "Replicas", Placeholder: "1–10",
    			Required: true, Validate: validReplicas,
    		}),
    	}
    	s.form = form.New(opts)
    }

    var nameRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

    func validName(v any) error {
    	if !nameRE.MatchString(v.(string)) {
    		return errors.New("lowercase letters, digits, dashes")
    	}
    	return nil
    }

    func validReplicas(v any) error {
    	n, err := strconv.Atoi(v.(string))
    	if err != nil || n < 1 || n > 10 {
    		return errors.New("a number from 1 to 10")
    	}
    	return nil
    }
    ```

That's the app. The [last page](next.md) covers testing it, and where to go
from here.

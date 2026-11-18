# 4. Drilling down

So far the app is one screen. This step adds a second, the service's logs,
opened by pressing `enter` on a row. Behind it is the screen stack:
`enter` pushes a screen, `esc` pops it, and the breadcrumb shows the path.

## A screen for the logs

Create `logs.go`:

```go
import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)
```

```go
type logs struct {
	api *Fleet
	key string
	log logview.Model
}

func newLogs(api *Fleet, key string, t theme.Theme) *logs {
	s := &logs{api: api, key: key}
	s.SetTheme(t)
	return s
}
```

A `logview` is the streaming counterpart to the textview: lines are appended
as they arrive, the view follows the tail, and scrolling up pauses it.

The constructor takes everything the screen needs, including which service,
and **calls `SetTheme` itself**. The shell calls `SetTheme` on the root
screen at startup, and on every screen when the theme changes, but not on a
screen you push: by the time it's pushed, it must already be built. That's
why the constructor takes the current theme.

This is how data moves down the stack: the parent constructs the child with
what it needs. There's no global to read from and nothing to look up.

```go
func (s *logs) Title() string         { return s.key }
func (s *logs) Init() tea.Cmd         { return s.tail() }
func (s *logs) OnEnter(any) tea.Cmd   { return nil }
func (s *logs) IsCapturingKeys() bool { return s.log.IsCapturingKeys() }
func (s *logs) Layout() layout.Node   { return layout.Sized(&s.log) }

func (s *logs) Help() []key.Binding          { return s.log.Help() }
func (s *logs) HelpSections() []help.Section { return s.log.HelpSections() }
```

The title is the service's key, so the breadcrumb reads
`Services › prod/checkout`. `Init` starts the tail.

## Streaming with a chained command

```go
// tailMsg carries the lines one tail read returned, addressed to the screen
// that asked for them.
type tailMsg struct {
	to    *logs
	lines []string
}

// tail waits half a second, reads whatever the service has written since,
// and posts it. Each tailMsg asks for the next one, so the chain runs for as
// long as this screen is on the stack and stops when it's popped.
func (s *logs) tail() tea.Cmd {
	api, key := s.api, s.key
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return tailMsg{to: s, lines: api.Logs(key)}
	})
}
```

A component never starts a goroutine of its own, and nothing outside
`Update` ever touches the screen's state. Anything that happens over time is
a command that produces a message, and the next command is asked for when
that message arrives.

`tea.Tick` waits, then calls its function on Bubble Tea's own goroutine to
build the message. The read happens there, off the UI, and its result comes
back as a `tailMsg`. Copying `api` and `key` into locals first makes it plain
that the function reads nothing that `Update` might be changing.

The message carries the screen it's for. Messages other than keys and mouse
events are delivered to *every* screen on the stack (more on that below), and
if the user closed the logs and reopened them quickly, the old screen's last
tick would otherwise land in the new one and start a second chain.

```go
func (s *logs) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if m, ok := msg.(tailMsg); ok && m.to == s {
		s.log.AppendLines(m.lines)
		return s, s.tail()
	}
	var cmd tea.Cmd
	s.log, cmd = s.log.Update(msg)
	return s, cmd
}
```

Each `tailMsg` appends its lines and returns the next tick. When the screen
is popped, it stops receiving messages, nobody asks for the next tick, and
the chain ends by itself. There's nothing to cancel.

This polls, which suits a backend you have to ask. When the source pushes
instead (a websocket, a server-sent event stream, a file being tailed), the
shape is the same with a channel in place of the timer: one goroutine owns
the source and writes to a channel, and a command reads *one* item and
returns it as a message. For subprocesses the library has that already
written, in `runner.Capture`.

```go
func (s *logs) SetTheme(t theme.Theme) {
	st := s.log.State()
	opts := t.Logview()
	opts.Title = s.key + " logs"
	opts.Searchable = true
	opts.MaxLines = 2000
	s.log = logview.New(opts)
	s.log.Restore(st)
}
```

`MaxLines` caps the buffer: the oldest lines are dropped once it's full, so a
stream left running all day can't grow without bound. The default is 10,000;
pick what suits the source.

## Opening it from the table

Back in `services.go`, the screen needs to keep the theme to hand to the
child. Add a field:

```go
type services struct {
	t    theme.Theme
	api  *Fleet
	svcs []Service
```

and keep it at the top of `SetTheme`:

```go
func (s *services) SetTheme(t theme.Theme) {
	s.t = t
```

Then, in `Update`, straight after the group's update:

```go
	if s.opens(msg) {
		if k, ok := s.table.SelectedKey(); ok {
			return s, tea.Batch(append(cmds, screen.Push(newLogs(s.api, k, s.t)))...)
		}
	}
```

`screen.Push` is a command: returning it asks the shell to put the new screen
on top of the stack, run its `Init`, and draw it. All stack operations work
this way (`screen.Pop`, `screen.Replace`, `screen.PopTo`), so a screen never
holds the stack itself.

```go
// opens reports enter typed into the table, or a double click on it.
func (s *services) opens(msg tea.Msg) bool {
	_, isKey := msg.(tea.KeyMsg)
	return (!isKey || s.focus.Is(&s.table)) && s.table.IsActivate(msg)
}
```

`IsActivate` is true for `enter` (when the table's filter isn't taking it)
and for a double click on the table. Enter and double click are the same
verb, "open what's selected", so routing both through one predicate means
the mouse and the keyboard can never drift apart. A single click only
selects; it's always safe to explore with.

The extra condition makes `enter` count only while the table has focus: an
`enter` typed into the detail pane isn't a request to open the table's row.
A double click needs no such check, because it carries the identity of the
component it landed on.

## Run it

Press `enter` on `checkout`:

```
 Services › prod/checkout
┌ prod/checkout logs ──────────────────────────────────────────────────────────────────────────┐
│/                                                                                             │
│───────────────────────────────────────────────────────────────────────────────────────────── │
│14:46:09 INFO  prod/checkout GET /v1/items?page=338 200 63ms                                 ░│
│14:46:10 INFO  prod/checkout POST /v1/orders 201 87ms                                        ░│
│14:46:10 INFO  prod/checkout slow query: 580ms                                               ░│
│14:46:10 WARN  prod/checkout GET /v1/items?page=887 200 63ms                                 ░│
│14:46:10 INFO  prod/checkout slow query: 253ms                                               ░│
│14:46:11 WARN  prod/checkout worker 388 picked up a job                                      ░│
│14:46:12 WARN  prod/checkout cache miss for key=304                                          ░│
│14:46:12 INFO  prod/checkout GET /healthz 200 2ms                                            ░│
│14:46:13 INFO  prod/checkout POST /v1/orders 201 87ms                                        █│
│14:46:13 INFO  prod/checkout slow query: 777ms                                               █│
│14:46:13 WARN  prod/checkout worker 845 picked up a job                                      █│
│                                                                                              │
└ FOLLOWING ───────────────────────────────────────────────────────────────────────────── 100% ┘
 ? help                                                                                  step 4
```

| Do | See |
|---|---|
| wait | lines arrive; the border says `FOLLOWING` |
| `k` | following pauses so you can read; `G` resumes it |
| `/`, type `WARN`, `enter`, then `n`/`N` | jump between matches |
| `\` | show only the matching lines |
| `t` | a new palette; the lines, the query and the scroll position survive |
| `esc` | back to the table, with the cursor, the filter and the detail where you left them |
| click `Services` in the breadcrumb | the same as `esc`: a crumb pops back to its screen |

You didn't write `esc`. The shell pops the stack on `esc` whenever it's deeper
than one screen and nothing is capturing keys, just as it quits on `q` at the
root.

## Covered screens keep running

Keys and mouse events go only to the screen on top. Every other message
reaches every screen on the stack, covered or not. That's why `tailMsg`
needed its `to` field, and it's also what makes step 7 work: the services
screen will poll for changes every two seconds, and it should keep polling
while the user reads logs over it, so that its data is current the moment
they come back. A screen never has to pause its timers when it's covered, or
restart them when it's uncovered.

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

??? example "logs.go"

    ```go
    package main

    import (
    	"time"

    	"github.com/charmbracelet/bubbles/key"
    	tea "github.com/charmbracelet/bubbletea"

    	"github.com/jsdrews/tuilib/pkg/help"
    	"github.com/jsdrews/tuilib/pkg/layout"
    	"github.com/jsdrews/tuilib/pkg/logview"
    	"github.com/jsdrews/tuilib/pkg/screen"
    	"github.com/jsdrews/tuilib/pkg/theme"
    )

    type logs struct {
    	api *Fleet
    	key string
    	log logview.Model
    }

    func newLogs(api *Fleet, key string, t theme.Theme) *logs {
    	s := &logs{api: api, key: key}
    	s.SetTheme(t)
    	return s
    }

    func (s *logs) Title() string         { return s.key }
    func (s *logs) Init() tea.Cmd         { return s.tail() }
    func (s *logs) OnEnter(any) tea.Cmd   { return nil }
    func (s *logs) IsCapturingKeys() bool { return s.log.IsCapturingKeys() }
    func (s *logs) Layout() layout.Node   { return layout.Sized(&s.log) }

    func (s *logs) Help() []key.Binding          { return s.log.Help() }
    func (s *logs) HelpSections() []help.Section { return s.log.HelpSections() }

    // tailMsg carries the lines one tail read returned, addressed to the screen
    // that asked for them.
    type tailMsg struct {
    	to    *logs
    	lines []string
    }

    // tail waits half a second, reads whatever the service has written since,
    // and posts it. Each tailMsg asks for the next one, so the chain runs for as
    // long as this screen is on the stack and stops when it's popped.
    func (s *logs) tail() tea.Cmd {
    	api, key := s.api, s.key
    	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
    		return tailMsg{to: s, lines: api.Logs(key)}
    	})
    }

    func (s *logs) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    	if m, ok := msg.(tailMsg); ok && m.to == s {
    		s.log.AppendLines(m.lines)
    		return s, s.tail()
    	}
    	var cmd tea.Cmd
    	s.log, cmd = s.log.Update(msg)
    	return s, cmd
    }

    func (s *logs) SetTheme(t theme.Theme) {
    	st := s.log.State()
    	opts := t.Logview()
    	opts.Title = s.key + " logs"
    	opts.Searchable = true
    	opts.MaxLines = 2000
    	s.log = logview.New(opts)
    	s.log.Restore(st)
    }
    ```

Next, [verbs](05-verbs.md).

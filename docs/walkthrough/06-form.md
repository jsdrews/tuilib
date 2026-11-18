# 6. A form

Step 4 sent data down the stack: the services screen built the logs screen
with the key it needed. This step sends data back up. A "New service" verb
opens a form; submitting it creates the service and closes the form,
handing the new service to the screen underneath.

## The form screen

Create `create.go`:

```go
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
```

```go
type create struct {
	api  *Fleet
	form form.Model
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
```

A pushed screen again, built with the theme in its constructor. Two things
are new.

`Init` returns the form's `Init`, which starts the text cursor blinking. A
component that needs a command to get going has an `Init`, and the screen
returns it.

`layout.Center(w, h, node)` draws its node at a fixed size in the middle of
the space it's given. A form stretched across a wide terminal is hard to
read, so this one is 56 by 16.

## Fields and validation

```go
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
```

A form is a stack of fields, each a bordered component with its label on the
border: `form.Text`, `form.Password`, `form.Select` and `form.Confirm` (a
yes/no toggle). Each has a `Key`, which is how you read its value back.
`Title` frames the whole form in a pane of its own, which suits a form that
stands alone like this one.

There's no `State`/`Restore` here, and none is needed: a form captures every
key while it's open, so the user can't switch palettes from it.

Validation belongs to the form, not the screen:

```go
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

`Required` rejects an empty field and puts a `*` on its label. `Validate`
receives the field's value and returns why it's unacceptable, or `nil`. The
form checks both when the user submits, and refuses an invalid submit: the
message goes on the field's own border, focus moves to the first field at
fault, and nothing is sent to the screen. After that the flagged fields are
rechecked as the user types, so a correction is acknowledged at once.

So the screen never sees invalid data, and can't act on it by forgetting to
check.

## Submitting and cancelling

```go
func (s *create) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch msg.(type) {
	case form.SubmittedMsg:
		svc := s.service()
		if err := s.api.Create(svc); err != nil {
			return s, app.ErrorOf(err)
		}
		return s, screen.Pop(svc)
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
```

The form reports its outcome as messages. `form.SubmittedMsg` arrives only
once every field is valid; the screen reads the values by key (`String`,
`Bool`, or `Value` for the untyped one) and calls the backend.

If the backend refuses (a service with that name already runs in that
environment), the error goes on the statusbar and the form stays open with
everything the user typed, ready to correct.

If it succeeds, `screen.Pop(svc)` closes the form and hands `svc` to whatever
screen is uncovered. `esc` produces `form.CancelledMsg`, and the screen pops
with nothing. (The shell's own `esc` handling doesn't apply here: the form is
capturing keys, so `esc` reaches it as an ordinary key.)

## Receiving the result

In `services.go`, replace the one-line `OnEnter` with:

```go
// OnEnter runs whenever this screen becomes the top of the stack. A child
// that popped with a value hands it over here.
func (s *services) OnEnter(result any) tea.Cmd {
	created, ok := result.(Service)
	if !ok {
		return nil
	}
	return tea.Batch(s.reload(), app.Info("Created "+created.Key()))
}
```

`OnEnter` runs on the services screen every time something above it pops. Its
argument is whatever the child popped with: a `Service` from the form, `nil`
from the logs screen or a cancelled form. The type switch is how the screen
tells them apart, and it ignores anything it doesn't recognise.

!!! note "A pop is not always a fresh visit"
    The output console is a pushed screen too, and closing it pops with a
    value of its own, `app.OutputClosed`. A screen that refetches its data in
    `OnEnter` would refetch every time the user glanced at the log. If you
    write one, return early on `app.OutputClosed` first. This one only acts on
    a `Service`, so it's safe already.

## A verb that doesn't need a selection

"New service" goes in the action menu, but it isn't about the selected rows,
so it should be offered even when the detail pane has focus. Replace
`Actions` with:

```go
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
			Label: "Restart", Desc: "roll every replica", Group: "write",
			Multi: true, Exclusive: true,
			Run: s.restart(keys),
		},
		action.Action{
			Label: "Delete", Group: "danger", Multi: true,
			Confirm: "Delete " + label + "? This can't be undone.",
			Run:     s.delete(keys),
		},
	)
	return set
}
```

The set starts with the verb that's always available, and the selection's
verbs are appended when there's a selection to act on. `Multi: true` on "New
service" stops the menu disabling it while rows are marked: it doesn't act on
them, so how many there are doesn't matter.

## Run it

Press `a`, choose New service:

```
 Services › New service



                    ┌ new service ─────────────────────────────────────────┐
                    │┌ Name * ───────────────────────────────────────────┐ │
                    ││lowercase-with-dashes                              │ │
                    │└───────────────────────────────────────────────────┘ │
                    │┌ Environment ──────────────────────────────────────┐ │
                    ││▸ prod                                             │ │
                    ││  staging                                          │ │
                    │└───────────────────────────────────────────────────┘ │
                    │┌ Owning team * ────────────────────────────────────┐ │
                    ││                                                   │ │
                    │└───────────────────────────────────────────────────┘ │
                    │┌ Replicas * ───────────────────────────────────────┐ │
                    ││1–10                                               │ │
                    │└───────────────────────────────────────────────────┘ │
                    │[ Create ]                                            │
                    └──────────────────────────────────────────────────────┘



 ? help                                                                        1 output  step 6
```

| Do | See |
|---|---|
| `tab` / `shift+tab`, or click a field | move between fields |
| `tab` to Create, `enter`, with fields empty | refused: each problem on its field's border, focus on the first |
| type `Bad Name` in Name, submit | "lowercase letters, digits, dashes" on the Name border |
| fix it | the complaint clears as you type |
| fill it in properly, submit | back on the table, the new row in it, `Created …` on the statusbar |
| submit `checkout` in `prod` | the backend refuses; the form stays open with your input |
| `esc` | closes the form, creating nothing |

## The files so far

The files this step changed, as they should stand now. Anything not listed is unchanged from the step before.

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
    func (s *services) IsCapturingKeys() bool { return s.focus.IsCapturingKeys() }

    func (s *services) Help() []key.Binding          { return s.focus.Help() }
    func (s *services) HelpSections() []help.Section { return s.focus.HelpSections() }

    // OnEnter runs whenever this screen becomes the top of the stack. A child
    // that popped with a value hands it over here.
    func (s *services) OnEnter(result any) tea.Cmd {
    	created, ok := result.(Service)
    	if !ok {
    		return nil
    	}
    	return tea.Batch(s.reload(), app.Info("Created "+created.Key()))
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
    			Label: "Restart", Desc: "roll every replica", Group: "write",
    			Multi: true, Exclusive: true,
    			Run: s.restart(keys),
    		},
    		action.Action{
    			Label: "Delete", Group: "danger", Multi: true,
    			Confirm: "Delete " + label + "? This can't be undone.",
    			Run:     s.delete(keys),
    		},
    	)
    	return set
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
    	api  *Fleet
    	form form.Model
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
    	switch msg.(type) {
    	case form.SubmittedMsg:
    		svc := s.service()
    		if err := s.api.Create(svc); err != nil {
    			return s, app.ErrorOf(err)
    		}
    		return s, screen.Pop(svc)
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

Next, [live data](07-live-data.md).

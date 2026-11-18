# 2. A table

Step 1 showed text. This step shows data: the fleet's services, in a table
the user can filter and sort. It also turns on palettes, surfaces and the
mouse, which takes three lines in `main` and nothing at all in the screen.

## A backend to talk to

A real app reads from an API, `kubectl` or a database. So that the
walkthrough can stay about the UI, the app gets a pretend backend instead.
Create `fleet.go` and paste this in. Nothing in it is tuilib; skim it for
what it offers and move on.

??? example "fleet.go"

    ```go
    // fleet.go is a pretend backend: a handful of services a real app would
    // reach over an API. It isn't tuilib, and it isn't the point; it stands in
    // for whatever your app talks to.

    package main

    import (
    	"context"
    	"errors"
    	"fmt"
    	"math/rand"
    	"sort"
    	"strings"
    	"sync"
    	"time"
    )

    // The statuses a service can be in. Deploying and Restarting are the two
    // that mean "work in progress".
    const (
    	Healthy    = "Healthy"
    	Degraded   = "Degraded"
    	Down       = "Down"
    	Deploying  = "Deploying"
    	Restarting = "Restarting"
    )

    // Service is one row of the fleet.
    type Service struct {
    	Name     string
    	Env      string
    	Owner    string
    	Version  string
    	Replicas int
    	Ready    int
    	Status   string
    }

    // FleetOptions shape how the backend behaves.
    type FleetOptions struct {
    	// Latency is how long each read and write takes, as a remote API would.
    	// Zero answers at once.
    	Latency time.Duration
    	// Drift makes services change between reads: a replica fails, a rollout
    	// starts and finishes. It is what gives a polling screen something to
    	// show.
    	Drift bool
    }

    // Fleet is the backend.
    type Fleet struct {
    	opts FleetOptions

    	mu        sync.Mutex
    	svcs      []Service
    	deploying map[string]int // reads left before a rollout finishes
    	rng       *rand.Rand
    }

    // NewFleet returns a fleet seeded with the same eight services every time.
    func NewFleet(opts FleetOptions) *Fleet {
    	return &Fleet{
    		opts:      opts,
    		deploying: map[string]int{},
    		rng:       rand.New(rand.NewSource(1)),
    		svcs: []Service{
    			{"api-gateway", "prod", "platform", "2.14.0", 3, 3, Healthy},
    			{"checkout", "prod", "payments", "1.8.2", 4, 3, Degraded},
    			{"search", "prod", "discovery", "3.1.0", 2, 2, Healthy},
    			{"notifications", "prod", "growth", "0.9.7", 2, 2, Healthy},
    			{"legacy-billing", "prod", "payments", "0.3.1", 1, 0, Down},
    			{"api-gateway", "staging", "platform", "2.15.0-rc1", 1, 1, Healthy},
    			{"checkout", "staging", "payments", "1.9.0-rc2", 1, 1, Healthy},
    			{"recommendations", "staging", "discovery", "0.1.4", 2, 1, Degraded},
    		},
    	}
    }

    // Key is a service's identity. The same name runs in more than one
    // environment, so the name alone is not one.
    func (s Service) Key() string { return s.Env + "/" + s.Name }

    // List returns every service, sorted by environment then name.
    func (f *Fleet) List() ([]Service, error) {
    	f.wait(context.Background())
    	f.mu.Lock()
    	defer f.mu.Unlock()
    	if f.opts.Drift {
    		f.drift()
    	}
    	out := append([]Service(nil), f.svcs...)
    	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
    	return out, nil
    }

    // Create adds a service. It refuses a key that already exists.
    func (f *Fleet) Create(s Service) error {
    	f.wait(context.Background())
    	f.mu.Lock()
    	defer f.mu.Unlock()
    	if f.find(s.Key()) >= 0 {
    		return fmt.Errorf("create %s: %w", s.Key(), ErrExists)
    	}
    	s.Ready, s.Status = s.Replicas, Healthy
    	f.svcs = append(f.svcs, s)
    	return nil
    }

    // ErrExists is returned by Create for a key already in the fleet.
    var ErrExists = errors.New("service already exists")

    // Delete removes a service.
    func (f *Fleet) Delete(key string) error {
    	f.wait(context.Background())
    	f.mu.Lock()
    	defer f.mu.Unlock()
    	i := f.find(key)
    	if i < 0 {
    		return fmt.Errorf("delete %s: no such service", key)
    	}
    	f.svcs = append(f.svcs[:i], f.svcs[i+1:]...)
    	return nil
    }

    // Restart rolls a service's replicas. It takes a couple of seconds, during
    // which List reports the service as Restarting, and it can be cancelled.
    // legacy-billing never comes back, so there is a failure to look at.
    func (f *Fleet) Restart(ctx context.Context, key string) error {
    	f.mu.Lock()
    	i := f.find(key)
    	if i < 0 {
    		f.mu.Unlock()
    		return fmt.Errorf("restart %s: no such service", key)
    	}
    	was := f.svcs[i]
    	f.svcs[i].Status = Restarting
    	f.mu.Unlock()

    	select {
    	case <-ctx.Done():
    	case <-time.After(2 * time.Second):
    	}

    	f.mu.Lock()
    	defer f.mu.Unlock()
    	if i = f.find(key); i < 0 {
    		return fmt.Errorf("restart %s: deleted while restarting", key)
    	}
    	if ctx.Err() != nil {
    		f.svcs[i].Status = was.Status
    		return ctx.Err()
    	}
    	if was.Name == "legacy-billing" {
    		f.svcs[i].Status = Down
    		return fmt.Errorf("restart %s: %w", key, errors.New("health check failed: connection refused on :8443"))
    	}
    	f.svcs[i].Ready, f.svcs[i].Status = f.svcs[i].Replicas, Healthy
    	return nil
    }

    var (
    	logLevels = []string{"INFO ", "INFO ", "INFO ", "DEBUG", "WARN "}
    	logLines  = []string{
    		"GET /healthz 200 2ms",
    		"GET /v1/items 200 41ms",
    		"POST /v1/orders 201 87ms",
    		"cache miss for key=%d",
    		"slow query: %dms",
    		"worker %d picked up a job",
    		"GET /v1/items?page=%d 200 63ms",
    	}
    )

    // Logs returns the lines a service has written since the last call: none,
    // one or a few, the way a tail does.
    func (f *Fleet) Logs(key string) []string {
    	f.mu.Lock()
    	defer f.mu.Unlock()
    	n := f.rng.Intn(4)
    	out := make([]string, n)
    	for i := range out {
    		line := logLines[f.rng.Intn(len(logLines))]
    		if strings.Contains(line, "%d") {
    			line = fmt.Sprintf(line, f.rng.Intn(900)+1)
    		}
    		out[i] = fmt.Sprintf("%s %s %s %s", time.Now().Format("15:04:05"), logLevels[f.rng.Intn(len(logLevels))], key, line)
    	}
    	return out
    }

    func (f *Fleet) find(key string) int {
    	for i, s := range f.svcs {
    		if s.Key() == key {
    			return i
    		}
    	}
    	return -1
    }

    func (f *Fleet) wait(ctx context.Context) {
    	if f.opts.Latency <= 0 {
    		return
    	}
    	select {
    	case <-ctx.Done():
    	case <-time.After(f.opts.Latency):
    	}
    }

    // drift moves the fleet on by one read: rollouts in flight advance, and now
    // and then a healthy service loses a replica or starts a rollout.
    func (f *Fleet) drift() {
    	for k, left := range f.deploying {
    		i := f.find(k)
    		switch {
    		case i < 0:
    			delete(f.deploying, k)
    		case left <= 1:
    			delete(f.deploying, k)
    			f.svcs[i].Status, f.svcs[i].Ready = Healthy, f.svcs[i].Replicas
    			f.svcs[i].Version = bump(f.svcs[i].Version)
    		default:
    			f.deploying[k] = left - 1
    		}
    	}
    	if f.rng.Intn(3) != 0 {
    		return
    	}
    	i := f.rng.Intn(len(f.svcs))
    	s := &f.svcs[i]
    	if s.Status != Healthy {
    		return
    	}
    	if f.rng.Intn(2) == 0 && s.Replicas > 1 {
    		s.Ready, s.Status = s.Replicas-1, Degraded
    		return
    	}
    	s.Status = Deploying
    	f.deploying[s.Key()] = 3
    }

    // bump increments a version's patch number: 1.8.2 → 1.8.3.
    func bump(v string) string {
    	var major, minor, patch int
    	if _, err := fmt.Sscanf(v, "%d.%d.%d", &major, &minor, &patch); err != nil {
    		return v
    	}
    	return fmt.Sprintf("%d.%d.%d", major, minor, patch+1)
    }
    ```

What the rest of the walkthrough uses:

| Call | Does |
|---|---|
| `NewFleet(FleetOptions{})` | eight services; `Latency` and `Drift` make it behave like a remote system (step 7) |
| `List()` | every service, sorted |
| `Create(svc)`, `Delete(key)` | add and remove |
| `Restart(ctx, key)` | takes two seconds; `legacy-billing` always fails |
| `Logs(key)` | the lines a service wrote since the last call |
| `svc.Key()` | `"prod/checkout"`: a service's identity, since a name runs in more than one environment |

## The screen

Create `services.go`. The imports:

```go
import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/ansi"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)
```

The screen holds the backend, the data it last read, and a table:

```go
type services struct {
	api   *Fleet
	svcs  []Service
	table table.Model
}

func newServices(api *Fleet) *services {
	return &services{api: api}
}
```

Keep the data and the component apart. `svcs` is what the screen knows about
the fleet; `table` is how it's displayed. Later steps look services up in
`svcs` by key, and the table never needs to know what a `Service` is.

The constructor takes what the screen needs (the backend) and nothing else.
It doesn't build the table, because `SetTheme` does that, and the shell calls
`SetTheme` before the screen is drawn.

## The methods

```go
func (s *services) Title() string         { return "Services" }
func (s *services) Init() tea.Cmd         { return s.reload() }
func (s *services) OnEnter(any) tea.Cmd   { return nil }
func (s *services) IsCapturingKeys() bool { return s.table.IsCapturingKeys() }
func (s *services) Layout() layout.Node   { return layout.Sized(&s.table) }

func (s *services) Help() []key.Binding          { return s.table.Help() }
func (s *services) HelpSections() []help.Section { return s.table.HelpSections() }

func (s *services) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.table, cmd = s.table.Update(msg)
	return s, cmd
}
```

These are step 1's methods with `table` where `text` was, and that's the
point: every component has the same surface (`Update`, `Help`,
`HelpSections`, `IsCapturingKeys`, `SetRect`, `State`, `Restore`), so the
screen around it doesn't change shape when the component does.

The one difference is `Init`, which now loads the data. It runs after the
shell's first `SetTheme`, so the table exists by then.

## Building the table

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
}
```

The same five-line shape as the textview, with a table's options in the
middle:

- `Filterable` puts a filter row inside the pane, focused with `/`.
- `Columns` declares the layout. `Width` is the column's width in visible
  cells. `Flex: 1` lets `Service` take whatever width is left over, with
  `Width` as its minimum, so the table fills its pane at any size.
  `Sortable` opts a column into sorting (and makes it two cells wider, so
  the ▲/▼ marker always fits).

Notice what's missing: the rows. A table's `State` includes its rows, so
`Restore` brings them back on every rebuild after the first. The rows go in
somewhere else.

## Loading the rows

```go
// reload reads the fleet and hands it to the table.
func (s *services) reload() tea.Cmd {
	svcs, err := s.api.List()
	if err != nil {
		return app.ErrorOf(err)
	}
	s.svcs = svcs
	s.table.SetKeyedRows(rowsOf(svcs))
	return nil
}
```

If the read fails, `app.ErrorOf(err)` returns a command that shows the error
on the statusbar. Screens never draw the statusbar themselves; they return
`app.Info`, `app.Error` or `app.ErrorOf` and the shell paints it.

The backend is in memory, so calling it directly from `Init` is fine for
now. A real backend is slow, and calling it here would freeze the UI until it
answered. Step 7 moves the read into a command.

```go
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
```

Each row is a `KeyedRow`: its cells, in column order, plus a `Key` that
identifies it. Always key rows that come from data that can change. When the
rows are replaced, the table finds the cursor's key in the new set and keeps
the cursor on it, even if the order changed underneath. Without keys, a
refresh moves the cursor onto whatever row now sits at the same index. Keys
also make marking possible (step 5).

`Data` carries the record itself. The table ignores it except where you ask
it to look (step 7).

```go
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

To colour a cell, use `ansi.CellColor` with a palette index, not a Lip Gloss
style. A Lip Gloss render ends with a full reset, which would cut the
highlighted row's background off halfway along. `CellColor` sets and resets
only the foreground.

## main turns on palettes, surfaces and the mouse

Replace `main.go` with:

```go
import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/surface"
)
```

```go
func main() {
	m := app.New(app.Options{
		Root:       newServices(NewFleet(FleetOptions{})),
		Palettes:   palette.All(),
		Surfaces:   surface.Builtins(),
		ThemeKey:   key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "palette")),
		SurfaceKey: key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "surface")),
		Mouse:      app.MouseClick,
		Version:    "step 2",
	})
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

The `home` screen from step 1 is gone; delete it, or keep it in another file
if you want it for reference (it won't be used).

The new options:

- `Palettes` is the set of colour schemes the user can cycle through, and
  `ThemeKey` is the key that cycles them. `palette.All()` is every built-in
  one.
- `Surfaces` is how components are drawn: a line border (the default), a
  filled card, a header band, a drop shadow. `SurfaceKey` cycles them,
  independently of the palette.
- `Mouse: app.MouseClick` turns on clicks, the wheel and dragging.

Each of these keys is off unless you set it, because a key the shell claims
is one no screen in your app can use for anything else.

## Run it

```sh
go run .
```

```
 Services
┌ services ────────────────────────────────────────────────────────────────────────────┐
│/ filter: checkout, env:prod, status:~^de                                             │
│───────────────────────────────────────────────────────────────────────────────────── │
│Service                                         │ Env        │ Status         │ Ready │
│───────────────────────────────────────────────────────────────────────────────────── │
│api-gateway                                     │ prod       │ ● Healthy      │   3/3 │
│checkout                                        │ prod       │ ● Degraded     │   3/4 │
│legacy-billing                                  │ prod       │ ● Down         │   0/1 │
│notifications                                   │ prod       │ ● Healthy      │   2/2 │
│search                                          │ prod       │ ● Healthy      │   2/2 │
│api-gateway                                     │ staging    │ ● Healthy      │   1/1 │
│checkout                                        │ staging    │ ● Healthy      │   1/1 │
│recommendations                                 │ staging    │ ● Degraded     │   1/2 │
│                                                                                      │
│                                                                                      │
└ 8 / 8 ──────────────────────────────────────────────────────────────────────── 1 / 8 ┘
 ? help                                                                          step 2
```

Things to try:

| Do | See |
|---|---|
| `j`/`k` or `↓`/`↑`, `g`/`G` | the cursor moves; the counter bottom-right follows |
| `/`, type `checkout`, `enter` | the rows narrow; `enter` hands the keys back to the table |
| `/`, type `env:prod status:~^de` | terms are ANDed; `env:` scopes to a column; `~` makes a regex |
| `/`, type `env:st`, then `tab` | the filter lists the column's matching values; `tab` completes to `staging` |
| `]` / `[`, then `s` | step the sort column, flip its direction |
| click a column header | sorts by it |
| click a row, scroll the wheel | moves the cursor |
| `t`, `T` | a new palette, a new surface; the cursor, the filter and the sort stay put |
| `?` | every key above, grouped by what it does |

That last row is `State`/`Restore` paying for itself. Each `t` calls
`SetTheme`, which builds a brand-new table, and the user can't tell.

All of the filtering, sorting, completion and mouse handling came from the
component. The screen wrote none of it, and the same is true of every
component: they arrive with their keys, their mouse handling and their help
already done. The keys are also the same across components. `j`/`k`, `g`/`G`
and `/` work alike in a table, a list, a tree and a log, so users learn them
once.

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
    		Mouse:      app.MouseClick,
    		Version:    "step 2",
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
    	"fmt"

    	"github.com/charmbracelet/bubbles/key"
    	tea "github.com/charmbracelet/bubbletea"
    	"github.com/charmbracelet/lipgloss"

    	"github.com/jsdrews/tuilib/pkg/ansi"
    	"github.com/jsdrews/tuilib/pkg/app"
    	"github.com/jsdrews/tuilib/pkg/help"
    	"github.com/jsdrews/tuilib/pkg/layout"
    	"github.com/jsdrews/tuilib/pkg/screen"
    	"github.com/jsdrews/tuilib/pkg/table"
    	"github.com/jsdrews/tuilib/pkg/theme"
    )

    type services struct {
    	api   *Fleet
    	svcs  []Service
    	table table.Model
    }

    func newServices(api *Fleet) *services {
    	return &services{api: api}
    }

    func (s *services) Title() string         { return "Services" }
    func (s *services) Init() tea.Cmd         { return s.reload() }
    func (s *services) OnEnter(any) tea.Cmd   { return nil }
    func (s *services) IsCapturingKeys() bool { return s.table.IsCapturingKeys() }
    func (s *services) Layout() layout.Node   { return layout.Sized(&s.table) }

    func (s *services) Help() []key.Binding          { return s.table.Help() }
    func (s *services) HelpSections() []help.Section { return s.table.HelpSections() }

    func (s *services) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    	var cmd tea.Cmd
    	s.table, cmd = s.table.Update(msg)
    	return s, cmd
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
    }

    // reload reads the fleet and hands it to the table.
    func (s *services) reload() tea.Cmd {
    	svcs, err := s.api.List()
    	if err != nil {
    		return app.ErrorOf(err)
    	}
    	s.svcs = svcs
    	s.table.SetKeyedRows(rowsOf(svcs))
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

Next, [two panes](03-two-panes.md).

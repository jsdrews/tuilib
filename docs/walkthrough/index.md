# Walkthrough

This walkthrough builds one app from an empty directory, a piece at a time:
`fleet`, a console for a handful of services. You write every line. Each step
adds one idea, explains the code as it goes in, and ends with something you
can run.

By the end the app has:

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

- a filterable, sortable table of services beside a detail pane, with focus
  moving between them by keyboard or mouse;
- a logs screen you drill into, streaming lines while you read;
- verbs in a menu (view logs, restart, delete) acting on one service or on
  several marked ones, with a confirmation before anything destructive;
- a form for creating a service, validated before it's submitted;
- data that refreshes every two seconds, with a spinner on just the rows the
  server is busy with;
- palette and surface switching that never loses the user's place.

None of that is special to services. It's the shape of most tools that sit in
front of a remote system (Kubernetes, CI, a job scheduler, a REST API), and by
the last step you'll have used every part of tuilib that such a tool needs.

## The steps

| Step | You add | You learn |
|---|---|---|
| [1. The shell and a screen](01-shell.md) | a screen with a page of text | the `Screen` interface, the app shell, theme builders, `State`/`Restore` |
| [2. A table](02-table.md) | the services, in a table | keyed rows, filter and sort, palettes and surfaces, the mouse |
| [3. Two panes](03-two-panes.md) | a detail pane beside the table | `HStack`, `Fixed`/`Flex`, `focus.Group`, how messages are routed |
| [4. Drilling down](04-drill-down.md) | a logs screen behind `enter` | the screen stack, the breadcrumb, streaming with a chained command |
| [5. Verbs](05-verbs.md) | restart and delete, on marked rows | multi-select, the action menu, confirmation, the output console |
| [6. A form](06-form.md) | a "new service" form | `pkg/form`, validation, `screen.Pop(result)` and `OnEnter` |
| [7. Live data](07-live-data.md) | polling, async reads, row spinners | `pkg/poll`, `SetLoading`, keyed refreshes, `pkg/activity` |
| [What next](next.md) | | testing a screen, and where the rest of the library fits |

Every step is a complete program. If something doesn't build, each step ends
with its files in full to compare against.

## Before you start

You need Go 1.25 or later, and a terminal at least 100 columns wide.

```sh
mkdir fleet && cd fleet
go mod init example.com/fleet
go get github.com/jsdrews/tuilib@latest
```

All of the app is one `main` package in this directory. When a step adds an
import you haven't fetched yet (Bubble Tea, Bubbles and Lip Gloss come in
with tuilib), `go mod tidy` picks it up.

A tuilib app is a [Bubble Tea](https://github.com/charmbracelet/bubbletea)
program, so it helps to know its loop: a model receives a message in
`Update`, returns a new model and a command, and the runtime runs the command
and delivers whatever message it produces back to `Update`. You won't write a
`tea.Model` yourself; the library's app shell is one, and you write screens
that it hosts.

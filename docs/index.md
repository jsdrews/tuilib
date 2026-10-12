# tuilib

A component library for building [Bubble Tea](https://github.com/charmbracelet/bubbletea)
terminal UIs quickly and reliably, aimed at TUIs over remote systems:
Kubernetes, CI runs, job schedulers, REST APIs.

```
 Deployments
┌ Deployments ────────────────────────────────────┐┌ api ────────────────────────────┐
│/                                                ││  image   : api:1.42.0           │
│──────────────────────────────────────────────── ││▾ labels                         │
│Name           │ Namespace │ Ready │ Status      ││    app : api                    │
│──────────────────────────────────────────────── ││    tier: backend                │
│api            │ prod      │   3/3 │ Running     ││  replicas: 3                    │
│web            │ prod      │   2/3 │ Progressing ││  strategy: RollingUpdate        │
│worker         │ prod      │   4/4 │ Running     ││                                 │
│cache-redis    │ infra     │   1/1 │ Running     ││                                 │
│cron-reports   │ batch     │   0/1 │ Failed      ││                                 │
│                                                 ││                                 │
└ 5 / 5 ─────────────────────────────────── 1 / 5 ┘└────────────────────────── 1 / 6 ┘
 ? help                                                                       v0.42.0
```

A filterable `table` and an `inspector` side by side under the app shell, as
rendered by the library (colors stripped).

## What you get

- **An app shell** (`pkg/app`) that owns the breadcrumb, the statusbar, theme
  cycling, global keys, the `?` key overlay, mouse support and an app-wide
  output console. You write screens; the shell does the chrome.
- **A screen stack** (`pkg/screen`) with push, pop and result passing, so
  "pick something and come back with it" is two lines.
- **A declarative layout engine** (`pkg/layout`): `VStack`, `HStack`,
  `ZStack`, `Fixed`, `Flex`. No `height - 2` arithmetic anywhere.
- **Themed components** that each own their bordered pane: list, table, tree,
  inspector, text and log viewers, an eventlog for paged timelines, inputs,
  toggles, forms, modals, tabs, an action menu and inline metrics.
- **Remote data built in**: tables and eventlogs that page through sources
  too large to hold, with stale-while-loading, cancellation and query history
  handled by the library rather than every screen.
- **Per-row activity**: spinners on just the rows that are busy, driven by
  your server's own status.
- **One palette struct** (`pkg/theme`) and 19 built-in palettes (plus one read from your terminal), swappable live
  without losing a cursor, filter or selection.

## Install

tuilib needs Go 1.25 or later.

```sh
go get github.com/jsdrews/tuilib@latest
```

Every package lives under `github.com/jsdrews/tuilib/pkg/...`; import the ones
you use. To browse every component in a running app, clone the repo and run the
examples launcher:

```sh
git clone https://github.com/jsdrews/tuilib.git
cd tuilib
go run ./examples/launcher     # or: task examples
```

## Where to go

<div class="grid cards" markdown>

- **[Quick start](quickstart.md)**: a two-screen app in about 100 lines.
- **[Architecture](architecture.md)**: how the shell, screens, layout and
  components fit together, and how messages move between them.
- **[Guide](guide/app-shell.md)**: one page per concern: navigation,
  layout, theming, focus and mouse, feedback, remote data, subprocesses.
- **[Components](components/index.md)**: every component, its options, its
  keys and its messages.
- **[Reference](reference/packages.md)**: the non-visual packages, every
  default key binding, the theme tokens and the library's rules.
- **[Examples](examples.md)**: the demo catalog and what each one shows.

</div>

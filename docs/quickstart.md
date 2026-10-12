# Quick start

This page builds a small app: a filterable list of cities that opens a detail
screen for the one you pick. Along the way it covers the four things every
tuilib app does: implement a screen, lay it out, theme it, and hand it to the
app shell.

## Install

```sh
mkdir cities && cd cities
go mod init example.com/cities
go get github.com/jsdrews/tuilib@latest
```

## The whole program

Save this as `main.go`. It is the repo's `examples/quickstart/main.go`, which
is compiled in CI, so it builds against the current API.

```go
--8<-- "examples/quickstart/main.go"
```

Run it:

```sh
go run .
```

Then try:

| Press | What happens |
|---|---|
| `↓`/`j`, `↑`/`k` | move the cursor |
| `/`, type `o`, `enter` | filter the list, then hand the keys back to it |
| `enter` (or double-click) | push the detail screen |
| `esc` | pop back; the cursor and filter are where you left them |
| `t` | cycle themes; nothing you did is lost |
| `?` | open the key overlay, grouped by what each key does |
| `q` | quit |

## What each piece is doing

### A screen is eight methods

`screen.Screen` is the unit the stack manages:

| Method | Purpose |
|---|---|
| `Title()` | the screen's crumb in the breadcrumb trail |
| `Init()` | runs once when the screen is constructed |
| `Update(msg)` | handles one message, returns the screen and a command |
| `Layout()` | returns a `layout.Node` tree the shell renders in the body |
| `Help()` | the bindings shown in the footer and the `?` overlay |
| `OnEnter(result)` | runs each time the screen becomes the top of the stack |
| `SetTheme(t)` | rebuilds themed components when the palette changes |
| `IsCapturingKeys()` | true while a text field owns the keyboard |

`HelpSections()` is optional. Forwarding it from the component groups the
`?` overlay by function (Navigate, Filter, …) instead of one flat list. See
[Help and key hints](guide/help.md).

### Layout is declared, not computed

`Layout()` returns `layout.Sized(&s.list)`: the list fills the body. The
shell gives it a rect after subtracting the breadcrumb and statusbar, so the
screen never knows the terminal size. A two-pane screen would return:

```go
return layout.HStack(
    layout.Fixed(28, layout.Sized(&s.list)),
    layout.Flex(1, layout.Sized(&s.detail)),
)
```

See [Layout](guide/layout.md).

### Components start from the theme

`t.List()` returns `list.Options` with every color, glyph and key binding
filled in from the palette. You set only what's specific to this list
(`Title`, `Items`, `Filterable`) and call `list.New`. Never set `Width` or
`Height` on a component you place with `layout.Sized`; the layout engine sizes
it.

### `SetTheme` rebuilds, `State`/`Restore` keep what the user did

When the user presses `t`, the shell calls `SetTheme` on every screen in the
stack. Each screen rebuilds its components from the new theme and carries
their state across:

```go
st := s.list.State()   // items, cursor, filter, marks, scroll…
s.list = list.New(opts)
s.list.Restore(st)
```

The shell calls `SetTheme` once before the first render too. A `State` taken
from a component that was never built restores nothing, so the same three
lines double as the constructor. See [Theming and state](guide/theming.md).

### Navigation is a stack

`screen.Push(child)` puts a screen on top. `esc` pops it (the shell does
this), and `screen.Pop(value)` pops it with a result that arrives in the
parent's `OnEnter(value)`. See [Screens and navigation](guide/screens.md).

### `IsActivate` is "open the selection"

`s.list.IsActivate(msg)` is true for `enter` (when the filter isn't taking
input) and for a double-click on the list. Routing both through one
predicate means mouse and keyboard can never drift apart.

### The shell owns the global keys

`q`, `esc`-to-pop, `?`, `t` and `ctrl+z` are handled by `app.Model`, and are
suppressed while `IsCapturingKeys()` is true. That's why typing `q` into the
filter doesn't quit.

## Next steps

- Read [Architecture](architecture.md) for the full picture.
- Run `task examples` (or `go run ./examples/launcher`) in a clone of the
  repo and open any demo; each one names the directory its code lives in.
- Browse the [component reference](components/index.md).

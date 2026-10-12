# Screens and navigation

## The Screen interface

```go
type Screen interface {
    Title() string
    Init() tea.Cmd
    Update(msg tea.Msg) (Screen, tea.Cmd)
    Layout() layout.Node
    Help() []key.Binding
    OnEnter(result any) tea.Cmd
    SetTheme(t theme.Theme)
    IsCapturingKeys() bool
}
```

| Method | Called | Notes |
|---|---|---|
| `Title` | every render | Its crumb in the breadcrumb. Free to change, e.g. `"Pods › " + selected`. |
| `Init` | once, when the screen is pushed | Usually `nil`, or the batched `Init` of a poll or focus group. |
| `Update` | every message routed to the screen | Return the same pointer and any command. |
| `Layout` | every render | Return a `layout.Node`; the shell renders it in the body rect. |
| `Help` | every render | Bindings for the footer and the overlay. |
| `OnEnter` | each time the screen becomes the top | `result` is `nil` on push, or whatever a child popped with. Start fetches here. |
| `SetTheme` | before first render, and on every theme change | Rebuild themed components, carrying `State`. |
| `IsCapturingKeys` | before global keys are handled | True while a text field or filter owns input. |

Implement screens as pointer types so `Update`, `OnEnter` and `SetTheme` can
mutate fields. Optional interfaces add more:

| Interface | Method | Adds |
|---|---|---|
| `help.Sectioned` | `HelpSections() []help.Section` | a grouped `?` overlay ([Help](help.md)) |
| `action.Provider` | `Actions() action.Set` | an action menu ([Verbs](actions.md)) |

## Pushing, popping, replacing

All stack operations are commands returned from `Update`:

```go
return s, screen.Push(newDetail(item, s.t))   // open a child
return s, screen.Pop(chosen)                  // close, handing a value back
return s, screen.Pop(nil)                     // close without a value
return s, screen.Replace(newList(s.t))        // swap the top for a fresh instance
return s, screen.PopTo(1, nil)                // unwind to the root
```

| Command | Effect |
|---|---|
| `Push(s)` | `s.Init()` and `s.OnEnter(nil)` run; `s` becomes the top. |
| `Pop(result)` | The top is discarded; the uncovered screen's `OnEnter(result)` runs. |
| `PopTo(depth, result)` | Unwinds to `depth` screens (1 = root). Screens in between are discarded without being activated. A depth at or above the current one is a no-op. |
| `Replace(s)` | Swaps the top for `s` in one step. `s.Init` and `s.OnEnter(nil)` run; the screen below is not notified. |

Push and Replace don't call `SetTheme`: construct the new screen with its theme
already applied. Keep the current theme in a field (`s.t`) for this.

Under the shell, `esc` pops automatically when the stack is deeper than one and
no screen is capturing keys, and clicking a crumb in the breadcrumb is a
`PopTo`.

## Passing data

Parent to child: pass it to the child's constructor.

```go
if s.list.IsActivate(msg) {
    if city, ok := s.list.Selected(); ok {
        return s, screen.Push(newCityDetail(city, s.t))
    }
}
```

Child to parent: pop with a value, and match it in the parent's `OnEnter`.

```go
// in the picker
return s, screen.Pop(tz)

// in the parent
func (s *cityDetail) OnEnter(result any) tea.Cmd {
    if tz, ok := result.(string); ok && tz != "" {
        s.chosen = tz
        s.rebuildInfo()
    }
    return nil
}
```

## OnEnter and fetches

`OnEnter` is where a screen starts work it needs whenever it's shown, such as
a fetch. Because `Pop` always fires `OnEnter`, a screen that fetches there
must ignore the pop from the output console, or glancing at the log refetches
silently:

```go
func (s *Screen) OnEnter(result any) tea.Cmd {
    if _, closed := result.(app.OutputClosed); closed {
        return nil // a glance at the log is not a fresh activation
    }
    return s.fetch()
}
```

## Fresh instances with Replace

Use `Replace` when the user wants "this view, from scratch": reset filter and
scroll, refetch. A `Pop` followed by a `Push` flickers and fires `OnEnter` on
the parent, which may start a fetch the user never asked for.

## A login-style prescreen

A root screen can push a child from its own `OnEnter` (for a login form, say),
take the result when it pops, and push it again later on logout. The child
never lives permanently on the stack. See `examples/shell/prescreen`.

## Messages reach covered screens

Keys and mouse go to the top screen only. Every other message (timers, fetch
results, spinner ticks) reaches every screen on the stack. A covered screen
therefore keeps its poll chain and spinners alive and is up to date when you
return to it. Don't gate a screen's timers on being visible.

## Tabs are not navigation

`pkg/tab` hosts several screens behind a one-row strip inside a single host
screen. It never touches the stack. See [Tabs](../components/tabs.md).

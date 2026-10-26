# Layout

`pkg/layout` describes what goes where; the engine divides the cells. A
screen's `Layout()` returns a tree of `layout.Node`, and the shell renders it
into the body rect.

```go
func (s *Screen) Layout() layout.Node {
    return layout.VStack(
        layout.Fixed(3, layout.Sized(&s.query)),        // a 3-row input
        layout.Flex(1, layout.HStack(                   // the rest of the height
            layout.Fixed(28, layout.Sized(&s.sidebar)), // 28 columns
            layout.Flex(2, layout.Sized(&s.table)),     // 2/3 of what's left
            layout.Flex(1, layout.Sized(&s.detail)),    // 1/3 of what's left
        )),
    )
}
```

## Primitives

| Function | Does |
|---|---|
| `VStack(items...)` | Stacks items top to bottom. |
| `HStack(items...)` | Stacks items left to right. |
| `Fixed(n, node)` | An item that takes exactly `n` cells on the stack's axis. |
| `Flex(weight, node)` | An item that takes a share of what Fixed items leave. `Flex(2)` beside `Flex(1)` gets twice as much. Non-positive weights count as 1. |
| `Sized(&c)` | Adapts a component (anything with `SetRect(geom.Rect)` and `View()`). Pass a pointer. |
| `Bar(&c)` | `Sized` under a name that says "one row"; use it in a `Fixed(1, …)` slot. |
| `ZStack(base, overlay)` | Draws `overlay` on top of `base`. Space cells in the overlay let the base show through. |
| `Center(w, h, node)` | Renders `node` at `w`×`h`, centered in its rect. The standard modal wrapper. |
| `RenderFunc(func(r geom.Rect) string)` | Escape hatch: render arbitrary content into `r.W`×`r.H`. |

## No size arithmetic

Never write `height - 2` to leave room for a bar. Put the bar in a
`Fixed(1, …)` sibling and let the body `Flex`. Never set `Width`/`Height` in a
component's Options when it's placed with `Sized`; the engine overwrites them
on every render.

## Rects carry position

A `Node` is rendered into a `geom.Rect{X, Y, W, H, Gen}`: absolute terminal
position and size, plus the render generation it was stamped in. Components
keep the rect they were given and hit-test the mouse against it. Nothing is
injected into the rendered string, so truncation and overlays can't corrupt
click targets.

`Rect.Hit(x, y)` is `Contains` plus a check that the rect is from the current
frame. A component that wasn't drawn this frame (a hidden tab body, a
dismissed modal) declines clicks without knowing it's hidden. See
[`pkg/geom`](../reference/packages.md#geom).

## Modals

A modal is a `ZStack` whose overlay is centered over the base layout:

```go
func (s *Screen) Layout() layout.Node {
    base := layout.Sized(&s.table)
    if s.confirmUp {
        return layout.ZStack(base, layout.Center(48, 7, layout.Sized(&s.confirm)))
    }
    return base
}
```

Both layers get the same rect, so the base still thinks it owns the cells the
modal covers. Occlusion is the screen's job: while a modal is up, forward
messages to the modal only and return `true` from `IsCapturingKeys()`.

Self-sizing overlays (an autosize `alert`, the `action` menu, the help
overlay) measure their content and center or anchor themselves, so they go in
with `layout.Sized` alone, without `Center`.

## Escape hatch

`RenderFunc` renders anything that isn't a component:

```go
layout.Fixed(1, layout.RenderFunc(func(r geom.Rect) string {
    return lipgloss.NewStyle().Width(r.W).Render("last refreshed 3s ago")
}))
```

## Rendering outside the shell

The root of a render calls `geom.NextGen()` once per frame and seeds its rect
with `geom.New`:

```go
geom.NextGen()
out := root.Render(geom.New(0, 0, termW, termH))
```

A nested root (as `pkg/tab` does for its active body) must not call
`NextGen`; it renders inside the frame it was given.

See `examples/appearance/layouts` for every primitive on its own sub-screen.

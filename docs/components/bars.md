# Breadcrumb and statusbar

The app shell's header and footer. Under `pkg/app` you don't create either;
the shell owns them, feeds the breadcrumb from the stack's titles, and paints
statusbar messages from `app.Info` / `app.Error`. Use these packages directly
only in a program that doesn't use the shell. Neither owns a pane: each
renders exactly one row, and goes in a `layout.Fixed(1, layout.Bar(&c))` slot.

## Breadcrumb

`pkg/breadcrumb` renders `Root › Modules › dev`. The last crumb is the
current one (bold); earlier crumbs are muted. When the line is too wide,
leading crumbs collapse into `…` so the current one is always visible.

```go
bc := breadcrumb.New(t.Breadcrumb())
bc.SetCrumbs(stack.Crumbs())
```

| Option | Default | Meaning |
|---|---|---|
| `Crumbs` | | Initial crumbs, root first. |
| `Separator` | `" › "` | Between crumbs. |
| `Ellipsis` | `"…"` | Replaces collapsed leading crumbs. |
| `BarBackground`, `BarForeground` | 236 / 252 | One color pair for the whole strip. |
| `BarStyle`, `CrumbStyle`, `CurrentStyle`, `SeparatorStyle` | derived | Style overrides (pointers; `nil` = default). |

Methods: `SetCrumbs`, `SetRect`, `Width`, `Rect`, `CrumbAt(x, y)` (which crumb
a click hit; the shell turns it into `screen.PopTo`), `BarBackground`,
`BarForeground`.

## Statusbar

`pkg/statusbar` is a one-line strip with three slots: a static left (the help
affordance), a center message region, and a static right (console badge and
version). The center has three states: neutral, info and error.

```go
sb := statusbar.New(t.Statusbar(helpLine, "v1.2.0"))
sb.SetInfo("Saved")
sb.SetError("Save failed")
sb.ClearMessage()
```

| Option | Default | Meaning |
|---|---|---|
| `Left`, `Right` | | Static slot text. |
| `BarBackground`, `BarForeground` | 236 / 252 | One color pair for the strip. |
| `LeftStyle`, `RightStyle`, `NeutralStyle`, `InfoStyle`, `ErrorStyle` | derived | Style overrides (pointers). |

`Update` clears the message on the next key press. Methods: `SetInfo`,
`SetError`, `ClearMessage`, `Message()`, `MessageKind()`, `SetLeft`,
`SetRight`, `SetRect`, plus `LeftContentRect`, `MiddleContentRect`,
`RightContentRect` for hit-testing.

Give anything embedded in the bar (such as help hints) the same background as
the bar, so the strip reads as one band: `t.Help()` does this for the help
footer.

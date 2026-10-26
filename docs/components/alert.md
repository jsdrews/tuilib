# Alert

`pkg/alert` is an acknowledge-this modal: a message and a single OK button,
resolving to `DismissedMsg`. Use it when the user must see something before
continuing: a surfaced failure, the result of a destructive action. For
passive feedback, `app.Info` / `app.Error` are lighter.

```
┏ Sync failed ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃argocd: rpc error: code = Unavailable       ┃
┃desc = connection refused                   ┃
┃                                            ┃
┃[ OK ]                                      ┃
┃                                            ┃
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
```

## Hosting

Exactly like [Confirm](confirm.md#hosting): the screen owns `alertUp`, captures
keys while it's up, forwards every message to it, and matches
`alert.DismissedMsg`.

**Fixed size**, centered by the layout:

```go
layout.ZStack(base, layout.Center(48, 7, layout.Sized(&s.alert)))
```

**Autosize**, for dynamic text such as stderr or a wrapped error. The modal
measures its message, word-wraps it at 80% of the width (40-column floor),
caps its height at 60%, scrolls internally when needed, and centers itself, so
there's no `Center` wrapper:

```go
opts := s.t.Alert()
opts.Title = "Sync failed"
opts.Message = err.Error()
opts.Autosize = true
opts.ActiveColor = s.t.ErrorBG // error-tinted border
s.alert = alert.New(opts)
s.alertUp = true

// Layout
layout.ZStack(base, layout.Sized(&s.alert))
```

The theme's alert chrome is deliberately neutral; set `ActiveColor` (and the
`OKStyle` foreground) to `t.ErrorBG` for an error look. When `Chips` is set
(a filled surface), tint the chip's background instead, with `t.ErrorFG` text.

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Title` | `string` | `"alert"` | Border title. |
| `Surface` | `surface.Resolved` | the surface's overlay form, filled so it covers what is under it | How the edge and body are drawn. Set by the theme; set it yourself only to pin this component to another surface, with `t.Resolve(s)`. See [Surfaces](../surfaces.md). |
| `Message` | `string` | `""` | Body text. Not wrapped unless `Autosize`. |
| `OK` | `string` | `"OK"` | Button label. |
| `OKStyle` | `lipgloss.Style` | bold accent | Button style. |
| `Chips` | `bool` | theme: on for filled surfaces | Draws the button as a filled chip, `  OK  `, in place of `[ OK ]`; the same width. `OKStyle` then carries the background. |
| `MessageStyle` | `lipgloss.Style` | theme | Body text. |
| `Autosize` | `bool` | `false` | Size to the message and center within the rect (see above). |
| `ActiveColor`, `InactiveColor`, `ActiveBorder`, `InactiveBorder`, `Glyphs`, `SlotBrackets` | | theme (thick border) | Pane chrome. |
| `Width`, `Height` | `int` | | Outer size (fixed mode). |

## Keys

| Key | Action |
|---|---|
| `enter`, `space`, `esc`, `o`, `O` | dismiss |
| `↑`/`k`, `↓`/`j`, `g`/`G`, `ctrl+u`/`ctrl+d`, `pgup`/`pgdown` | scroll the message (autosize, when it overflows) |

Mouse: clicking OK dismisses.

## Methods

`SetTitle`, `SetMessage` (re-measures and scrolls to the top in autosize),
`SetMessageStyle`, `SetOKStyle`, `Help()`, `HelpSections()`.

## Messages

`alert.DismissedMsg{}`.

See `examples/workbench`.

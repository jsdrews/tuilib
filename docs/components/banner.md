# Banner

`pkg/banner` draws a title in large block letters with a drop shadow. A colour
gradient sweeps across the letters while the shadow stays still. It is the
heading of an intro or sign-in screen.

```
████████╗██╗   ██╗██╗██╗     ██╗██████╗
╚══██╔══╝██║   ██║██║██║     ██║██╔══██╗
   ██║   ██║   ██║██║██║     ██║██████╔╝
   ██║   ██║   ██║██║██║     ██║██╔══██╗
   ██║   ╚██████╔╝██║███████╗██║██████╔╝
   ╚═╝    ╚═════╝ ╚═╝╚══════╝╚═╝╚═════╝
```

The faces (`█`) take the gradient; the box-drawing strokes beside and under
them are the shadow.

## With a form

A form draws one for you. Set its text and the form puts it above the card,
dropping it when the rect is too narrow or too short:

```go
o := t.Form()
o.Banner.Text = "tuilib"
```

## On its own

```go
o := t.Banner()
o.Text = "deploy"
s.banner = banner.New(o)

func (s *Screen) Init() tea.Cmd { return s.banner.Init() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    var cmd tea.Cmd
    s.banner, cmd = s.banner.Update(msg)
    return s, cmd
}
```

It is a component like a spinner: batch `Init` and forward every message to
`Update`. Each tick carries the banner's identity and a serial number, and a
banner honours only the one it issued last. Two banners on a screen don't
advance each other, and a tick delivered twice doesn't double the animation.

A banner has no pane. It is as wide as its text needs: check `Fits(w)` before
placing one, and fall back to a plain title when it doesn't fit. `Width()` and
`Height()` (six rows) say how much room it takes.

## Letters

A–Z, 0–9, space and `- . ! ? :`. Lower case draws as upper case, and anything
else as a space, so a banner never refuses its text.

## Options

| Field | Default | Meaning |
|---|---|---|
| `Text` | | What it spells. Empty draws nothing. |
| `Colours` | theme: accent, info, success | The gradient's stops. The sweep wraps from the last back to the first. One colour draws the letters flat. |
| `Shadow` | theme: `Subtle` | The shadow strokes' colour. |
| `Static` | false | Draw the gradient where it starts and never move it. |
| `Period` | 4s | How long one sweep takes. |

With `Static`, or fewer than two colours, the banner asks for no ticks.

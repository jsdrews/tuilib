# Confirm

`pkg/confirm` is a yes/no modal: a bordered pane with a message and two
buttons, resolving to `ConfirmedMsg` or `CancelledMsg`. Use it before a
destructive action.

```
┏ Delete cache-redis ━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃This removes the deployment and its volumes.    ┃
┃                                                ┃
┃[ Keep ]  [ Delete ]                            ┃
┃                                                ┃
┃                                                ┃
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
```

The cancel side starts highlighted.

## Hosting

The screen owns visibility. While the modal is up it captures keys, gets every
message, and sits in a `ZStack` over the base layout:

```go
func (s *Screen) ask(target string) {
    opts := s.t.Confirm()
    opts.Title = "Delete " + target
    opts.Message = "This removes the deployment and its volumes."
    opts.Confirm, opts.Cancel = "Delete", "Keep"
    s.confirm = confirm.New(opts)
    s.confirmUp = true
}

func (s *Screen) IsCapturingKeys() bool { return s.confirmUp || s.table.IsCapturingKeys() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    if s.confirmUp {
        switch msg.(type) {
        case confirm.ConfirmedMsg:
            s.confirmUp = false
            return s, s.delete()
        case confirm.CancelledMsg:
            s.confirmUp = false
            return s, nil
        }
        var cmd tea.Cmd
        s.confirm, cmd = s.confirm.Update(msg)
        return s, cmd
    }
    // … normal handling
}

func (s *Screen) Layout() layout.Node {
    base := layout.Sized(&s.table)
    if s.confirmUp {
        return layout.ZStack(base, layout.Center(50, 7, layout.Sized(&s.confirm)))
    }
    return base
}

func (s *Screen) HelpSections() []help.Section {
    if s.confirmUp {
        return s.confirm.HelpSections()
    }
    return help.SectionsOf(&s.table)
}
```

For a verb in the [action menu](action-menu.md), set `Action.Confirm` instead
and the shell does all of this.

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Title` | `string` | `"confirm"` | Border title. |
| `Surface` | `surface.Resolved` | the surface's overlay form, filled so it covers what is under it | How the edge and body are drawn. Set by the theme; set it yourself only to pin this component to another surface, with `t.Resolve(s)`. See [Surfaces](../surfaces.md). |
| `Message` | `string` | `""` | Body text; multi-line allowed, not wrapped. |
| `Confirm`, `Cancel` | `string` | `"Yes"`, `"No"` | Button labels. |
| `Initial` | `bool` | `false` | `true` starts on the confirm button. |
| `SelectedStyle`, `UnselectedStyle` | `lipgloss.Style` | theme | Buttons. |
| `MessageStyle` | `lipgloss.Style` | theme | Body text. |
| `ActiveColor`, `InactiveColor`, `ActiveBorder`, `InactiveBorder`, `Glyphs`, `SlotBrackets` | | theme (thick border) | Pane chrome. |
| `Width`, `Height` | `int` | | Outer size; `layout.Center` usually sets it. |

## Keys

| Key | Action |
|---|---|
| `←`/`h`, `→`/`l`, `tab`, `shift+tab` | move between buttons |
| `y` / `Y` | confirm |
| `n` / `N`, `esc` | cancel |
| `enter` | commit the highlighted button |

Mouse: clicking a button commits it.

## Methods

`Value()` (true = confirm), `SetValue(b)`, `SetTitle`, `SetMessage`,
`SetMessageStyle`, `SetSelectedStyle`, `SetUnselectedStyle`, `Help()`,
`HelpSections()`, `Rect()`.

## Messages

`confirm.ConfirmedMsg{}` and `confirm.CancelledMsg{}`.

See `examples/patterns/modals`.

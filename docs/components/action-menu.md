# Action menu

`pkg/action` turns a screen's verbs into data (an `action.Set`) and renders
them in `action.Menu`: a bordered picker that sizes itself to its content,
anchors where the user right-clicked or centers itself for the keyboard, shows
unavailable verbs dimmed with a reason, and resolves to a `ChosenMsg`.

Under the app shell you don't host the menu at all: set `app.Options.ActionsKey`
and implement `action.Provider`. That's covered in
[Verbs and multi-select](../guide/actions.md), including every `Action` and
`Set` field. This page is the component itself, for hosting it without the
shell.

## Hosting

```go
func (s *Screen) openMenu(at *mouse.Msg) {
    opts := s.t.Actions()
    opts.Set = s.Actions()
    s.menu = action.New(opts)
    if at != nil {
        s.menu.Anchor(at.X, at.Y)
    } else {
        s.menu.Center()
    }
    s.menuUp = true
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    if s.menuUp {
        switch m := msg.(type) {
        case action.ChosenMsg:
            s.menuUp = false
            return s, s.run(m.Action, m.Targets)
        case action.CancelledMsg:
            s.menuUp = false
            return s, nil
        }
        var cmd tea.Cmd
        s.menu, cmd = s.menu.Update(msg)
        return s, cmd
    }
    // …
}

func (s *Screen) Layout() layout.Node {
    base := layout.Sized(&s.table)
    if s.menuUp {
        return layout.ZStack(base, layout.Sized(&s.menu)) // no Center: it places itself
    }
    return base
}
```

The menu reports what was chosen and never runs anything; the host decides
(run `Run` in the background, return `Do()`'s command, confirm first if
`Confirm` is set).

## Groups

Set `Action.Group` to file verbs under headers, such as read and write:

```
┏ Actions · cache-redis ━━━━━━━┓
┃read                          ┃
┃▸ Describe                    ┃
┃  Logs                        ┃
┃write                         ┃
┃  Restart                     ┃
┃  Delete                      ┃
┃other                         ┃
┃  Shell                       ┃
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
```

Headers appear only when the set names two or more groups, so a menu whose
verbs are all one kind stays flat. Groups appear in the order first seen, with
header text as written. Actions without a group go last, under `other`. The
cursor, clicks and the half-page keys skip headers, and `tab` / `shift+tab`
jump to the first enabled action of the next or previous group. The menu is
modal, so `tab` has no panes to cycle while it's up. The group rules are the
same ones [Menu](menu.md) uses; the menu, which sits beside other
components, jumps with `[` / `]` instead.

`HeaderStyle` styles every header. `GroupStyle func(group string)
lipgloss.Style` styles each one by its group instead, for example a write
group in a warning colour. Only headers take these styles, not the rows
under them.

The app shell builds the menu's options from the theme on every palette or
surface swap, so under the shell set `app.Options.ActionGroupStyle`, which is
handed the current theme each time, instead of `GroupStyle`:

```go
ActionGroupStyle: func(group string, t theme.Theme) lipgloss.Style {
    if group == "write" {
        return lipgloss.NewStyle().Bold(true).Foreground(t.Warning)
    }
    return t.Actions().HeaderStyle
},
```

`t.Warning` is the palette's warning colour; the app decides which group gets it.

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Title` | `string` | `"Actions"` | Border title; the set's `Target` is appended. |
| `Surface` | `surface.Resolved` | the surface's overlay form, filled so it covers what is under it | How the edge and body are drawn. Set by the theme; set it yourself only to pin this component to another surface, with `t.Resolve(s)`. See [Surfaces](../surfaces.md). |
| `Set` | `action.Set` | | The verbs and what they act on. |
| `LabelStyle`, `DescStyle`, `KeyStyle` | `lipgloss.Style` | theme | Row text. |
| `SelectedStyle` | `lipgloss.Style` | bold accent | Cursor row. |
| `DisabledStyle` | `lipgloss.Style` | subtle | Unavailable rows. |
| `HeaderStyle` | `lipgloss.Style` | bold, `Current` | Group headers. |
| `GroupStyle` | `func(group string) lipgloss.Style` | nil | Per-group header style; replaces `HeaderStyle` when set. |
| `MultiReason` | `string` | `"one item at a time"` | Reason shown on a non-`Multi` verb under a multi-selection. |
| `RunningReason` | `string` | `"already running"` | Reason shown on an `Exclusive` verb already in flight. |
| `Keys` | `action.Keys` | `DefaultKeys()` | Key bindings. |
| `ActiveColor`, `InactiveColor`, `ActiveBorder`, `InactiveBorder`, `Glyphs`, `SlotBrackets` | | theme (overlay border) | Pane chrome. |

## Keys

| Key | Binding field | Action |
|---|---|---|
| `↑`/`k`, `↓`/`j` | `Up`, `Down` | move (skipping disabled rows) |
| `g`, `G`, `ctrl+u`, `ctrl+d` | `Top`, `Bottom`, `HalfUp`, `HalfDown` | jump (when the menu scrolls) |
| `shift+tab`, `tab` | `PrevGroup`, `NextGroup` | previous / next group (when grouped) |
| `enter` | `Choose` | choose |
| `esc` | `Cancel` | dismiss |
| an action's `Key` | | choose that action directly |

Mouse: click a row to choose it, click outside to dismiss. A right-press
outside the menu emits `action.RetargetMsg{Event}` so the host can move its
selection to what's under the pointer and reopen.

## Methods

| Method | Does |
|---|---|
| `Anchor(x, y)` | Place the top-left at a point (clamped on screen). |
| `Center()` | Center within the bounds. |
| `SetActions(set)`, `Set()` | Swap or read the verbs. |
| `SetRunning(map[string]bool)` | Which `Exclusive` actions are in flight, keyed by `action.RunKey(a, target)`. |
| `Selected()`, `Cursor()`, `SetCursor(i)` | Cursor. |
| `CanScroll()`, `Rect()` | Layout state. |

## Messages

| Message | When |
|---|---|
| `action.ChosenMsg{Action, Target, Targets}` | a verb was picked |
| `action.CancelledMsg{}` | dismissed |
| `action.RetargetMsg{Event}` | right-press outside the open menu |

`action.Validate(set) []error` catches authoring mistakes; call it from a test.

See `examples/patterns/actions` and `examples/patterns/multiselect`.

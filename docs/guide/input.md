# Keys, focus and mouse

## The key vocabulary

The same keys mean the same thing in every component.

| Keys | Meaning everywhere |
|---|---|
| `↑`/`k`, `↓`/`j` | move the cursor, or scroll, one row |
| `g` / `G` | top / bottom |
| `ctrl+u` / `ctrl+d` | half a page up / down |
| `←`/`h`, `→`/`l` | scroll horizontally |
| `0`/`home`, `$`/`end` | horizontal start / end |
| `/` | filter or search |
| `n` / `N` | next / previous match |
| `enter` | open the focused selection |
| `tab` / `shift+tab` | move focus between panes |
| `esc` | leave a field; at screen level, go back |

Arrows are aliases for `hjkl`, not separate verbs, and **no component binds an
arrow or `hjkl` to anything but scrolling**. That's why `tree` and
`inspector` expand with `space` and `E`/`C` instead of `←`/`→`, and why
`pkg/tab` switches tabs with `shift+←`/`shift+→` and `1`–`9` rather than `tab`.

Single letters are for app-wide affordances (`q`, `t`, `?`, `/`). Per-screen
verbs ("delete", "restart") belong in a menu: a list and `enter`, or the
[action menu](actions.md).

The full per-component list is in [Default key bindings](../reference/keys.md).

## Overriding bindings

Every interactive component exposes its keymap as `Options.Keys`, built from
`DefaultKeys()`. Each binding carries both its keys and its help label, so an
override shows up in the hint strip and overlay without restating anything:

```go
opts := t.Table()
opts.Keys.Filter = key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter"))
opts.Keys.Pane.Left = key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "scroll left"))
```

Unset fields fall back to the defaults field by field. Horizontal scroll lives
on the nested `Keys.Pane`.

## Capturing keys

When a text field has focus, global keys must reach it as text. A screen
reports that through `IsCapturingKeys()`, and the shell then suppresses `q`,
theme cycling, `esc`-pop and its other globals:

```go
func (s *Screen) IsCapturingKeys() bool { return s.list.IsCapturingKeys() }
```

Every component that swallows printable keys implements `IsCapturingKeys()`:
true while its filter or field has focus. A screen with a modal up returns
`true` too.

## Focus groups

A screen with more than one interactive component holds a `focus.Group`:

```go
s.focus = focus.NewGroup(&s.query, &s.results, &s.caseToggle)

func (s *Screen) Init() tea.Cmd { return s.focus.Init() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    var cmds []tea.Cmd
    var cmd tea.Cmd

    s.focus, cmd = s.focus.Update(msg) // every message, not just tab
    cmds = append(cmds, cmd)

    switch msg.(type) {
    case tea.KeyMsg: // keys go to the focused component only
        switch {
        case s.focus.Is(&s.query):
            s.query, cmd = s.query.Update(msg)
        case s.focus.Is(&s.results):
            s.results, cmd = s.results.Update(msg)
        case s.focus.Is(&s.caseToggle):
            s.caseToggle, cmd = s.caseToggle.Update(msg)
        }
        cmds = append(cmds, cmd)
    default: // mouse and everything else go to every component
        s.query, cmd = s.query.Update(msg)
        cmds = append(cmds, cmd)
        s.results, cmd = s.results.Update(msg)
        cmds = append(cmds, cmd)
        s.caseToggle, cmd = s.caseToggle.Update(msg)
        cmds = append(cmds, cmd)
    }
    return s, tea.Batch(cmds...)
}

func (s *Screen) IsCapturingKeys() bool          { return s.focus.IsCapturingKeys() }
func (s *Screen) Help() []key.Binding            { return s.focus.Help() }
func (s *Screen) HelpSections() []help.Section   { return s.focus.HelpSections() }
```

The group:

- cycles with `tab` / `shift+tab` (`WithKeys` relabels them, `WithoutWrap`
  stops at the ends);
- blurs everything and focuses one;
- grants focus to a component that was **clicked**: the component sends a
  `focus.RequestMsg`, which is why the group needs every message, not just
  keys;
- declines to cycle while the focused component is capturing keys, so a
  half-typed filter isn't stranded (and `pkg/table` can use `tab` to complete
  a `key:value` term). Leave the field with `enter` or `esc`, then cycle;
- answers `Help()` with the cycling keys plus the focused component's, and
  `HelpSections()` with every pane's groups, qualified by pane title.

!!! warning "Route mouse to every component"
    Keys go to the focused component; mouse events go to **all** of them, and
    each tests the position against its own rect. Sending mouse only to the
    focused component means a click on another pane never reaches it, so it
    can't take focus. This mistake reads as a broken component rather than a
    routing bug.

On a theme swap, rebuild the group over the same field addresses and restore
the index with `SetIndex`. See `examples/patterns/focus`.

### Filterable components have two regions

A filterable list, table, tree, inspector, logview or textview has two
focusable regions behind one `Focusable`: the filter row and the body.

- `Focus()` lights the pane and never blurs the filter.
- `Blur()` clears both, so a blurred pane can't keep swallowing keys.
- `Focused()` is true if either region is active.
- `FocusFilter()` / `BlurFilter()` move input between them.
- Any click inside the pane other than on the filter row hands input back to
  the body. A click on the scrollbar is the exception and leaves a half-typed
  query alone.

`examples/patterns/filters` is a screen with two filterable panes.

## Mouse

Turn it on per app:

```go
app.New(app.Options{ /* … */ Mouse: app.MouseClick })
```

It's off by default because mouse reporting takes the terminal's own
click-drag text selection away. With it on, the shell translates each
`tea.MouseMsg` into a `mouse.Msg` with a resolved click count, so components
never see the raw event and none of them needs a double-click threshold.

The verbs, library-wide:

| Gesture | Effect |
|---|---|
| click | focuses the component and moves the cursor to the clicked row |
| double-click | does what `enter` does: the component emits an `ActivatedMsg` |
| wheel | does what `↓`/`j` does, **for the component under the pointer**, focused or not; it never takes focus |
| shift+click | extends a marked range (multi-select) |
| right-click | opens the action menu at the pointer (when `ActionsKey` is set) |
| click on chrome | table headers sort, tree/inspector `▸`/`▾` toggle, modal buttons commit, tab labels switch, breadcrumb crumbs unwind, scrollbars jump and drag, the statusbar message opens the console |

Inputs too: clicking an `input` focuses it, clicking a side of a `toggle`
picks it, clicking a `form` field moves the form's focus there.

### Opting in to double-click

A component reports activation; the screen decides what "open" means. Use one
predicate for both inputs:

```go
if s.table.IsActivate(msg) {          // enter, or this table's double-click
    if row, ok := s.table.Selected(); ok {
        return s, screen.Push(newDetail(row, s.t))
    }
}
```

`list`, `table` and `eventlog` provide `IsActivate`. The `ActivatedMsg` carries
a `focus.Token`, so two lists on one screen never claim each other's double
clicks. A single click only focuses and selects; it's always safe to explore
with.

### Double-click timing

The window is `app.Options.DoubleClickInterval`, else the user's
`double_click_ms` config setting, else 500ms. It's a per-machine preference, so
no component carries mouse configuration.

### Writing a mouse-aware component

Match `mouse.Msg`, test your rect with `Hit`, and return the message untouched
when it isn't yours:

```go
case mouse.Msg:
    if !m.rect.Hit(msg.X, msg.Y) {
        return m, nil // not ours; a sibling may claim it
    }
    if msg.IsDoubleClick() {
        return m, m.activate()
    }
    if msg.IsPress() {
        return m, focus.RequestSelf(m.token)
    }
```

See `examples/patterns/mouse`.

# Menu

`pkg/menu` is a grouped, filterable list of things to open: the sections of
an app, the screens a home page leads to. Use it where tabs run out of room.

```
┌ Sections ─────────────────────────────────────────┐
│/ filter by label or group…                        │
│────────────────────────────────────────────────── │
│── database ────────────────────────────────────── │
│▸ postgres › instance  GET /v1/db/postgres/instance│
│  mysql › instance     GET /v1/db/mysql/instance   │
│── network ─────────────────────────────────────── │
│  vpc                  GET /v1/net/vpc             │
│  firewall rules       GET /v1/net/firewall        │
│── other ───────────────────────────────────────── │
│  about                what this demo shows        │
└ 6 / 6 ────────────────────────────────────────────┘
```

Each item is a label, a dimmed description beside it (cut off when the pane
runs out of room), and an optional group. Choosing an item emits
`menu.ChosenMsg`. The menu doesn't decide what opening means; the screen does,
usually with `screen.Push`.

It's a pane component like [List](list.md): it takes focus, filters with `/`,
and handles the mouse within its own rect, so it can sit next to other
components on a screen. Don't confuse it with the [action menu](action-menu.md),
which is a popup over a screen's verbs. Both draw groups by the same rules.

## Usage

```go
opts := t.Menu()
opts.Title = "Sections"
opts.Filterable = true
opts.Items = []menu.Item{
    {ID: "pg", Label: "postgres › instance", Group: "database",
        Description: "GET /v1/db/postgres/instance", Data: pgScreen},
    // …
}
s.menu = menu.New(opts)

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    if it, ok := s.menu.Chosen(msg); ok { // enter or double-click
        return s, screen.Push(it.Data.(func(theme.Theme) screen.Screen)(s.t))
    }
    var cmd tea.Cmd
    s.menu, cmd = s.menu.Update(msg)
    return s, cmd
}

func (s *Screen) IsCapturingKeys() bool         { return s.menu.IsCapturingKeys() }
func (s *Screen) HelpSections() []help.Section { return help.SectionsOf(&s.menu) }

func (s *Screen) SetTheme(t theme.Theme) {
    st := s.menu.State()
    s.menu = menu.New(opts)    // rebuilt from t.Menu() as above
    s.menu.Restore(st)
}
```

`Chosen` matches only this menu's `ChosenMsg`, using its focus token, so two
menus on one screen never claim each other's picks.

## Groups

- Headers appear only when the items name **two or more groups**. A menu
  whose items all share one group, or have none, is drawn flat.
- Groups appear in the order first seen, and items keep their order within a
  group. Header text is the group name as written.
- Items without a group go last, under `other`, merged with any group named
  `other`.
- The cursor, clicks, the wheel and the half-page keys skip headers. `tab` /
  `shift+tab` and `]` / `[` jump to the first item of the next or previous
  group, wrapping past either end.
- A header is a thin divider across the menu with the group's name set into
  it, the way a pane sets its title into its border: `── network ──────`. The
  rule is the surface's divider glyph in `RuleStyle`, the name in
  `HeaderStyle`, so the headers stay quieter than the items under them. On a
  card (any surface without a line border) there is no edge for the rule to
  meet, so the name sits in the label column and the rule trails after it:
  `  network ──────`.

## Filter

With `Filterable`, `/` focuses a filter row inside the pane. It's a
case-insensitive substring match on the label **and the group**, so typing a
group's name keeps the whole group. Descriptions aren't searched. A group with
no items left hides its header. A menu that started grouped keeps its headers
even when the filter narrows it to one group, because labels like "instance"
mean little without the group above them.

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Title` | `string` | `"Menu"` | Border title. |
| `Items` | `[]menu.Item` | | The content, copied. |
| `Filterable` | `bool` | `false` | Adds the `/` filter row. |
| `LabelStyle` | `lipgloss.Style` | plain | Labels. |
| `DescStyle` | `lipgloss.Style` | muted | Descriptions. |
| `HeaderStyle` | `lipgloss.Style` | `Muted` | The name in each group header. |
| `GroupStyle` | `func(group string) lipgloss.Style` | nil | Per-group name style; replaces `HeaderStyle` when set. |
| `RuleStyle` | `lipgloss.Style` | `Subtle` | The divider either side of the name. |
| `SelectedStyle`, `SelectedBlurredStyle` | `lipgloss.Style` | theme | The cursor row, focused and not. |
| `Filter` | `filter.Options` | `t.Filter()` | The embedded filter. |
| `Keys` | `menu.Keys` | `DefaultKeys()` | Key bindings. |
| `Surface`, `ActiveColor`, `InactiveColor`, `ActiveBorder`, `InactiveBorder`, `Glyphs`, `SlotBrackets` | | theme | Pane chrome. |

`menu.Item` is `{ID, Label, Group, Description, Data}`. `Data` is your
payload, returned in `ChosenMsg`. `ID` is optional; when set, it keeps the
cursor on the same item across `SetItems`.

## Keys

| Key | Field | Action |
|---|---|---|
| `↑`/`k`, `↓`/`j` | `Up`, `Down` | move |
| `g`, `G` | `Top`, `Bottom` | first / last |
| `ctrl+u`, `ctrl+d` | `HalfUp`, `HalfDown` | half page |
| `shift+tab`/`[`, `tab`/`]` | `PrevGroup`, `NextGroup` | previous / next group |
| `/` | `Filter` | filter |
| `enter` | `Choose` | open |

In a `focus.Group`, `tab` and `shift+tab` move focus between components, so
the menu gives them up and jumps on `[` / `]` alone; `focus.NewGroup` tells it
which keys cycle (`focus.Member`). Mouse: click focuses
and moves the cursor (a click on a header only focuses), double-click opens,
and the wheel moves the cursor in whichever menu is under the pointer.

## Methods

| Method | Does |
|---|---|
| `Chosen(msg) (Item, bool)` | This menu's pick, if `msg` is one. |
| `SetItems(items)`, `Items()` | Swap or read the content. |
| `Selected() (Item, bool)`, `Select(id) bool` | Cursor. |
| `Value()`, `SetValue(s)`, `Filtering()` | Filter. |
| `Focus()`, `Blur()`, `Focused()`, `FocusFilter()`, `BlurFilter()`, `IsCapturingKeys()` | Focus (satisfies `focus.Focusable` and `focus.Capturer`). |
| `State()`, `Restore(st)` | Carry items, filter, cursor and scroll across a rebuild. |
| `SetTitle(s)`, `Title()`, `Rect()` | Chrome and placement. |

See `examples/navigation`.

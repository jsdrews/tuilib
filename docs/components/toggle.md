# Toggle

`pkg/toggle` is a yes/no selector: two labeled buttons side by side in a
bordered pane, with the question on the border.

```
┌ Send notifications? ─────────┐
│[ yes ]  [ no ]               │
└──────────────────────────────┘
```

## Usage

```go
opts := t.Toggle()
opts.Title = "Send notifications?"
opts.YesLabel, opts.NoLabel = "on", "off"
s.notify = toggle.New(opts)

s.notify, cmd = s.notify.Update(msg)
send := s.notify.Value()
```

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Title` | `string` | `"toggle"` | Border title; use the question. |
| `Initial` | `bool` | `false` | Starting value. |
| `YesLabel`, `NoLabel` | `string` | `"yes"`, `"no"` | Button labels. |
| `SelectedStyle`, `UnselectedStyle` | `lipgloss.Style` | theme | The chosen and unchosen sides. |
| `ActiveColor`, `InactiveColor`, `ActiveBorder`, `InactiveBorder`, `Glyphs`, `SlotBrackets` | | theme | Pane chrome. |
| `Width` | `int` | | Set by the layout engine. Height is always 3. |

## Keys (while focused)

| Key | Action |
|---|---|
| `←`/`h`, `→`/`l`, `space` | flip |
| `y` / `n` | set yes / no |

Mouse: clicking a side picks it outright, and focuses the toggle if it wasn't.

## Methods

`Value()`, `SetValue(b)`, `Focus()`, `Blur()`, `Focused()`, `SetTitle`,
`SetTopRight` (validation messages), `SetSelectedStyle`,
`SetUnselectedStyle`, `Help()`, `HelpSections()` (under Edit),
`State()` / `Restore(st)`.

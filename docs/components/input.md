# Input

`pkg/input` is a single-line text field in a bordered pane, with the label on
the border. It has no commit or clear keys of its own (that's
[Filter](filter.md)); what `enter` means is up to the screen.

```
┌ Name ────────────────────────┐
│your name                     │
└──────────────────────────────┘
```

## Usage

```go
opts := t.Input()
opts.Title = "Name"
opts.Placeholder = "your name"
s.name = input.New(opts)

func (s *Screen) Init() tea.Cmd { return s.name.Focus() } // starts the cursor blink

s.name, cmd = s.name.Update(msg)
name := s.name.Value()
```

Password field:

```go
opts.Echo = input.EchoMask   // renders •••• ; Value() is still the real text
```

Masking is display-only, so "reveal password" is just
`s.pass.SetEcho(input.EchoNormal)`. Inside a [form](form.md), use
`form.Password`.

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Title` | `string` | `"input"` | Border title; use the field's label. |
| `Placeholder` | `string` | `""` | Shown while empty. |
| `Initial` | `string` | `""` | Pre-filled value. |
| `Prompt` | `string` | `""` | Drawn before the cursor. |
| `CharLimit` | `int` | 0 (unlimited) | Maximum length. |
| `Echo` | `input.EchoMode` | `EchoNormal` | `EchoMask` draws `MaskChar` per character; `EchoNone` draws nothing. |
| `MaskChar` | `rune` | `•` | Mask glyph. |
| `PromptStyle`, `TextStyle`, `PlaceholderStyle` | `lipgloss.Style` | theme | Text styling. |
| `CursorColor` | `lipgloss.TerminalColor` | theme | |
| `ActiveColor`, `InactiveColor`, `ActiveBorder`, `InactiveBorder`, `Glyphs`, `SlotBrackets` | | theme | Pane chrome. |
| `Width` | `int` | | Set by the layout engine. Height is always 3. |

## Keys

While focused, printable keys edit the text. `esc` blurs the field, keeping the
value. That's what lets a user leave an input inside a `focus.Group`, which
won't cycle away from a component that's capturing keys.

Mouse: a click focuses the field.

## Methods

| Method | Does |
|---|---|
| `Focus() tea.Cmd`, `Blur()`, `Focused()` | Focus. Return `Focus`'s command for the cursor blink. |
| `Value()`, `SetValue(s)`, `Reset()` | Text. `Reset` clears and blurs. |
| `Echo()`, `SetEcho(mode)` | Masking. |
| `SetTitle`, `SetTopRight`, `Title()` | Border text; `SetTopRight` is where forms show validation errors. |
| `SetTextStyle`, `SetPlaceholderStyle`, `SetCursorColor`, `SetActiveColor`, … | Restyle in place. |
| `IsCapturingKeys()` | True while focused. |
| `State()`, `Restore(st)` | Carry the text across a rebuild. |

See `examples/patterns/focus`.

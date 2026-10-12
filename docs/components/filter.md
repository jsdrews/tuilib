# Filter

`pkg/filter` is a single-line text field with the "press `/` to search,
`enter` to commit, `esc` to clear" behavior. The caller reads `Value()` after
each update and applies it to whatever it filters.

You rarely need it directly: `list`, `table`, `tree`, `inspector`, `logview`,
`textview` and `eventlog` embed one when filterable, drawn **inside** their own
pane as a row above a rule. Use a standalone filter when what it drives isn't
one of those, and use [Input](input.md) for a plain text field.

## Usage

```go
opts := t.Filter()
opts.Title = "search"
opts.Placeholder = "name or label"
s.query = filter.New(opts)

case tea.KeyMsg:
    if msg.String() == "/" && !s.query.Focused() {
        return s, s.query.Focus()   // return the cursor-blink command
    }

// forward every message; the filter acts only while focused
s.query, cmd = s.query.Update(msg)
s.apply(s.query.Value())
```

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Title` | `string` | `"filter"` | Border title. |
| `Prompt` | `string` | `"/ "` | Drawn before the cursor. |
| `Placeholder` | `string` | `""` | Shown while empty. |
| `CharLimit` | `int` | 64 | Maximum length. |
| `PromptStyle`, `TextStyle`, `PlaceholderStyle` | `lipgloss.Style` | theme | Text styling. |
| `CursorColor` | `lipgloss.TerminalColor` | theme | |
| `ActiveColor`, `InactiveColor`, `ActiveBorder`, `InactiveBorder`, `Glyphs`, `SlotBrackets` | | theme | Pane chrome. |
| `Width` | `int` | | Set by the layout engine. Height is always 3. |

## Keys (while focused)

| Key | Action |
|---|---|
| `enter` | commit: blur, keep the value |
| `esc` | cancel: clear the value and blur |
| anything else | edit the text |

## Methods

| Method | Does |
|---|---|
| `Focus() tea.Cmd`, `Blur()`, `Focused()` | Focus. `Blur` keeps the value. |
| `Value()`, `SetValue(s)`, `Reset()` | Text. `Reset` clears and blurs. |
| `SetBottomLeft(s)`, `SetBottomRight(s)` | Write the border's bottom slots (hints, counts, completions). |
| `IsCapturingKeys()` | True while focused. |
| `Help()` | The commit/clear bindings while focused. |
| `InlineView()`, `SetInlineRect(r)`, `Inline()` | Render as a bare row inside a host's pane (how components embed it). |
| `State()`, `Restore(st)` | Carry the text across a rebuild. |

`filter` has no `HelpSections`: `pkg/help` imports it for the overlay's own
search field, so the reverse import would be a cycle. Its bindings reach the
overlay through whichever component embeds it.

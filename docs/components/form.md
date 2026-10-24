# Form

`pkg/form` is a vertical form: text, password, select and confirm fields, each
a bordered component with its label on the border, then a submit button. It
owns focus cycling and validation.

```
┌ Name * ──────────────────────────────┐
│                                      │
└──────────────────────────────────────┘
┌ Password ────────────────────────────┐
│•••••••                               │
└──────────────────────────────────────┘
┌ Role ────────────────────────────────┐
│▸ admin                               │
│  user                                │
│  viewer                              │
└──────────────────────────────────────┘
┌ Send welcome email? ─────────────────┐
│[ yes ]  [ no ]                       │
└──────────────────────────────────────┘
[ Submit ]
```

## Usage

```go
s.form = form.New(t.Form().With([]form.Field{
    form.Text(form.TextOptions{Key: "name", Label: "Name", Required: true}),
    form.Text(form.TextOptions{Key: "email", Label: "Email", Validate: validEmail}),
    form.Password(form.PasswordOptions{Key: "pass", Label: "Password", Required: true}),
    form.Select(form.SelectOptions{Key: "role", Label: "Role",
        Options: []string{"admin", "user", "viewer"}, RequirePick: true}),
    form.Confirm(form.ConfirmOptions{Key: "welcome", Label: "Send welcome email?"}),
}))

func (s *Screen) Init() tea.Cmd         { return s.form.Init() } // cursor blink
func (s *Screen) IsCapturingKeys() bool { return s.form.IsCapturingKeys() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    switch m := msg.(type) {
    case form.SubmittedMsg:
        return s, s.createUser(m.Values["name"].(string), s.form.String("role"))
    case form.CancelledMsg:
        return s, screen.Pop(nil)
    }
    var cmd tea.Cmd
    s.form, cmd = s.form.Update(msg)
    return s, cmd
}
```

Because a form always captures keys, the shell's `esc`-pop is suppressed;
handle `form.CancelledMsg` to leave.

## Fields

| Constructor | Value type | Options |
|---|---|---|
| `form.Text(TextOptions)` | `string` | `Key`, `Label`, `Placeholder`, `Initial`, `Required`, `Validate` |
| `form.Password(PasswordOptions)` | `string` | as Text, plus `MaskChar` |
| `form.Select(SelectOptions)` | `string` (the highlighted option) | `Key`, `Label`, `Options`, `Initial` (index), `Height` (default fits up to 4 rows), `RequirePick`, `Validate` |
| `form.Confirm(ConfirmOptions)` | `bool` | `Key`, `Label`, `Initial`, `Validate` |

`Validate` is `func(any) error`, receiving the field's value. Implement
`form.Field` yourself only for a genuinely new kind of entry.

## Validation

Validation belongs to the form, not the screen:

- `Required` rejects an empty value and adds `*` to the label.
- `RequirePick` starts a select with nothing highlighted and rejects submission
  until the user picks one. (A select always has a value otherwise.)
- `Validate` runs after `Required`, so a blank required field says "required"
  rather than a format error.
- Validation runs **on submit**, then live for fields already flagged.
- An invalid submit is **refused**: no `SubmittedMsg`, focus moves to the
  first offender, and `form.InvalidMsg{Keys}` lets the screen react if it
  wants to.
- Errors render on the field's own border (a tint plus a short message in the
  top-right slot), so nothing reflows while the user corrects it.

Cross-field rules (password confirmation, date ranges) aren't supported:
compare values in the screen on `SubmittedMsg`.

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Fields` | `[]form.Field` | **required** | The fields, top to bottom. |
| `Title` | `string` | | When set, frames the whole form in a titled pane, the way `confirm` frames its buttons. Use it when the form stands alone, such as a prompt or an action's inputs. Without it the form is a bare stack of bordered fields. |
| `ActiveColor`, `InactiveColor`, `ActiveBorder`, `InactiveBorder`, `Glyphs`, `SlotBrackets`, `Surface` | | theme | The frame's chrome, used only with `Title`. |
| `SubmitText` | `string` | `"Submit"` | Submit button label. |
| `FieldSpacing` | `*int` | 0 | Blank lines between fields. |
| `Styles` | `form.Styles` | theme | Input, placeholder, cursor, selected, pane colors and border shapes, error color and text, submit button styles. |
| `Width`, `Height` | `int` | | Set by the layout engine. |

`opts.With(fields)` returns a copy with `Fields` replaced, for chaining from
`t.Form()`.

## Keys

| Key | Action |
|---|---|
| `tab` / `shift+tab` | next / previous field |
| `enter` | on the submit button: submit |
| `esc` | cancel (`CancelledMsg`) |
| field keys | typing, `↑`/`↓` in a select, `←`/`→`/`y`/`n` in a confirm |

Mouse: clicking a field moves focus to it; clicking submit submits.

## Methods

| Method | Does |
|---|---|
| `Values() map[string]any` | Every value by key. |
| `Value(key)`, `String(key)`, `Bool(key)` | One value, typed. |
| `FocusedIndex()` | Which field has the keyboard (`len(Fields)` = submit). |
| `SetStyles(s)` | Restyle every field. |
| `SetTitle(s)`, `Title()` | The frame's title (no effect without one). |
| `FieldRect(i)`, `SubmitRect()` | Geometry. |
| `IsCapturingKeys()` | Always true while the form is active. |

## Messages

| Message | When |
|---|---|
| `form.SubmittedMsg{Values}` | submit with every field valid |
| `form.InvalidMsg{Keys}` | submit refused; `Keys` names the invalid fields |
| `form.CancelledMsg{}` | `esc` |

See `examples/components/form`.

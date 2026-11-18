# Form

`pkg/form` is a form drawn as one card. Each field is a label over its value,
with a note under it saying what it is for. Typed values sit in a well a tone
lighter than the card, and the focused field carries a bar down its left edge.
Submit and Cancel close the card, and a line under it lists the keys that work.
A form can be a single page or a wizard of steps, and on an intro screen it
draws a [banner](banner.md) above itself.

```
                ████████╗██╗   ██╗██╗██╗     ██╗██████╗
                ╚══██╔══╝██║   ██║██║██║     ██║██╔══██╗
                   ██║   ██║   ██║██║██║     ██║██████╔╝
                   ██║   ██║   ██║██║██║     ██║██╔══██╗
                   ██║   ╚██████╔╝██║███████╗██║██████╔╝
                   ╚═╝    ╚═════╝ ╚═╝╚══════╝╚═╝╚═════╝

      ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄
        sign in · Account                                  ●──○
       ┃ Username *
       ┃  alice█
       ┃ The name you sign in with.

         Password *
          ••••••••

         Role *
         ● admin
         ○ editor
         ○ viewer

           Next      Cancel
      ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
                 ⏎ next · ⇥ next · ⇧⇥ prev · esc cancel
```

## Usage

```go
o := t.Form()
o.Title = "sign in"
o.SubmitText = "Sign in"
o.Banner.Text = "tuilib"          // an intro screen: big letters above the card
o.Fields = []form.Field{
    form.Text(form.TextOptions{Key: "user", Label: "Username", Required: true,
        Note: "The name you sign in with."}),
    form.Password(form.PasswordOptions{Key: "pass", Label: "Password", Required: true}),
    form.Select(form.SelectOptions{Key: "role", Label: "Role",
        Options: []string{"admin", "editor", "viewer"}, RequirePick: true}),
    form.Confirm(form.ConfirmOptions{Key: "remember", Label: "Remember me?", Initial: true}),
}
s.form = form.New(o)

func (s *Screen) Init() tea.Cmd         { return s.form.Init() } // cursor blink, banner
func (s *Screen) IsCapturingKeys() bool { return s.form.IsCapturingKeys() }
func (s *Screen) Layout() layout.Node   { return layout.Sized(&s.form) }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    switch m := msg.(type) {
    case form.SubmittedMsg:
        return s, s.signIn(m.Values.String("user"), m.Values.String("role"))
    case form.CancelledMsg:
        return s, screen.Pop(nil)
    }
    var cmd tea.Cmd
    s.form, cmd = s.form.Update(msg)
    return s, cmd
}

func (s *Screen) SetTheme(t theme.Theme) {
    st := s.form.State()
    s.form = form.New(optionsFrom(t))
    s.form.Restore(st)
}
```

The form always captures keys, so the shell's esc-pop is suppressed. Handle
`form.CancelledMsg` to leave.

## Size and placement

The form sizes and places itself. The card is at most `MaxWidth` (60) columns
wide and exactly as tall as its fields, centred in whatever rect it is given.
On an intro screen the banner and the help line sit above and below it. So:

- **The whole screen:** `layout.Sized(&s.form)` in the body. Nothing else to do.
- **A modal:** the same node in a `ZStack` over the screen, with
  `Options.Modal = true`. The help line moves inside the card, since below it
  would sit on whatever the modal covers, and no banner is drawn.
- **Too small:** a rect too short for the card scrolls the fields so the
  focused one stays in view. The banner is dropped when it doesn't fit.

`Height(w)` is the rows the form needs at width `w`, including the card and, on
an intro screen, the banner and help line. Use it when a host wants the size
anyway.

## Fields

| Constructor | Value | Options beyond `Key`, `Label`, `Note`, `Validate` |
|---|---|---|
| `form.Text(TextOptions)` | `string` | `Placeholder`, `Initial`, `CharLimit`, `Required` |
| `form.Password(PasswordOptions)` | `string` | as Text, plus `MaskChar` (default `•`) |
| `form.TextArea(TextAreaOptions)` | `string` | as Text, plus `Rows` (default 3) |
| `form.Select(SelectOptions)` | `string` | `Options`, `Initial`, `Visible` (default 5), `RequirePick` |
| `form.MultiSelect(MultiSelectOptions)` | `[]string` | `Options`, `Initial`, `Visible`, `Min`, `Max` |
| `form.Confirm(ConfirmOptions)` | `bool` | `Initial`, `Yes`, `No` |

- **Select** is a list that is always open. The option under the cursor is the
  choice, so `↑`/`↓` change it. Past `Visible` options it scrolls under a fixed
  `↓ n more` row, so the form never changes height.
- **MultiSelect** marks options with `■`/`□` (the select's `●`/`○` are for
  choosing one). `↑`/`↓` move and `space` toggles. A positive `Min` makes it
  required, and a toggle past `Max` is ignored.
- **TextArea** takes enter as a newline. Leave it with tab. Text longer than
  `Rows` scrolls inside it.
- **Confirm** picks a side with `←`/`→` (or `y`/`n`), and `space` flips it.

`Field` is closed: the form draws every field's label, note, well and bar
itself, so a field kind is something the package adds, not an interface an
app implements.

Read values with `Values.String(key)`, `.Bool(key)` and `.Strings(key)`, on
`SubmittedMsg.Values` or on `Model.Values()`.

## Steps

Set `Steps` instead of `Fields` to make the form a wizard:

```go
o.Steps = []form.Step{
    {Title: "Host", Fields: []form.Field{host, auth}},
    {Title: "Key", Fields: []form.Field{keyfile},
        Skip: func(v form.Values) bool { return v.String("auth") != "key" }},
    {Title: "Confirm", Fields: []form.Field{save}},
}
```

- The frame reads `title · step` and shows the steps that will run as dots.
- **Next** (or enter on a step's last field) validates the step before moving
  on. **Back**, or esc, goes back a step with every answer kept. esc on the
  first step cancels.
- `Skip` leaves a step out for the answers so far. It is asked whenever the
  user moves past a neighbouring step, so a step comes and goes as earlier
  answers change. A skipped step's fields are left out of `SubmittedMsg`.
- Submit validates every step that runs, and goes back to the first one that
  fails.

## Validation

Validation belongs to the form, not the screen:

- `Required` rejects an empty value and adds `*` to the label. For a
  MultiSelect, a positive `Min` does the same.
- `RequirePick` starts a select with nothing chosen. Reaching the field
  chooses `Initial`, so it is never focused with no visible choice. A form
  submitted without reaching it is refused.
- `Validate` runs after the obligation, so a blank required field says
  "required" rather than a format error.
- Validation runs when the user leaves a step or submits, then live for the
  fields it flagged: nothing nags mid-typing, but a fix is acknowledged at once.
- A refused step or submit sends nothing on. Focus moves to the first offender,
  and `form.InvalidMsg{Keys}` lets the screen react if it wants to.
- An error tints the field's label and appears right-aligned on the label row,
  so nothing reflows while the user corrects it.

Cross-field rules (password confirmation, date ranges) aren't supported.
Compare values in the screen on `SubmittedMsg`.

## Keys

| Key | Action |
|---|---|
| `enter` | next field; on a step's last field, next step or submit; on a button, press |
| `tab` / `shift+tab` | next / previous field or button |
| `esc` | back a step, or cancel on the first |
| `↑`/`↓` | choose (select), move (multi-select) |
| `space` | toggle (multi-select), flip (confirm) |
| `←`/`→` | choose a side (confirm), move between buttons |

Mouse: clicking a field focuses it; clicking an option chooses or toggles it;
clicking a button presses it.

## Options

| Field | Default | Meaning |
|---|---|---|
| `Title` | | The frame's title. |
| `Fields` | | A single page. |
| `Steps` | | A wizard; replaces `Fields`. |
| `SubmitText`, `NextText`, `BackText`, `CancelText` | `Submit`, `Next`, `Back`, `Cancel` | Button labels. |
| `MaxWidth` | 60 | The card's widest. |
| `FieldSpacing` | 1 | Blank rows between fields. |
| `Modal` | false | Draw as a modal: help inside the card, no banner. |
| `Banner` | theme | A `banner.Options`; drawn above the card when `Text` is set. |
| `Styles` | theme | Text, labels, notes, placeholder, cursor, bar, glyph colours, wells, error, buttons and chips, help line. |
| `ActiveColor`, `InactiveColor`, `ActiveBorder`, `InactiveBorder`, `Glyphs`, `SlotBrackets`, `Surface` | theme | The frame. `theme.Form()` uses the surface's overlay form. |

`opts.With(fields)` returns a copy with `Fields` replaced, for chaining from
`t.Form()`.

## Methods

| Method | Does |
|---|---|
| `Values()`, `Value(key)`, `String(key)`, `Bool(key)`, `Strings(key)` | Current answers. |
| `Step()`, `FocusedIndex()` | Where the user is. |
| `Height(w)` | Rows needed at width `w`. |
| `Rect()`, `FieldRect(i)`, `SubmitRect()` | Where things were drawn. |
| `State()`, `Restore(s)` | Carry everything across a theme rebuild. |
| `SetTitle(s)`, `Title()` | The frame's title. |
| `IsCapturingKeys()` | Always true. |

## Messages

| Message | When |
|---|---|
| `form.SubmittedMsg{Values}` | submit with every step that runs valid |
| `form.InvalidMsg{Keys}` | a step or submit refused; `Keys` names the fields |
| `form.CancelledMsg{}` | esc on the first step, or Cancel |

See `examples/appearance/form` (a wizard with every field kind) and
`examples/navigation/prescreen` (a sign-in intro screen with a banner).

# Changelog

Every change to tuilib's public API (`pkg/`) is recorded here, newest first.
Each release's section is also its GitHub release notes.

## Upgrading

To move an app from one version to another, read every section above the
version you are on up to the version you are going to, and work **oldest
first**:

1. `go get github.com/jsdrews/tuilib@<version>`
2. Apply each **Breaking** entry's migration. Each one gives the code before
   and after the change.
3. Run `go fix ./...` (Go 1.26+). A **Deprecated** entry marked `go:fix
   inline` is rewritten for you; the old name keeps working until a later
   release removes it under **Breaking**.
4. `go build ./... && go test ./...`

**Added** and **Changed** entries need no action. They say what is now
available and what behaves differently, so an app can adopt them.

## Writing entries

Add entries under `## Unreleased` in the same change that makes them. The
release job reads that section to pick the next version, publishes it as the
release notes, and moves it under the new version's heading. Never write a
version heading by hand.

| Section | For | Version bump |
|---|---|---|
| `### Breaking` | anything that makes a consumer change code: a removal, a rename, a changed signature, a behavior an app has to adjust to | minor before v1, major after |
| `### Deprecated` | an old name kept working beside its replacement | minor |
| `### Added` | new API | minor |
| `### Changed` | a behavior change that needs no code change | minor |
| `### Fixed` | a bug fix | patch, when it is the only section |

Each Breaking change gets a `####` heading of its own and a `**Migration:**`
block (`go run ./internal/changelog/cmd/changelog check` enforces both):

````markdown
### Breaking

#### `table.Options.Foo` is now `table.Options.Bar`

One or two sentences on what changed and why.

**Migration:**

```go
// before
opts.Foo = 3
// after
opts.Bar = 3
```
````

Other sections are bullets naming the symbol: ``- `palette.Palette.Success`:
a foreground for good outcomes.``

## Unreleased

## v0.46.0 - 2026-11-18

### Breaking

#### The default surface is `surface.Filled`, not `surface.Outlined`

An app that sets no `app.Options.Surfaces` now draws Filled, and so does
`theme.New(p)`. Most apps were choosing Filled anyway. On a terminal that can't
draw it (too few colours, or a palette with no tones), it falls back along its
chain as any surface does. A fallback from this default is not written to the
output console, because the app never chose the surface. Code or tests that
assume a one-cell border (`width-2`, `Y+1`, clicks at fixed coordinates) now
land in the wrong place, as they always did under a filled surface.

**Migration:** to keep the line-border look, name it:

```go
// before
app.New(app.Options{Root: root})
th := theme.New(palette.Dark())
// after
app.New(app.Options{Root: root, Surfaces: []surface.Surface{surface.Outlined}})
th := theme.Compose(palette.Dark(), surface.Outlined, surface.Caps{Profile: termenv.TrueColor, BlockGlyphs: true})
```

#### `pkg/form` draws one card, and sizes and places itself

A form was a stack of bordered boxes, one per field, that filled whatever rect
it was given and drew no key hints. It is now one card: each field a label over
its value with an optional note, typed values in a well a tone up from the
card, a focus bar on the focused field, Submit and Cancel, and a help line. The
card is at most `MaxWidth` (60) wide, exactly as tall as its fields, and
centred in its rect. Behaviour that changed with it:

- Every form is framed: `Options.Title` names the frame and no longer decides
  whether there is one.
- Enter moves to the next field, and on the last field submits. It used to
  move on to the Submit button, and only the button submitted.
- A Select is an always-open list whose cursor is the choice. A `RequirePick`
  select chooses `Initial` when it is reached, rather than waiting for an
  arrow key with nothing highlighted.
- `FieldSpacing` defaults to 1 blank row between fields, not 0.
- There is a Cancel button, and esc still cancels.

**Migration:** most forms need no change beyond the renamed options below. A
screen that sized the form by hand, or wrapped it in `layout.Center` to fake a
modal, hands it the rect instead:

```go
// before
layout.ZStack(base, layout.Center(60, rows+5, layout.Sized(&s.form)))
// after
o.Modal = true // help line inside the card
layout.ZStack(base, layout.Sized(&s.form))
```

#### `form.Options.Width` / `Height` and `SelectOptions.Height` are gone

The layout engine sized the form through `Width` and `Height`, and a select's
`Height` counted its border rows. The form now sizes itself; a select has no
border, and counts options.

**Migration:**

```go
// before
o.Width = 60
form.Select(form.SelectOptions{Key: "role", Options: opts, Height: 6})
// after
o.MaxWidth = 60
form.Select(form.SelectOptions{Key: "role", Options: opts, Visible: 4})
```

#### `form.Styles` has new fields, and `Model.SetStyles` is gone

Fields have no borders or panes of their own any more, so the pane colours,
border shapes and field surfaces have nothing to style. `theme.Form()` fills
the new fields, so a form built from it needs no change. A theme swap rebuilds
the form and carries its state with `State` / `Restore` (rule 4).

**Migration:**

```go
// before
o.Styles.Input = text
o.Styles.Selected = accent
o.Styles.PaneActiveColor, o.Styles.ErrorColor = blue, red
o.Styles.Submit, o.Styles.SubmitActive = idle, active
s.form.SetStyles(o.Styles)
// after
o.Styles.Text = text
o.Styles.Chosen = accent
o.Styles.Bar, o.Styles.Error = lipgloss.NewStyle().Foreground(blue), lipgloss.NewStyle().Foreground(red)
o.Styles.Button, o.Styles.ButtonActive = idle, active
st := s.form.State()
s.form = form.New(o)
s.form.Restore(st)
```

#### `form.Field` is closed

The form now draws every field's label, note, well and focus bar itself, so
`Field` is no longer an interface an app implements: its methods are
unexported. Use the six constructors (`Text`, `Password`, `TextArea`,
`Select`, `MultiSelect`, `Confirm`).

**Migration:** replace a custom field with the nearest built-in, and a custom
check with `Validate`:

```go
// before
fields = append(fields, &myPortField{key: "port"})
// after
fields = append(fields, form.Text(form.TextOptions{Key: "port", Label: "Port",
	Validate: func(v any) error { _, err := strconv.Atoi(v.(string)); return err }}))
```

#### `form.SubmittedMsg.Values` is a `form.Values`

`Values` is `map[string]any` with typed accessors, so indexing still works.
Code that assigns it to a `map[string]any` variable or parameter converts it.

**Migration:**

```go
// before
role := fmt.Sprint(m.Values["role"])
var vals map[string]any = m.Values
// after
role := m.Values.String("role")
vals := map[string]any(m.Values)
```

### Added

- `pkg/banner`: a title in block letters with a drop shadow and a gradient
  that sweeps across the faces, for an intro screen. `theme.Banner()` fills
  its colours. `form.Options.Banner` draws one above a form's card.
- `form.Options.Steps` / `form.Step`: a form as a wizard, with Back and Next,
  esc back a step, per-step validation, and `Step.Skip` to leave a step out
  for the answers so far. `Model.Step()` says where the user is.
- `form.MultiSelect`: any number of choices from an always-open list, with
  `Min` and `Max`. Its value is a `[]string`.
- `form.TextArea`: multi-line text that takes enter as a newline.
- `Note` on every field's options: what the field is for, drawn muted under it.
- `form.Options.Modal`, `MaxWidth`, `NextText`, `BackText`, `CancelText`.
- `form.Model.Height(w)`: the rows the form needs at width `w`.
- `form.Model.State` / `Restore`, `Strings`, and `form.Values.String` /
  `Bool` / `Strings`.
- `form.ConfirmOptions.Yes` / `No`: label the two sides.
- `glyph.Set.RadioOn` / `RadioOff` / `CheckOn` / `CheckOff` / `FieldBar`: the
  marks a form draws.

### Changed

- `surface.Builtins()`: Filled is listed first, so a picker built from it
  starts on the default.

### Fixed

- `pkg/pane`: a title wider than its border row ends in `…` instead of being
  cut off with no sign anything was lost. Every titled component gets this.

## v0.44.0 - 2026-10-26

### Added

- `palette.Palette.Success`: a foreground for a good outcome (Running, Ready,
  Succeeded, Healthy), set in every built-in and named palette. Base16
  palettes take `base0B`. Left unset in a custom palette, `theme.Compose`
  falls back to `InfoBG`, so existing palettes need no change. Use
  `t.Success` instead of a literal green for status values that should
  follow the palette.

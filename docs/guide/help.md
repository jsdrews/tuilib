# Help and key hints

The shell shows key hints in two places:

- **The footer**: the statusbar's left slot. By default it shows only
  `? help`. With `app.Options.HelpVerbose` it packs as many of the screen's
  bindings inline as fit, then the affordance.
- **The overlay**: `?` (or a click on the affordance) opens a bordered modal
  listing every binding the screen exposes, grouped into sections, scrollable,
  and searchable with `/`. `esc`, `q`, `?` again or a click outside closes it.

Both read from the screen. `Help()` returns the flat list; the optional
`HelpSections()` returns the grouping.

## Sections are named by function

Groups say what the keys **do**, from one shared vocabulary:

| Constant | Title | Covers |
|---|---|---|
| `help.SectionNavigate` | Navigate | vertical cursor or line movement |
| `help.SectionScroll` | Scroll | horizontal scroll |
| `help.SectionFilter` | Filter | narrowing what's shown |
| `help.SectionSearch` | Search | finding within what's shown |
| `help.SectionSelect` | Select | marking rows |
| `help.SectionSort` | Sort | column sorting |
| `help.SectionExpand` | Expand | opening and closing branches |
| `help.SectionView` | View | how content renders (wrap, follow) |
| `help.SectionEdit` | Edit | typing, flipping a toggle |
| `help.SectionSubmit` | Submit | committing or abandoning |
| `help.SectionTabs` | Tabs | switching tab bodies |

"Navigate" means the same thing in a list, a table and a tree. Never name a
group after its owner (the screen or component): that heading would end up
over every binding the owner has, including scroll keys it doesn't describe.
When more than one pane is on screen, the owner becomes a **qualifier**,
`files · Navigate`, applied by `help.Qualify` (and by `focus.Group` for you).

## What a screen writes

Forward the component's groups, and add a group for your own verbs, named
after the subject:

```go
func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

func (s *Screen) HelpSections() []help.Section {
    return help.SectionsOf(&s.table, help.Group("Deployments", s.refreshKey))
}
```

`help.SectionsOf` accepts any mix of `help.Sectioned` values (components,
focus groups, tab hosts), anything with `Help() []key.Binding`, a `Section`,
or a `[]Section`.

| Screen holds | `HelpSections` returns |
|---|---|
| one component | `help.SectionsOf(&s.comp, …)` |
| several, in a `focus.Group` | `s.focus.HelpSections()` |
| a `tab.Model` | `s.tabs.HelpSections()` (the strip's keys plus the active body's) |
| a modal that's up | include `&s.modal` while it's up |
| nothing interactive | omit it; the shell makes one group titled with the screen's name |

A screen that hosts a component but doesn't implement `HelpSections` gets the
fallback: one group named after the screen, standing over bindings it doesn't
describe. Forward the groups.

## The Global group

The shell writes a **Global** section itself: quit or back (depending on
stack depth), theme, suspend, and the opt-in output and actions keys. Then
`help.Suppress` removes those keys from the screen's own groups, so a screen
that lists `q` and `t` in its `Help()` isn't shown twice. Repeats across panes
are kept: two panes binding `↑/k` are two different verbs.

Don't list the output key in a screen's help. The shell advertises it, and
relabels it "close output" while the console is open.

## Mouse affordances

Advertise mouse gestures as bindings with sentinel keys so they appear in the
overlay:

```go
key.NewBinding(key.WithKeys("mouse:click"), key.WithHelp("click", "focus + select"))
```

The sentinel keeps two keyless bindings from being deduplicated into one, and
can never match a real key press.

## Configuration

| Option | Effect |
|---|---|
| `app.Options.HelpKey` | The key that opens the overlay (default `?`; `key.NewBinding()` disables it). |
| `app.Options.HelpVerbose` | Inline bindings in the footer. |
| `app.Options.DisableHelpSearch` | Remove the overlay's `/` search. |

The affordance is the source of truth: if `? help` isn't drawn, the key does
nothing.

## Using the pieces directly

Outside the shell, `help.New(t.Help())` is the footer (`ShortView`,
`ShortViewBudget`) and `help.NewOverlay(t.HelpOverlay())` is the modal
(`SetSections`, then host it in a `ZStack` with `layout.Sized`, and dismiss on
`help.ClosedMsg`). See [Packages: help](../reference/packages.md#help).

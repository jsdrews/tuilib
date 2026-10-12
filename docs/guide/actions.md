# Verbs and multi-select

A screen's verbs (restart, delete, sync, view logs) are declared as data and
shown in a menu, not bound to letters. A footer holds one row and a screen can
easily have nine verbs; letters run out, collide with the reserved scroll
keys, and hide the verbs from discovery.

## The action menu

Turn it on in the shell:

```go
app.New(app.Options{
    // …
    ActionsKey: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "actions")),
})
```

Then implement `action.Provider` on any screen that has verbs:

```go
func (s *Screen) Actions() action.Set {
    return action.Set{
        Target:  s.table.SelectionLabel(), // "cache-redis" or "3 items"
        Targets: s.table.Selection(),
        Actions: []action.Action{
            {
                Label: "Restart",
                Multi: true,
                Exclusive: true,
                Run: func(ctx context.Context, out io.Writer) error {
                    for _, name := range s.table.Selection() {
                        fmt.Fprintf(out, "restarting %s\n", name)
                        if err := s.api.Restart(ctx, name); err != nil {
                            return err
                        }
                    }
                    return nil
                },
            },
            {
                Label:   "Delete",
                Multi:   true,
                Confirm: "Delete " + s.table.SelectionLabel() + "?",
                Run:     s.deleteSelected,
            },
            {
                Label: "View logs",
                Do: func() tea.Cmd {
                    key, _ := s.table.SelectedKey()
                    return screen.Push(newLogs(key, s.t))
                },
            },
        },
    }
}
```

`a`, or a right-click (with `app.MouseClick`), opens the menu over the
screen, titled with the target. The shell runs the chosen action:

- a `Run` action runs in the background through `runner.Go`: its output goes
  to the output console, it's listed in the kill picker, and the shell posts
  `<Label> completed` (or the error) when it returns;
- a `Do` action's command is returned as-is.

### `Action` fields

| Field | Meaning |
|---|---|
| `Label` | The verb. Required. |
| `Desc` | A short gloss beside the label. |
| `Key` | Optional shortcut while the menu is open (shown in its key column, not in the footer). |
| `Run` | Background work: `func(ctx context.Context, out io.Writer) error`. Prefer it. |
| `Do` | Escape hatch for verbs that aren't background work (push a screen, open `$EDITOR`). Set exactly one of `Run` and `Do`. |
| `Confirm` | When non-empty, a yes/no modal with this text sits between the pick and the run. |
| `Disabled` | When non-empty, the row is dimmed and unselectable, with this text as the reason. Showing an unavailable verb beats hiding it. |
| `Multi` | The action accepts more than one target. Off by default: a non-`Multi` action is disabled under a multi-selection. |
| `Exclusive` | Refuse a second concurrent run for the same target; the row shows "already running". |
| `ID` | Scopes `Exclusive`. Defaults to `Label`. |
| `Receipt` | Replaces the "`<Label>` completed" text for a `Run` action. Use "Sync requested" when `Run` returning means the server accepted the work, not finished it. |

`action.Validate(set)` reports authoring mistakes (missing label, both or
neither of `Run`/`Do`, duplicate shortcuts or identities). Call it from a test.

### `Set` fields

| Field | Meaning |
|---|---|
| `Target` | Display name of what the verbs act on; titles the menu. |
| `Targets` | The keys the verbs act on (what `Selection()` returned). Sharpens `Exclusive` to per-target, and rides on `ChosenMsg` so you can start row spinners. |
| `Count` | Number of targets when there are no `Targets`. |
| `Actions` | The verbs. |

`Actions()` is called on menu open and on key dispatch: keep it cheap, with no
I/O.

### Hosting the menu yourself

Outside the shell, `action.New(t.Actions())` is a self-sizing menu. Place it
with `Anchor(x, y)` (at a right-click) or `Center()`, host it in a `ZStack`
with `layout.Sized`, forward messages while it's up, and match
`action.ChosenMsg` / `action.CancelledMsg`. See
[Action menu](../components/action-menu.md).

## Multi-select

`list`, `table` and `tree` let the user mark rows. Turn it on with
`Options.Markable`:

| Key / gesture | Effect |
|---|---|
| `x` | toggle the mark on the cursor row |
| `X`, or shift+click | mark every row between the anchor (the last marked row) and the cursor, in either direction; additive |
| `A` | mark every row the filter currently shows; press again to clear them |
| `D` | drop every mark |
| click the `✓` gutter | toggle that row without opening it |

`D` is separate from `A` so that clearing never routes through "everything
marked", which is a bad state to pass through just before a destructive verb.
`X` is the range key because terminals don't send `shift+space` distinctly,
and shift+click may be reserved by the terminal.

### Read `Selection()`

```go
keys := s.table.Selection()       // marked keys, or the cursor row's key if none
label := s.table.SelectionLabel() // "cache-redis" or "3 items"
```

`Selection()` is "the marks, or the cursor row when nothing is marked", the
branch every verb needs. Its absence leads to a verb acting on one row when
the user marked six. Use `Marks()` only when you specifically want marks
without the fallback.

### Marks are held by key

In `list` and `table`, marking requires keyed data (`SetKeyedItems`,
`SetKeyedRows`). A polled refresh that reorders rows can't slide the
selection onto neighbors. On anonymous data (`SetItems`, `SetRows`) marking is
a deliberate no-op, so don't set `Markable` there. A windowed table
(`SetWindow`) can't be marked either.

In `tree`, a node's path is its key, so no keyed setter is needed. Marking a
branch marks that node alone; a verb that wants the subtree can prefix-test
the returned paths. `A` marks only visible rows: expanded, and surviving the
filter.

Marks survive filtering: a user can mark a row, filter it out of view, and
still act on it. That's why the menu puts the target ("3 items") on its own
border. Marks come back across `SetTheme` with the rest of `State()`.

See `examples/patterns/multiselect` and `examples/patterns/treeactions`.

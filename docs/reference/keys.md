# Default key bindings

Every binding a component dispatches lives in its `Options.Keys` struct (from
`DefaultKeys()`), so each one can be overridden. See
[Overriding bindings](../guide/input.md#overriding-bindings).

## App shell

| Key | Action | Option |
|---|---|---|
| `q`, `ctrl+c` | quit (at the root) | `QuitKey` |
| `esc` | pop the stack (when deeper than one) | `DisableAutoEscPop` |
| `?` | key overlay | `HelpKey` |
| `ctrl+z` | suspend | `SuspendKey`, `DisableSuspend` |
| none | cycle palette | `ThemeKey` (opt-in; `t` by convention) |
| none | cycle surface | `SurfaceKey` (opt-in; `T` by convention) |
| none | output console | `OutputKey` (opt-in) |
| none | action menu | `ActionsKey` (opt-in) |

All are suppressed while the active screen reports `IsCapturingKeys()`.

## Pane (horizontal scroll, shared by every component as `Keys.Pane`)

| Key | Field | Action |
|---|---|---|
| `←`, `h` | `Left` | scroll left |
| `→`, `l` | `Right` | scroll right |
| `0`, `home` | `LeftEdge` | start |
| `$`, `end` | `RightEdge` | end |

## List

| Key | Field | Action |
|---|---|---|
| `↑`, `k` / `↓`, `j` | `Up` / `Down` | move |
| `g` / `G` | `Top` / `Bottom` | first / last |
| `ctrl+u` / `ctrl+d` | `HalfUp` / `HalfDown` | half page |
| `/` | `Filter` | filter |
| `x` | `Mark` | mark |
| `X` | `MarkRange` | mark to here |
| `A` | `MarkAll` | mark all |
| `D` | `ClearMarks` | drop marks |

## Menu

| Key | Field | Action |
|---|---|---|
| `↑`, `k` / `↓`, `j` | `Up` / `Down` | move (skipping headers) |
| `g` / `G` | `Top` / `Bottom` | first / last |
| `ctrl+u` / `ctrl+d` | `HalfUp` / `HalfDown` | half page |
| `[` / `]` | `PrevGroup` / `NextGroup` | previous / next group |
| `/` | `Filter` | filter |
| `enter` | `Choose` | open |

## Table

Everything in List, plus:

| Key | Field | Action |
|---|---|---|
| `[` / `]` | `SortPrev` / `SortNext` | step the sort column |
| `s` | `SortDir` | toggle sort direction |
| `shift+←` / `shift+→` | `ColPrev` / `ColNext` | snap to previous / next column |
| `tab` (in the filter) | | complete a `key:value` term |

## Tree

| Key | Field | Action |
|---|---|---|
| `↑`, `k` / `↓`, `j` | `Up` / `Down` | move |
| `g` / `G` | `Top` / `Bottom` | first / last |
| `space` | `Toggle` | expand / collapse |
| `enter` | `Enter` | select |
| `E` / `C` | `ExpandAll` / `CollapseAll` | expand / collapse all |
| `}` / `{` | `NextSibling` / `PrevSibling` | sibling |
| `J` / `K` | `NextLeaf` / `PrevLeaf` | leaf |
| `/` | `Search` | search |
| `n` / `N` | `NextMatch` / `PrevMatch` | match |
| `\` | `Filter` | filter mode |
| `x`, `X`, `A`, `D` | `Mark`, `MarkRange`, `MarkAll`, `ClearMarks` | multi-select |

## Inspector

| Key | Field | Action |
|---|---|---|
| `↑`, `k` / `↓`, `j` | `Up` / `Down` | move |
| `g` / `G` | `Top` / `Bottom` | first / last |
| `ctrl+u` / `ctrl+d` | `HalfUp` / `HalfDown` | half page |
| `space` | `Toggle` | expand / collapse |
| `E` / `C` | `ExpandAll` / `CollapseAll` | expand / collapse all |
| `}` / `{` | `NextSibling` / `PrevSibling` | sibling |
| `J` / `K` | `NextLeaf` / `PrevLeaf` | leaf |
| `/` | `Search` | search |
| `n` / `N` | `NextMatch` / `PrevMatch` | match |
| `\` | `Filter` | filter mode |

## LogView

| Key | Field | Action |
|---|---|---|
| `/` | `Search` | search |
| `n` / `N` | `NextMatch` / `PrevMatch` | match |
| `g` / `G` | `Top` / `Bottom` | top / bottom and follow |
| `\` | `Filter` | filter mode |

Vertical scrolling (`↑`/`k`, `↓`/`j`, `pgup`, `pgdown`) comes from the pane.

## TextView

| Key | Field | Action |
|---|---|---|
| `/` | `Search` | search |
| `n` / `N` | `NextMatch` / `PrevMatch` | match |
| `g` / `G` | `Top` / `Bottom` | top / bottom |
| `w` | `Wrap` | toggle wrap |

## Eventlog

| Key | Field | Action |
|---|---|---|
| `↑`, `k` / `↓`, `j` | `Up` / `Down` | move |
| `g` / `G` | `Top` / `Bottom` | first / newest and follow |
| `ctrl+u` / `ctrl+d` | `HalfUp` / `HalfDown` | half page |
| `enter` | `Open` | open |
| `f` | `Filter` | filter |
| `/` | `Search` | search |
| `n` / `N` | `NextMatch` / `PrevMatch` | match |

## Filter and Input (while focused)

| Key | Filter | Input |
|---|---|---|
| `enter` | commit (blur, keep value) | passed to the screen |
| `esc` | clear and blur | blur, keep value |

## Toggle

`←`/`h`, `→`/`l`, `space` flip; `y` / `n` set.

## Form

`tab` / `shift+tab` move between fields; `enter` on the button submits;
`esc` cancels.

## Confirm

`←`/`h`, `→`/`l`, `tab`, `shift+tab` move; `y`/`Y` confirm; `n`/`N`, `esc`
cancel; `enter` commits the highlighted button.

## Alert

`enter`, `space`, `esc`, `o`, `O` dismiss. With `Autosize` and overflow:
`↑`/`k`, `↓`/`j`, `g`/`G`, `ctrl+u`/`ctrl+d`, `pgup`/`pgdown` scroll.

## Action menu

| Key | Field | Action |
|---|---|---|
| `↑`, `k` / `↓`, `j` | `Up` / `Down` | move |
| `g` / `G`, `ctrl+u` / `ctrl+d` | `Top` / `Bottom`, `HalfUp` / `HalfDown` | jump |
| `shift+tab` / `tab` | `PrevGroup` / `NextGroup` | previous / next group |
| `enter` | `Choose` | run |
| `esc` | `Cancel` | cancel |

## Tabs

`shift+←` / `shift+→` (`PrevKey` / `NextKey`) and `1`–`9`.

## Focus group

`tab` / `shift+tab` (`focus.Keys{Next, Prev}`).

## Help overlay

`/` search, `g` / `G` top / bottom, `esc` or `q` close.

## Output console

`c` clear, `x` kill a run, `w` export (`output.Keys`), plus the logview's keys.

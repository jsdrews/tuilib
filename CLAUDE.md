# Agent guidance for tuilib

This file is read automatically when an agent enters this repo. It encodes
the rules that keep generated code consistent with the library's design. For
recipes and the full component reference, read `README.md` and the relevant
example in `examples/`.

## The five rules

1. **Always start from a `theme` builder.** Every component has a
   `th.Component()` method on `Theme` that returns a pre-styled `Options`.
   Set the task-specific fields (`Width`, `Height`, `Title`, data) and pass
   it to `component.New()`. Don't set colors inline unless you're
   deliberately overriding one.

2. **Rebuild everything on `WindowSizeMsg` and theme swap.** Implement an
   `apply()` method that reconstructs every themed component from the
   current theme + dimensions. Preserve per-component state across the
   rebuild via accessors (`m.list.Cursor()`, `m.list.Value()`, …) and
   setters (`m.list.SetCursor`, `m.list.SetValue`). See
   `examples/data/list/main.go` for the canonical pattern.

3. **Gate global keys on focus.** When any component owns text input, its
   keystrokes must not leak to global handlers. Check first:
   ```go
   if !m.list.Filtering() {
       if key.Matches(msg, quitKey) { return m, tea.Quit }
   }
   m.list, cmd = m.list.Update(msg)  // always forward afterwards
   ```
   The rule is: *global keys only when no embedded component is in input
   mode*. Use `list.Filtering()`, `filter.Focused()`, etc.

4. **Forward every `tea.Msg` to embedded components.** Don't conditionally
   skip forwarding — each component decides what to act on. Even if you
   intercepted a key above, still forward it so focus + viewport behavior
   stays correct.

5. **Use the standard app shape.** Breadcrumb (1 row) + body (flex) +
   statusbar (1 row). For filterable widgets, the filter is *inside* the
   body — `list.Model` with `Filterable=true` absorbs its own 3-row filter
   pane. Body gets `m.h - 2`.

## Anti-patterns

- **Don't instantiate `textinput.New()` directly.** Use `filter.Model` or
  `list.Model` with `Filterable=true`.
- **Don't wrap `bubbles/table`.** We deliberately don't provide a table
  component — bubbles/table already owns rendering + scrolling + cursor,
  and wrapping it is passthrough bloat. For a filterable table, compose
  `bubbles/table` + `filter.Model` + `pane.Pane` directly. See
  `examples/data/table/main.go`.
- **Don't set colors in `Options` literals.** Start from the theme builder.
- **Don't skip state preservation in `apply()`.** If you forget to carry
  cursor/value across rebuilds, theme-swap and resize will silently reset
  the user's state.
- **Don't write per-component reset codes.** If bar colors drift between
  embedded segments, the fix is usually "make sure every embedded style
  sets the same `Background()`," not a manual `\x1b[0m`.
- **Don't add a comment explaining what well-named code does.** Component
  doc comments belong at the package and exported-symbol level; inline
  code should be self-describing.

## Layout math reference

| Component | Rows consumed |
|---|---|
| `breadcrumb.Model` | 1 |
| `statusbar.Model` | 1 |
| `filter.Model` | 3 (border + content + border) |
| `pane.Pane` | caller-controlled, min 3 |
| `list.Model` (Filterable=false) | caller-controlled |
| `list.Model` (Filterable=true) | caller-controlled, internally splits 3 for filter + rest for body |

Typical body height:
- Plain body pane: `m.h - 2`
- Body pane + standalone `filter.Model` above: `m.h - 5`
- `list.Model` filterable: `m.h - 2` (the filter is inside it)

## Where to learn more

- **Package overview:** `go doc ./pkg/<name>` prints the package doc
  comment + every exported symbol's signature and doc.
- **Full config surface:** `go doc ./pkg/<name>.Options`.
- **Wiring patterns:** `examples/<area>/<name>/main.go` — each is a flat,
  self-contained `main.go`.
- **Color vocabulary:** `pkg/theme/theme.go` — field comments on the
  `Theme` struct name every semantic slot.

When in doubt: read the nearest example and copy its structure. The
examples are maintained as the source of truth for idiomatic composition.

# Library rules

tuilib is opinionated. These rules keep apps built on it consistent with each
other and with the library's own components. The full text, with the reasoning
and the anti-patterns each rule prevents, is in the repository's
[`CLAUDE.md`](https://github.com/jsdrews/tuilib/blob/main/CLAUDE.md), which
is written for people and coding agents alike.

## Structure

1. **Default to `pkg/app` + `pkg/screen` + `pkg/layout`.** A tuilib app is a
   root screen handed to `app.New`. Drop to a bare `tea.Model` only when you
   need something the shell's shape can't express.
2. **Describe layout declaratively.** `VStack`/`HStack`/`ZStack` with
   `Fixed`/`Flex`. Never `height - 2`.
3. **Start from a theme builder.** `t.List()`, `t.Table()`, …; set only
   instance-specific fields. Don't set `Width`/`Height` on components placed
   with `layout.Sized`.
4. **Rebuild in `SetTheme`, carrying `State()` / `Restore()`.** Never restore
   field by field.
5. **Gate global keys with `IsCapturingKeys()`.** True while any embedded
   field or filter owns input, or a modal is up.
6. **Forward every message.** Keys and mouse go to the top screen; everything
   else reaches every screen on the stack. In a focus group, keys go to the
   focused component and mouse to all.
7. **Pass data through the stack.** Constructor parameters down,
   `screen.Pop(result)` → `OnEnter(result)` up.
8. **Make interaction menu-driven.** Lists and `enter`, or the action menu, for
   per-screen verbs. Single letters for app-wide affordances only.
9. **Components own their pane.** Label with `Title`; never wrap a component in
   another pane or put a label line above it. Filters live inside their pane.
10. **Expose `Help()` and `HelpSections()`.** Groups are named by function from
    the shared vocabulary, never after their owner.

## Interaction

- **Arrows and `hjkl` are for scrolling**, library-wide, including the mouse
  wheel. No component binds them to anything else.
- **`enter` means "open the focused selection"**; double-click is the same
  verb. Route both through `IsActivate`.
- **`tab` / `shift+tab` cycle focus**; tabs switch with `shift+←`/`→` and
  `1`–`9`.
- **Click focuses and selects; it's always safe.** Only double-click opens.
- **Override keys through `Options.Keys`**, never a package-level keymap.
- **Compose focus with `focus.Group`**, never an int index.
- **Mouse comes from rects**, never markers injected into the output.

## Feedback

- **`app.Info` / `app.Error` for passing notices**; they clear on the next key.
- **Verbose output goes to the console** (`app.ErrorOf`, `InfoDetail`,
  `runner.Capture`), never into the statusbar.
- **`pkg/confirm` for yes/no, `pkg/alert` for acknowledge.** Don't roll your
  own modal.
- **Ignore `app.OutputClosed` in `OnEnter`.**
- **Severity comes from exit status**, never from stderr.

## Data

- **`SetLoading` while a whole body is in flight**; drop stale data first.
- **Let the pane truncate long lines**; pre-wrap only prose.
- **Color table cells with `ansi.CellColor`**, and links with
  `ansi.Hyperlink`.
- **Auto-refresh with `pkg/poll` and keyed rows**, so the cursor and marks
  follow keys, not positions.
- **Cap streaming buffers.** Use `-1` only for bounded producers.
- **Remote data: bind with `pkg/remote`; scrolling is the pagination.** No
  page keys, no `SetRows` per page, no `SetLoading` or `SetCursor(0)` in the
  loop, no query per keystroke.
- **Choose the component by the data's shape** (Record / Event / Line ×
  Seekable / Anchored / Streamed). Events never go in a table; search jumps,
  filter narrows.
- **Multi-select needs keyed data; read `Selection()`, not `Marks()`.**
- **Row activity follows the server.** Pick `Observed` or `Held` by who knows
  when the work ends; switch on what `Done` returns; no hold timers, no outcome
  glyphs.

## Subprocesses

- **`runner.Run` hands over the terminal; `runner.Capture` keeps the TUI
  live.** Don't call `tea.ExecProcess` directly or hand-roll a pipe reader.
- **Stream other producers through a chained `tea.Cmd`**: one goroutine owns
  the source, a command reads one item, `Update` re-dispatches. No goroutine
  touches a model.

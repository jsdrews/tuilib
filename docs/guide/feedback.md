# Feedback and the console

tuilib has three weights of feedback. Pick the lightest that's honest.

| Weight | Use for | Tool |
|---|---|---|
| Passing | "Refreshed 14 deployments", "Sync requested" | `app.Info` / `app.Error` |
| Readable later | a command's stderr, an error chain, an API response | the output console |
| Blocking | the user must decide or acknowledge before continuing | `pkg/confirm`, `pkg/alert` |

## Statusbar messages

```go
return s, app.Info("Deployment triggered")
return s, app.Error("Error: API request failed")
return s, app.ClearStatus()   // wipe a stale message on a non-key event
```

The shell paints the statusbar's center slot (green for info, red for error).
Messages clear on the next keypress, so don't manage their lifetime. The slot
holds one truncated line, so anything worth reading twice needs the console.

Screens never touch the statusbar directly; return these commands from
`Update`.

## The output console

Turn it on by giving it a key:

```go
app.New(app.Options{
    // …
    OutputKey: key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "output")),
})
```

It's a pushed screen (a logview over the app's log) opened with the key,
by clicking the statusbar's output badge, or by **clicking a status message**,
since the message is a truncated echo of what the log holds in full. Inside
it, `/` searches, `\` filters, `c` clears, `x` kills a running capture, `w`
exports to a file, and `esc` or the key closes it.

### What feeds it

| Source | Goes to the statusbar | Goes to the console |
|---|---|---|
| `app.Info(s)` / `app.Error(s)` | `s` | `s` |
| `app.InfoDetail(summary, body)` / `app.ErrorDetail(…)` | `summary` | `summary`, then `body` as continuation lines |
| `app.ErrorOf(err)` | `err.Error()` | the unwrapped `%w` chain, one wrap per line |
| `runner.Capture(cmd)` / `runner.Go(…)` | nothing per line | every line, stdout and stderr |
| `runner.Run(cmd)` | n/a | the exit status |
| `pkg/source` query history | failures | every answered, failed and cancelled query |

So existing `app.Info`/`app.Error` calls become recoverable with no change,
and `app.ErrorOf(err)` is the right default for surfacing an error.

### The badge

The statusbar's right slot shows `3 output` while there are unread events,
and `output` once everything has been seen. It counts **events, not lines**:
one `Info`, or one whole capture. It turns red from a run's **exit status**,
never from stderr, because many well-behaved tools write progress to stderr.

### Opening and closing

Opening the console clears the statusbar message (it's about to be shown in
full). Closing it pops with the sentinel `app.OutputClosed`, which a screen
that fetches in `OnEnter` must ignore:

```go
func (s *Screen) OnEnter(result any) tea.Cmd {
    if _, closed := result.(app.OutputClosed); closed {
        return nil
    }
    return s.fetch()
}
```

### Options

`app.Options.Output` takes an `output.Options` (start from
`output.OptionsFrom(t)`):

| Field | Default | Meaning |
|---|---|---|
| `MaxRecords` | 10000 | Ring size. Whole events are trimmed from the front, so a surviving line still has the head naming its command. |
| `ExportDir` | `os.TempDir()` | Where `w` writes. |
| `SourceWidth` | 10 | Column the source name is padded to. |
| `Keys` | `c` clear, `x` kill, `w` export | The console's own keys. |
| `Logview`, `Confirm`, `Picker` | from the theme | The console's body, kill confirmation and run picker. |

Lines are rendered at view time, so a theme swap recolors the whole log.

## Modals

Use a modal only when the user must stop.

- **`pkg/confirm`**: a yes/no question before a destructive action. Cancel is
  highlighted by default. See [Confirm](../components/confirm.md).
- **`pkg/alert`**: something to acknowledge: a failure, a blocking notice.
  Use `Autosize` for dynamic text like stderr. See
  [Alert](../components/alert.md).

Both are hosted the same way: the screen owns an `xxxUp bool`, returns `true`
from `IsCapturingKeys()` while it's up, forwards every message to the modal
while it's up and none while it's down, and matches the result messages.

```go
func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    if s.confirmUp {
        switch msg.(type) {
        case confirm.ConfirmedMsg:
            s.confirmUp = false
            return s, s.deleteSelected()
        case confirm.CancelledMsg:
            s.confirmUp = false
            return s, nil
        }
        var cmd tea.Cmd
        s.confirm, cmd = s.confirm.Update(msg)
        return s, cmd
    }
    // … normal handling
}
```

An action declared with `Confirm: "Delete cache-redis?"` gets this modal from
the action menu without any of the above. See [Verbs](actions.md).

`examples/patterns/modals` shows all three weights on one screen.

# Running subprocesses

`pkg/runner` runs work from inside a Bubble Tea program in three shapes. They
differ in what happens to the terminal and to the output.

| Function | The TUI | The output | Use for |
|---|---|---|---|
| `runner.Run(cmd)` | suspended; the subprocess gets the real terminal | gone (the program owns the screen) | `$EDITOR`, `less`, `htop`, `ssh`, `kubectl exec` |
| `runner.Capture(cmd)` | stays live | streamed back line by line, into the console | builds, scripts, CLIs whose output you want to read |
| `runner.Go(label, fn)` | stays live | whatever `fn` writes to its `io.Writer` | HTTP calls, API requests, file writes |

`pkg/runner` imports nothing from tuilib. The app shell turns its messages
into console records.

## Handing over the terminal: `Run`

```go
case tea.KeyMsg:
    if msg.String() == "e" {
        return s, runner.Run(exec.Command(os.Getenv("EDITOR"), s.path))
    }
case runner.Result:
    if msg.Err != nil {
        return s, app.ErrorOf(msg.Err)
    }
    return s, s.reload()
```

`Run` suspends the alt-screen, connects the subprocess to the real terminal,
sets `LINES` and `COLUMNS` (for programs that miss the post-resume `SIGWINCH`),
clears the screen first, and posts a `runner.Result` when it exits.

| Variant | Use |
|---|---|
| `runner.RunWithNotice(cmd, "connecting…")` | Prints a line during a slow handoff (ssh, kubectl exec). |
| `runner.RunWith(runner.Options{Cmd, Notice, NoClear})` | Full control; `NoClear` keeps the normal screen visible. |

Don't call `tea.ExecProcess` directly. The shell re-enables mouse reporting
after `Run` and after `ctrl+z`, which Bubble Tea's restore doesn't.

## Capturing in the background: `Capture`

```go
return s, runner.Capture(exec.Command("go", "build", "./..."))
```

The sequence is `CaptureStarted`, one `CapturedLine` per line (with `Stderr`
set for stderr), then exactly one `Captured` (with `Err` set on a non-zero
exit). Under the app shell, the shell drives the reads for you and forwards
every message on to your screen; with `OutputKey` set, every line also goes to
the output console. A screen that also wants the output on screen just
matches the messages:

```go
case runner.CaptureStarted:
    s.log.Clear()
case runner.CapturedLine:
    s.log.Append(msg.Text)
case runner.Captured:
    if msg.Err != nil {
        return s, app.Error("build failed")
    }
    return s, app.Info("build succeeded")
```

Outside the shell, chain the reads yourself with `runner.Next(msg)` after each
`CaptureStarted` and `CapturedLine`. A capture nobody reads eventually stalls
the subprocess rather than buffering without limit.

`runner.CaptureWith(runner.CaptureOptions{Cmd, Label, Tag})` sets the label
shown in the console and kill picker, and a tag echoed on every message.
`runner.Kill(started)` stops the process **and everything it spawned** (the
whole process group on Unix), so killing a shell that runs a compiler stops the
compiler too. In the console, `x` does this.

Severity comes from the exit status, never from stderr: many tools write
progress to stderr.

## Go functions: `Go`

```go
return s, runner.Go("restart api", func(ctx context.Context, out io.Writer) error {
    fmt.Fprintln(out, "scaling to 0")
    return api.Restart(ctx, "api")
})
```

`Go` runs the function on its own goroutine and emits the same message
sequence as `Capture`, so the console, the badge and the kill picker treat it
the same way. Killing it cancels `ctx`; stopping is up to the function, so
pass `ctx` to your HTTP calls. `runner.GoWith(runner.GoOptions{Label, Detail,
Run, Tag, Context})` is the long form. A `Run` action in the
[action menu](actions.md) is executed this way.

## Streaming anything else

For other producers (an SSE response, a tailed file, a websocket) use the same
shape `Capture` uses internally: one goroutine owns the source and pushes into
a channel; a `tea.Cmd` reads one item and returns it as a message; `Update`
appends it and returns the read command again. No goroutine touches a model
directly. Cap the buffer: `logview` defaults to 10,000 lines.

## Suspend

`ctrl+z` suspends the program to the shell (`app.Options.SuspendKey`, off with
`DisableSuspend`). It's suppressed while a screen is capturing keys, and
ignored on Windows.

See `examples/processes`, which runs all three: a capture, a terminal handoff,
and the console.

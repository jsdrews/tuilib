# The app shell

`pkg/app` is the standard frame for a tuilib program: a breadcrumb on top, the
active screen's layout in the middle, a statusbar at the bottom, and every
app-wide behavior around them.

```go
m := app.New(app.Options{
    Root:     newRootScreen(),
    Themes:   theme.All(),
    Version:  "v1.2.0",
    ThemeKey: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
    Mouse:    app.MouseClick,
})
if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
    log.Fatal(err)
}
```

`app.New` resolves the initial theme, calls the root's `SetTheme` with it,
and returns a `tea.Model`. You don't pass mouse options to `tea.NewProgram`;
the shell enables reporting from `Init` when `Mouse` asks for it.

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Root` | `screen.Screen` | **required** | The bottom of the navigation stack. |
| `Themes` | `[]theme.Theme` | **required** | Themes the user cycles through. `Themes[0]` is the initial theme after resolution (below). |
| `Version` | `string` | `""` | Drawn on the right of the statusbar. |
| `QuitKey` | `key.Binding` | `q`, `ctrl+c` | Quits when the stack is at the root and no screen is capturing keys. |
| `ThemeKey` | `key.Binding` | unset (off) | Cycles themes. Leave zero to pin the app to one theme. |
| `HelpKey` | `key.Binding` | `?` | Opens the key overlay. Pass `key.NewBinding()` to disable. |
| `SuspendKey` | `key.Binding` | `ctrl+z` | Suspends to the shell. Ignored on Windows. |
| `DisableSuspend` | `bool` | `false` | Turns the suspend key off entirely. |
| `OutputKey` | `key.Binding` | unset (off) | Opens the [output console](feedback.md#the-output-console). Setting it is what turns the console on. |
| `Output` | `output.Options` | `output.OptionsFrom(theme)` | Console configuration: record cap, export directory, keys. |
| `ActionsKey` | `key.Binding` | unset (off) | Opens the [action menu](actions.md) on screens that implement `action.Provider`. |
| `Actions` | `action.Options` | `theme.Actions()` | Menu configuration. |
| `HelpVerbose` | `bool` | `false` | Pack the screen's bindings inline in the footer instead of showing only `? help`. |
| `DisableHelpSearch` | `bool` | `false` | Remove the `/` search field from the key overlay. |
| `Mouse` | `app.MouseMode` | `MouseOff` | `MouseClick` enables presses, releases, the wheel and drags. |
| `DoubleClickInterval` | `time.Duration` | config file, then 500ms | Window for a second press to count as a double click. |
| `ThemeEnvVar` | `string` | `""` | Env var naming the initial theme, e.g. `MYAPP_THEME`. |
| `SkipConfig` | `bool` | `false` | Don't reorder `Themes` from the env var and config file. |
| `DisableAutoEscPop` | `bool` | `false` | Stop `esc` from popping the stack. |

!!! note "Why some keys are opt-in"
    A key the shell claims is a key no component in any app built on it may
    bind. `ThemeKey`, `OutputKey` and `ActionsKey` therefore default to off, and
    turning one on is a line the app author writes on purpose.

## Global keys

| Key | Default | When it acts |
|---|---|---|
| quit | `q`, `ctrl+c` | stack depth is 1 |
| back | `esc` | stack depth > 1 (pops the top screen) |
| theme | opt-in | always |
| help | `?` | toggles the key overlay |
| suspend | `ctrl+z` | not on Windows |
| output | opt-in | toggles the output console |
| actions | opt-in | the screen implements `action.Provider` |

All of them are suppressed while the active screen's `IsCapturingKeys()`
returns true, so they reach a focused text field as ordinary characters.
Screens should list `q` and `t` in their own `Help()` if they like; the shell
removes duplicates of its own globals from the overlay. Don't handle these
keys in a screen.

## Initial theme resolution

Unless `SkipConfig` is set, `app.New` reorders `Themes` so the first entry
is, in priority order:

1. the theme whose `Name` matches the env var named by `ThemeEnvVar`,
2. the `theme:` field of `$XDG_CONFIG_HOME/tuilib/config.yaml`
   (default `~/.config/tuilib/config.yaml`),
3. `Themes[0]` unchanged.

Unknown names fall through silently. The library never creates or writes the
config file. See [Theming and state](theming.md#user-defaults).

## Statusbar messages

Screens post transient feedback by returning a command; the shell paints the
statusbar's center slot:

```go
return s, app.Info("Deployment triggered")
return s, app.Error("API request failed")
return s, app.ClearStatus()
```

Messages clear on the next keypress. The clear runs before the screen handles
that key, so a screen can set a fresh message in response to the same key.
See [Feedback and the console](feedback.md).

## Live theme preview

`app.SetTheme(name)` returns a command that switches to the named theme (if
it's in `Themes`) and rebroadcasts it across the stack. A theme picker can call
it as its cursor moves.

## Subprocesses and suspend

The shell re-enables mouse reporting after a `runner.Run` subprocess and after
resuming from `ctrl+z`, since Bubble Tea's terminal restore doesn't track mouse
state. If you drive `tea.ExecProcess` or `tea.Suspend` without the shell, you
have to do that yourself. See [Running subprocesses](subprocesses.md).

## Without the shell

You can use every component in a bare `tea.Model`. You then wire the
breadcrumb, statusbar, resize handling, theme rebuilds and mouse translation
yourself. Mouse support in particular needs something to turn `tea.MouseMsg`
into `mouse.Msg`; the shell is what does that. Prefer the shell unless you need
a frame it can't express.

# Tabs

`pkg/tab` hosts several `screen.Screen` bodies behind a one-row strip, inside
one host screen. Each body keeps its own state across switches. Tabs are not a
navigation level: they never touch the screen stack.

## Usage

```go
type host struct {
    cities  *citiesTab // hold pointers so SetTheme can reuse them
    logs    *logsTab
    counter *counterTab
    tabs    tab.Model
}

func newHost(t theme.Theme) *host {
    h := &host{cities: newCities(t), logs: newLogs(t), counter: newCounter(t)}
    h.tabs = tab.New(tab.Options{
        Theme: t,
        Tabs: []tab.Tab{
            {Label: "Cities", Body: h.cities},
            {Label: "Logs", Body: h.logs},
            {Label: "Counter", Body: h.counter},
        },
    })
    return h
}

func (h *host) Title() string                { return "Tabs › " + h.tabs.ActiveLabel() }
func (h *host) Init() tea.Cmd                { return h.tabs.Init() }
func (h *host) OnEnter(r any) tea.Cmd        { return h.tabs.OnEnterActive(r) }
func (h *host) IsCapturingKeys() bool        { return h.tabs.IsCapturingKeys() }
func (h *host) SetTheme(t theme.Theme)       { h.tabs.SetTheme(t) }
func (h *host) Help() []key.Binding          { return h.tabs.Help() }
func (h *host) HelpSections() []help.Section { return h.tabs.HelpSections() }
func (h *host) Layout() layout.Node          { return layout.Sized(&h.tabs) }

func (h *host) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    var cmd tea.Cmd
    h.tabs, cmd = h.tabs.Update(msg)
    return h, cmd
}
```

Bodies are ordinary screens; anything you'd build as a top-level screen drops
in. Their `Title()` and usually `Init()` go unused: the label comes from
`Tab.Label` and most bodies start work in `OnEnter`, which runs when the tab
becomes active.

A body that returns `screen.Push(child)` pushes over the whole host. When the
child pops with a value, the host's `OnEnter` forwards it with
`OnEnterActive`, so the body that pushed is the one that receives it.

## Message routing

- `tea.KeyMsg` and `mouse.Msg` go to the **active** body only, so hidden tabs
  can't race on `/`, `j`/`k` or clicks.
- Everything else fans out to **every** body, so a hidden tab's timers,
  fetches and streams keep running.

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Tabs` | `[]tab.Tab` | **required** | `{Label, Body}` entries. Empty panics. |
| `Theme` | `theme.Theme` | | Derives the strip's styles. |
| `Initial` | `int` | `0` | Starting tab. |
| `PrevKey`, `NextKey` | `key.Binding` | `shift+←`, `shift+→` | Switch tabs. Don't rebind to `tab`/`shift+tab`; those cycle focus within a body. |
| `DisableNumberKeys` | `bool` | `false` | Turn off `1`–`9` to jump to a tab. |
| `StripPos` | `tab.StripPos` | `StripTop` | `StripBottom` puts the strip on the last row. |
| `ActiveStyle`, `InactiveStyle`, `SeparatorStyle`, `BarStyle` | `*lipgloss.Style` | from `Theme` | Strip styling. |
| `Separator` | `string` | `" │ "` | Between labels. |

## Keys

| Key | Action |
|---|---|
| `shift+←` / `shift+→` | previous / next tab |
| `1`–`9` | jump to a tab (with at least 2 tabs) |

Tab keys are ignored while the active body is capturing keys. Mouse: click a
label to switch.

## Methods

`ActiveTab()`, `ActiveLabel()`, `SetActiveTab(i) tea.Cmd` (returns the new
body's `OnEnter`), `OnEnterActive(result)`, `SetTheme(t)` (fans out to every
body), `IsCapturingKeys()`, `Help()`, `HelpSections()` (the strip's keys plus
the active body's groups), `Rect()`.

See `examples/shell/tabs`, where the Logs tab keeps streaming while you're on
another tab.

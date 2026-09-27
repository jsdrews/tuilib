// Package activityrecipes is the row-activity API one shape at a time, against
// an Argo-shaped demoapi. Each tab is a recipe small enough to copy:
//
//   - observe.go: rows spin because the server says so. BusyWhen, BeginRead,
//     ApplyRead. Every other recipe embeds this one.
//   - sync.go: a POST that answers at once (activity.Observed), plus
//     UnobservedMsg for a sync that finished between polls.
//   - refresh.go: a GET that blocks until the work is done (activity.Held).
//   - job.go: a job handle polled to the end (activity.Held).
//
// Only one decision differs between them: who knows when the work ends. If
// the server's status does, use Observed. If the request or a job does, use
// Held.
package activityrecipes

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/tab"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the recipes, one per tab.
func New(t theme.Theme) screen.Screen {
	h := &host{bodies: []screen.Screen{
		newObserve(t, "observe"), newSync(t), newRefresh(t), newJob(t),
	}}
	h.SetTheme(t)
	return h
}

type host struct {
	bodies []screen.Screen
	tabs   tab.Model
}

func (h *host) SetTheme(t theme.Theme) {
	for _, b := range h.bodies {
		b.SetTheme(t)
	}
	labels := []string{"Observe", "Sync", "Refresh", "Job handle"}
	tabs := make([]tab.Tab, len(h.bodies))
	for i, b := range h.bodies {
		tabs[i] = tab.Tab{Label: labels[i], Body: b}
	}
	h.tabs = tab.New(tab.Options{Theme: t, Initial: h.tabs.ActiveTab(), Tabs: tabs})
}

// Actions forwards to the active recipe, so the menu offers its verbs.
func (h *host) Actions() action.Set {
	if p, ok := h.bodies[h.tabs.ActiveTab()].(action.Provider); ok {
		return p.Actions()
	}
	return action.Set{}
}

func (h *host) Title() string         { return "Activity recipes › " + h.tabs.ActiveLabel() }
func (h *host) Init() tea.Cmd         { return h.tabs.Init() }
func (h *host) OnEnter(r any) tea.Cmd { return h.tabs.OnEnterActive(r) }
func (h *host) IsCapturingKeys() bool { return h.tabs.IsCapturingKeys() }
func (h *host) Layout() layout.Node   { return layout.Sized(&h.tabs) }
func (h *host) Help() []key.Binding   { return help.Flatten(h.HelpSections()) }

func (h *host) HelpSections() []help.Section {
	return help.SectionsOf(&h.tabs, help.Group("Recipes",
		key.NewBinding(key.WithKeys("mouse:right"), key.WithHelp("right-click", "actions")),
	))
}

func (h *host) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	h.tabs, cmd = h.tabs.Update(msg)
	return h, cmd
}

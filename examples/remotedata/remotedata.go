// Package remotedata is the remote-data showcase: data too large for one
// request, one tab per shape (rule 34), each read from demoapi over HTTP the
// way the real API is read.
//
//	Records       a table over a paged API — Seekable records, bound with
//	              pkg/remote; scrolling is the pagination (rule 29)
//	Events        an AWX job's events, still arriving — Seekable, Growing,
//	              in pkg/eventlog, with an inspector one enter away
//	Finished job  the same job once it has ended, so nothing follows
//	Anchored      an Elasticsearch-style log index read with search_after —
//	              no offsets, no total, a search that re-anchors
//	Pod logs      a Kubernetes pod's log — Streamed, a tail then a follow,
//	              in pkg/logview with pkg/resume keeping reconnects exact
//
// The tabs are pkg/tab (rule 21): shift+←/→ or 1–5 switch, keys and the mouse
// reach the active tab only, and every other message reaches all of them, so
// a tab you are not looking at keeps following its stream. Three of the bodies
// are pkg/remote bindings sharing that one message stream; each acts only on
// its own view's viewport and filter, which the views name by token.
//
// Each tab is its own package beside this one, so it can be read, copied and
// tested alone.
package remotedata

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/examples/remotedata/anchored"
	"github.com/jsdrews/tuilib/examples/remotedata/events"
	"github.com/jsdrews/tuilib/examples/remotedata/podlogs"
	"github.com/jsdrews/tuilib/examples/remotedata/records"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/tab"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the remote-data screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

// Screen holds each body so a theme rebuild reuses it, with its window, its
// query and its stream, rather than starting again.
type Screen struct {
	t      theme.Theme
	bodies []tab.Tab
	tabs   tab.Model
}

func (s *Screen) Title() string         { return "Remote data › " + s.tabs.ActiveLabel() }
func (s *Screen) Init() tea.Cmd         { return tea.Batch(textinput.Blink, s.tabs.Init()) }
func (s *Screen) OnEnter(r any) tea.Cmd { return s.tabs.OnEnterActive(r) }
func (s *Screen) IsCapturingKeys() bool { return s.tabs.IsCapturingKeys() }
func (s *Screen) Layout() layout.Node   { return layout.Sized(&s.tabs) }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.tabs, cmd = s.tabs.Update(msg)
	return s, cmd
}

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

// HelpSections is the strip's keys plus the active tab's groups: a hidden
// tab's keys do nothing until you switch to it.
func (s *Screen) HelpSections() []help.Section { return s.tabs.HelpSections() }

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	if s.bodies == nil {
		s.bodies = []tab.Tab{
			{Label: "Records", Body: records.New(t)},
			{Label: "Events", Body: events.New(t)},
			{Label: "Finished job", Body: events.NewFinished(t)},
			{Label: "Anchored", Body: anchored.New(t)},
			{Label: "Pod logs", Body: podlogs.New(t)},
		}
	} else {
		for _, b := range s.bodies {
			b.Body.SetTheme(t)
		}
	}
	active := s.tabs.ActiveTab() // 0 on the first call
	s.tabs = tab.New(tab.Options{Theme: t, Initial: active, Tabs: s.bodies})
}

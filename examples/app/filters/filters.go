// Package filters is a two-filterable-pane screen — the shape that exposes
// how focus behaves when a component has more than one place to type.
//
// A filterable component holds two focusable regions behind a single
// focus.Focusable: the filter and the body. Put two such components on one
// screen and the interesting cases appear:
//
//   - Exactly one region anywhere on screen should read as active. Clicking
//     a filter must take the highlight off its own body, not just add a
//     second highlight.
//   - Clicking a body must take input back from its filter. Otherwise the
//     filter keeps swallowing keys with nothing on screen saying so.
//   - Moving focus to the other pane must clear the region you left,
//     including its filter. A filter left focused on a blurred component is
//     invisible and still eats keystrokes.
//   - tab must not cycle while a filter is taking input — it would strand a
//     half-typed query, and pkg/table binds tab to complete a key:value
//     term.
//
// The two panes are deliberately different components (a list and a table)
// because their filters differ: the table's parses `key:value` terms and
// completes them on tab, which is exactly the binding a focus group would
// otherwise steal.
package filters

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the two-filter demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t     theme.Theme
	files list.Model
	rows  table.Model

	focus focus.Group
}

func (s *Screen) Title() string       { return "Filters" }
func (s *Screen) OnEnter(any) tea.Cmd { return nil }
func (s *Screen) Init() tea.Cmd       { return s.focus.Init() }

// IsCapturingKeys is answered by whichever pane holds focus, which in turn
// reports true only while its own filter is taking input.
func (s *Screen) IsCapturingKeys() bool { return s.focus.IsCapturingKeys() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmds []tea.Cmd

	var gcmd tea.Cmd
	s.focus, gcmd = s.focus.Update(msg)
	cmds = append(cmds, gcmd)

	switch m := msg.(type) {
	case list.ActivatedMsg:
		return s, app.Info("opened file: " + m.Item)
	case table.ActivatedMsg:
		return s, app.Info("opened deployment: " + m.Cells[0])
	}

	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "esc" && !s.IsCapturingKeys() {
		return s, screen.Pop(nil)
	}

	if _, isMouse := msg.(mouse.Msg); isMouse {
		var c tea.Cmd
		s.files, c = s.files.Update(msg)
		cmds = append(cmds, c)
		s.rows, c = s.rows.Update(msg)
		cmds = append(cmds, c)
	} else {
		var c tea.Cmd
		switch {
		case s.focus.Is(&s.files):
			s.files, c = s.files.Update(msg)
		case s.focus.Is(&s.rows):
			s.rows, c = s.rows.Update(msg)
		}
		cmds = append(cmds, c)
	}
	return s, tea.Batch(cmds...)
}

func (s *Screen) Layout() layout.Node {
	return layout.HStack(
		layout.Flex(2, layout.Sized(&s.files)),
		layout.Flex(3, layout.Sized(&s.rows)),
	)
}

func (s *Screen) Help() []key.Binding {
	return append(s.focus.Help(),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	)
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	cursor, q := s.files.Cursor(), s.files.Value()
	lOpts := t.List()
	lOpts.Title = "files · / to filter"
	lOpts.Filterable = true
	lOpts.Filter.Placeholder = "filter files…"
	lOpts.Items = fileNames
	s.files = list.New(lOpts)
	s.files.SetCursor(cursor)
	if q != "" {
		s.files.SetValue(q)
	}

	tCursor, tQuery := s.rows.Cursor(), s.rows.Value()
	tOpts := t.Table()
	tOpts.Title = "deployments · / to filter, tab completes key:value"
	tOpts.Filterable = true
	tOpts.Filter.Placeholder = "e.g. region:eu"
	tOpts.Columns = []table.Column{
		{Title: "Name", Flex: 2, Sortable: true},
		{Title: "Region", Width: 12, Sortable: true},
		{Title: "Health", Width: 10},
	}
	tOpts.Rows = deployments
	s.rows = table.New(tOpts)
	s.rows.SetCursor(tCursor)
	if tQuery != "" {
		s.rows.SetValue(tQuery)
	}

	at := s.focus.Index()
	s.focus = focus.NewGroup(&s.files, &s.rows).WithKeys(focus.Keys{
		Next: key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "next pane")),
		Prev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "prev pane")),
	})
	s.focus.SetIndex(at)
}

var fileNames = []string{
	"main.go", "server.go", "server_test.go", "router.go",
	"handler_users.go", "handler_auth.go", "middleware.go",
	"config.go", "config_test.go", "logging.go", "metrics.go",
	"store.go", "store_postgres.go", "store_memory.go",
}

var deployments = []table.Row{
	{"api-gateway", "eu-west-1", "healthy"},
	{"auth-service", "us-east-1", "healthy"},
	{"billing", "eu-west-1", "degraded"},
	{"search-indexer", "ap-south-1", "healthy"},
	{"notifier", "us-east-1", "down"},
	{"webhooks", "eu-central-1", "healthy"},
	{"reporting", "ap-south-1", "degraded"},
	{"scheduler", "us-west-2", "healthy"},
}

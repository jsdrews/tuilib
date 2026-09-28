package activityrecipes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/demoapi"
	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/poll"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// Recipe 1 — observe only. Rows spin because the server says they are
// working; nobody here starts anything. Every other recipe embeds this one
// and adds a verb.
//
// Three things make it work:
//
//   - BusyWhen reads the status that means "working" from the row's Data.
//     For Argo that is operationState.phase, not the sync status.
//   - Each fetch is stamped with BeginRead when it is issued.
//   - The reply goes in with ApplyRead, which drops it if a newer one has
//     already landed. If a fetch fails, rows the last good read called busy
//     turn to a static "?" and ReadsFailing goes true until one succeeds.
//     The title shows that and "refreshed Ns ago" side by side (titleText).
//   - Revision (optional) names a value that moves when an operation
//     finishes, so a sync that ends where it began still reads as done.

// argoApp is one application as the server reports it.
type argoApp struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Sync   string `json:"sync"`
	Health string `json:"health"`
	Phase  string `json:"phase"`
	Rev    int64  `json:"rev"`
}

type observe struct {
	title string
	api   demoapi.Target
	table table.Model
	poll  poll.Model
}

type fetchedMsg struct {
	read activity.Read
	apps []argoApp
	err  error
}

func newObserve(t theme.Theme, title string) *observe {
	s := &observe{
		title: title,
		api:   demoapi.From(demoapi.Options{Apps: 6, Schedule: 3 * time.Second}),
		poll:  poll.New(poll.Options{Interval: 2 * time.Second}),
	}
	s.SetTheme(t)
	return s
}

func (s *observe) SetTheme(t theme.Theme) {
	rows, act := s.table.KeyedRows(), s.table.ActivityState()

	o := t.Table()
	o.Title = s.title
	o.Columns = []table.Column{
		{Title: "Name", Width: 24},
		{Title: "Sync", Width: 14}, // wide enough for "⣾ Refreshing"
		{Title: "Health", Width: 12},
	}
	o.ActivityColumn = "Sync"
	o.BusyWhen = func(r table.KeyedRow) (string, bool) {
		phase := r.Data.(argoApp).Phase
		return phase, phase == demoapi.PhaseRunning
	}
	o.Revision = func(r table.KeyedRow) string { return strconv.FormatInt(r.Data.(argoApp).Rev, 10) }
	s.table = table.New(o)
	// Rows first, so the adopted claims land on rows that exist. The dropped
	// command is the spinner's first tick; the table re-arms on its next
	// message.
	s.table.SetKeyedRows(rows)
	_ = s.table.SetActivityState(act)
}

func (s *observe) Init() tea.Cmd { return tea.Batch(s.poll.Init(), s.fetch(), s.tick()) }

// titleTickMsg re-renders the title once a second so "refreshed Ns ago"
// counts. It names its screen because a tab host delivers ticks to every tab,
// and a body re-arming on another body's tick would double its chain.
type titleTickMsg struct{ s *observe }

func (s *observe) tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return titleTickMsg{s} })
}

// titleText says two different things, and an outage needs both: whether
// reads are failing, and how stale the rows are. "refreshed Ns ago" counts
// from the last read that landed (poll.LastRefresh), so it keeps climbing
// through an outage — which is when it matters most. Don't swap one for the
// other.
func (s *observe) titleText() string {
	title := s.title
	if s.table.ReadsFailing() {
		title += " · reads failing"
	}
	if last := s.poll.LastRefresh(); !last.IsZero() {
		title += fmt.Sprintf(" · refreshed %ds ago", int(time.Since(last).Seconds()))
	}
	return title
}

func (s *observe) fetch() tea.Cmd {
	api, rd := s.api, s.table.BeginRead()
	return func() tea.Msg {
		resp, err := api.Client.Get(api.URL("/apps?limit=6"))
		if err != nil {
			return fetchedMsg{read: rd, err: err}
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fetchedMsg{read: rd, err: fmt.Errorf("list apps: %s", resp.Status)}
		}
		var body struct {
			Rows []argoApp `json:"rows"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		return fetchedMsg{read: rd, apps: body.Rows, err: err}
	}
}

// update is the part every recipe shares: poll, fetch, apply, forward.
func (s *observe) update(msg tea.Msg) tea.Cmd {
	cmds := []tea.Cmd{s.poll.Update(msg)}
	switch m := msg.(type) {
	case poll.RefreshMsg:
		cmds = append(cmds, s.fetch())
	case fetchedMsg:
		rows := make([]table.KeyedRow, len(m.apps))
		for i, a := range m.apps {
			rows[i] = table.KeyedRow{Key: a.ID, Cells: []string{a.Name, a.Sync, a.Health}, Data: a}
		}
		wasFailing := s.table.ReadsFailing()
		if s.table.ApplyRead(m.read, rows, m.err) {
			s.poll.MarkRefreshed()
		} else if m.err != nil && !wasFailing {
			// Once, when reads start failing — not on every poll into the
			// outage. The title says it for as long as it lasts.
			cmds = append(cmds, app.ErrorOf(m.err))
		}
		s.table.SetTitle(s.titleText())
	case titleTickMsg:
		if m.s == s {
			s.table.SetTitle(s.titleText())
			cmds = append(cmds, s.tick())
		}
	}
	var cmd tea.Cmd
	s.table, cmd = s.table.Update(msg)
	return tea.Batch(append(cmds, cmd)...)
}

func (s *observe) Update(msg tea.Msg) (screen.Screen, tea.Cmd) { return s, s.update(msg) }

func (s *observe) Title() string         { return s.title }
func (s *observe) OnEnter(any) tea.Cmd   { return nil }
func (s *observe) IsCapturingKeys() bool { return s.table.IsCapturingKeys() }
func (s *observe) Layout() layout.Node   { return layout.Sized(&s.table) }
func (s *observe) Help() []key.Binding   { return s.table.Help() }

// selection is what a verb acts on.
func (s *observe) selection() []string { return s.table.Selection() }

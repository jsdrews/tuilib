// Package polltable demonstrates pkg/poll driving auto-refresh of a
// table.Model, with SetKeyedRows pinning the cursor to the same row by
// Key across every refresh.
//
// The synthetic data is a deployments table — fixed set of services,
// each with a Sync state, Health state, replica count, and "age". On
// every tick the underlying state mutates (a deployment flips Syncing
// → Synced, a Health goes Degraded → Healthy or vice versa, replica
// counts drift, ordering changes), and the table is re-applied via
// SetKeyedRows. Because each row carries a stable deployment ID, the
// cursor sticks even when the row's display position moves — the
// payoff that "keyed rows for auto-refresh" makes visible.
//
// Keys: p pauses/resumes, r refreshes immediately, +/- adjust cadence,
// /↑↓ behave normally on the table, [/]/s sort by column.
package polltable

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/ansi"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/poll"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

const (
	defaultInterval = 2 * time.Second
	minInterval     = 500 * time.Millisecond
	maxInterval     = 10 * time.Second
)

type deployment struct {
	id       string
	name     string
	env      string
	sync     string // Synced, Syncing, OutOfSync
	health   string // Healthy, Degraded, Down
	replicas int
	want     int
	age      time.Duration
}

type Screen struct {
	t    theme.Theme
	tab  table.Model
	poll poll.Model
	deps []deployment
	rng  *rand.Rand
}

// New returns the polled-table demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{
		rng:  rand.New(rand.NewSource(time.Now().UnixNano())),
		deps: seedDeployments(),
	}
	s.poll = poll.New(poll.Options{Interval: defaultInterval})
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string         { return "PollTable" }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.tab.Filtering() }

func (s *Screen) Init() tea.Cmd {
	s.poll.MarkRefreshed()
	return s.poll.Init()
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case poll.RefreshMsg:
		s.advance()
		s.poll.MarkRefreshed()
		s.applyRows()
		return s, nil
	case tea.KeyMsg:
		if !s.tab.Filtering() {
			switch m.String() {
			case "p":
				if s.poll.Paused() {
					cmd := s.poll.Resume()
					s.refreshTitle()
					return s, cmd
				}
				s.poll.Pause()
				s.refreshTitle()
				return s, nil
			case "r":
				return s, s.poll.Refresh()
			case "+", "=":
				if d := s.poll.Interval() - 500*time.Millisecond; d >= minInterval {
					return s, s.poll.SetInterval(d)
				}
				return s, nil
			case "-", "_":
				if d := s.poll.Interval() + 500*time.Millisecond; d <= maxInterval {
					return s, s.poll.SetInterval(d)
				}
				return s, nil
			}
		}
	}
	cmd := s.poll.Update(msg)
	var tcmd tea.Cmd
	s.tab, tcmd = s.tab.Update(msg)
	s.refreshTitle()
	return s, tea.Batch(cmd, tcmd)
}

func (s *Screen) Layout() layout.Node { return layout.Sized(&s.tab) }

func (s *Screen) Help() []key.Binding {
	out := s.tab.Help()
	out = append(out,
		key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pause/resume")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh now")),
		key.NewBinding(key.WithKeys("+", "-"), key.WithHelp("+/-", "interval")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	)
	return out
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	cursor, value := s.tab.Cursor(), s.tab.Value()
	sortCol, sortDesc := s.tab.SortColumn(), s.tab.SortDescending()
	prevKey, hadKey := s.tab.SelectedKey()

	opts := t.Table()
	opts.Title = "deployments"
	opts.Filterable = true
	opts.Filter.Placeholder = "filter, env:prod, health:~degraded…"
	opts.Columns = []table.Column{
		{Title: "Deployment", Width: 22, Sortable: true},
		{Title: "Env", Width: 8, Sortable: true},
		{Title: "Sync", Width: 12, Sortable: true, Less: syncLess},
		{Title: "Health", Width: 14, Sortable: true, Less: healthLess},
		{Title: "Replicas", Width: 10, Sortable: true, Less: replicasLess},
		{Title: "Age", Width: 8, Sortable: true, Less: ageLess},
	}
	s.tab = table.New(opts)
	s.applyRows()
	if value != "" {
		s.tab.SetValue(value)
	}
	if hadKey {
		s.snapToKey(prevKey)
	} else {
		s.tab.SetCursor(cursor)
	}
	s.tab.SetSort(sortCol, sortDesc)
	s.refreshTitle()
}

func (s *Screen) applyRows() {
	rows := make([]table.KeyedRow, len(s.deps))
	for i, d := range s.deps {
		rows[i] = table.KeyedRow{Key: d.id, Cells: depCells(d)}
	}
	s.tab.SetKeyedRows(rows)
	s.refreshTitle()
}

func (s *Screen) snapToKey(key string) {
	rows := make([]table.KeyedRow, len(s.deps))
	for i, d := range s.deps {
		rows[i] = table.KeyedRow{Key: d.id, Cells: depCells(d)}
	}
	s.tab.SetKeyedRows(rows)
	for i, d := range s.deps {
		if d.id == key {
			s.tab.SetCursor(i)
			return
		}
	}
}

func depCells(d deployment) []string {
	return []string{
		d.name,
		d.env,
		colorSync(d.sync),
		colorHealth(d.health),
		fmt.Sprintf("%d/%d", d.replicas, d.want),
		d.age.Round(time.Second).String(),
	}
}

func colorSync(s string) string {
	switch s {
	case "Synced":
		return ansi.CellColor(2, "✓ Synced")
	case "Syncing":
		return ansi.CellColor(4, "↻ Syncing")
	case "OutOfSync":
		return ansi.CellColor(3, "● OutOfSync")
	}
	return s
}

func colorHealth(h string) string {
	switch h {
	case "Healthy":
		return ansi.CellColor(2, "✓ Healthy")
	case "Degraded":
		return ansi.CellColor(3, "● Degraded")
	case "Down":
		return ansi.CellColor(1, "✗ Down")
	}
	return h
}

func (s *Screen) refreshTitle() {
	if s.poll.Paused() {
		s.tab.SetTitle("deployments · paused")
		return
	}
	last := s.poll.LastRefresh()
	if last.IsZero() {
		s.tab.SetTitle("deployments")
		return
	}
	ago := time.Since(last).Round(time.Second)
	s.tab.SetTitle(fmt.Sprintf("deployments · refreshed %ds ago · every %s", int(ago.Seconds()), s.poll.Interval()))
}

// advance mutates the deployments slice — flip sync states, drift health,
// drift replica counts, bump ages, then re-sort. The reorder is what makes
// keyed cursor preservation visible.
func (s *Screen) advance() {
	for i := range s.deps {
		s.deps[i].age += defaultInterval
		switch s.deps[i].sync {
		case "Syncing":
			if s.rng.Float64() < 0.6 {
				s.deps[i].sync = "Synced"
			}
		case "Synced":
			if s.rng.Float64() < 0.1 {
				s.deps[i].sync = "Syncing"
			} else if s.rng.Float64() < 0.05 {
				s.deps[i].sync = "OutOfSync"
			}
		case "OutOfSync":
			if s.rng.Float64() < 0.4 {
				s.deps[i].sync = "Syncing"
			}
		}
		switch s.deps[i].health {
		case "Healthy":
			if s.rng.Float64() < 0.08 {
				s.deps[i].health = "Degraded"
			}
		case "Degraded":
			if s.rng.Float64() < 0.5 {
				s.deps[i].health = "Healthy"
			} else if s.rng.Float64() < 0.1 {
				s.deps[i].health = "Down"
			}
		case "Down":
			if s.rng.Float64() < 0.4 {
				s.deps[i].health = "Degraded"
			}
		}
		// Drift replicas toward want, occasionally a pod dies.
		if s.deps[i].replicas < s.deps[i].want && s.rng.Float64() < 0.4 {
			s.deps[i].replicas++
		}
		if s.deps[i].replicas > 0 && s.rng.Float64() < 0.05 {
			s.deps[i].replicas--
		}
	}
	// Reorder so dirty rows float up — visible cursor drift without keys.
	sort.SliceStable(s.deps, func(i, k int) bool {
		ri, rk := dirtiness(s.deps[i]), dirtiness(s.deps[k])
		if ri != rk {
			return ri > rk
		}
		return s.deps[i].name < s.deps[k].name
	})
}

func dirtiness(d deployment) int {
	score := 0
	if d.sync != "Synced" {
		score += 2
	}
	switch d.health {
	case "Down":
		score += 4
	case "Degraded":
		score += 2
	}
	if d.replicas != d.want {
		score++
	}
	return score
}

func seedDeployments() []deployment {
	specs := []struct {
		name string
		env  string
		want int
	}{
		{"api-gateway", "prod", 6},
		{"api-gateway", "stage", 3},
		{"orders-service", "prod", 4},
		{"orders-service", "stage", 2},
		{"payments-svc", "prod", 5},
		{"payments-svc", "stage", 2},
		{"events-ingest", "prod", 8},
		{"events-ingest", "stage", 3},
		{"warehouse-loader", "prod", 2},
		{"recs-ml", "prod", 4},
		{"notifications", "prod", 3},
		{"notifications", "stage", 2},
		{"static-site", "prod", 2},
	}
	out := make([]deployment, len(specs))
	for i, sp := range specs {
		out[i] = deployment{
			id:       fmt.Sprintf("%s/%s", sp.env, sp.name),
			name:     sp.name,
			env:      sp.env,
			sync:     "Synced",
			health:   "Healthy",
			replicas: sp.want,
			want:     sp.want,
			age:      time.Duration(60+i*23) * time.Second,
		}
	}
	// Seed a few mid-flight states so the first frame shows variety.
	out[2].sync = "Syncing"
	out[5].health = "Degraded"
	out[7].sync = "OutOfSync"
	out[7].health = "Down"
	out[10].replicas = 1
	return out
}

func syncLess(a, b string) bool       { return syncRank(a) < syncRank(b) }
func healthLess(a, b string) bool     { return healthRank(a) < healthRank(b) }
func replicasLess(a, b string) bool   { return parseRatio(a) < parseRatio(b) }
func ageLess(a, b string) bool        { return parseDur(a) < parseDur(b) }

func syncRank(s string) int {
	switch {
	case strings.Contains(s, "OutOfSync"):
		return 2
	case strings.Contains(s, "Syncing"):
		return 1
	case strings.Contains(s, "Synced"):
		return 0
	}
	return -1
}

func healthRank(s string) int {
	switch {
	case strings.Contains(s, "Down"):
		return 2
	case strings.Contains(s, "Degraded"):
		return 1
	case strings.Contains(s, "Healthy"):
		return 0
	}
	return -1
}

func parseRatio(s string) float64 {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0
	}
	have, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	want, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	if want == 0 {
		return 0
	}
	return float64(have) / float64(want)
}

func parseDur(s string) time.Duration {
	d, _ := time.ParseDuration(strings.TrimSpace(s))
	return d
}

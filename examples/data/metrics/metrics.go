// Package metrics demonstrates pkg/metrics inline primitives — Badge,
// Ratio, Bar, Spark — composed into a polled deployments table. Each row
// shows a deployment with its replica ratio, pod-status badge, CPU bar,
// and a 12-sample CPU sparkline. The data ticks every 2s; pkg/poll +
// SetKeyedRows keep the cursor pinned to the same deployment ID across
// every refresh.
//
// Demonstrates: Badge for state breakdowns, Ratio for "N of M", Bar for
// utilization, Spark for short history. The history buffer is owned by
// this screen — pkg/metrics is rendering-only, so each tick the screen
// rolls the buffer (drop oldest, append newest) and re-renders.
package metrics

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/metrics"
	"github.com/jsdrews/tuilib/pkg/poll"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

const (
	defaultInterval = 2 * time.Second
	minInterval     = 500 * time.Millisecond
	maxInterval     = 10 * time.Second
	historyLen      = 12
	sparkWidth      = 12
	barWidth        = 8
)

type deployment struct {
	id       string
	name     string
	env      string
	replicas int
	want     int
	healthy  int
	warn     int
	down     int
	cpuPct   float64
	cpuHist  []float64
}

type Screen struct {
	t    theme.Theme
	tab  table.Model
	poll poll.Model
	deps []deployment
	rng  *rand.Rand
}

// New returns the metrics demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{
		rng:  rand.New(rand.NewSource(time.Now().UnixNano())),
		deps: seedDeployments(),
	}
	s.poll = poll.New(poll.Options{Interval: defaultInterval})
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string         { return "Metrics" }
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
					return s, s.poll.Resume()
				}
				s.poll.Pause()
				s.applyRows()
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
	opts.Title = "deployments · inline metrics"
	opts.Filterable = true
	opts.Filter.Placeholder = "filter, env:prod…"
	opts.Columns = []table.Column{
		{Title: "Deployment", Width: 22, Sortable: true},
		{Title: "Env", Width: 8, Sortable: true},
		{Title: "Replicas", Width: 10, Sortable: true, Less: ratioLess},
		{Title: "Pods", Width: 14},
		{Title: "CPU", Width: 14, Sortable: true, Less: cpuLess},
		{Title: "CPU 24s", Width: sparkWidth},
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
		metrics.Ratio(d.replicas, d.want),
		metrics.Badge(d.healthy, d.warn, d.down),
		metrics.BarStyled(d.cpuPct, 100, barWidth, ansiColorForCPU(d.cpuPct)) +
			fmt.Sprintf(" %3.0f%%", d.cpuPct),
		metrics.Spark(d.cpuHist, sparkWidth),
	}
}

// ansiColorForCPU is a non-severity colorization (CPU is always blue
// when low and shifts warm at high utilization). Demonstrates BarStyled
// for cases where the default severity inference doesn't apply.
func ansiColorForCPU(pct float64) int {
	switch {
	case pct >= 90:
		return 1 // red
	case pct >= 70:
		return 3 // yellow
	default:
		return 4 // blue
	}
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

// advance simulates one tick: drift CPU, occasionally lose/recover pods,
// roll the CPU history ring buffer.
func (s *Screen) advance() {
	for i := range s.deps {
		d := &s.deps[i]
		// Drift CPU with a random walk, clamped.
		d.cpuPct += (s.rng.Float64() - 0.5) * 18
		if d.cpuPct < 5 {
			d.cpuPct = 5
		}
		if d.cpuPct > 99 {
			d.cpuPct = 99
		}
		d.cpuHist = append(d.cpuHist, d.cpuPct)
		if len(d.cpuHist) > historyLen {
			d.cpuHist = d.cpuHist[len(d.cpuHist)-historyLen:]
		}
		// Pod state drift.
		if d.healthy < d.want && s.rng.Float64() < 0.5 {
			d.healthy++
			if d.warn > 0 {
				d.warn--
			} else if d.down > 0 {
				d.down--
			}
		}
		if s.rng.Float64() < 0.06 && d.healthy > 0 {
			d.healthy--
			if s.rng.Float64() < 0.5 {
				d.warn++
			} else {
				d.down++
			}
		}
		d.replicas = d.healthy + d.warn
	}
	// Resort with troubled rows (down > 0 or short replicas) on top so
	// the cursor-by-key pinning is visible.
	sort.SliceStable(s.deps, func(i, k int) bool {
		ri, rk := dirtiness(s.deps[i]), dirtiness(s.deps[k])
		if ri != rk {
			return ri > rk
		}
		return s.deps[i].name < s.deps[k].name
	})
}

func dirtiness(d deployment) int {
	score := d.down*4 + d.warn*2
	if d.replicas < d.want {
		score += 2
	}
	if d.cpuPct >= 90 {
		score++
	}
	return score
}

func seedDeployments() []deployment {
	specs := []struct {
		name string
		env  string
		want int
		cpu  float64
	}{
		{"api-gateway", "prod", 6, 42},
		{"api-gateway", "stage", 3, 28},
		{"orders-service", "prod", 4, 65},
		{"orders-service", "stage", 2, 31},
		{"payments-svc", "prod", 5, 71},
		{"payments-svc", "stage", 2, 22},
		{"events-ingest", "prod", 8, 88},
		{"events-ingest", "stage", 3, 47},
		{"warehouse-loader", "prod", 2, 18},
		{"recs-ml", "prod", 4, 54},
		{"notifications", "prod", 3, 33},
		{"notifications", "stage", 2, 19},
		{"static-site", "prod", 2, 8},
	}
	out := make([]deployment, len(specs))
	for i, sp := range specs {
		hist := make([]float64, historyLen)
		for h := range hist {
			hist[h] = sp.cpu + (float64(h%4)-1.5)*5 // mild oscillation
		}
		out[i] = deployment{
			id:       fmt.Sprintf("%s/%s", sp.env, sp.name),
			name:     sp.name,
			env:      sp.env,
			want:     sp.want,
			healthy:  sp.want,
			replicas: sp.want,
			cpuPct:   sp.cpu,
			cpuHist:  hist,
		}
	}
	// Seed a few troubled rows so the first frame shows variety.
	out[2].healthy = 3
	out[2].warn = 1
	out[6].healthy = 6
	out[6].down = 2
	out[6].replicas = 6
	out[10].healthy = 2
	out[10].warn = 1
	return out
}

func ratioLess(a, b string) bool {
	return parseRatio(a) < parseRatio(b)
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

func cpuLess(a, b string) bool {
	return parseCPU(a) < parseCPU(b)
}

func parseCPU(s string) float64 {
	// CPU cell is "<bar>  42%". Find the trailing percent number.
	idx := strings.LastIndex(s, "%")
	if idx <= 0 {
		return 0
	}
	// Walk backwards for digits.
	end := idx
	start := end
	for start > 0 && (s[start-1] >= '0' && s[start-1] <= '9') {
		start--
	}
	v, _ := strconv.ParseFloat(s[start:end], 64)
	return v
}

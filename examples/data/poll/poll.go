// Package poll demonstrates pkg/poll driving auto-refresh of a list, with
// keyed items so the user's cursor survives every refresh even as the
// underlying set churns.
//
// The synthetic data is a "task runner" — a fixed set of jobs whose
// status, age, and ordering change on every tick (some flip from running
// to succeeded, occasionally a new job appears or finishes off the
// list). Each job carries a stable Key, so SetKeyedItems lets the list
// snap back to the previously-selected job by ID after the swap.
//
// Keys: p pauses/resumes the ticker, r refreshes immediately, +/- adjust
// the cadence, /↑↓ all behave normally on the list. The pane title
// reflects "refreshed Xs ago" or "paused" so the user can see the
// ticker's state without leaving the screen.
package poll

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/ansi"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/poll"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

const (
	defaultInterval = 2 * time.Second
	minInterval     = 500 * time.Millisecond
	maxInterval     = 10 * time.Second
)

type job struct {
	id     string
	name   string
	status string // running, succeeded, failed, pending
	age    time.Duration
}

type Screen struct {
	t     theme.Theme
	list  list.Model
	poll  poll.Model
	jobs  []job
	rng   *rand.Rand
	tickN int
}

// New returns the poll demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{
		rng:  rand.New(rand.NewSource(time.Now().UnixNano())),
		jobs: seedJobs(),
	}
	s.poll = poll.New(poll.Options{Interval: defaultInterval})
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string         { return "Poll" }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.list.Filtering() }

func (s *Screen) Init() tea.Cmd {
	s.poll.MarkRefreshed()
	return tea.Batch(textinput.Blink, s.poll.Init())
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case poll.RefreshMsg:
		s.advance()
		s.poll.MarkRefreshed()
		s.applyJobs()
		return s, nil
	case tea.KeyMsg:
		if !s.list.Filtering() {
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
	var lcmd tea.Cmd
	s.list, lcmd = s.list.Update(msg)
	s.refreshTitle()
	return s, tea.Batch(cmd, lcmd)
}

func (s *Screen) Layout() layout.Node { return layout.Sized(&s.list) }

func (s *Screen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pause/resume")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh now")),
		key.NewBinding(key.WithKeys("+", "-"), key.WithHelp("+/-", "interval")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	cursor, value := s.list.Cursor(), s.list.Value()
	prevKey, hadKey := s.list.SelectedKey()
	opts := t.List()
	opts.Title = "tasks"
	opts.Filterable = true
	opts.Filter.Placeholder = "filter tasks…"
	s.list = list.New(opts)
	s.applyJobs()
	if value != "" {
		s.list.SetValue(value)
	}
	if hadKey {
		s.snapToKey(prevKey)
	} else {
		s.list.SetCursor(cursor)
	}
	s.refreshTitle()
}

// applyJobs rebuilds the list contents from the current jobs slice via
// SetKeyedItems so the cursor sticks to the same job ID across the swap.
func (s *Screen) applyJobs() {
	items := make([]list.KeyedItem, len(s.jobs))
	for i, j := range s.jobs {
		items[i] = list.KeyedItem{Key: j.id, Display: formatJob(j)}
	}
	s.list.SetKeyedItems(items)
	s.refreshTitle()
}

func (s *Screen) snapToKey(key string) {
	for i, j := range s.jobs {
		if j.id == key {
			items := make([]list.KeyedItem, len(s.jobs))
			for k, jj := range s.jobs {
				items[k] = list.KeyedItem{Key: jj.id, Display: formatJob(jj)}
			}
			s.list.SetKeyedItems(items)
			s.list.SetCursor(i)
			return
		}
	}
}

func (s *Screen) refreshTitle() {
	if s.poll.Paused() {
		s.list = withTitle(s.list, "tasks · paused")
		return
	}
	last := s.poll.LastRefresh()
	if last.IsZero() {
		s.list = withTitle(s.list, "tasks")
		return
	}
	ago := time.Since(last).Round(time.Second)
	s.list = withTitle(s.list, fmt.Sprintf("tasks · refreshed %ds ago · every %s", int(ago.Seconds()), s.poll.Interval()))
}

// withTitle is a helper because list.Model's setter is on a pointer receiver
// but our state holds a value — keep it inline so the call site stays terse.
func withTitle(m list.Model, title string) list.Model {
	m.SetTitle(title)
	return m
}

// advance mutates the synthetic jobs slice — flip statuses, bump ages,
// occasionally insert/remove jobs. This is what makes keyed selection
// preservation visible: without keys, every cursor would drift on each
// reordering.
func (s *Screen) advance() {
	s.tickN++
	for i := range s.jobs {
		s.jobs[i].age += defaultInterval
		if s.jobs[i].status == "running" && s.rng.Float64() < 0.25 {
			if s.rng.Float64() < 0.85 {
				s.jobs[i].status = "succeeded"
			} else {
				s.jobs[i].status = "failed"
			}
		} else if s.jobs[i].status == "pending" && s.rng.Float64() < 0.5 {
			s.jobs[i].status = "running"
		}
	}
	if s.rng.Float64() < 0.4 {
		s.jobs = append(s.jobs, job{
			id:     fmt.Sprintf("job-%03d", 100+s.tickN),
			name:   randName(s.rng),
			status: "pending",
		})
	}
	if len(s.jobs) > 12 && s.rng.Float64() < 0.3 {
		// Drop a finished job from the head.
		for i, j := range s.jobs {
			if j.status == "succeeded" || j.status == "failed" {
				s.jobs = append(s.jobs[:i], s.jobs[i+1:]...)
				break
			}
		}
	}
	sort.SliceStable(s.jobs, func(i, k int) bool {
		// Reorder so the cursor would visibly drift without keys: running
		// first, then pending, then terminal — newest first within each.
		ord := func(st string) int {
			switch st {
			case "running":
				return 0
			case "pending":
				return 1
			default:
				return 2
			}
		}
		oi, ok := ord(s.jobs[i].status), ord(s.jobs[k].status)
		if oi != ok {
			return oi < ok
		}
		return s.jobs[i].age < s.jobs[k].age
	})
}

func formatJob(j job) string {
	icon, color := statusGlyph(j.status)
	left := ansi.CellColor(color, icon+" "+padRight(j.status, 9))
	return fmt.Sprintf("%s  %s  %s  age=%s", j.id, left, padRight(j.name, 16), j.age.Round(time.Second))
}

func statusGlyph(status string) (string, int) {
	switch status {
	case "running":
		return "●", 4 // blue
	case "succeeded":
		return "✓", 2 // green
	case "failed":
		return "✗", 1 // red
	case "pending":
		return "◌", 3 // yellow
	}
	return "·", 8
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + spaces(n-len(s))
}

func spaces(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = ' '
	}
	return string(out)
}

func seedJobs() []job {
	names := []string{
		"ingest-orders", "transform-events", "load-warehouse",
		"sync-customers", "rebuild-index", "compact-table",
		"backup-replica", "rotate-keys",
	}
	out := make([]job, len(names))
	for i, n := range names {
		out[i] = job{
			id:     fmt.Sprintf("job-%03d", i+1),
			name:   n,
			status: "running",
			age:    time.Duration(20+i*7) * time.Second,
		}
	}
	return out
}

func randName(r *rand.Rand) string {
	verbs := []string{"refresh", "compact", "snapshot", "drain", "scan", "audit"}
	nouns := []string{"cache", "index", "queue", "log", "shard", "stream"}
	return verbs[r.Intn(len(verbs))] + "-" + nouns[r.Intn(len(nouns))]
}

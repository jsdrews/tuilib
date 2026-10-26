package liveops

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/metrics"
	"github.com/jsdrews/tuilib/pkg/table"
)

// The workloads pane: pkg/metrics' inline primitives in a polled table. Badge
// breaks pods down by state, Ratio reads "N of M" in severity colours, Bar is
// utilisation and Spark is the last few ticks of it. pkg/metrics renders and
// nothing more, so the history buffer is this screen's: each tick drops the
// oldest sample and appends the newest.
//
// It has no server behind it: the numbers are a random walk, advanced on the
// same poll that fetches the applications, and "fetched" after a short delay
// so the pane's loading state shows.

const (
	historyLen = 12
	sparkWidth = 10
	barWidth   = 6
	// loadDelay is how long a workloads fetch takes, so the spinner is seen.
	loadDelay = 700 * time.Millisecond
)

type workload struct {
	id, name, env       string
	replicas, want      int
	healthy, warn, down int
	cpuPct              float64
	cpuHist             []float64
}

func seedWorkloads() []workload {
	specs := []struct {
		name, env string
		want      int
		cpu       float64
	}{
		{"api-gateway", "prod", 6, 42},
		{"orders-service", "prod", 4, 65},
		{"payments-svc", "prod", 5, 71},
		{"events-ingest", "prod", 8, 88},
		{"recs-ml", "prod", 4, 54},
		{"notifications", "stage", 2, 19},
		{"static-site", "prod", 2, 8},
	}
	out := make([]workload, len(specs))
	for i, sp := range specs {
		hist := make([]float64, historyLen)
		for h := range hist {
			hist[h] = sp.cpu + (float64(h%4)-1.5)*5
		}
		out[i] = workload{
			id: sp.env + "/" + sp.name, name: sp.name, env: sp.env,
			want: sp.want, healthy: sp.want, replicas: sp.want,
			cpuPct: sp.cpu, cpuHist: hist,
		}
	}
	// A few troubled rows, so the first frame shows every colour.
	out[1].healthy, out[1].warn = 3, 1
	out[3].healthy, out[3].down, out[3].replicas = 6, 2, 6
	return out
}

// workloadsMsg is a workloads fetch landing, with what changed for the log.
type workloadsMsg struct {
	rows   []workload
	events []string
}

// fetchWorkloads advances the walk one tick and returns it after loadDelay,
// the way a slow endpoint would.
func fetchWorkloads(prev []workload, rng *rand.Rand) tea.Cmd {
	next := make([]workload, len(prev))
	copy(next, prev)
	var events []string
	for i := range next {
		w := &next[i]
		w.cpuHist = append(append([]float64(nil), w.cpuHist...), 0)
		w.cpuPct = min(99, max(5, w.cpuPct+(rng.Float64()-0.5)*18))
		w.cpuHist[len(w.cpuHist)-1] = w.cpuPct
		if len(w.cpuHist) > historyLen {
			w.cpuHist = w.cpuHist[len(w.cpuHist)-historyLen:]
		}
		if w.healthy < w.want && rng.Float64() < 0.5 {
			w.healthy++
			if w.warn > 0 {
				w.warn--
			} else if w.down > 0 {
				w.down--
			}
			events = append(events, fmt.Sprintf("%s: pod recovered (%d/%d)", w.name, w.healthy, w.want))
		}
		if rng.Float64() < 0.06 && w.healthy > 0 {
			w.healthy--
			if rng.Float64() < 0.5 {
				w.warn++
			} else {
				w.down++
			}
			events = append(events, fmt.Sprintf("%s: pod lost (%d/%d)", w.name, w.healthy, w.want))
		}
		w.replicas = w.healthy + w.warn
	}
	// Troubled rows sort to the top, so the cursor visibly stays on its row
	// by key while the order moves under it (rule 24).
	sort.SliceStable(next, func(i, k int) bool {
		if a, b := trouble(next[i]), trouble(next[k]); a != b {
			return a > b
		}
		return next[i].name < next[k].name
	})
	return tea.Tick(loadDelay, func(time.Time) tea.Msg { return workloadsMsg{rows: next, events: events} })
}

func trouble(w workload) int {
	score := w.down*4 + w.warn*2
	if w.replicas < w.want {
		score += 2
	}
	if w.cpuPct >= 90 {
		score++
	}
	return score
}

func workloadRows(ws []workload) []table.KeyedRow {
	rows := make([]table.KeyedRow, len(ws))
	for i, w := range ws {
		rows[i] = table.KeyedRow{Key: w.id, Cells: []string{
			w.name,
			metrics.Ratio(w.replicas, w.want),
			metrics.Badge(w.healthy, w.warn, w.down),
			metrics.BarStyled(w.cpuPct, 100, barWidth, cpuColour(w.cpuPct)) + fmt.Sprintf(" %3.0f%%", w.cpuPct),
			metrics.Spark(w.cpuHist, sparkWidth),
		}}
	}
	return rows
}

// cpuColour is a non-severity colouring — blue while low, warm only near
// saturation — which is what BarStyled is for.
func cpuColour(pct float64) int {
	switch {
	case pct >= 90:
		return 1
	case pct >= 70:
		return 3
	}
	return 4
}

func ratioLess(a, b string) bool { return parseRatio(a) < parseRatio(b) }

func parseRatio(s string) float64 {
	have, want, ok := strings.Cut(stripped(s), "/")
	if !ok {
		return 0
	}
	h, _ := strconv.Atoi(strings.TrimSpace(have))
	w, _ := strconv.Atoi(strings.TrimSpace(want))
	if w == 0 {
		return 0
	}
	return float64(h) / float64(w)
}

func cpuLess(a, b string) bool { return parseCPU(a) < parseCPU(b) }

// parseCPU reads the percentage at the end of a CPU cell, "<bar>  42%".
func parseCPU(s string) float64 {
	s = stripped(s)
	end := strings.LastIndex(s, "%")
	start := end
	for start > 0 && s[start-1] >= '0' && s[start-1] <= '9' {
		start--
	}
	v, _ := strconv.ParseFloat(s[start:max(end, start)], 64)
	return v
}

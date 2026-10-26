package processes

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
)

// kind is how a command runs, and so which group it is filed under.
type kind int

const (
	// capture pipes the command's output into the output pane and the
	// console while the TUI stays live (runner.Capture).
	capture kind = iota
	// tail streams synthetic lines into the output pane on a tick, the way a
	// log being followed does. It toggles.
	tail
	// terminal hands the real terminal to the command and suspends the TUI
	// until it exits (runner.Run). Its output is not recoverable; the result
	// is.
	terminal
	// console says something to the statusbar, which the console keeps.
	console
)

var groupOf = map[kind]string{capture: "capture", tail: "capture", terminal: "terminal", console: "console"}

type command struct {
	kind  kind
	label string
	blurb string
	build func() *exec.Cmd
	// notice, on a terminal command, is a line shown during the handoff
	// (runner.RunWithNotice).
	notice string
	run    func() tea.Cmd
}

func sh(script string) func() *exec.Cmd {
	return func() *exec.Cmd { return exec.Command("sh", "-c", script) }
}

var commands = []command{
	{kind: capture, label: "seq with sleep (50 lines)",
		blurb: "runner.Capture streams each line as it is written. Scroll, search or tab away while it runs; the TUI never stops.",
		build: sh("for i in $(seq 1 50); do echo line $i; sleep 0.05; done")},
	{kind: capture, label: "stdout and stderr (40 lines)",
		blurb: "stderr lines are marked 2> here and drawn with a heavier gutter in the console, but they are not errors: plenty of tools log progress there. Severity comes from the exit status.",
		build: sh(`for i in $(seq 1 40); do if [ $((i % 7)) -eq 0 ]; then echo "warn: retry $i" 1>&2; else echo "step $i ok"; fi; sleep 0.05; done`)},
	{kind: capture, label: "exit 2 after a few lines",
		blurb: "A failed run: its completion lands at error level and turns the console badge red. It is one event, however many lines it wrote.",
		build: sh("echo building…; echo 'cc: error: undefined symbol' 1>&2; exit 2")},
	{kind: capture, label: "slow build (30s)",
		blurb: "Start it and keep working. The console badge carries ⟳ while it runs; Kill is on the action menu (a), and on the console itself (o, then x).",
		build: sh(`for i in $(seq 1 60); do echo "building chunk $i"; sleep 0.5; done`)},
	{kind: capture, label: "burst (500 lines)",
		blurb: "A fast producer. The output pane is capped (rule 13) and the console trims whole events off the front, never half a run.",
		build: sh("seq 1 500")},
	{kind: capture, label: "ls -la /usr/bin",
		blurb: "A long listing: search it with /, step matches with n and N.",
		build: func() *exec.Cmd { return exec.Command("ls", "-la", "/usr/bin") }},
	{kind: tail, label: "synthetic tail (start / stop)",
		blurb: "A log being followed: lines arrive on a tick and the pane follows the newest (FOLLOWING). Scroll up and it stops following; G follows again. Search highlights matches as they arrive."},

	{kind: terminal, label: "$EDITOR /tmp/tuilib-scratch.txt",
		blurb: "runner.Run hands the real terminal to your editor and suspends the TUI; you come back here when it exits.",
		build: func() *exec.Cmd {
			ed := os.Getenv("EDITOR")
			if ed == "" {
				ed = "vi"
			}
			return exec.Command(ed, "/tmp/tuilib-scratch.txt")
		}},
	{kind: terminal, label: "less /etc/hosts",
		blurb: "A pager wants the whole terminal, which is why it goes through Run and not Capture.",
		build: func() *exec.Cmd { return exec.Command("less", "/etc/hosts") }},
	{kind: terminal, label: "man ls", build: func() *exec.Cmd { return exec.Command("man", "ls") },
		blurb: "Run sets LINES and COLUMNS, so a full-screen program sizes itself to the terminal it was handed."},
	{kind: terminal, label: "htop", build: func() *exec.Cmd { return exec.Command("htop") },
		blurb: "A full-screen TUI inside this one. Quit it to come back."},
	{kind: terminal, label: "sh: connect, with a notice", notice: "connecting…",
		build: sh("sleep 2; echo connected"),
		blurb: "runner.RunWithNotice shows a line during the handoff, so the two seconds before the command prints anything are not a blank screen."},

	{kind: console, label: "Info — one short line",
		blurb: "app.Info paints the statusbar and the console keeps it. The next keypress wipes the bar; o shows it again.",
		run:   func() tea.Cmd { return app.Info("deployment triggered") }},
	{kind: console, label: "Error — one short line",
		blurb: "app.Error: the same at error level, which tints the console badge red.",
		run:   func() tea.Cmd { return app.Error("deployment rejected: quota exceeded") }},
	{kind: console, label: "Error with a body (12 lines)",
		blurb: "app.ErrorDetail: the summary on the bar, the body in the console alone — the case a one-line statusbar cannot serve.",
		run:   func() tea.Cmd { return app.ErrorDetail("apply failed: 3 resources rejected", stderrBlob) }},
	{kind: console, label: "Wrapped error (%w chain)",
		blurb: "app.ErrorOf: err.Error() on the bar, and the unwrapped chain in the console, one wrap per line.",
		run:   func() tea.Cmd { return app.ErrorOf(wrappedFailure()) }},
}

const stderrBlob = `+ kubectl apply -f manifests/
deployment.apps/api configured
error: unable to recognize "manifests/rollout.yaml": no matches for kind "Rollout" in version "argoproj.io/v1alpha1"
error: unable to recognize "manifests/scaler.yaml": no matches for kind "ScaledObject" in version "keda.sh/v1alpha1"
error: unable to recognize "manifests/policy.yaml": no matches for kind "PodMonitor" in version "monitoring.coreos.com/v1"

The cluster is missing three CRDs. Install them with:
  kubectl apply -f https://.../argo-rollouts/crds.yaml
  kubectl apply -f https://.../keda/crds.yaml
  kubectl apply -f https://.../prometheus-operator/crds.yaml

3 resources rejected, 1 configured.`

func wrappedFailure() error {
	base := errors.New("dial tcp 10.0.4.12:443: connect: connection refused")
	api := fmt.Errorf("GET /api/v1/deployments: %w", base)
	client := fmt.Errorf("cluster \"prod-eu\" unreachable: %w", api)
	return fmt.Errorf("deploy aborted: %w", client)
}

// The synthetic tail's vocabulary.
var (
	levels   = []string{"INFO", "INFO", "INFO", "DEBUG", "WARN", "ERROR"}
	services = []string{"api", "worker", "scheduler", "cache", "auth"}
	verbs    = []string{
		"started", "completed", "failed", "retrying", "queued", "rate-limited",
		"cache miss", "cache hit", "rolled back", "committed", "evicted",
	}
)

type tailMsg struct{ gen int }

func tailTick(gen int) tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return tailMsg{gen} })
}

func (s *Screen) tailBatch() []string {
	n := 1 + rand.Intn(3)
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s  %-5s  %-9s  job#%-5d  %s",
			time.Now().Format("15:04:05.000"),
			levels[rand.Intn(len(levels))], services[rand.Intn(len(services))],
			s.seq, verbs[rand.Intn(len(verbs))])
		s.seq++
	}
	return out
}

// Package podlogs demonstrates Streamed data: a pod's log, read the way the
// Kubernetes API offers it — a tail, then a follow, and nothing else. There
// is no pagination and no offset, so it goes in a pkg/logview, with
// pkg/resume keeping reconnects exact (CLAUDE.md rule 34).
//
// The fake kubelet here behaves like the real one where it matters:
//
//   - It writes four lines a second, and "since" is truncated to the
//     second, so a naive reconnect repeats up to three lines. resume.Tracker
//     remembers how many it saw in the last second and drops exactly those.
//   - The connection drops every 20 seconds. Each reconnect is marked in the
//     log — lines written while it was down are fetched, but anything the
//     kubelet rotated away in the meantime would be lost silently, and
//     nothing can tell, so the marker says so rather than guessing.
//   - The container restarts every 100 seconds. Follow ends with it; the new
//     container's lines continue after a marker, and P opens the previous
//     run on its own, never interleaved.
//   - Only the newest minute (240 lines) is retained. O loads older lines by asking
//     for a longer tail — which re-sends every newer line, so it is a key you
//     press, never something scrolling does — until the start is reached.
package podlogs

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/resume"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

const (
	lineEvery = 250 * time.Millisecond
	runLines  = 400 // a container run lasts 100s
	retained  = 240 // what the kubelet still has: the newest minute
	dropAfter = 20 * time.Second
	initTail  = 100
	latency   = 300 * time.Millisecond
	pod       = "web-7f9c4d-x2kq"
)

// ---- the fake kubelet ---------------------------------------------------

type kubelet struct{ start time.Time }

type logLine struct {
	n  int
	ts time.Time
}

func (k kubelet) at(n int) logLine {
	return logLine{n: n, ts: k.start.Add(time.Duration(n) * lineEvery)}
}

// written is how many lines exist by now.
func (k kubelet) written() int { return int(time.Since(k.start) / lineEvery) }

func runOf(n int) int { return n / runLines }

func (l logLine) text() string {
	msg := fmt.Sprintf("GET /api/orders/%d 200 %dms", l.n, 4+l.n%37)
	switch {
	case l.n%runLines == 0:
		msg = "starting server on :8080"
	case l.n%23 == 0:
		msg = fmt.Sprintf("WARN slow upstream: %dms", 800+l.n%400)
	}
	return fmt.Sprintf("%s  %s", l.ts.Format("15:04:05.000"), msg)
}

// logs answers GET .../log for run (the current one, or previous): the
// retained lines, then since (inclusive, truncated to the second), then the
// last tail of them.
func (k kubelet) logs(run int, since time.Time, tail int) []logLine {
	hi := min(k.written(), (run+1)*runLines)
	lo := max(run*runLines, hi-retained)
	var out []logLine
	for n := lo; n < hi; n++ {
		l := k.at(n)
		if !since.IsZero() && l.ts.Before(since.Truncate(time.Second)) {
			continue
		}
		out = append(out, l)
	}
	if tail > 0 && len(out) > tail {
		out = out[len(out)-tail:]
	}
	return out
}

// ---- the screen ---------------------------------------------------------

// New returns the pod logs demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{kube: kubelet{start: time.Now().Add(-60 * time.Second)}, tr: resume.New(resume.Options{})}
	s.run = runOf(s.kube.written())
	s.SetTheme(t)
	return s
}

type Screen struct {
	t    theme.Theme
	log  logview.Model
	kube kubelet
	tr   resume.Tracker

	run       int
	gen       int
	connected time.Time
	tail      int
	atStart   bool
	loading   bool
}

type batchMsg struct {
	gen   int
	lines []logLine
}
type lineMsg struct {
	gen  int
	line logLine
}
type endedMsg struct {
	gen     int
	restart bool
}
type olderMsg struct {
	asked int
	lines []logLine
}
type reconnectMsg struct{}

func (s *Screen) Title() string         { return "Pod logs" }
func (s *Screen) IsCapturingKeys() bool { return s.log.IsCapturingKeys() }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) Layout() layout.Node   { return layout.Sized(&s.log) }
func (s *Screen) Help() []key.Binding   { return help.Flatten(s.HelpSections()) }

func (s *Screen) HelpSections() []help.Section {
	return help.SectionsOf(&s.log, help.Group("Pod",
		key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "load older")),
		key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "previous run")),
	))
}

func (s *Screen) Init() tea.Cmd { return s.connect() }

// connect opens a follow: the tail on the first connect, and after that
// from where the tracker says the stream left off.
func (s *Screen) connect() tea.Cmd {
	s.gen++
	s.connected = time.Now()
	gen, run, kube := s.gen, s.run, s.kube
	since, _, ok := s.tr.Resume(time.Now())
	tail := initTail
	if ok {
		tail = 0
	}
	return func() tea.Msg {
		time.Sleep(latency)
		if !ok {
			since = time.Time{}
		}
		return batchMsg{gen: gen, lines: kube.logs(run, since, tail)}
	}
}

// next waits for the line after n: the follow half of the request.
func (s *Screen) next(n int) tea.Cmd {
	gen, run, kube, connected := s.gen, s.run, s.kube, s.connected
	return func() tea.Msg {
		if runOf(n) != run {
			// The container ended; so does the follow.
			time.Sleep(time.Until(kube.at(n).ts))
			return endedMsg{gen: gen, restart: true}
		}
		l := kube.at(n)
		if l.ts.Sub(connected) > dropAfter {
			time.Sleep(time.Until(connected.Add(dropAfter)))
			return endedMsg{gen: gen}
		}
		time.Sleep(time.Until(l.ts))
		return lineMsg{gen: gen, line: l}
	}
}

// take appends the lines a reconnect hasn't already shown.
func (s *Screen) take(lines []logLine) {
	var out []string
	for _, l := range lines {
		if s.tr.Drop(l.ts) {
			continue
		}
		s.tr.Observe(l.ts)
		out = append(out, l.text())
	}
	s.log.AppendLines(out)
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case batchMsg:
		if m.gen != s.gen {
			return s, nil
		}
		if gap := s.tr.Reconnected(); gap > 0 {
			s.log.AppendMarker(fmt.Sprintf("── reconnected after %s · lines may be missing if the log rotated ──", gap.Round(time.Second)))
		}
		s.take(m.lines)
		s.retitle()
		next := s.kube.written()
		if len(m.lines) > 0 {
			next = m.lines[len(m.lines)-1].n + 1
		}
		return s, s.next(next)

	case lineMsg:
		if m.gen != s.gen {
			return s, nil
		}
		s.take([]logLine{m.line})
		return s, s.next(m.line.n + 1)

	case endedMsg:
		if m.gen != s.gen {
			return s, nil
		}
		s.tr.Lost(time.Now())
		if m.restart {
			s.run++
			s.tr = resume.New(resume.Options{})
			s.atStart = false
			s.log.AppendMarker(fmt.Sprintf("── container restarted (restart %d) ──", s.run))
			s.retitle()
			return s, s.connect()
		}
		return s, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return reconnectMsg{} })

	case reconnectMsg:
		return s, s.connect()

	case olderMsg:
		s.loading = false
		stamps := make([]time.Time, len(m.lines))
		for i, l := range m.lines {
			stamps[i] = l.ts
		}
		cut := s.tr.OlderCut(stamps)
		var older []string
		for _, l := range m.lines[:cut] {
			older = append(older, l.text())
		}
		s.log.Prepend(older)
		s.tr.ObserveOlder(stamps[:cut])
		if len(m.lines) < m.asked {
			s.atStart = true
		}
		s.retitle()
		return s, nil
	}

	if km, ok := msg.(tea.KeyMsg); ok && !s.log.IsCapturingKeys() {
		switch km.String() {
		case "O":
			return s, s.loadOlder()
		case "P":
			if s.run == 0 {
				return s, app.Info("no previous run")
			}
			return s, screen.Push(newPrevious(s.t, s.kube, s.run-1))
		}
	}
	var cmd tea.Cmd
	s.log, cmd = s.log.Update(msg)
	return s, cmd
}

// loadOlder asks for twice the tail held — the only way back through a log
// the API can't page — and keeps what is older than the first line held.
func (s *Screen) loadOlder() tea.Cmd {
	if s.atStart || s.loading {
		return nil
	}
	s.loading = true
	s.tail = max(initTail, len(s.log.Lines())) * 2
	s.retitle()
	asked, run, kube := s.tail, s.run, s.kube
	return func() tea.Msg {
		time.Sleep(latency)
		return olderMsg{asked: asked, lines: kube.logs(run, time.Time{}, asked)}
	}
}

func (s *Screen) retitle() {
	title := fmt.Sprintf("pod %s · app · restarts %d", pod, s.run)
	switch {
	case s.loading:
		title += " — loading older…"
	case s.atStart:
		title += " — start of retained log"
	default:
		title += " — ↑ older available (O)"
	}
	s.log.SetTitle(title)
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	st := s.log.State()
	opts := t.Logview()
	opts.Searchable = true
	opts.LineNumbers = true
	s.log = logview.New(opts)
	s.log.Restore(st)
	s.retitle()
}

// previous is a pod's previous run, shown on its own (previous=true).
type previous struct{ log logview.Model }

func newPrevious(t theme.Theme, k kubelet, run int) screen.Screen {
	opts := t.Logview()
	opts.Title = fmt.Sprintf("pod %s · app · previous run (%d)", pod, run)
	opts.Searchable = true
	p := &previous{log: logview.New(opts)}
	var lines []string
	for _, l := range k.logs(run, time.Time{}, 0) {
		lines = append(lines, l.text())
	}
	p.log.AppendLines(lines)
	p.log.SetFollow(false)
	return p
}

func (p *previous) Title() string         { return "Previous run" }
func (p *previous) Init() tea.Cmd         { return nil }
func (p *previous) OnEnter(any) tea.Cmd   { return nil }
func (p *previous) IsCapturingKeys() bool { return p.log.IsCapturingKeys() }
func (p *previous) Layout() layout.Node   { return layout.Sized(&p.log) }
func (p *previous) Help() []key.Binding   { return p.log.Help() }
func (p *previous) SetTheme(theme.Theme)  {}
func (p *previous) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	p.log, cmd = p.log.Update(msg)
	return p, cmd
}

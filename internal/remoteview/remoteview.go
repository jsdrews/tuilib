// Package remoteview is the machinery pkg/table and pkg/eventlog share for
// showing data that lives behind a slow remote source: which query the
// items on screen answer, whether that is still the one the user committed,
// what failed, what the border says about it, and — in Range — which items
// are resident at all.
//
// It holds state and answers questions; it renders nothing and owns no
// component. Each host keeps its own cursor, rendering and query model, and
// asks this package the questions that must be answered the same way in
// every component that shows remote data: are these items stale, what does
// the border suffix read, is a spinner due.
package remoteview

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/focus"
)

// Answer names the query a set of items answers: the committed filter
// text, the sort column's title, and its direction. Hosts export it under
// their own name (table.Answer, eventlog.Answer) as an alias.
type Answer struct {
	Raw  string
	Sort string
	Desc bool
}

// Normalized is a with the forms that mean the same query made equal.
func (a Answer) Normalized() Answer {
	a.Raw = strings.TrimSpace(a.Raw)
	if a.Sort == "" {
		a.Desc = false
	}
	return a
}

// Label renders the query for the border suffix.
func (a Answer) Label(asc, desc string) string {
	var parts []string
	if a.Raw != "" {
		parts = append(parts, "filter "+a.Raw)
	}
	if a.Sort != "" {
		dir := asc
		if a.Desc {
			dir = desc
		}
		parts = append(parts, "sort "+a.Sort+dir)
	}
	if len(parts) == 0 {
		return "all"
	}
	return strings.Join(parts, " · ")
}

// Frames is the spinner drawn in the border suffix.
var Frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// FrameEvery is how often the suffix spinner advances.
const FrameEvery = 90 * time.Millisecond

// TickMsg advances a host's suffix spinner.
type TickMsg struct {
	Token focus.Token
	Seq   int
}

// Missing is a run of items on screen that are not resident.
type Missing struct {
	First, Last int
	OK          bool
}

// Status is where a host stands with its source: the query its items
// answer, whether the committed query failed, and a page failure scoped to
// the items that were missing when it happened.
type Status struct {
	Answer    Answer
	HasAnswer bool
	Failed    bool
	// Retry marks that committing the unchanged, failed query again must
	// still be reported, since that is how the user retries it.
	Retry bool

	winFailed bool
	winFail   Missing

	frame    int
	tickSeq  int
	tickArmd bool
}

// Stale reports whether the items answer a query other than committed.
func (s Status) Stale(committed Answer) bool {
	return s.HasAnswer && s.Answer != committed.Normalized()
}

// SetAnswer records that newly installed items answer a. It reports
// whether they end a stale spell — the moment a host moves its cursor to
// where the new answer begins.
func (s *Status) SetAnswer(a, committed Answer) (fresh bool) {
	a = a.Normalized()
	s.winFailed = false
	was := s.Stale(committed) || (s.HasAnswer && s.Failed)
	s.Answer, s.HasAnswer = a, true
	if a == committed.Normalized() {
		s.Failed = false
		return was
	}
	return false
}

// AnswerCommitted records that items installed with no Answer — a host's
// plain setters — answer the committed query.
func (s *Status) AnswerCommitted(committed Answer) {
	s.Answer, s.HasAnswer, s.Failed = committed.Normalized(), true, false
}

// SetFailed records a failed fetch for q. A failure for anything but the
// committed query is already superseded and ignored. When the committed
// query is answered and current, only a page of it failed: that failure is
// scoped to the items missing now, and is not a failed query.
func (s *Status) SetFailed(q, committed Answer, missing Missing) {
	if q.Normalized() != committed.Normalized() {
		return
	}
	if s.HasAnswer && !s.Stale(committed) {
		s.winFail, s.winFailed = missing, missing.OK
		return
	}
	s.Failed = true
}

// PageFailed reports whether the items missing now are the ones whose
// page failed.
func (s Status) PageFailed(missing Missing) bool {
	return missing.OK && s.winFailed && missing.First == s.winFail.First && missing.Last == s.winFail.Last
}

// Waiting reports whether the border is showing a spinner: stale items
// waiting on the committed query, or items on screen not yet resident.
func (s Status) Waiting(committed Answer, missing Missing) bool {
	if s.Failed {
		return false
	}
	return s.Stale(committed) || (missing.OK && !s.PageFailed(missing))
}

// Loading reports whether the host's body should show its Loading spinner:
// nothing has answered yet, and nothing has failed.
func (s Status) Loading() bool { return !s.HasAnswer && !s.Failed }

// Suffix is the border text after the host's title, or "" when there is
// nothing to say. noun names what is missing ("rows", "items").
func (s Status) Suffix(committed Answer, missing Missing, noun, asc, desc string) string {
	spin := Frames[s.frame%len(Frames)]
	c := committed.Label(asc, desc)
	if !s.Stale(committed) && s.HasAnswer {
		switch {
		case !missing.OK:
			return ""
		case s.PageFailed(missing):
			return fmt.Sprintf("✗ failed loading %s %d–%d", noun, missing.First+1, missing.Last+1)
		default:
			return fmt.Sprintf("%s loading %s %d–%d", spin, noun, missing.First+1, missing.Last+1)
		}
	}
	switch {
	case s.Failed && !s.HasAnswer:
		return "✗ failed " + c
	case s.Failed && s.Stale(committed):
		return "showing " + s.Answer.Label(asc, desc) + " · ✗ failed " + c
	case s.Stale(committed):
		return "showing " + s.Answer.Label(asc, desc) + " · " + spin + " loading " + c
	}
	return ""
}

// Spin reports the current spinner frame, for hosts drawing other
// waiting states (an edge loading) with the same glyph.
func (s Status) Spin() string { return Frames[s.frame%len(Frames)] }

// Arm returns the next spinner tick when waiting and none is armed.
func (s *Status) Arm(token focus.Token, waiting bool) tea.Cmd {
	if !waiting || s.tickArmd {
		return nil
	}
	s.tickArmd = true
	s.tickSeq++
	msg := TickMsg{Token: token, Seq: s.tickSeq}
	return tea.Tick(FrameEvery, func(time.Time) tea.Msg { return msg })
}

// Handle consumes the host's spinner tick. handled is true for any
// TickMsg; advanced is true when it was this host's current tick and the
// frame moved, so the host should redraw and re-arm.
func (s *Status) Handle(token focus.Token, msg tea.Msg) (handled, advanced bool) {
	t, ok := msg.(TickMsg)
	if !ok {
		return false, false
	}
	if t.Token != token || t.Seq != s.tickSeq {
		return true, false
	}
	s.tickArmd = false
	s.frame++
	return true, true
}

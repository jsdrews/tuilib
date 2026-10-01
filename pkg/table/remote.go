package table

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/internal/remoteview"
	"github.com/jsdrews/tuilib/internal/tick"
	"github.com/jsdrews/tuilib/pkg/focus"
)

// DefaultSortDebounce is how long sort input must go quiet before a remote
// sort commits, when Options.SortDebounce is zero.
const DefaultSortDebounce = 400 * time.Millisecond

// Answer names the query a set of rows answers: the committed filter text,
// the sort column's title, and its direction. Pass it with every window so
// the table can tell whether its rows answer what the user last committed.
// Build it from the request that produced the page — source.Query carries
// the same three fields.
type Answer = remoteview.Answer

type sortSettleMsg struct {
	token focus.Token
	seq   int
}

// remoteState is what the table tracks about a remote source: the sort the
// user has staged but not committed, the query its rows answer, and whether
// the committed query failed. None of it is used by a fully local table.
type remoteState struct {
	debounce time.Duration

	// The committed sort. sortCol/sortDesc are what the header shows, which
	// under SortRemote may be a staged sort still inside its quiet period.
	cSortCol  int
	cSortDesc bool
	staged    bool
	stageSeq  int
	stageArmd bool

	// st is where the table stands with its source: the answered query,
	// failure, and the border suffix's spinner.
	st remoteview.Status

	baseTitle string
	loadCmd   tea.Cmd
}

func (m Model) remote() bool {
	return m.filterMode == FilterRemote || m.sortMode == SortRemote
}

// committed is the query the source should be answering, as an Answer.
func (m Model) committed() Answer {
	raw, sortCol, sortDesc := m.currentQuery()
	a := Answer{Raw: raw}
	if sortCol >= 0 && sortCol < len(m.cols) {
		a.Sort, a.Desc = m.cols[sortCol].Title, sortDesc
	}
	return a.Normalized()
}

// Stale reports whether the rows on screen answer a query other than the
// one the user last committed — a newer one is in flight, or failed. Stale
// rows are real records of the previous query; a verb that must not act on
// them can check this first.
func (m Model) Stale() bool {
	return m.remote() && m.rq.st.Stale(m.committed())
}

// Answered returns the query the rows on screen answer, and false before
// any has arrived. Carry it across a SetTheme rebuild by passing it back
// to SetWindow with the rows.
func (m Model) Answered() (Answer, bool) { return m.rq.st.Answer, m.rq.st.HasAnswer }

// Failed reports whether the committed query's fetch failed. The rows stay
// as they were — stale, or none — until a retry or a new query answers.
func (m Model) Failed() bool { return m.rq.st.Failed }

// SetFailed tells the table the fetch for q failed. It takes effect only
// when q is the committed query; a failure answering anything else is
// already superseded. Restore it after a SetTheme rebuild the same way,
// once the filter and sort have been restored.
func (m *Model) SetFailed(q Answer) {
	if !m.remote() {
		return
	}
	m.rq.st.SetFailed(q, m.committed(), m.missingOnScreen())
	m.refresh()
}

// StagedSort reports a remote sort the user has moved to but not yet
// committed: it commits once sort input has been quiet for
// Options.SortDebounce. ok is false when nothing is staged.
func (m Model) StagedSort() (col int, desc bool, ok bool) {
	return m.sortCol, m.sortDesc, m.rq.staged
}

// CommittedSort reports the sort the source was last asked for. Under
// SortRemote it trails SortColumn while a sort is staged.
func (m Model) CommittedSort() (col int, desc bool) {
	if m.sortMode != SortRemote {
		return m.sortCol, m.sortDesc
	}
	return m.rq.cSortCol, m.rq.cSortDesc
}

// SetStagedSort restores a staged sort after a SetTheme rebuild: the header
// shows it and the quiet period starts again, so the sort the user asked
// for still commits. Call it after SetSort has restored the committed sort.
func (m *Model) SetStagedSort(col int, desc bool) {
	if m.sortMode != SortRemote || col < 0 || col >= len(m.cols) || !m.cols[col].Sortable {
		return
	}
	m.sortCol, m.sortDesc = col, desc
	m.sortChanged()
	m.refresh()
}

// sortChanged runs after sort input moved sortCol/sortDesc. Under
// SortRemote the change is staged until input goes quiet; stepping back to
// the committed sort unstages it, unless that query failed, when landing
// on it again is how the user retries.
func (m *Model) sortChanged() {
	if m.sortMode != SortRemote {
		return
	}
	if m.rq.debounce <= 0 {
		m.commitSort()
		return
	}
	if m.sortCol == m.rq.cSortCol && m.sortDesc == m.rq.cSortDesc && !m.rq.st.Failed {
		m.rq.staged = false
		return
	}
	m.rq.staged = true
	m.rq.stageSeq++
	m.rq.stageArmd = false
}

func (m *Model) commitSort() {
	if m.sortCol == m.rq.cSortCol && m.sortDesc == m.rq.cSortDesc && m.rq.st.Failed {
		m.rq.st.Retry = true
	}
	m.rq.cSortCol, m.rq.cSortDesc = m.sortCol, m.sortDesc
	m.rq.staged = false
}

// setAnswer records the query a newly installed window answers. The move
// from stale to fresh is when the cursor goes to the top: the rows it had
// been on answered another query.
func (m *Model) setAnswer(a Answer) {
	if !m.remote() {
		return
	}
	if m.rq.st.SetAnswer(a, m.committed()) {
		m.cursor = 0
		m.viewStart = 0
	}
}

// handleRemote consumes the table's own timers.
func (m *Model) handleRemote(msg tea.Msg) (bool, tea.Cmd) {
	switch t := msg.(type) {
	case sortSettleMsg:
		if t.token != m.token || t.seq != m.rq.stageSeq || !m.rq.staged {
			return true, nil
		}
		m.commitSort()
		m.refresh()
		return true, m.flushMsgs()
	}
	if handled, advanced := m.rq.st.Handle(m.token, msg); handled {
		if !advanced {
			return true, nil
		}
		m.refresh()
		return true, m.flushMsgs()
	}
	return false, nil
}

// flushRemote arms whatever timer the current state needs and hands back a
// loading tick raised inside a setter with no return path.
func (m *Model) flushRemote() tea.Cmd {
	if !m.remote() {
		return nil
	}
	var cmds []tea.Cmd
	if m.rq.loadCmd != nil {
		cmds = append(cmds, m.rq.loadCmd)
		m.rq.loadCmd = nil
	}
	if m.rq.staged && !m.rq.stageArmd {
		m.rq.stageArmd = true
		msg := sortSettleMsg{token: m.token, seq: m.rq.stageSeq}
		cmds = append(cmds, tick.After(m.rq.debounce, func(time.Time) tea.Msg { return msg }))
	}
	waiting := m.rq.st.Waiting(m.committed(), m.missingOnScreen()) || m.spanEdgeLoading() != ""
	cmds = append(cmds, m.rq.st.Arm(m.token, waiting))
	return tea.Batch(cmds...)
}

// syncRemote brings the chrome in line with the remote state: Loading
// before the first answer, the title suffix while stale or failed.
func (m *Model) syncRemote() {
	if !m.remote() {
		return
	}
	loading := m.rq.st.Loading()
	if cmd := m.body.SetLoading(loading); cmd != nil {
		m.rq.loadCmd = cmd
	}
	title := m.rq.baseTitle
	if suffix := m.staleSuffix(); suffix != "" {
		title += " — " + suffix
	}
	m.body.SetTitle(title)
}

// missingOnScreen reports the first and last rows on screen that the
// window doesn't hold — the rows a scroll is waiting for.
func (m Model) missingOnScreen() remoteview.Missing {
	var miss remoteview.Missing
	if !m.windowed || !m.rq.st.HasAnswer {
		return miss
	}
	end := min(m.viewStart+m.dataRows(), m.rowCount())
	for i := m.viewStart; i < end; i++ {
		if _, resident := m.rowAt(i); resident {
			continue
		}
		if !miss.OK {
			miss.First, miss.OK = i, true
		}
		miss.Last = i
	}
	return miss
}

func (m Model) staleSuffix() string {
	if s := m.spanEdgeLoading(); s != "" {
		return s
	}
	return m.rq.st.Suffix(m.committed(), m.missingOnScreen(), "rows", m.glyphs.SortAsc, m.glyphs.SortDesc)
}

// failedBody is drawn in place of the rows when the first load failed.
func (m Model) failedBody() string {
	c := m.body.ContentRect()
	return lipgloss.Place(max(1, c.W), max(1, c.H), lipgloss.Center, lipgloss.Center, "✗ failed to load")
}

// answerCommitted records that rows installed without an explicit Answer —
// Options.Rows, SetRows, SetKeyedRows — answer the committed query.
func (m *Model) answerCommitted() {
	if !m.remote() {
		return
	}
	m.rq.st.AnswerCommitted(m.committed())
}

// Committed returns the query the source was last asked for, as an Answer —
// what SetFailed compares against when a rebuild restores a failure.
func (m Model) Committed() Answer { return m.committed() }

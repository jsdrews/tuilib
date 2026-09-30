package table

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/geom"
)

// flatten runs cmd, flattening batches. Ticks sleep, so tests using it keep
// their durations tiny.
func flatten(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, flatten(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func newLatency(t *testing.T) Model {
	t.Helper()
	m := New(Options{
		Title: "Apps",
		Columns: []Column{
			{Title: "Name", Width: 10, Sortable: true},
			{Title: "Region", Width: 10, Sortable: true},
		},
		Filterable:   true,
		FilterMode:   FilterRemote,
		SortMode:     SortRemote,
		SortDebounce: time.Millisecond,
	})
	m.SetRect(geom.New(0, 0, 60, 14))
	return m
}

func page(n int, prefix string) []Row {
	rows := make([]Row, n)
	for i := range rows {
		rows[i] = Row{prefix, "eu"}
	}
	return rows
}

func commitQ(m Model, text string) (Model, *QueryChangedMsg) {
	m, cmd := commitFilter(m, text)
	return m, drainQueryMsg(cmd)
}

func TestRemoteLoadsUntilFirstAnswer(t *testing.T) {
	m := newLatency(t)
	if !m.Loading() {
		t.Fatal("a remote table with no answer should be Loading")
	}
	m.SetWindow(page(10, "a"), 0, 10, Answer{})
	if m.Loading() || m.Stale() {
		t.Errorf("after the first answer: Loading=%v Stale=%v, want neither", m.Loading(), m.Stale())
	}
}

func TestCommitMakesRowsStaleNotLoading(t *testing.T) {
	m := newLatency(t)
	m.SetWindow(page(10, "a"), 0, 10, Answer{})
	m, q := commitQ(m, "eu")
	if q == nil || q.Raw != "eu" {
		t.Fatalf("q = %+v", q)
	}
	if !m.Stale() || m.Loading() {
		t.Fatalf("Stale=%v Loading=%v, want stale rows, not Loading", m.Stale(), m.Loading())
	}
	if len(m.rows) != 10 {
		t.Error("stale rows were dropped")
	}
	if !strings.Contains(m.body.Title(), "showing all") || !strings.Contains(m.body.Title(), "loading filter eu") {
		t.Errorf("border title = %q, want the stale suffix", m.body.Title())
	}
	if m.Title() != "Apps" {
		t.Errorf("Title() = %q, want the label alone", m.Title())
	}
	m.SetWindow(page(3, "b"), 0, 3, Answer{Raw: "eu"})
	if m.Stale() {
		t.Error("the committed query's answer should clear staleness")
	}
	if m.body.Title() != "Apps" {
		t.Errorf("border title = %q, want no suffix once settled", m.body.Title())
	}
}

func TestStaleRowsRenderDimmed(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prev)

	m := newLatency(t)
	m.SetWindow(page(10, "a"), 0, 10, Answer{})
	fresh := m.View()
	m, _ = commitQ(m, "eu")
	stale := m.View()
	if strings.Count(stale, "\x1b[2m") <= strings.Count(fresh, "\x1b[2m") {
		t.Error("stale rows should render faint")
	}
}

func TestSupersededAnswerStaysStale(t *testing.T) {
	m := newLatency(t)
	m.SetWindow(page(10, "a"), 0, 10, Answer{})
	m, _ = commitQ(m, "eu")
	m.SetWindow(page(10, "a"), 0, 10, Answer{}) // a refetch of the old query
	if !m.Stale() {
		t.Error("rows answering the previous query must stay stale")
	}
}

func TestCursorStaysWhileStaleThenTops(t *testing.T) {
	m := newLatency(t)
	m.SetWindow(page(100, "a"), 0, 5000, Answer{})
	m.SetCursor(40)
	m, _ = commitQ(m, "eu")
	if m.Cursor() != 40 {
		t.Fatalf("cursor = %d at commit, want it left on the stale rows", m.Cursor())
	}
	m.SetCursor(60)
	m.SetWindow(page(100, "b"), 0, 800, Answer{Raw: "eu"})
	if m.Cursor() != 0 {
		t.Errorf("cursor = %d, want row 0 once the new query answers", m.Cursor())
	}
	m.SetCursor(30)
	m.SetWindow(page(100, "b"), 0, 800, Answer{Raw: "eu"}) // same query, refreshed
	if m.Cursor() != 30 {
		t.Errorf("cursor = %d, a refetch of the same query must not move it", m.Cursor())
	}
}

func TestSortDebounced(t *testing.T) {
	m := newLatency(t)
	m.SetWindow(page(10, "a"), 0, 10, Answer{})
	m, cmd1 := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	m, cmd2 := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	msgs1, msgs2 := flatten(cmd1), flatten(cmd2) // a Tick runs once only
	for _, msg := range append(append([]tea.Msg{}, msgs1...), msgs2...) {
		if _, ok := msg.(QueryChangedMsg); ok {
			t.Fatal("sort keys committed before the quiet period")
		}
	}
	if col, _, ok := m.StagedSort(); !ok || col != 1 {
		t.Fatalf("staged = %d,%v, want Region staged", col, ok)
	}
	if m.Stale() {
		t.Error("a staged sort is not a committed query; rows should not be stale yet")
	}
	var last sortSettleMsg
	for _, msg := range msgs1 {
		if s, ok := msg.(sortSettleMsg); ok {
			var c tea.Cmd
			m, c = m.Update(s)
			if drainQueryMsg(c) != nil {
				t.Fatal("an overtaken timer committed")
			}
		}
	}
	for _, msg := range msgs2 {
		if s, ok := msg.(sortSettleMsg); ok {
			last = s
		}
	}
	m, cmd := m.Update(last)
	q := drainQueryMsg(cmd)
	if q == nil || q.Sort != "Region" {
		t.Fatalf("q = %+v, want one query sorted by Region", q)
	}
	if !m.Stale() {
		t.Error("a committed sort should make the rows stale")
	}
}

func TestSortSteppedBackCommitsNothing(t *testing.T) {
	m := newLatency(t)
	m.SetWindow(page(10, "a"), 0, 10, Answer{})
	m.SetSort(0, false)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if _, _, ok := m.StagedSort(); ok {
		t.Error("returning to the committed sort should unstage it")
	}
	if drainQueryMsg(cmd) != nil {
		t.Error("stepping back emitted a query")
	}
}

func TestFilterCommitFlushesStagedSort(t *testing.T) {
	m := newLatency(t)
	m.SetWindow(page(10, "a"), 0, 10, Answer{})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	m, q := commitQ(m, "eu")
	if q == nil || q.Raw != "eu" || q.Sort != "Name" {
		t.Fatalf("q = %+v, want filter and staged sort in one query", q)
	}
	if _, _, ok := m.StagedSort(); ok {
		t.Error("the staged sort should have been committed")
	}
}

func TestStagedSortSurvivesRebuild(t *testing.T) {
	m := newLatency(t)
	m.SetWindow(page(10, "a"), 0, 10, Answer{})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})

	n := newLatency(t)
	col, desc := m.CommittedSort()
	n.SetSort(col, desc)
	a, _ := m.Answered()
	n.SetWindow(m.rows, 0, 10, a)
	if sc, sd, ok := m.StagedSort(); ok {
		n.SetStagedSort(sc, sd)
	}
	_, cmd := n.Update(struct{}{})
	var settled tea.Cmd
	for _, msg := range flatten(cmd) {
		if s, ok := msg.(sortSettleMsg); ok {
			n, settled = n.Update(s)
		}
	}
	if q := drainQueryMsg(settled); q == nil || q.Sort != "Name" {
		t.Fatalf("q = %+v, want the staged sort to commit after a rebuild", q)
	}
}

func TestFailedQueryStaysStaleAndRetries(t *testing.T) {
	m := newLatency(t)
	m.SetWindow(page(10, "a"), 0, 10, Answer{})
	m, _ = commitQ(m, "eu")
	m.SetFailed(Answer{Raw: "other"})
	if m.Failed() {
		t.Fatal("a failure for another query must be ignored")
	}
	m.SetFailed(Answer{Raw: "eu"})
	if !m.Failed() || !m.Stale() {
		t.Fatalf("Failed=%v Stale=%v, want a failed, still-stale table", m.Failed(), m.Stale())
	}
	if !strings.Contains(m.body.Title(), "✗ failed filter eu") {
		t.Errorf("border title = %q", m.body.Title())
	}
	if m.Value() != "eu" {
		t.Error("the user's input must be kept")
	}
	m, q := commitQ(m, "")
	if q == nil || q.Raw != "eu" {
		t.Fatalf("q = %+v, enter on a failed query should retry it", q)
	}
	if m.Failed() {
		t.Error("a retry should clear the failure")
	}
}

func TestFailedFirstLoad(t *testing.T) {
	m := newLatency(t)
	m.SetFailed(Answer{})
	if m.Loading() {
		t.Error("a failed first load should stop Loading")
	}
	if !strings.Contains(m.View(), "failed to load") {
		t.Error("a failed first load should say so in the body")
	}
	_ = errors.New
}

func TestSetRowsAnswersCommittedQuery(t *testing.T) {
	m := newRemote(t, FilterRemote, SortLocal)
	if m.Loading() {
		t.Error("rows given up front answer the committed query")
	}
	m.SetRows(remoteRows)
	if m.Stale() {
		t.Error("SetRows should count as the committed query's answer")
	}
}

func TestLocalTableHasNoRemoteChrome(t *testing.T) {
	m := New(Options{Title: "T", Columns: []Column{{Title: "A", Width: 5}}})
	m.SetRect(geom.New(0, 0, 30, 8))
	if m.Loading() || m.Stale() || m.body.Title() != "T" {
		t.Error("a local table must be untouched by remote state")
	}
}

func TestMissingRowsShowLoadingOnBorder(t *testing.T) {
	m := newLatency(t)
	m.SetWindow(page(100, "a"), 0, 5000, Answer{})
	if m.body.Title() != "Apps" {
		t.Fatalf("border = %q, want no suffix while every row on screen is held", m.body.Title())
	}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if !strings.Contains(m.body.Title(), "loading rows") || !strings.HasSuffix(m.body.Title(), "–5000") {
		t.Errorf("border = %q, want the missing rows named", m.body.Title())
	}
	armed := false
	for _, msg := range flatten(cmd) {
		if _, ok := msg.(staleTickMsg); ok {
			armed = true
		}
	}
	if !armed {
		t.Error("the spinner should animate while rows are missing")
	}
	m.SetWindow(page(100, "a"), 4900, 5000, Answer{})
	if m.body.Title() != "Apps" {
		t.Errorf("border = %q, want the suffix gone once the rows land", m.body.Title())
	}
}

func TestFailedPageNamesItsRowsOnly(t *testing.T) {
	m := newLatency(t)
	m.SetWindow(page(100, "a"), 0, 5000, Answer{})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m.SetFailed(Answer{})
	if m.Failed() {
		t.Error("a failed page is not a failed query")
	}
	if !strings.Contains(m.body.Title(), "✗ failed loading rows") {
		t.Fatalf("border = %q, want the failed page named", m.body.Title())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	m.SetCursor(300)
	if strings.Contains(m.body.Title(), "failed") {
		t.Errorf("border = %q — other missing rows are being fetched afresh, not failed", m.body.Title())
	}
}

package table

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
)

var remoteRows = []Row{
	{"Oslo", "Europe", "700"},
	{"Osaka", "Asia", "2700"},
	{"Lima", "Americas", "9700"},
}

func newRemote(t *testing.T, fm FilterMode, sm SortMode) Model {
	t.Helper()
	m := New(Options{
		Columns: []Column{
			{Title: "Name", Width: 10, Sortable: true},
			{Title: "Region", Width: 10},
			{Title: "Pop", Width: 8, Sortable: true},
		},
		Rows:       remoteRows,
		Filterable: true,
		FilterMode: fm,
		SortMode:   sm,
	})
	m.SetRect(geom.New(0, 0, 40, 12))
	return m
}

// drainQueryMsg runs cmd and returns the first QueryChangedMsg it produces,
// or nil if none. Mirrors drainViewportMsg.
func drainQueryMsg(cmd tea.Cmd) *QueryChangedMsg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if q, ok := msg.(QueryChangedMsg); ok {
		return &q
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range batch {
			if q := drainQueryMsg(sub); q != nil {
				return q
			}
		}
	}
	return nil
}

func typeRunes(m Model, s string) (Model, []tea.Cmd) {
	var cmds []tea.Cmd
	for _, r := range s {
		var cmd tea.Cmd
		m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		cmds = append(cmds, cmd)
	}
	return m, cmds
}

// commitFilter focuses the filter, types q, and presses enter.
func commitFilter(m Model, q string) (Model, tea.Cmd) {
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, _ = typeRunes(m, q)
	return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

func TestFilterRemoteDoesNotFilterRows(t *testing.T) {
	m := newRemote(t, FilterRemote, SortLocal)
	m, _ = commitFilter(m, "oslo")
	if len(m.Visible()) != len(remoteRows) {
		t.Errorf("visible = %d rows, want all %d — a remote table displays what it was given",
			len(m.Visible()), len(remoteRows))
	}
}

func TestFilterLocalStillFilters(t *testing.T) {
	m := newRemote(t, FilterLocal, SortLocal)
	m, _ = commitFilter(m, "oslo")
	if len(m.Visible()) != 1 {
		t.Errorf("visible = %d rows, want 1", len(m.Visible()))
	}
}

func TestSortRemoteDoesNotReorderRows(t *testing.T) {
	m := newRemote(t, FilterLocal, SortRemote)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if got := m.Visible()[0][0]; got != "Oslo" {
		t.Errorf("first row = %q, want %q — remote sort must leave order alone", got, "Oslo")
	}
	if m.SortColumn() < 0 {
		t.Error("remote sort should still track the active column")
	}
}

func TestSortLocalStillReorders(t *testing.T) {
	m := newRemote(t, FilterLocal, SortLocal)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if got := m.Visible()[0][0]; got != "Lima" {
		t.Errorf("first row = %q, want %q", got, "Lima")
	}
}

func TestSortRemoteKeepsHeaderMarker(t *testing.T) {
	m := newRemote(t, FilterLocal, SortRemote)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	cells := m.headerCells()
	if cells[m.SortColumn()] == "Name" {
		t.Error("active sort column should carry a ▲/▼ marker even when the source sorts")
	}
}

func TestCommitEmitsQuery(t *testing.T) {
	m := newRemote(t, FilterRemote, SortLocal)
	m, cmd := commitFilter(m, "region:eu")
	q := drainQueryMsg(cmd)
	if q == nil {
		t.Fatal("committing a filter emitted no QueryChangedMsg")
	}
	if q.Raw != "region:eu" {
		t.Errorf("Raw = %q", q.Raw)
	}
	if len(q.Terms) != 1 {
		t.Fatalf("Terms = %d, want 1", len(q.Terms))
	}
	if q.Terms[0].Title != "Region" || q.Terms[0].Value != "eu" {
		t.Errorf("term = %+v, want the resolved column title and value", q.Terms[0])
	}
}

func TestTypingDoesNotEmit(t *testing.T) {
	m := newRemote(t, FilterRemote, SortLocal)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, cmds := typeRunes(m, "europe")
	for i, cmd := range cmds {
		if q := drainQueryMsg(cmd); q != nil {
			t.Fatalf("keystroke %d emitted %+v — filters commit, they don't stream", i, *q)
		}
	}
}

func TestTabCompletionDoesNotEmit(t *testing.T) {
	m := newRemote(t, FilterRemote, SortLocal)
	m.SetDistinct(1, []string{"Europe", "Americas"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, _ = typeRunes(m, "region:eu")
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := m.Value(); got != "region:europe" {
		t.Fatalf("Value after tab = %q, want completion to have happened", got)
	}
	if q := drainQueryMsg(cmd); q != nil {
		t.Errorf("tab completion emitted %+v — it edits a term, it doesn't commit one", *q)
	}
}

func TestEscClearingCommittedFilterEmitsEmpty(t *testing.T) {
	m := newRemote(t, FilterRemote, SortLocal)
	m, _ = commitFilter(m, "oslo")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	q := drainQueryMsg(cmd)
	if q == nil {
		t.Fatal("clearing a committed filter emitted nothing")
	}
	if q.Raw != "" || q.Terms != nil {
		t.Errorf("q = %+v, want an empty query", *q)
	}
}

func TestEscOnUncommittedFilterDoesNotEmit(t *testing.T) {
	m := newRemote(t, FilterRemote, SortLocal)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, _ = typeRunes(m, "oslo")
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if q := drainQueryMsg(cmd); q != nil {
		t.Errorf("abandoning an uncommitted filter emitted %+v", *q)
	}
}

func TestSortKeyEmitsQuery(t *testing.T) {
	m := newRemote(t, FilterLocal, SortRemote)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	q := drainQueryMsg(cmd)
	if q == nil {
		t.Fatal("sort key emitted no QueryChangedMsg")
	}
	if q.Sort != "Name" || q.SortColumn != 0 || q.Desc {
		t.Errorf("q = %+v, want Name ascending", *q)
	}
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	q = drainQueryMsg(cmd)
	if q == nil || !q.Desc {
		t.Errorf("direction toggle = %+v, want Desc", q)
	}
}

func TestHeaderClickEmitsQuery(t *testing.T) {
	m := New(Options{
		Columns:  []Column{{Title: "Name", Width: 10, Sortable: true}},
		Rows:     []Row{{"Oslo"}, {"Lima"}},
		SortMode: SortRemote,
	})
	m.SetRect(geom.New(0, 0, 40, 12))
	m, cmd := m.Update(press(2, 1, 1))
	q := drainQueryMsg(cmd)
	if q == nil {
		t.Fatal("clicking a sortable header emitted no QueryChangedMsg")
	}
	if q.Sort != "Name" {
		t.Errorf("q = %+v", *q)
	}
}

func TestDuplicateQueryElided(t *testing.T) {
	m := newRemote(t, FilterRemote, SortLocal)
	m, cmd := commitFilter(m, "oslo")
	if drainQueryMsg(cmd) == nil {
		t.Fatal("first commit should emit")
	}
	// Re-commit the identical text: focus, change nothing, press enter.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if q := drainQueryMsg(cmd); q != nil {
		t.Errorf("re-committing the same query emitted %+v", *q)
	}
}

func TestNoRepeatEmitOnIdleUpdate(t *testing.T) {
	m := newRemote(t, FilterRemote, SortLocal)
	m, cmd := commitFilter(m, "oslo")
	if drainQueryMsg(cmd) == nil {
		t.Fatal("commit should emit once")
	}
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if q := drainQueryMsg(cmd); q != nil {
		t.Errorf("a later unrelated key re-emitted %+v — the pending flag did not clear", *q)
	}
}

func TestSetSortDoesNotEmit(t *testing.T) {
	m := newRemote(t, FilterRemote, SortRemote)
	m.SetSort(2, true)
	if q := drainQueryMsg(m.flushMsgs()); q != nil {
		t.Errorf("SetSort emitted %+v — a SetTheme rebuild would refetch on every theme swap", *q)
	}
	if m.SortColumn() != 2 || !m.SortDescending() {
		t.Error("SetSort should still apply the sort it was given")
	}
}

func TestSetValueDoesNotEmit(t *testing.T) {
	m := newRemote(t, FilterRemote, SortLocal)
	m.SetValue("region:europe")
	if q := drainQueryMsg(m.flushMsgs()); q != nil {
		t.Errorf("SetValue emitted %+v — restoring filter text is not a new query", *q)
	}
	if m.Value() != "region:europe" {
		t.Error("SetValue should still apply the text it was given")
	}
}

func TestLocalTableNeverEmits(t *testing.T) {
	m := newRemote(t, FilterLocal, SortLocal)
	m, cmd := commitFilter(m, "oslo")
	if q := drainQueryMsg(cmd); q != nil {
		t.Errorf("a fully local table emitted %+v", *q)
	}
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if q := drainQueryMsg(cmd); q != nil {
		t.Errorf("a fully local table emitted %+v on sort", *q)
	}
}

func TestQueryAccessorStartsEmpty(t *testing.T) {
	m := newRemote(t, FilterRemote, SortRemote)
	q := m.Query()
	if q.Raw != "" || q.Terms != nil || q.Sort != "" || q.SortColumn != -1 || q.Desc {
		t.Errorf("initial Query() = %+v, want zero-valued with SortColumn -1", q)
	}
}

func TestQueryAccessorReflectsCommittedState(t *testing.T) {
	m := newRemote(t, FilterRemote, SortRemote)
	m, _ = commitFilter(m, "oslo")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	q := m.Query()
	if q.Raw != "oslo" || q.Sort != "Name" {
		t.Errorf("Query() = %+v", q)
	}
}

func TestMidEditSortDoesNotLeakHalfTypedFilter(t *testing.T) {
	m := newRemote(t, FilterRemote, SortRemote)
	m, _ = commitFilter(m, "oslo")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, _ = typeRunes(m, "xyz")
	if got := m.Query().Raw; got != "oslo" {
		t.Errorf("Query().Raw = %q while mid-edit, want the last committed text", got)
	}
}

func TestRemoteDistinctIgnoresResidentRows(t *testing.T) {
	m := newRemote(t, FilterRemote, SortLocal)
	if got := m.distinct; len(got) > 0 && len(got[1]) > 0 {
		t.Errorf("remote mode scraped candidates from resident rows: %v", got[1])
	}
}

func TestSetDistinctSurvivesSetRows(t *testing.T) {
	m := newRemote(t, FilterRemote, SortLocal)
	m.SetDistinct(1, []string{"Europe", "Asia", "Americas", "Africa"})
	m.SetRows([]Row{{"Cairo", "Africa", "9"}})
	got := m.distinct[1]
	if len(got) != 4 {
		t.Fatalf("candidates after SetRows = %v, want the fed set to survive", got)
	}
	if got[0] != "africa" {
		t.Errorf("candidates = %v, want normalized + sorted", got)
	}
}

func TestSetDistinctRejectsOutOfRange(t *testing.T) {
	m := newRemote(t, FilterRemote, SortLocal)
	m.SetDistinct(-1, []string{"x"})
	m.SetDistinct(99, []string{"x"})
	if len(m.distinct) != 0 && len(m.distinct) != len(m.cols) {
		t.Errorf("distinct sized %d, want 0 or %d", len(m.distinct), len(m.cols))
	}
}

func TestLocalDistinctStillScrapesRows(t *testing.T) {
	m := newRemote(t, FilterLocal, SortLocal)
	got := m.distinct[1]
	if len(got) != 3 {
		t.Errorf("local candidates = %v, want one per region", got)
	}
}

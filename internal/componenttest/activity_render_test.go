// Derived activity, rendered.
//
// The plainest configuration the feature has: no action, no shell, no
// revision, no second endpoint. A status arrives on a poll, it is in the set
// that means work, the row spins. Everything else in pkg/activity is a
// refinement on top of this, so this is the part that has to be right.
//
// What is asserted here is specifically the *rendering*, because substituting
// a spinner into a cell is where the hazards are: a cell too narrow for the
// label, a column that reflows under the user, a sort or filter that moves the
// row out from under its own indicator, a mark gutter beside it, a styled cell
// the predicate has to see through.
package componenttest

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/ansi"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// spinnerFrames is every frame of spinner.Dot, for asserting that something is
// animating without pinning which frame it stopped on.
const spinnerFrames = "⣾⣽⣻⢿⡿⣟⣯⣷"

func spinning(s string) bool { return strings.ContainsAny(s, spinnerFrames) }

// rowWidths reports the visible width of every non-empty rendered line, which
// is how a reflow shows up: the widths change when the data does.
func rowWidths(view string) []int {
	var out []int
	for _, line := range strings.Split(view, "\n") {
		if strings.TrimSpace(xansi.Strip(line)) != "" {
			out = append(out, xansi.StringWidth(line))
		}
	}
	return out
}

// derivedTable is the whole configuration under test: two columns, a status
// column, and one predicate. Nothing else.
func derivedTable(t *testing.T, statusWidth int, opts ...func(*table.Options)) table.Model {
	t.Helper()
	o := theme.Dark().Table()
	o.Columns = []table.Column{
		{Title: "Name", Width: 14},
		{Title: "Status", Width: statusWidth},
	}
	o.ActivityColumn = "Status"
	o.ActivityWhen = func(c table.Row) (string, bool) {
		return activity.Busy("running", "pending")(c[1])
	}
	for _, fn := range opts {
		fn(&o)
	}
	m := table.New(o)
	m.SetRect(geom.New(0, 0, 60, 12))
	return m
}

func statuses(m *table.Model, rows ...[2]string) {
	keyed := make([]table.KeyedRow, 0, len(rows))
	for _, r := range rows {
		keyed = append(keyed, table.KeyedRow{Key: r[0], Cells: []string{r[0], r[1]}})
	}
	m.SetKeyedRows(keyed)
	m.SetRect(geom.New(0, 0, 60, 12))
}

// The headline: a status in the busy set spins, with no setter call anywhere.
func TestAStatusInTheBusySetSpins(t *testing.T) {
	m := derivedTable(t, 12)
	statuses(&m, [2]string{"api", "running"}, [2]string{"web", "succeeded"})

	view := m.View()
	if !spinning(view) {
		t.Errorf("no spinner for a running row:\n%s", view)
	}
	if !strings.Contains(xansi.Strip(view), "running") {
		t.Errorf("the label is missing:\n%s", view)
	}
	if m.ActivityCount() != 1 {
		t.Errorf("ActivityCount = %d, want only the running row", m.ActivityCount())
	}
}

// And stops when the status leaves the set, with no outcome glyph: the cell's
// own value already says how it ended.
func TestAStatusLeavingTheBusySetStopsSpinning(t *testing.T) {
	m := derivedTable(t, 12)
	statuses(&m, [2]string{"api", "running"})
	statuses(&m, [2]string{"api", "failed"})

	view := xansi.Strip(m.View())
	if spinning(view) {
		t.Errorf("still spinning after the status settled:\n%s", view)
	}
	if !strings.Contains(view, "failed") {
		t.Errorf("the cell did not go back to its own value:\n%s", view)
	}
}

// Widths come from the rows the table holds, never from the indicator, so no
// arrangement of busy rows may change the geometry.
func TestNoArrangementOfBusyRowsReflowsTheTable(t *testing.T) {
	m := derivedTable(t, 0) // content-auto: the column most at risk
	statuses(&m, [2]string{"api", "ok"}, [2]string{"web", "ok"}, [2]string{"job", "ok"})
	want := rowWidths(m.View())

	for _, arrangement := range [][]([2]string){
		{{"api", "running"}, {"web", "ok"}, {"job", "ok"}},
		{{"api", "running"}, {"web", "running"}, {"job", "running"}},
		{{"api", "ok"}, {"web", "ok"}, {"job", "running"}},
	} {
		statuses(&m, arrangement...)
		if got := rowWidths(m.View()); !equalInts(got, want) {
			t.Fatalf("widths %v, want %v — the indicator reflowed the table:\n%s",
				got, want, m.View())
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A column too narrow for the label draws the glyph alone rather than a
// half-word, and must not overflow the cell it was given.
func TestANarrowColumnKeepsTheGlyphAndDropsTheWord(t *testing.T) {
	// The label is uppercase and the row names are not, so "any character of
	// the label survived" is a question the rendered text can answer. Looking
	// for the whole word, or a few letters of it, is not enough: the failure
	// this guards against cuts the cell to its width, so what leaks through is
	// one or two characters, and the assertion has to be able to see those.
	const label = "WORKING"
	busy := func(o *table.Options) {
		o.ActivityWhen = func(c table.Row) (string, bool) { return activity.Busy(label)(c[1]) }
	}

	wide := derivedTable(t, 14, busy)
	statuses(&wide, [2]string{"api", label})
	if !strings.Contains(xansi.Strip(wide.View()), label) {
		t.Fatalf("the label does not render even where it fits:\n%s", wide.View())
	}

	narrow := derivedTable(t, 3, busy)
	statuses(&narrow, [2]string{"api", label})
	view := narrow.View()

	if !spinning(view) {
		t.Errorf("the glyph was dropped along with the label:\n%s", view)
	}
	// Scoped to the row that carries the indicator: the column headers and the
	// pane title hold letters of their own, so asking the whole view whether
	// any character of the label appears answers about "Name", not the cell.
	for _, line := range strings.Split(view, "\n") {
		if !spinning(line) {
			continue
		}
		if plain := xansi.Strip(line); strings.ContainsAny(plain, label) {
			t.Errorf("part of the label was drawn in a cell too narrow for it:\n%q", plain)
		}
	}

	// And the geometry: a cell that overflowed would widen the row past what
	// the same table renders with no indicator at all.
	idle := derivedTable(t, 3, busy)
	statuses(&idle, [2]string{"api", "ok"})
	if got, want := rowWidths(view), rowWidths(idle.View()); !equalInts(got, want) {
		t.Errorf("widths %v with an indicator, %v without", got, want)
	}
}

// Sorting moves rows; the indicator is held by key, so it must move with its
// own row rather than staying at the index it started on.
func TestSortingCarriesTheIndicatorWithItsRow(t *testing.T) {
	m := derivedTable(t, 12, func(o *table.Options) {
		o.Columns[0].Sortable = true
	})
	statuses(&m, [2]string{"api", "running"}, [2]string{"web", "ok"}, [2]string{"zzz", "ok"})
	m.SetSort(0, true) // descending: zzz, web, api
	m.SetRect(geom.New(0, 0, 60, 12))

	for _, line := range strings.Split(m.View(), "\n") {
		plain := xansi.Strip(line)
		if strings.Contains(plain, "api") && !spinning(line) {
			t.Errorf("the running row lost its spinner after a sort:\n%s", m.View())
		}
		if strings.Contains(plain, "web") && spinning(line) {
			t.Errorf("the spinner landed on a settled row after a sort:\n%s", m.View())
		}
	}
}

// Marks survive a filter because a key does not care whether its row is on
// screen (rule 32); activity is held the same way and must behave the same.
func TestAFilteredAwayBusyRowKeepsItsIndicator(t *testing.T) {
	m := derivedTable(t, 12, func(o *table.Options) { o.Filterable = true })
	statuses(&m, [2]string{"api", "running"}, [2]string{"web", "ok"})

	m.SetValue("web")
	m.SetRect(geom.New(0, 0, 60, 12))
	if spinning(m.View()) {
		t.Errorf("the filtered-out row's spinner is being drawn on a visible row:\n%s", m.View())
	}
	if m.ActivityCount() != 1 {
		t.Errorf("ActivityCount = %d, want the hidden row still counted", m.ActivityCount())
	}

	m.SetValue("")
	m.SetRect(geom.New(0, 0, 60, 12))
	if !spinning(m.View()) {
		t.Errorf("the indicator did not come back with its row:\n%s", m.View())
	}
}

// The mark gutter and the indicator occupy different space; neither may eat
// the other, and marking must not disturb what the status cell shows.
func TestTheMarkGutterAndTheIndicatorCoexist(t *testing.T) {
	m := derivedTable(t, 12, func(o *table.Options) { o.Markable = true })
	statuses(&m, [2]string{"api", "running"}, [2]string{"web", "ok"})
	before := rowWidths(m.View())

	m.SetMarks([]string{"api"})
	m.SetRect(geom.New(0, 0, 60, 12))

	view := m.View()
	if !spinning(view) {
		t.Errorf("marking the row took its indicator:\n%s", view)
	}
	if !strings.Contains(xansi.Strip(view), "✓") {
		t.Errorf("the mark is missing:\n%s", view)
	}
	if got := rowWidths(view); !equalInts(got, before) {
		t.Errorf("widths %v, want %v — marking reflowed a table with an indicator", got, before)
	}
}

// The predicate reads data, not presentation: a status the screen has coloured
// is the same status.
func TestAStyledStatusCellStillMatches(t *testing.T) {
	m := derivedTable(t, 12)
	m.SetKeyedRows([]table.KeyedRow{
		{Key: "api", Cells: []string{"api", ansi.CellColor(2, "running")}},
	})
	m.SetRect(geom.New(0, 0, 60, 12))

	if !spinning(m.View()) {
		t.Errorf("a coloured status cell did not match the predicate:\n%s", m.View())
	}
}

// Rule 19: the indicator must not close with a full reset, or it punches a
// hole in the selected row's background mid-cell.
func TestTheIndicatorDoesNotBreakTheSelectedRowBackground(t *testing.T) {
	m := derivedTable(t, 12)
	statuses(&m, [2]string{"api", "running"})
	m.SetCursor(0)
	m.SetRect(geom.New(0, 0, 60, 12))

	for _, line := range strings.Split(m.View(), "\n") {
		if !spinning(line) {
			continue
		}
		// The cell must close foreground-only. Scanning the line for any
		// \x1b[0m is too crude to assert that: the pane border emits one
		// before the row begins, and the row's own style legitimately closes
		// with one at the end. What matters is the reset following the label.
		if !strings.Contains(line, "running\x1b[39m") {
			t.Errorf("the indicator did not close foreground-only:\n%q", line)
		}
		if strings.Contains(line, "running\x1b[0m") {
			t.Errorf("a full reset inside the selected row ends its background:\n%q", line)
		}
		return
	}
	t.Fatalf("no row showed an indicator:\n%s", m.View())
}

// Setting only a predicate used to build derived state, animate a tick chain
// forever, and render nothing at all — invisible, and not free.
func TestAPredicateWithNoColumnStillDraws(t *testing.T) {
	o := theme.Dark().Table()
	o.Columns = []table.Column{{Title: "Name", Width: 14}, {Title: "Status", Width: 12}}
	o.ActivityWhen = func(c table.Row) (string, bool) {
		return activity.Busy("running")(c[1])
	}
	m := table.New(o)
	m.SetRect(geom.New(0, 0, 60, 12))
	m.SetKeyedRows([]table.KeyedRow{{Key: "api", Cells: []string{"api", "running"}}})
	m.SetRect(geom.New(0, 0, 60, 12))

	if !spinning(m.View()) {
		t.Errorf("a predicate with no ActivityColumn rendered nothing:\n%s", m.View())
	}
}

// Every row busy at once is the case where an off-by-one in the window or the
// key mapping shows up.
func TestEveryRowBusyAtOnce(t *testing.T) {
	m := derivedTable(t, 12)
	// Five rows, which this pane shows at once. A row scrolled out of view is
	// not a missing indicator, and a line scan cannot tell the two apart —
	// nor can it be asked whether a line is a data row, since the pane title
	// holds letters too.
	keys := []string{"alpha", "bravo", "charlie", "delta", "echo"}
	rows := make([][2]string, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, [2]string{k, "running"})
	}
	statuses(&m, rows...)

	if m.ActivityCount() != len(keys) {
		t.Fatalf("ActivityCount = %d, want %d", m.ActivityCount(), len(keys))
	}
	spun := 0
	for _, line := range strings.Split(m.View(), "\n") {
		if spinning(line) {
			spun++
		}
	}
	if spun != len(keys) {
		t.Errorf("%d rows spinning, want %d:\n%s", spun, len(keys), m.View())
	}
}

// Settled treats an empty value as settled: a blank cell is not work.
func TestABlankStatusIsNotBusy(t *testing.T) {
	o := theme.Dark().Table()
	o.Columns = []table.Column{{Title: "Name", Width: 14}, {Title: "Status", Width: 12}}
	o.ActivityColumn = "Status"
	o.ActivityWhen = func(c table.Row) (string, bool) {
		return activity.Settled("ok", "failed")(c[1])
	}
	m := table.New(o)
	m.SetRect(geom.New(0, 0, 60, 12))
	m.SetKeyedRows([]table.KeyedRow{{Key: "api", Cells: []string{"api", ""}}})
	m.SetRect(geom.New(0, 0, 60, 12))

	if spinning(m.View()) {
		t.Errorf("a blank status was treated as work in progress:\n%s", m.View())
	}
}

// A label wider than one cell per rune must be measured, not counted.
func TestAWideCharacterLabelIsMeasuredNotCounted(t *testing.T) {
	o := theme.Dark().Table()
	o.Columns = []table.Column{{Title: "Name", Width: 14}, {Title: "Status", Width: 10}}
	o.ActivityColumn = "Status"
	o.ActivityWhen = func(c table.Row) (string, bool) { return activity.Busy("同期中")(c[1]) }
	m := table.New(o)
	m.SetRect(geom.New(0, 0, 60, 12))
	m.SetKeyedRows([]table.KeyedRow{{Key: "api", Cells: []string{"api", "同期中"}}})
	m.SetRect(geom.New(0, 0, 60, 12))

	idle := derivedTable(t, 10)
	statuses(&idle, [2]string{"api", "ok"})
	if got, want := rowWidths(m.View()), rowWidths(idle.View()); !equalInts(got, want) {
		t.Errorf("widths %v with a wide-character label, %v without:\n%s", got, want, m.View())
	}
}

// The indicator is an overlay computed at render time and must never be
// written back into the rows the table holds.
//
// This is the invariant that makes the feature safe to reason about: derived
// state is a pure function of the data, so nothing the renderer does can feed
// back into what the next observation concludes. Substituting into the cells
// slice in place rather than a copy breaks it quietly — the row keeps spinning
// until the next swap, and then stops, because the predicate is now reading
// the spinner instead of the status.
func TestTheIndicatorNeverWritesBackIntoTheRows(t *testing.T) {
	m := derivedTable(t, 12)
	statuses(&m, [2]string{"api", "running"})

	for i := 0; i < 3; i++ {
		m.View()
	}

	row, ok := m.Selected()
	if !ok {
		t.Fatal("no selected row")
	}
	if row[1] != "running" {
		t.Errorf("the status cell is now %q; rendering mutated the data", row[1])
	}
	if m.ActivityCount() != 1 {
		t.Errorf("ActivityCount = %d after rendering, want 1", m.ActivityCount())
	}
}

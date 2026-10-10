package remote

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/source"
	"github.com/jsdrews/tuilib/pkg/table"
)

// Table is a table of Records bound to a remote source: the filter and sort
// are the source's, and scrolling is the pagination. Everything the
// component offers is promoted from the embedded Model.
type Table struct {
	table.Model
	d *driver[table.KeyedRow]
}

// NewTable builds a table over s. Seekable data is windowed (rows carry no
// keys there, so marking is inert); Anchored data is a span keyed by
// cursor. Start opts from theme.Table(); the remote filter and sort modes,
// and Anchored, are set here.
func NewTable(opts table.Options, s Shape[table.KeyedRow]) *Table {
	d := newDriver[table.KeyedRow](s, 0)
	opts.Anchored = d.anch != nil
	if opts.Filterable {
		opts.FilterMode = table.FilterRemote
	}
	opts.SortMode = table.SortRemote
	return &Table{Model: table.New(opts), d: d}
}

// Init requests the first page.
func (t *Table) Init() tea.Cmd { return t.d.init() }

// SetGrowing says whether rows are still arriving; while they are, the
// source polls.
func (t *Table) SetGrowing(b bool) tea.Cmd { return t.d.setGrowing(b) }

// SetAnchor moves an Anchored table to a, replacing its rows once a's
// first page arrives. It does nothing on Seekable data.
func (t *Table) SetAnchor(a source.Anchor) tea.Cmd {
	if t.d.anch == nil {
		return nil
	}
	t.Reanchor()
	return t.d.anch.SetAnchor(a)
}

// Refresh refetches what is on screen under the same query — a retry.
func (t *Table) Refresh() tea.Cmd { return t.d.refresh() }

// Update routes msg between the table and its source, runs fetches, and
// updates the table. Forward every message.
func (t *Table) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	switch m := msg.(type) {
	case table.ViewportChangedMsg:
		if t.d.seek != nil {
			cmds = append(cmds, t.d.seek.Viewport(m.FirstVisible, m.LastVisible))
		} else {
			cmds = append(cmds, t.d.anch.Viewport(m.FirstVisible, m.TotalRows-1-m.LastVisible))
		}
	case table.QueryChangedMsg:
		cmds = append(cmds, t.d.setQuery(Filter{Raw: m.Raw, Terms: m.Terms, Sort: m.Sort, Desc: m.Desc}))
	case source.RequestMsg:
		if m.Source == t.d.id() {
			older, newer := t.Edges()
			return t.d.fetch(m.Query, older, newer)
		}
	case result[table.KeyedRow]:
		if m.src == t.d.id() {
			cmds = append(cmds, t.apply(m))
		}
	}
	cmds = append(cmds, t.d.update(msg))
	var cmd tea.Cmd
	t.Model, cmd = t.Model.Update(msg)
	return tea.Batch(append(cmds, cmd)...)
}

func (t *Table) apply(r result[table.KeyedRow]) tea.Cmd {
	ok, cmd := t.d.deliver(r.page)
	if !ok {
		return nil
	}
	a := answerOf(r.q)
	switch {
	case r.q.Find || r.q.Probe:
		// Tables neither search remotely nor count unseen rows.
	case r.page.Err != nil:
		t.SetFailed(a)
	case t.d.seek != nil:
		rows := make([]table.Row, len(r.items))
		for i, kr := range r.items {
			rows[i] = kr.Cells
		}
		t.SetWindow(rows, r.page.Offset, r.page.Total, a)
	case r.q.Dir == source.Older:
		t.PrependRows(r.items, a)
	default:
		t.AppendRows(r.items, a)
	}
	if t.d.landsAt(r) {
		t.SetCursor(len(r.items) - 1)
	}
	if t.d.anch != nil {
		t.SetMore(t.d.anch.More())
	}
	return cmd
}

// Restyle swaps in opts — a new theme — keeping the rows, filter, sort
// (including one still staged), cursor and failure.
func (t *Table) Restyle(opts table.Options) {
	st := t.Model.State()
	opts.Anchored = t.d.anch != nil
	if opts.Filterable {
		opts.FilterMode = table.FilterRemote
	}
	opts.SortMode = table.SortRemote
	t.Model = table.New(opts)
	t.Model.Restore(st)
}

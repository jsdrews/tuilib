package remote

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/eventlog"
	"github.com/jsdrews/tuilib/pkg/source"
)

// Eventlog is an eventlog bound to a remote source. Everything the
// component offers is promoted from the embedded Model; Update, Init,
// SetGrowing, SetAnchor and Restyle are the binding's.
type Eventlog struct {
	eventlog.Model
	d *driver[eventlog.Item]
}

// NewEventlog builds an eventlog over s — Seekable (a job's numbered
// events) or Anchored (a log searched with search_after). Start opts from
// theme.Eventlog(); Anchored is set from s.
func NewEventlog(opts eventlog.Options, s Shape[eventlog.Item]) *Eventlog {
	d := newDriver[eventlog.Item](s, max(opts.MaxItems, eventlog.DefaultMaxItems))
	opts.Anchored = d.anch != nil
	e := &Eventlog{Model: eventlog.New(opts), d: d}
	if d.anch != nil && d.anch.Anchor().IsOldest() {
		e.SetFollow(false)
	}
	return e
}

// Init requests the first page.
func (e *Eventlog) Init() tea.Cmd { return e.d.init() }

// SetGrowing says whether the data is still gaining items — a job still
// running. While it is, the view follows and the source polls.
func (e *Eventlog) SetGrowing(b bool) tea.Cmd {
	e.Model.SetGrowing(b)
	return e.d.setGrowing(b)
}

// SetAnchor moves an Anchored view to a — a "jump to time" — replacing
// what it holds once a's first page arrives. It does nothing on Seekable
// data.
func (e *Eventlog) SetAnchor(a source.Anchor) tea.Cmd {
	if e.d.anch == nil {
		return nil
	}
	e.Reanchor()
	return e.d.anch.SetAnchor(a)
}

// Refresh refetches what is on screen under the same query — a retry.
func (e *Eventlog) Refresh() tea.Cmd { return e.d.refresh() }

// Update routes msg between the view and its source, runs fetches, and
// updates the view. Forward every message.
func (e *Eventlog) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	switch m := msg.(type) {
	case eventlog.ViewportChangedMsg:
		if e.d.seek != nil {
			e.d.seek.SetHeld(m.HeldStart, m.HeldCount)
			cmds = append(cmds, e.d.seek.Viewport(m.FirstVisible, m.LastVisible))
		} else {
			cmds = append(cmds, e.d.anch.Viewport(m.ToOlder, m.ToNewer))
		}
	case eventlog.QueryChangedMsg:
		cmds = append(cmds, e.d.setQuery(Filter{Raw: m.Raw, Terms: m.Terms}))
	case eventlog.FindMsg:
		if m.Token == e.FocusToken() {
			cmds = append(cmds, e.find(m))
		}
	case source.RequestMsg:
		if m.Source == e.d.id() {
			older, newer := e.Edges()
			return e.d.fetch(m.Query, older, newer)
		}
	case result[eventlog.Item]:
		if m.src == e.d.id() {
			cmds = append(cmds, e.apply(m))
		}
	}
	cmds = append(cmds, e.d.update(msg))
	var cmd tea.Cmd
	e.Model, cmd = e.Model.Update(msg)
	return tea.Batch(append(cmds, cmd)...)
}

func (e *Eventlog) find(m eventlog.FindMsg) tea.Cmd {
	if !e.d.canFind() {
		// Only what is loaded can be searched.
		if e.d.seek != nil {
			e.Found(false, 0)
		} else {
			e.FoundAt(false, "")
		}
		return nil
	}
	return e.d.find(m.Term, m.Newer, m.From, m.FromKey)
}

// apply turns a fetch's outcome into what the view shows.
func (e *Eventlog) apply(r result[eventlog.Item]) tea.Cmd {
	ok, cmd := e.d.deliver(r.page)
	if !ok {
		return nil
	}
	cmds := []tea.Cmd{cmd}
	a := answerOf(r.q)
	switch {
	case r.q.Find && e.d.seek != nil:
		e.Found(r.page.Found, r.page.Offset)
	case r.q.Find && r.page.Found:
		// Re-anchor on the hit: the span is replaced by it and its context.
		e.FoundAt(true, r.page.Cursor)
		e.Reanchor()
		cmds = append(cmds, e.d.anch.SetAnchor(source.At(r.page.Cursor)))
	case r.q.Find:
		e.FoundAt(false, "")
	case r.page.Err != nil:
		e.SetFailed(a)
	case r.q.Probe:
		e.SetNew(r.page.Count, r.page.More)
	case e.d.seek != nil:
		e.SetPage(r.items, r.page.Offset, r.page.Total, a)
	case r.q.Dir == source.Older:
		e.Prepend(r.items, a)
	default:
		e.Append(r.items, a)
	}
	if e.d.landsAt(r) {
		e.SetFollow(false)
	}
	if e.d.anch != nil {
		e.SetMore(e.d.anch.More())
	}
	return tea.Batch(cmds...)
}

// Restyle swaps in opts — a new theme — keeping everything the view holds:
// items, filter, search, cursor, follow and failure.
func (e *Eventlog) Restyle(opts eventlog.Options) {
	st := e.Model.State()
	opts.Anchored = e.d.anch != nil
	e.Model = eventlog.New(opts)
	e.Model.Restore(st)
}

// Package remote binds a component to a remote source, so a screen supplies
// only what varies between APIs — the fetch — and nothing of the routing
// between them.
//
// Without it, a screen showing paged remote data is the glue between two
// halves of one state machine: it forwards the component's viewport to the
// source, mirrors what the component holds back into it, turns each kind of
// reply into the right component call, lands search hits, keeps "growing"
// set on both, and replays all of it after a theme swap in the right order.
// Every one of those is an ordering rule a screen can get wrong silently.
// Here they live once:
//
//	s.log = remote.NewEventlog(t.Eventlog(), remote.Seekable[eventlog.Item]{
//	    Page: s.page,   // func(ctx, remote.Window) ([]eventlog.Item, total int, error)
//	    Find: s.find,   // optional: nil searches only what is loaded
//	})
//
//	func (s *Screen) Init() tea.Cmd { return s.log.Init() }
//	func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
//	    return s, s.log.Update(msg)
//	}
//	func (s *Screen) SetTheme(t theme.Theme) { s.log.Restyle(t.Eventlog()) }
//
// The fetch functions are ordinary synchronous Go: take a context and a
// typed request, return the items. They run off the UI goroutine, their
// context is cancelled when a newer request supersedes them, and a reply
// nobody is waiting for is dropped — none of which the function sees.
//
// Each shape of data (CLAUDE.md rule 34) has its own request type:
// Seekable data is asked for by Window (offset and limit), Anchored data by
// Edge (a direction and the cursor to walk from), and both can answer a
// Find. Growing data is one call: SetGrowing.
//
// The views embed their component, so everything else — Selected,
// IsActivate, focus, help, SetRect — is the component's own. pkg/source,
// pkg/table and pkg/eventlog stay public for anything this doesn't cover.
package remote

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/internal/remoteview"
	"github.com/jsdrews/tuilib/pkg/query"
	"github.com/jsdrews/tuilib/pkg/source"
)

// Filter is the committed query a request is for: the filter text as
// typed, parsed into terms (scoped terms carry their resolved column
// title), and the sort, when the view has one.
type Filter struct {
	Raw   string
	Terms []query.Term
	Sort  string
	Desc  bool
}

// Window asks Seekable data for Limit items starting at Offset.
type Window struct {
	Filter
	Offset, Limit int
}

// Edge asks Anchored data for up to Limit items walking from Cursor:
// newer ones when Newer, older ones otherwise, oldest first either way.
// An empty Cursor starts at the end being walked from — the newest item
// for an older walk, the oldest for a newer one. Inclusive includes the
// item at Cursor itself: the first request of a view anchored there.
type Edge struct {
	Filter
	Newer     bool
	Cursor    string
	Inclusive bool
	Limit     int
}

// Find asks for the first item matching Term beyond a position, within the
// Filter: after it when Newer, before it otherwise. Seekable data gives
// the position as From, and the key of the item there as Cursor — the id
// to search past for an API that searches by its own ids, since positions
// stop matching ids under a filter. Anchored data gives only Cursor.
type Find struct {
	Filter
	Term   string
	Newer  bool
	From   int
	Cursor string
}

// Shape is a source's shape: Seekable or Anchored.
type Shape[T any] interface{ shape() }

// Seekable is data whose item N can be fetched directly, with a total.
type Seekable[T any] struct {
	// Page returns the items of w and the total. For Growing data the
	// total is how many exist now.
	Page func(ctx context.Context, w Window) (items []T, total int, err error)
	// Find returns the position of the first match, and whether there was
	// one. Nil searches only what is loaded.
	Find func(ctx context.Context, f Find) (at int, found bool, err error)

	// PageSize, Follow, ViewportDelay and Context mean what they do on
	// source.Options.
	PageSize      int
	Follow        time.Duration
	ViewportDelay time.Duration
	Context       context.Context
}

// Anchored is data reachable only by walking from an anchor.
type Anchored[T any] struct {
	// Edge returns the items of e, oldest first, and whether that edge has
	// more beyond them. Each item's key is its cursor.
	Edge func(ctx context.Context, e Edge) (items []T, more bool, err error)
	// Find returns the cursor of the first match, and whether there was
	// one. The view re-anchors there. Nil searches only what is loaded.
	Find func(ctx context.Context, f Find) (cursor string, found bool, err error)

	PageSize      int
	Follow        time.Duration
	ViewportDelay time.Duration
	Context       context.Context
}

func (Seekable[T]) shape() {}
func (Anchored[T]) shape() {}

// result carries a fetch's outcome back to the view that asked.
type result[T any] struct {
	src   int64
	q     source.Query
	page  source.Page
	items []T
}

// driver owns the source and turns the shape's functions into requests.
type driver[T any] struct {
	seek    *source.Model
	anch    *source.Anchored
	page    func(context.Context, Window) ([]T, int, error)
	edge    func(context.Context, Edge) ([]T, bool, error)
	findAt  func(context.Context, Find) (int, bool, error)
	findKey func(context.Context, Find) (string, bool, error)
	growing bool
	// findFrom is the key of the item a Seekable find searches past.
	findFrom string
}

func newDriver[T any](s Shape[T], maxHeld int) *driver[T] {
	d := &driver[T]{}
	switch sh := s.(type) {
	case Seekable[T]:
		src := source.New(source.Options{
			PageSize: sh.PageSize, MaxHeld: maxHeld, Follow: sh.Follow,
			ViewportDelay: sh.ViewportDelay, Context: sh.Context,
		})
		d.seek, d.page, d.findAt = &src, sh.Page, sh.Find
	case Anchored[T]:
		src := source.NewAnchored(source.AnchoredOptions{
			PageSize: sh.PageSize, Follow: sh.Follow,
			ViewportDelay: sh.ViewportDelay, Context: sh.Context,
		})
		d.anch, d.edge, d.findKey = &src, sh.Edge, sh.Find
	}
	return d
}

func (d *driver[T]) id() int64 {
	if d.seek != nil {
		return d.seek.ID()
	}
	return d.anch.ID()
}

func (d *driver[T]) canFind() bool { return d.findAt != nil || d.findKey != nil }

func (d *driver[T]) init() tea.Cmd {
	if d.seek != nil {
		return d.seek.Init()
	}
	return d.anch.Init()
}

func (d *driver[T]) update(msg tea.Msg) tea.Cmd {
	if d.seek != nil {
		return d.seek.Update(msg)
	}
	return d.anch.Update(msg)
}

func (d *driver[T]) setQuery(f Filter) tea.Cmd {
	if d.seek != nil {
		return d.seek.SetQuery(f.Raw, f.Terms, f.Sort, f.Desc)
	}
	return d.anch.SetQuery(f.Raw, f.Terms, f.Sort, f.Desc)
}

func (d *driver[T]) setGrowing(b bool) tea.Cmd {
	d.growing = b
	if d.seek != nil {
		return d.seek.SetGrowing(b)
	}
	return d.anch.SetGrowing(b)
}

func (d *driver[T]) refresh() tea.Cmd {
	if d.seek != nil {
		return d.seek.Refresh()
	}
	return d.anch.Refresh()
}

func (d *driver[T]) deliver(p source.Page) (bool, tea.Cmd) {
	if d.seek != nil {
		return d.seek.Deliver(p)
	}
	return d.anch.Deliver(p)
}

func (d *driver[T]) find(term string, newer bool, from int, fromKey string) tea.Cmd {
	d.findFrom = fromKey
	dir := source.Older
	if newer {
		dir = source.Newer
	}
	if d.seek != nil {
		return d.seek.Find(term, dir, from)
	}
	return d.anch.Find(term, dir)
}

func filterOf(q source.Query) Filter {
	return Filter{Raw: q.Raw, Terms: q.Terms, Sort: q.Sort, Desc: q.Desc}
}

func answerOf(q source.Query) remoteview.Answer {
	return remoteview.Answer{Raw: q.Raw, Sort: q.Sort, Desc: q.Desc}
}

// fetch runs the shape's function for q off the UI goroutine. older and
// newer are the view's edge cursors, which an Anchored request walks from.
func (d *driver[T]) fetch(q source.Query, older, newer string) tea.Cmd {
	src, fromKey := d.id(), d.findFrom
	return func() tea.Msg {
		ctx, f := q.Ctx, filterOf(q)
		if ctx == nil {
			ctx = context.Background()
		}
		p := source.Page{Gen: q.Gen}
		var items []T
		newerDir := q.Dir == source.Newer
		edgeCursor := older
		if newerDir {
			edgeCursor = newer
		}
		switch {
		case q.Find && d.seek != nil:
			p.Offset, p.Found, p.Err = d.findAt(ctx, Find{Filter: f, Term: q.Term, Newer: newerDir, From: q.Offset, Cursor: fromKey})
		case q.Find:
			p.Cursor, p.Found, p.Err = d.findKey(ctx, Find{Filter: f, Term: q.Term, Newer: newerDir, Cursor: edgeCursor})
		case d.seek != nil:
			var total int
			items, total, p.Err = d.page(ctx, Window{Filter: f, Offset: q.Offset, Limit: q.Limit})
			p.Offset, p.Count, p.Total = q.Offset, len(items), total
		default:
			cursor := edgeCursor
			if q.FromAnchor {
				cursor = q.Cursor
			}
			items, p.More, p.Err = d.edge(ctx, Edge{Filter: f, Newer: newerDir, Cursor: cursor, Inclusive: q.Inclusive, Limit: q.Limit})
			p.Count = len(items)
		}
		return result[T]{src: src, q: q, page: p, items: items}
	}
}

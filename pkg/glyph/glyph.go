// Package glyph collects the single-character marks tuilib components draw —
// row cursors, expand arrows, scrollbar thumbs, rules, sort indicators — into
// one vocabulary so a theme can set them once and every component agrees.
//
// It exists for the same reason pkg/geom and pkg/query do: more than one
// component needs the identical values, and none of them should import
// another to get them. It imports nothing from tuilib.
//
// Glyphs are raw text, never pre-styled. Color is the caller's business —
// pkg/table wraps its separators via pkg/ansi so a selected row's background
// survives, while a list styles its cursor with lipgloss. Putting an escape
// in a Set here would take that choice away from the component drawing it.
//
// The zero Set means "use the defaults". Fill it through Resolve rather than
// reading fields off a Set you did not build, so a caller that set two fields
// does not get empty strings for the rest.
package glyph

// Set is the full glyph vocabulary. A component reads only the fields it
// draws; the rest are inert.
type Set struct {
	// Cursor marks the focused row in list, logview, and the action menu.
	Cursor string
	// Mark marks a selected row where multi-select is enabled.
	Mark string

	// ExpandOpen / ExpandClosed are the disclosure arrows in tree and
	// inspector.
	ExpandOpen   string
	ExpandClosed string

	// Rule is the horizontal line under an inline filter, drawn identically
	// by every filterable component.
	Rule string

	// ScrollThumb / ScrollTrack are the vertical scrollbar; HScrollThumb /
	// HScrollTrack the horizontal one.
	ScrollThumb  string
	ScrollTrack  string
	HScrollThumb string
	HScrollTrack string

	// SortAsc / SortDesc follow the active column's title in table.
	SortAsc  string
	SortDesc string

	// ColumnSep divides table columns.
	ColumnSep string
	// Placeholder fills a row a windowed table has not received yet.
	Placeholder string
}

// Default is the library's glyph vocabulary — what every component drew
// before Set existed, so an app that sets nothing sees no change.
func Default() Set {
	return Set{
		Cursor:       "▸",
		Mark:         "✓",
		ExpandOpen:   "▾",
		ExpandClosed: "▸",
		Rule:         "─",
		ScrollThumb:  "█",
		ScrollTrack:  "░",
		HScrollThumb: "━",
		HScrollTrack: "─",
		SortAsc:      "▲",
		SortDesc:     "▼",
		ColumnSep:    "│",
		Placeholder:  "·",
	}
}

// Resolve fills every empty field of s from Default. Partial sets are the
// common case — an app overriding one arrow should not have to restate the
// other twelve.
func (s Set) Resolve() Set {
	d := Default()
	fields := []struct {
		dst *string
		def string
	}{
		{&s.Cursor, d.Cursor},
		{&s.Mark, d.Mark},
		{&s.ExpandOpen, d.ExpandOpen},
		{&s.ExpandClosed, d.ExpandClosed},
		{&s.Rule, d.Rule},
		{&s.ScrollThumb, d.ScrollThumb},
		{&s.ScrollTrack, d.ScrollTrack},
		{&s.HScrollThumb, d.HScrollThumb},
		{&s.HScrollTrack, d.HScrollTrack},
		{&s.SortAsc, d.SortAsc},
		{&s.SortDesc, d.SortDesc},
		{&s.ColumnSep, d.ColumnSep},
		{&s.Placeholder, d.Placeholder},
	}
	for _, f := range fields {
		if *f.dst == "" {
			*f.dst = f.def
		}
	}
	return s
}

# tuilib

A Go library for building terminal UIs over remote systems: themed components,
a screen stack, and an app shell that owns the chrome around them.

## Language

### Row activity

**Activity**:
The indicator a row shows while work against that specific row is in progress — a spinner and a label drawn over the row, never written into it. A rendering of data the component already holds, not data of its own.
_Avoid_: progress, in-flight state, status

**Key**:
A row's stable identity across refreshes — a keyed item or row's key, or a tree node's path. Activity, marks and cursor restoration are all held by key, never by position.
_Avoid_: index, id

**Observation**:
One read of the data, taken as the whole truth about which keys are busy at that moment. Each observation replaces the previous one outright; nothing accumulates across them.
_Avoid_: poll, refresh, snapshot

**Busy**:
A key whose last observation says work is in progress on it.
_Avoid_: in flight, running, active

**Settled**:
A key whose last observation says nothing is in progress on it. Everything not busy is settled.
_Avoid_: resting, idle, done

**Label**:
The word an active row shows beside its spinner. For a busy key it is the server's own value ("Syncing"); for a claimed key it is the screen's guess until an observation replaces it.

**Claim**:
The screen's statement that it has asked the server to work on a key which no observation has yet reported busy. It exists only to cover the gap between a keypress and the next observation, and it is never a second opinion about the same question — the data always outranks it.
_Avoid_: expected entry, local activity, handoff, pending

**Confirm** (a claim):
An observation reports the claimed key busy; the claim is subsumed by real activity.

**Retire** (a claim):
An observation ends a claim without confirming it — because the key settled at a different value than it had when claimed, or because the claim ran out of uninformative observations to wait through.

**Retract** (a claim):
The screen withdraws a claim itself, because the request was refused or observations have stopped arriving.

**Scope**:
The keys a component currently holds. Activity for a key outside scope is kept but not shown, so a row on another page lights up when it scrolls into view.

### Neighbours activity is not

**Loading**:
A whole component having no data yet; its body is replaced. Activity is per-row and leaves the body intact.

**Mark**:
The user's selection of rows. Activity is the system's state about rows. Both are held by key, so marking rows and then acting on them makes exactly those rows active.
_Avoid_: select (as a noun for the set)

### Remote queries

**Committed query**:
The filter and sort the user last committed for a remote source to answer. Filter text commits on enter, esc or blur; a sort commits once sort input goes quiet.
_Avoid_: pending query, current query, in-flight query

**Answered query**:
The query the rows on screen are the source's answer to. It trails the committed query while the source is still answering.
_Avoid_: showing query, displayed query

**Staged sort**:
A sort the user has moved to but not yet committed. It is the only way the sort a table shows can differ from its committed query.
_Avoid_: pending sort, draft sort

**Stale**:
Rows whose answered query differs from the committed query. They stay on screen, drawn dimmed, until the committed query's answer arrives. A state of the rows, not of the component.
_Avoid_: loading (which replaces the body), reloading, outdated

**Failed query**:
A committed query whose fetch failed. The rows stay stale, still answering the previous query, and the user's input is kept so the same query can be retried or edited.
_Avoid_: error state, reverted query

**Query history**:
The console's record of what happened to each committed query: answered (with how long it took), failed, or cancelled. Routine page fetches are left out; failed ones are not.
_Avoid_: request log, fetch log

### Remote data shapes

**Record**:
An item with fields that the user compares across items — a flow run, an application.
_Avoid_: row (a rendering, not the data), entity

**Event**:
An item with fields that sits in a timeline the user reads in order — a job event, a log document.
_Avoid_: entry, message

**Line**:
Raw text read in order, with no fields the user works with.
_Avoid_: log entry, message

**Seekable**:
Data whose item N can be fetched directly, with a total that is known or grows.
_Avoid_: paginated, offset-based

**Anchored**:
Data reachable only by walking forwards or backwards from an anchor — the newest item, a time, or a given item — with no reliable total.
_Avoid_: cursor-based, keyset

**Streamed**:
Data offered only as its tail and whatever arrives after it.
_Avoid_: live, tailing

**Growing**:
Data that gains items at its end while the user watches.
_Avoid_: live, streaming

**Anchor**:
Where a view of Anchored data begins — its newest item, its oldest, or a given item.
_Avoid_: cursor (a cursor marks an edge), seek point

**Span**:
The contiguous items a component holds of Anchored data, grown or trimmed at either edge. Unlike a window, it has no logical offsets.
_Avoid_: run (a flow or job run), window, buffer

**Edge**:
Either end of a span, which either has more beyond it or does not.
_Avoid_: boundary, end

**Eventlog**:
The component for Events and Lines read in order from Seekable or Anchored data: it holds a window or a span of items, each drawn as its lines, with a cursor on one item. Streamed data stays in a logview.
_Avoid_: windowed logview, paged log, timeline

**Follow**:
Keeping the view pinned to the newest item of Growing data, fetching what arrives. On at the newest item, off as soon as the user moves away; the view never moves by itself.
_Avoid_: tail, live mode, auto-scroll

**Probe**:
A follow poll made while the user is not following: it learns how much is new without fetching it into view.
_Avoid_: peek, count query

### Surfaces

**Surface**:
How a component's edge and body are drawn: whether it has a line, a fill, a header band or a shadow. Independent of the palette, so any palette can be drawn with any surface.
_Avoid_: theme (a palette plus chrome), style (a lipgloss.Style), skin, card style

**Outlined**:
The surface of a line border around an unfilled body. It is today's look.

**Filled**:
The surface of a solid fill whose half-block rows are its own edge, with no line.

**Banded**:
The surface of a fill under a header band that carries the title, where focus colours the band.

**Elevated**:
The surface of a fill with a drop shadow below and to the right.

**Accented**:
The surface of an unfilled body with a one-cell bar down its left edge, coloured when focused.

**Palette**:
The colours an app is drawn in: the chrome colours, the `Background` and `Foreground` seeds, and any surface-role overrides. One of two independent axes the user picks, the other being the surface.
_Avoid_: theme, when only the colours are meant

**Theme**:
What components are built from: the current palette and surface together with the glyphs. The shell assembles it and hands it to `SetTheme`; nobody picks a theme directly.
_Avoid_: palette, skin

**Screen background**:
The tone painted behind and between surfaces, so that a filled surface reads as raised against it.
_Avoid_: terminal background (the terminal's own default, which no filled surface relies on)

**Edge**:
What a surface draws on one side of its rect: glyphs, the background each glyph sits on, and the cells it takes. A shadow is edge too, because a surface never draws outside its rect.
_Avoid_: border (only Outlined's edge is a line), frame, inset

**Role**:
A named colour slot a surface refers to (`Screen`, `Card`, `Band`, `Shadow`, …), resolved against the palette when the theme builds a component. A surface never holds a literal colour.
_Avoid_: token, when the surface's view is meant

**Text row**:
The top or bottom row of a surface's edge that carries the pane's slots: the title, counters, hints and errors. Every surface has both.
_Avoid_: border line, footer

**Divider**:
The row the pane draws under a pinned header, as the surface describes it: a line on Outlined, possibly a tone step on a filled surface.
_Avoid_: rule, separator

**Seed**:
A palette colour that surface roles are derived from: `Background` (which the screen is) and `Foreground` (which body text on a filled surface is). Every other surface role is derived from these unless the palette overrides it.
_Avoid_: base colour, primary

**Margin**:
The part of a surface's edge, on its right and bottom, that separates it from its neighbours. It is drawn on the parent's background, so whatever contains the surface paints it. Outlined has none.
_Avoid_: gutter (the visible gap, which margins produce), gap, padding

**Field form**:
The surface a single-line component (input, filter, toggle) is drawn in, taking at most one cell of edge per side so it fits a three-row slot. An outlined field unless the surface says otherwise.
_Avoid_: compact surface, input style

**Fallback**:
The surface a surface becomes when the terminal can't meet what it needs (colour depth, block glyphs), followed as a chain that always ends at Outlined.
_Avoid_: degrade mode, compatibility mode

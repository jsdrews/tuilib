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

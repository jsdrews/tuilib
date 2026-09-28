package activity

import (
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Operations: work the user started, and the one decision a screen makes
// about it — who knows when it ends.
//
// Dispatch opens the claim, Done closes the request, and BeginRead / Accept
// put every read on one clock with the acknowledgements, so the library rather
// than the screen decides whether a read can speak to a claim.

// Mode is who knows when an operation's work ends.
type Mode int

const (
	// Observed: the server's status does. The request is only an
	// acknowledgement — Argo's sync POST answers at once and the controller
	// does the work — so the claim covers the gap until a read reports the
	// key busy, and ends when reads taken after the acknowledgement say the
	// work is over or run out of allowance (Options.Settle).
	Observed Mode = iota

	// Held: the request does. Argo's refresh GET holds the connection until
	// the refresh is done and shows nothing on the app while it runs; a job
	// handle is the same, with the screen calling Done when the job resource
	// says it finished. Observations cannot end a Held claim — an open
	// request is the server not having answered yet — though a read that
	// reports the key busy still shows the server's own label.
	Held
)

// Op is one dispatched operation. It is a small value meant to ride the
// screen's own messages from Dispatch to Done.
type Op struct {
	id   uint64
	Keys []string
	Mode Mode
}

// ID identifies the operation; UnobservedMsg carries it.
func (o Op) ID() uint64 { return o.id }

// opSeq is package-wide so that two components on one screen can never mistake
// each other's operations.
var opSeq atomic.Uint64

// UnobservedMsg reports an Observed operation's keys that ended without any
// read ever reporting them busy.
//
// Label is the claim's label as passed to Dispatch — usually the server's
// word for work in progress, "Running" — not the name of the verb, so
// m.Label+" finished" reads "Running finished". Name the verb yourself, from
// Op if the screen has several.
//
// The work finished between two reads, was a no-op, or never started — from
// the rows alone these are the same, which is why this is a message and not
// an indicator.
//
// It arrives through the component's Update on the turn after the read that
// ended the claim — ApplyRead and SetKeyedRows are setters and return nothing
// — so a screen that forwards every message to its component (rule 6) and
// matches this in its own Update receives it. Changed separates the first from the other two when the
// status moved while nobody was looking. Without this the row simply returns
// to what it said before, and the user concludes the action was ignored.
type UnobservedMsg struct {
	Op      uint64
	Keys    []string
	Label   string
	Changed bool
}

// Dispatch opens a claim on keys for work the screen is about to request.
//
// Each key's status as of the last observation is recorded with the claim; a
// later read reporting a different one is how the Set notices the server
// acted. The returned command is the animation's first tick.
func (s *Set) Dispatch(keys []string, label string, mode Mode) (Op, tea.Cmd) {
	op := Op{id: opSeq.Add(1), Keys: append([]string(nil), keys...), Mode: mode}
	if s.expected == nil {
		s.expected = map[string]claim{}
	}
	now := time.Now()
	for _, k := range keys {
		if k == "" {
			continue
		}
		s.expected[k] = claim{
			st:    State{Label: label, Since: now},
			at:    s.statuses[k],
			spare: s.settle,
			op:    op.id,
			mode:  mode,
		}
	}
	return op, s.armTick()
}

// Outcome is what Done did to an operation — and so what the screen may tell
// the user when the reply arrives.
//
// Done means different things by Mode, which is why it says which: after an
// Observed request answers, the work has only been accepted and is still
// running; after a Held one, it is over. A screen with both kinds of verb that
// writes "completed" on every reply is wrong for half of them, and nothing
// else would tell it so.
type Outcome int

const (
	// Acknowledged: an Observed request answered. The server has the work and
	// the row keeps spinning until reads say it finished. Say "requested",
	// never "completed".
	Acknowledged Outcome = iota

	// Ended: a Held request answered, so the work is over. "Completed" is
	// true now.
	Ended

	// Withdrawn: the request failed, so nothing is coming and the claim is
	// gone. Report the error.
	Withdrawn
)

func (o Outcome) String() string {
	switch o {
	case Acknowledged:
		return "acknowledged"
	case Ended:
		return "ended"
	default:
		return "withdrawn"
	}
}

// Done reports that op's request answered, and returns what that means.
//
// An error withdraws the claim: the server refused, so nothing is coming. A
// Held operation ends here. An Observed one is acknowledged, and from now on
// reads taken after this moment are entitled to end it.
//
// Only the claims op itself made are touched, so a key re-dispatched since is
// left to its newer operation.
func (s *Set) Done(op Op, err error) Outcome {
	s.clock++
	for _, k := range op.Keys {
		c, ok := s.expected[k]
		if !ok || c.op != op.id {
			continue
		}
		if err != nil || c.mode == Held {
			delete(s.expected, k)
			continue
		}
		c.acked, c.ackAt = true, s.clock
		s.expected[k] = c
	}
	switch {
	case err != nil:
		return Withdrawn
	case op.Mode == Held:
		return Ended
	default:
		return Acknowledged
	}
}

// Read is a token for one request/response read, taken when it is issued.
type Read struct{ at uint64 }

// BeginRead stamps a read at the moment it is issued.
//
// What a read can say is fixed when it is issued, not when it lands, so this
// is the point that is compared against acknowledgements and against other
// reads. A stream has no such point — its events are taken as they arrive, in
// order — and needs no token.
func (s *Set) BeginRead() Read {
	s.clock++
	return Read{at: s.clock}
}

// Accept decides whether a read that has landed may be applied, and must be
// called before its rows are pushed.
//
// It refuses a read overtaken by a newer one already applied. It refuses a
// failed read too, and withdraws every acknowledged Observed claim when it
// does: those are waiting on an observation, and a read that failed is the
// case where none is coming. Held claims and claims whose request is still in
// flight are left alone, since their own request will end them.
//
// On true, the next observation is taken as of rd. On a failure, rows busy only
// because an earlier read said so turn Unknown until the next observation.
func (s *Set) Accept(rd Read, err error) bool {
	if rd.at < s.applied {
		return false
	}
	if err != nil {
		for k, c := range s.expected {
			if c.op != 0 && c.mode == Observed && c.acked {
				delete(s.expected, k)
			}
		}
		// Keys the last good read reported busy stay, drawn Unknown: clearing
		// them would claim a read nobody took.
		s.failing = true
		return false
	}
	s.applied = rd.at
	s.obsAt, s.fromRd = rd.at, true
	return true
}

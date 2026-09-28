// The claim contract, asserted once across list, table and tree.
//
// A claim is what Expect puts on a row between the keypress and the
// observation that reports it. Everything here is about how it ends, because
// that is the only part with a way to go wrong: a claim that outlives the data
// is the feature asserting something the server never said.
//
// Shared behaviour lives here rather than in whichever component was edited
// last — see the header of activity_test.go for why that rule exists.
package componenttest

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// claimable is one component that can be told about work before it can see it.
type claimable interface {
	observe(status map[string]string)
	view() string
	state(key string) (activity.State, bool)
	count() int

	expect(keys []string, label string)
	retract(keys ...string)
	retractAll()
}

// --- the three components, configured with an allowance ------------------

// Every component speaks the operation API. An acknowledged Observed
// dispatch is the claim this contract is about — ended by the observations
// that follow — and one operation per key lets a single key be refused on its
// own.
type listClaim struct {
	listDerive
	ops map[string]activity.Op
}

func newListClaim(settle int) claimable {
	o := theme.Dark().List()
	o.BusyWhen = listBusyWhen
	o.Activity.Settle = settle
	d := &listClaim{ops: map[string]activity.Op{}}
	d.m = list.New(o)
	d.observe(statusOf(nil))
	return d
}

func (d *listClaim) expect(keys []string, label string) {
	for _, k := range keys {
		d.ops[k] = dispatched(&d.m, k, label)
	}
}
func (d *listClaim) retract(keys ...string) {
	for _, k := range keys {
		d.m.Done(d.ops[k], errRefused)
	}
}
func (d *listClaim) retractAll() { d.m.ApplyRead(d.m.BeginRead(), nil, errRefused) }

func newTableClaim(settle int) claimable {
	o := theme.Dark().Table()
	o.ActivityColumn = "Status"
	o.Columns = []table.Column{{Title: "Name", Width: 16}, {Title: "Status", Width: 14}}
	o.Activity.Settle = settle
	o.BusyWhen = func(r table.KeyedRow) (string, bool) {
		return activity.Busy("running", "pending")(r.Cells[1])
	}
	d := &tableClaim{ops: map[string]activity.Op{}}
	d.m = table.New(o)
	d.observe(statusOf(nil))
	return d
}

// The table speaks the operation API, which has no Expect. An acknowledged
// Observed dispatch is the same claim — ended by the observations that follow
// — so the contract applies unchanged. One operation per key, so a single key
// can be refused on its own the way Retract did it.
type tableClaim struct {
	tableDerive
	ops map[string]activity.Op
}

func (d *tableClaim) expect(keys []string, label string) {
	for _, k := range keys {
		d.ops[k] = dispatched(&d.m, k, label)
	}
}
func (d *tableClaim) retract(keys ...string) {
	for _, k := range keys {
		d.m.Done(d.ops[k], errRefused)
	}
}
func (d *tableClaim) retractAll() { d.m.ApplyRead(d.m.BeginRead(), nil, errRefused) }

// dispatcher is the operation surface all three components share.
type dispatcher interface {
	Dispatch(keys []string, label string, mode activity.Mode) (activity.Op, tea.Cmd)
	Done(op activity.Op, err error) activity.Outcome
}

// dispatched is an acknowledged Observed claim on one key.
func dispatched(m dispatcher, key, label string) activity.Op {
	op, _ := m.Dispatch([]string{key}, label, activity.Observed)
	m.Done(op, nil)
	return op
}

var errRefused = errors.New("refused")

type treeClaim struct {
	treeDerive
	ops map[string]activity.Op
}

func newTreeClaim(settle int) claimable {
	o := theme.Dark().Tree()
	o.InitialDepth = 2
	o.Root = derivedTree(statusOf(nil))
	o.Activity.Settle = settle
	o.BusyWhen = treeBusyWhen
	d := &treeClaim{ops: map[string]activity.Op{}}
	d.m = tree.New(o)
	d.setRect(placed())
	return d
}

// A tree addresses nodes by path; translate at the boundary the way treeDerive
// does for reads.
func (d *treeClaim) expect(keys []string, label string) {
	for _, k := range keys {
		d.ops[k] = dispatched(&d.m, d.path(k), label)
	}
}
func (d *treeClaim) retract(keys ...string) {
	for _, k := range keys {
		d.m.Done(d.ops[k], errRefused)
	}
}
func (d *treeClaim) retractAll() { d.m.ApplyRead(d.m.BeginRead(), nil, errRefused) }
func (d *treeClaim) paths(keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = d.path(k)
	}
	return out
}

func eachClaimable(t *testing.T, settle int, fn func(t *testing.T, c claimable)) {
	t.Helper()
	for name, mk := range map[string]func(int) claimable{
		"list":  newListClaim,
		"table": newTableClaim,
		"tree":  newTreeClaim,
	} {
		t.Run(name, func(t *testing.T) { fn(t, mk(settle)) })
	}
}

// --- the window the claim exists for -------------------------------------

// Press the verb and the row reacts, with nothing from the server yet. This is
// the whole point: for up to a poll interval the data cannot say anything, and
// without this the row the verb was about reads exactly as it did before.
func TestAClaimShowsBeforeAnyObservation(t *testing.T) {
	eachClaimable(t, 0, func(t *testing.T, c claimable) {
		c.expect([]string{"web"}, "Syncing")
		st, ok := c.state("web")
		if !ok {
			t.Fatal("no state for a key just claimed")
		}
		if st.Label != "Syncing" {
			t.Errorf("label = %q, want the claimed %q", st.Label, "Syncing")
		}
		if !strings.Contains(c.view(), "Syncing") {
			t.Error("the claim is not on screen")
		}
	})
}

// The ordinary ending: the server catches up and says the same thing. The row
// must not blink — the claim is replaced by the observation in one step, and
// the label becomes the server's own word rather than the guess.
func TestAnObservationConfirmingTheClaimHasNoSeam(t *testing.T) {
	eachClaimable(t, 0, func(t *testing.T, c claimable) {
		c.expect([]string{"web"}, "Syncing")
		c.observe(statusOf(map[string]string{"web": "running"}))

		st, ok := c.state("web")
		if !ok {
			t.Fatal("the row stopped working across the handoff")
		}
		if st.Label != "running" {
			t.Errorf("label = %q, want the observed %q — the data's word beats the guess", st.Label, "running")
		}
	})
}

// The other ordinary ending, and the reason no timer is needed: an observation
// that has nothing new to say retires the claim on its own.
func TestAnUninformativeObservationRetiresAClaimAtZeroSettle(t *testing.T) {
	eachClaimable(t, 0, func(t *testing.T, c claimable) {
		c.expect([]string{"web"}, "Syncing")
		c.observe(statusOf(nil)) // still "successful": nothing has changed

		if _, ok := c.state("web"); ok {
			t.Error("the claim survived an observation that said nothing new")
		}
	})
}

// A reconciler accepts the request and keeps reporting the old value for a
// while. The allowance is what carries the row across that, and it is spent in
// observations rather than seconds because no duration here is knowable.
func TestSettleSpendsOnlyUninformativeObservations(t *testing.T) {
	eachClaimable(t, 2, func(t *testing.T, c claimable) {
		c.expect([]string{"web"}, "Syncing")
		for i := range 2 {
			c.observe(statusOf(nil))
			if _, ok := c.state("web"); !ok {
				t.Fatalf("claim died on uninformative observation %d of its allowance of 2", i+1)
			}
		}
		c.observe(statusOf(nil))
		if _, ok := c.state("web"); ok {
			t.Error("claim outlived its allowance")
		}
	})
}

// The allowance is a ceiling, not a floor. ?blackhole=1 — a request accepted,
// given an id and never acted on — is indistinguishable from one still
// reconciling, so the only thing that can end it is the bound.
func TestAnUnconfirmedClaimAlwaysEnds(t *testing.T) {
	eachClaimable(t, 3, func(t *testing.T, c claimable) {
		c.expect([]string{"web"}, "Syncing")
		for range 12 {
			c.observe(statusOf(nil))
		}
		if _, ok := c.state("web"); ok {
			t.Error("a claim nothing ever confirmed is still spinning")
		}
	})
}

// The third ending, and the one that makes a small allowance enough: the key
// is settled at a value it did not hold when the claim was made, so the server
// demonstrably acted and there is nothing left to wait for.
//
// All three, the tree included: the value compared is the status BusyWhen
// reports, not the node's label, so a tree can see it change too.
func TestAChangedSettledValueRetiresAClaimWhateverTheAllowance(t *testing.T) {
	for name, mk := range map[string]func(int) claimable{
		"list":  newListClaim,
		"table": newTableClaim,
		"tree":  newTreeClaim,
	} {
		t.Run(name, func(t *testing.T) {
			c := mk(9)
			c.expect([]string{"web"}, "Syncing")
			c.observe(statusOf(map[string]string{"web": "failed"}))

			if _, ok := c.state("web"); ok {
				t.Error("claim held past an observation proving the server had acted")
			}
		})
	}
}

// Claims are per key, and an observation retires only what it can speak to.
func TestClaimsAreIndependent(t *testing.T) {
	eachClaimable(t, 4, func(t *testing.T, c claimable) {
		c.expect([]string{"web"}, "Syncing")
		c.expect([]string{"api"}, "Syncing")
		c.observe(statusOf(map[string]string{"web": "running"}))

		if _, ok := c.state("web"); !ok {
			t.Fatal("web stopped working across the handoff")
		}
		if st, _ := c.state("web"); st.Label != "running" {
			t.Errorf("web label = %q, want the observed %q — its claim should be gone", st.Label, "running")
		}
		if st, ok := c.state("api"); !ok || st.Label != "Syncing" {
			t.Error("api's claim went with web's; nothing observed said anything about api")
		}
	})
}

// --- taking it back ------------------------------------------------------

// A write the server refused. Politeness — the next observation would retire
// it anyway — so it has to leave the siblings alone.
func TestRetractClearsOneClaimAndLeavesItsSiblings(t *testing.T) {
	eachClaimable(t, 5, func(t *testing.T, c claimable) {
		c.expect([]string{"web", "api"}, "Syncing")
		c.retract("web")

		if _, ok := c.state("web"); ok {
			t.Error("retracted claim is still there")
		}
		if _, ok := c.state("api"); !ok {
			t.Error("retracting one claim took its sibling with it")
		}
	})
}

// A read that failed. Not politeness: an outage is exactly the case where no
// observation is coming to retire anything, so a claim held through it asserts
// something nothing can support for as long as the outage lasts.
func TestRetractAllClearsEveryClaim(t *testing.T) {
	eachClaimable(t, 5, func(t *testing.T, c claimable) {
		c.expect([]string{"web", "api", "worker"}, "Syncing")
		c.retractAll()

		for _, k := range derivedKeys {
			if _, ok := c.state(k); ok {
				t.Errorf("%s still claimed after the reads stopped", k)
			}
		}
	})
}

// --- the theme-swap carry (rule 4) ---------------------------------------

// A claim is state like any other, and the swap happens under the user's hands
// — dropping it would blank the row they just acted on.
func TestAClaimSurvivesAThemeRebuild(t *testing.T) {
	eachDerivable(t, func(t *testing.T, d derivable) {
		claim(t, d, "web")

		fresh := d.rebuilt()
		fresh.adopt(d.activityState())
		if _, ok := fresh.state("web"); !ok {
			t.Error("the claim did not survive the rebuild")
		}
	})
}

// claim calls Expect through whichever concrete component d is.
func claim(t *testing.T, d derivable, key string) {
	t.Helper()
	switch c := d.(type) {
	case *listDerive:
		dispatched(&c.m, key, "Syncing")
	case *tableDerive:
		dispatched(&c.m, key, "Syncing")
	case *treeDerive:
		dispatched(&c.m, c.path(key), "Syncing")
	default:
		t.Fatalf("no Expect for %T", d)
	}
}

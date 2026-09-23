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
	"strings"
	"testing"

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

type listClaim struct{ listDerive }

func newListClaim(settle int) claimable {
	o := theme.Dark().List()
	o.ActivityWhen = lastField("running", "pending")
	o.Activity.Settle = settle
	d := &listClaim{}
	d.m = list.New(o)
	d.observe(statusOf(nil))
	return d
}

func (d *listClaim) expect(keys []string, label string) { d.m.Expect(keys, label) }
func (d *listClaim) retract(keys ...string)             { d.m.Retract(keys...) }
func (d *listClaim) retractAll()                        { d.m.RetractAll() }

type tableClaim struct{ tableDerive }

func newTableClaim(settle int) claimable {
	o := theme.Dark().Table()
	o.ActivityColumn = "Status"
	o.Columns = []table.Column{{Title: "Name", Width: 16}, {Title: "Status", Width: 14}}
	o.Activity.Settle = settle
	o.ActivityWhen = func(c table.Row) (string, bool) {
		return activity.Busy("running", "pending")(c[1])
	}
	d := &tableClaim{}
	d.m = table.New(o)
	d.observe(statusOf(nil))
	return d
}

func (d *tableClaim) expect(keys []string, label string) { d.m.Expect(keys, label) }
func (d *tableClaim) retract(keys ...string)             { d.m.Retract(keys...) }
func (d *tableClaim) retractAll()                        { d.m.RetractAll() }

type treeClaim struct{ treeDerive }

func newTreeClaim(settle int) claimable {
	o := theme.Dark().Tree()
	o.InitialDepth = 2
	o.Root = derivedTree(statusOf(nil))
	o.Activity.Settle = settle
	o.ActivityWhen = func(n tree.Node) (string, bool) {
		sn, ok := n.(statusNode)
		if !ok {
			return "", false
		}
		return activity.Busy("running", "pending")(sn.status)
	}
	d := &treeClaim{}
	d.m = tree.New(o)
	d.setRect(placed())
	return d
}

// A tree addresses nodes by path; translate at the boundary the way treeDerive
// does for reads.
func (d *treeClaim) expect(keys []string, label string) {
	d.m.Expect(d.paths(keys), label)
}
func (d *treeClaim) retract(keys ...string) { d.m.Retract(d.paths(keys)...) }
func (d *treeClaim) retractAll()            { d.m.RetractAll() }
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
// List and table only. A tree has no value to compare — a node's label is its
// identity, so a label that changed is a different node rather than the same
// one reporting something new. See the tree case below.
func TestAChangedSettledValueRetiresAClaimWhateverTheAllowance(t *testing.T) {
	for name, mk := range map[string]func(int) claimable{
		"list":  newListClaim,
		"table": newTableClaim,
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

// The tree's exception, asserted rather than assumed — a reader finding two
// components with a third ending and one without should find out here that it
// is a consequence of identity rather than an omission.
func TestATreeClaimEndsOnlyByConfirmationOrAllowance(t *testing.T) {
	c := newTreeClaim(9)
	c.expect([]string{"web"}, "Syncing")
	c.observe(statusOf(map[string]string{"web": "failed"}))

	if _, ok := c.state("web"); !ok {
		t.Error("a tree retired a claim on a value change it cannot actually observe")
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
		c.m.Expect([]string{key}, "Syncing")
	case *tableDerive:
		c.m.Expect([]string{key}, "Syncing")
	case *treeDerive:
		c.m.Expect([]string{c.path(key)}, "Syncing")
	default:
		t.Fatalf("no Expect for %T", d)
	}
}

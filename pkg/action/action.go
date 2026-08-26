// Package action is a screen's verbs: the named things a user can do to
// whatever is currently selected, declared as data rather than wired up as
// keypresses.
//
// A screen that has verbs implements Provider, returning a Set of the actions
// that apply right now. The Menu component renders that Set as a bordered
// picker anchored where the user asked for it, and resolves to a ChosenMsg —
// the same shape pkg/confirm and pkg/alert use, so hosting one is the
// familiar pattern:
//
//	func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
//	    if s.menuUp {
//	        switch m := msg.(type) {
//	        case action.ChosenMsg:
//	            s.menuUp = false
//	            return s, s.run(m.Action)
//	        case action.CancelledMsg:
//	            s.menuUp = false
//	            return s, nil
//	        }
//	        var cmd tea.Cmd
//	        s.menu, cmd = s.menu.Update(msg)
//	        return s, cmd
//	    }
//	    …
//	}
//
// # Why the verbs are data
//
// Because a footer holds one row and a screen can easily have nine verbs. The
// alternative — a letter binding per verb — runs out of letters, collides with
// the vocabulary rule 25 reserves, and puts discovery in a strip that was
// already truncating. Declaring them lets one surface list them all, and lets
// the menu answer questions the author would otherwise answer by hand: whether
// a verb applies to a multi-selection (Multi), whether one is already running
// (Exclusive), and why a row the user can see is not available (Disabled).
//
// # Run and Do
//
// Run is background work: a context, an io.Writer for progress, an error. Do
// is the escape hatch for verbs that are not background work — pushing a child
// screen, handing the terminal to $EDITOR. Exactly one may be set; Validate
// enforces it. This package does not execute either. The Menu reports what was
// chosen and the host decides, which is what keeps the component testable
// without a shell, a goroutine, or a subprocess.
package action

import (
	"context"
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/mouse"
)

// Func is background work launched by an action.
//
// The signature is deliberate on all three counts. ctx makes cancellation
// non-optional, so an action is no harder to stop than a subprocess. out is an
// io.Writer rather than a channel or a log closure because it composes with
// everything that already writes — fmt.Fprintf, io.Copy from a response body,
// an exec.Cmd's Stdout — where a bespoke callback composes with nothing. The
// error return is the outcome: an action either worked or it didn't, and
// ctx.Err() reading as a failure is correct, because the user stopped it.
type Func func(ctx context.Context, out io.Writer) error

// Action is one verb over the current selection.
type Action struct {
	// Label names the verb and titles its log event. Required.
	Label string

	// Desc is an optional gloss rendered beside the label.
	Desc string

	// Key is an optional shortcut. The menu dispatches it while open and
	// renders it in a right-hand column; it is deliberately not advertised
	// in the screen's Help(), since moving discovery off the footer and into
	// the menu is most of the point.
	Key key.Binding

	// Confirm, when non-empty, puts a yes/no modal between the pick and the
	// run. Use it for anything destructive rather than hand-rolling the
	// sequence.
	Confirm string

	// Disabled, when non-empty, renders the row dimmed and unselectable with
	// this text as the reason.
	//
	// Showing an unavailable verb beats hiding it: hidden, the user learns
	// the verb does not exist and goes looking in the docs; shown with a
	// reason, they learn it does not apply yet. The menu also fills this in
	// on its own — for a non-Multi action under a multi-selection, and for
	// an Exclusive action already in flight.
	Disabled string

	// Multi reports whether this action accepts a selection of more than
	// one. The zero value is false: an action acts on exactly one target
	// unless it says otherwise.
	//
	// The default runs the safe way round. "View logs" pushes one screen, so
	// forgetting to think about arity leaves a disabled row with an
	// explanation — noticed in the first five seconds of using the screen.
	// Were the default reversed, forgetting would ship a verb that picks one
	// of three marked targets arbitrarily, which is found in production.
	Multi bool

	// Exclusive refuses a second concurrent run for the same target. The
	// menu renders it disabled with a reason rather than dropping the press,
	// so the refusal is visible.
	Exclusive bool

	// ID scopes the Exclusive check. Defaults to Label; set it when two
	// screens share a label but not an identity.
	ID string

	// Run is background work. Prefer it: it can be attributed, grouped,
	// cancelled and reported on, none of which Do can be.
	Run Func

	// Do is the escape hatch for verbs that are not background work.
	//
	// Navigational Do actions are one-at-a-time by construction rather than
	// by declaration — pushing a screen replaces what is on top, so there is
	// no second one to push. It is a Do that does not navigate, one that
	// fires a request and returns, that may still want Exclusive.
	Do func() tea.Cmd
}

// Ident is the action's identity for the Exclusive check. Defaults to Label.
func (a Action) Ident() string {
	if a.ID != "" {
		return a.ID
	}
	return a.Label
}

// RunKey is the identity an Exclusive action is held against: its Ident
// paired with the target it was launched for.
//
// Pairing with the target is what makes exclusivity useful rather than
// merely safe — restarting web while api restarts is fine, restarting web
// twice is not, and an identity that ignored the target could not tell those
// apart.
func RunKey(a Action, target string) string {
	return a.Ident() + "\x00" + target
}

// Set is a screen's actions plus what they will act on.
type Set struct {
	// Target names the object of the verbs for display — "cache-redis",
	// "3 items". It titles the menu.
	//
	// It is the whole reason Set is a struct rather than a bare slice: once
	// rows can be marked, "Delete" means one row or twelve, and the menu is
	// the last surface that can say which before it happens.
	Target string

	// Count is how many targets Actions will act on. 0 and 1 both mean a
	// single target; above that, actions without Multi are disabled.
	Count int

	Actions []Action
}

// Empty reports whether the set has no actions.
func (s Set) Empty() bool { return len(s.Actions) == 0 }

// Provider is implemented by screens that have verbs.
//
// It is an optional interface rather than a method on screen.Screen so every
// existing screen keeps compiling and simply has no actions, which is the
// honest state of affairs for a screen that has not declared any.
//
// Implementations are called on menu open and on key dispatch, so the same
// contract applies as to Help(): cheap, allocation-light, no I/O.
type Provider interface {
	Actions() Set
}

// ChosenMsg reports the action the user picked. The host decides what
// "chosen" means — run it, confirm it first, push a screen.
type ChosenMsg struct {
	Action Action
	Target string
}

// CancelledMsg reports that the menu was dismissed without a pick.
type CancelledMsg struct{}

// RetargetMsg reports a right-press that landed outside the open menu — the
// gesture meaning "ask me about this one instead."
//
// The menu emits it rather than acting, because retargeting needs to know
// what is under the pointer in the layer beneath and the menu cannot see
// there. A host that handles it moves its own selection to the event and
// reopens; a host that ignores it leaves the menu exactly as it was, which is
// why adding this could not change any existing behaviour.
type RetargetMsg struct {
	// Event is the press, forwarded untouched so the host can route it to
	// whatever is underneath before it reopens.
	Event mouse.Msg
}

func chosen(a Action, target string) tea.Cmd {
	return func() tea.Msg { return ChosenMsg{Action: a, Target: target} }
}

func cancelled() tea.Cmd {
	return func() tea.Msg { return CancelledMsg{} }
}

func retarget(e mouse.Msg) tea.Cmd {
	return func() tea.Msg { return RetargetMsg{Event: e} }
}

// Validate reports everything structurally wrong with a Set: a missing label,
// neither or both of Run and Do, a duplicate shortcut, a duplicate identity.
//
// It exists to be called from a test. These are all authoring mistakes whose
// symptoms show up far from their cause — a duplicate shortcut silently
// resolves to whichever action is listed first, and a duplicate identity makes
// one Exclusive action disable an unrelated one — so the useful place to catch
// them is at build time, not by noticing the menu behaving oddly.
func Validate(s Set) []error {
	var errs []error
	idents := map[string]int{}
	keys := map[string]int{}

	for i, a := range s.Actions {
		switch {
		case a.Label == "":
			errs = append(errs, fmt.Errorf("action %d: Label is required", i))
		case a.Run == nil && a.Do == nil:
			errs = append(errs, fmt.Errorf("action %q: neither Run nor Do is set", a.Label))
		case a.Run != nil && a.Do != nil:
			errs = append(errs, fmt.Errorf("action %q: both Run and Do are set; exactly one may be", a.Label))
		}

		if a.Label != "" {
			if prev, dup := idents[a.Ident()]; dup {
				errs = append(errs, fmt.Errorf("action %q: identity %q already used by action %d",
					a.Label, a.Ident(), prev))
			} else {
				idents[a.Ident()] = i
			}
		}

		for _, k := range a.Key.Keys() {
			if prev, dup := keys[k]; dup {
				errs = append(errs, fmt.Errorf("action %q: key %q already bound by action %d",
					a.Label, k, prev))
			} else {
				keys[k] = i
			}
		}
	}
	return errs
}

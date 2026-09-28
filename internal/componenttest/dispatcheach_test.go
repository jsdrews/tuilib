// DispatchEach, asserted once across list, table and tree: one operation per
// key, so refusing one row's request stops that row alone.
package componenttest

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/tree"
)

type eachReply struct {
	op  activity.Op
	key string
}

type eacher interface {
	DispatchEach(keys []string, label string, mode activity.Mode, request func(activity.Op, string) tea.Cmd) tea.Cmd
	Done(op activity.Op, err error) activity.Outcome
	ActivityState() activity.Set
}

// replies runs cmd and collects every eachReply it produces.
func replies(cmd tea.Cmd) []eachReply {
	if cmd == nil {
		return nil
	}
	switch m := cmd().(type) {
	case eachReply:
		return []eachReply{m}
	case tea.BatchMsg:
		var out []eachReply
		for _, c := range m {
			out = append(out, replies(c)...)
		}
		return out
	}
	return nil
}

func TestDispatchEachGivesEveryKeyItsOwnOperation(t *testing.T) {
	lm := list.New(theme.Dark().List())
	tm := table.New(theme.Dark().Table())
	to := theme.Dark().Tree()
	to.InitialDepth = 2
	to.Root = derivedTree(statusOf(nil))
	trm := tree.New(to)

	for name, tc := range map[string]struct {
		c    eacher
		keys []string
	}{
		"list":  {&lm, []string{"api", "web"}},
		"table": {&tm, []string{"api", "web"}},
		"tree":  {&trm, []string{"cluster/api", "cluster/web"}},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := tc.c.DispatchEach(tc.keys, "Running", activity.Observed,
				func(op activity.Op, key string) tea.Cmd {
					return func() tea.Msg { return eachReply{op, key} }
				})
			got := replies(cmd)
			if len(got) != 2 || got[0].op.ID() == got[1].op.ID() {
				t.Fatalf("replies = %+v, want one per key with distinct operations", got)
			}
			var refused, accepted eachReply
			for _, r := range got {
				if r.key == tc.keys[0] {
					refused = r
				} else {
					accepted = r
				}
			}
			if out := tc.c.Done(refused.op, errors.New("409")); out != activity.Withdrawn {
				t.Errorf("refused Done = %v, want Withdrawn", out)
			}
			if out := tc.c.Done(accepted.op, nil); out != activity.Acknowledged {
				t.Errorf("accepted Done = %v, want Acknowledged", out)
			}
			if _, ok := tc.c.ActivityState().State(tc.keys[0]); ok {
				t.Error("the refused row is still claimed")
			}
			if _, ok := tc.c.ActivityState().State(tc.keys[1]); !ok {
				t.Error("refusing one row ended the other's claim")
			}
		})
	}
}

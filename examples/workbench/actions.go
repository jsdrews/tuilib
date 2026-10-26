package workbench

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/action"
)

// Actions satisfies action.Provider: the verbs for whatever the focused pane
// has selected, rebuilt on every call so they always describe the selection
// as it is now. Panes with nothing to act on offer nothing, and the shell
// leaves the actions key inert there.
//
// Every verb reads Selection(), which is the marked keys or the cursor's key
// when nothing is marked. The arity difference lives in Multi, and the menu
// enforces it: mark a second row and Describe dims itself with a reason.
func (s *Screen) Actions() action.Set {
	switch {
	case s.focus.Is(&s.table):
		return s.deploymentActions()
	case s.focus.Is(&s.tree):
		return s.treeActions()
	case s.focus.Is(&s.pods):
		return s.podActions()
	}
	return action.Set{}
}

func (s *Screen) deploymentActions() action.Set {
	keys := s.table.Selection()
	if len(keys) == 0 {
		return action.Set{}
	}
	label := s.table.SelectionLabel()
	return action.Set{
		Target: label,
		Count:  len(keys),
		Actions: []action.Action{
			{Label: "Describe", Desc: "one deployment only", Group: "read", Run: describe(keys)},
			{
				Label: "Restart", Desc: "roll the pods", Group: "write",
				Key:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "restart")),
				Multi: true, Exclusive: true, Run: rollout(keys, 700*time.Millisecond),
			},
			{Label: "Scale to zero", Desc: "drain and stop", Group: "write", Multi: true, Run: report(keys, "scaled %s to 0 replicas")},
			{
				Label: "Delete", Group: "danger", Multi: true,
				Disabled: s.terminatingGuard(keys),
				Confirm:  "Delete " + label + "? This cannot be undone.",
				Run:      refuse(keys, "deleted deployment/%s"),
			},
			{Label: "Clear marks", Desc: "drop the selection", Multi: true, Do: func() tea.Cmd {
				s.table.ClearMarks()
				s.refreshTitles()
				return nil
			}},
		},
	}
}

func (s *Screen) terminatingGuard(keys []string) string {
	for _, k := range keys {
		for _, d := range s.deps {
			if d.key() == k && d.terminating {
				return d.name + " is already terminating"
			}
		}
	}
	return ""
}

// treeActions resolve a marked branch to the pods under it, because a tree
// marks the node the user pressed x on and nothing below it (rule 32). Only
// the screen knows whether a verb cascades: Restart and Cordon do, Describe
// reads the marked node itself.
func (s *Screen) treeActions() action.Set {
	sel := s.tree.Selection()
	if len(sel) == 0 {
		return action.Set{}
	}
	pods := s.podsUnder(sel)
	label := podsLabel(pods)
	return action.Set{
		Target: label,
		Count:  len(pods),
		Actions: []action.Action{
			{Label: "Describe", Desc: "the marked node itself", Group: "read", Run: describe(sel)},
			{
				Label: "Restart", Desc: "roll every pod in the selection", Group: "write",
				Key:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "restart")),
				Multi: true, Exclusive: true, Run: rollout(pods, 400*time.Millisecond),
			},
			{Label: "Cordon", Desc: "stop scheduling here", Group: "write", Multi: true, Run: report(pods, "cordoned %s")},
			{
				Label: "Delete", Group: "danger", Multi: true,
				Confirm:  "Delete " + label + "? This cannot be undone.",
				Disabled: stagingGuard(sel),
				Run:      refuse(pods, "deleted pod/%s"),
			},
			{Label: "Drop marks", Desc: "clear the selection", Multi: true, Do: func() tea.Cmd {
				s.tree.ClearMarks()
				s.refreshTitles()
				return nil
			}},
		},
	}
}

// podsUnder resolves tree paths to the pod paths at or below them. Paths are
// hierarchical strings, so "below" is a prefix test.
func (s *Screen) podsUnder(sel []string) []string {
	seen := map[string]bool{}
	for _, p := range sel {
		for _, cand := range s.podPaths() {
			if cand == p || strings.HasPrefix(cand, p+"/") {
				seen[cand] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func (s *Screen) podPaths() []string {
	var out []string
	var walk func(n *node, path string)
	walk = func(n *node, path string) {
		if n.kind == "pod" {
			out = append(out, path)
			return
		}
		for _, k := range n.kids {
			walk(k, path+"/"+k.label)
		}
	}
	walk(s.root, s.root.label)
	return out
}

// stagingGuard is an author-supplied rule that belongs on the action, where
// the menu shows it as a reason, rather than inside a Run already confirmed.
func stagingGuard(sel []string) string {
	for _, p := range sel {
		if !strings.HasPrefix(p, "cluster/staging") {
			return "only staging can be deleted in this demo"
		}
	}
	return ""
}

func podsLabel(pods []string) string {
	if len(pods) == 1 {
		return short(pods[0])
	}
	return strconv.Itoa(len(pods)) + " pods"
}

func (s *Screen) podActions() action.Set {
	keys := s.pods.Selection()
	if len(keys) == 0 {
		return action.Set{}
	}
	label := s.pods.SelectionLabel()
	return action.Set{
		Target: label,
		Count:  len(keys),
		Actions: []action.Action{
			{Label: "Logs", Desc: "the last lines", Group: "read", Run: report(keys, "%s: listening on :8080")},
			{
				Label: "Delete", Group: "danger", Multi: true,
				Confirm: "Delete " + label + "? The deployment will replace it.",
				Run:     report(keys, "deleted pod/%s"),
			},
		},
	}
}

// The verbs are background work: a context to watch, a writer whose lines
// become one console event, and an error that decides the outcome. A verb
// over a selection is a loop and nothing more.

func describe(keys []string) action.Func {
	return func(_ context.Context, out io.Writer) error {
		fmt.Fprintf(out, "Name:     %s\nStatus:   Running\nStrategy: RollingUpdate\n", short(keys[0]))
		return nil
	}
}

func rollout(keys []string, each time.Duration) action.Func {
	return func(ctx context.Context, out io.Writer) error {
		for _, k := range keys {
			fmt.Fprintf(out, "restarting %s\n", short(k))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(each):
			}
		}
		fmt.Fprintf(out, "rolled out %d\n", len(keys))
		return nil
	}
}

func report(keys []string, format string) action.Func {
	return func(_ context.Context, out io.Writer) error {
		for _, k := range keys {
			fmt.Fprintf(out, format+"\n", short(k))
		}
		return nil
	}
}

func refuse(keys []string, format string) action.Func {
	return func(ctx context.Context, out io.Writer) error {
		_ = report(keys, format)(ctx, out)
		return fmt.Errorf("refusing to delete in a demo")
	}
}

// short is a key's last segment, which is what reads well in a sentence.
func short(k string) string {
	if i := strings.LastIndex(k, "/"); i >= 0 {
		return k[i+1:]
	}
	return k
}

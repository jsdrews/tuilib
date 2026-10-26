package workbench

import (
	"fmt"
	"strings"

	"github.com/jsdrews/tuilib/pkg/ansi"
	"github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// One cluster feeds every pane: the table lists its deployments, the tree
// draws it as namespaces, workloads and pods, and the pods list shows the
// table's current deployment. A change to one view that breaks another shows
// up as two panes disagreeing about the same data.

type pod struct {
	name     string
	phase    string // Running, Pending, CrashLoopBackOff
	restarts int
	node     string
}

type deployment struct {
	name, env, age string
	pods           []pod
	// terminating supplies an author-written Disabled reason, distinct from
	// the two the action menu fills in on its own (arity and exclusivity).
	terminating bool
}

// key is the deployment's identity, used for the table's keyed rows and its
// marks. The same name runs in two environments, so the name alone is not one.
func (d deployment) key() string { return d.env + "/" + d.name }

func (d deployment) ready() int {
	n := 0
	for _, p := range d.pods {
		if p.phase == "Running" {
			n++
		}
	}
	return n
}

func (d deployment) health() string {
	switch r := d.ready(); {
	case r == 0:
		return "Down"
	case r < len(d.pods):
		return "Degraded"
	}
	return "Healthy"
}

func pods(prefix, node string, phases ...string) []pod {
	suffixes := []string{"7f3a", "9b21", "2c88", "4de1", "6a07", "0f5b"}
	out := make([]pod, len(phases))
	for i, ph := range phases {
		restarts := 0
		if ph == "CrashLoopBackOff" {
			restarts = 7 + i
		}
		out[i] = pod{name: prefix + "-" + suffixes[i], phase: ph, restarts: restarts, node: node}
	}
	return out
}

func seed() []deployment {
	return []deployment{
		{name: "api-server", env: "prod", age: "4d", pods: pods("api-server", "node-a", "Running", "Running", "CrashLoopBackOff")},
		{name: "web-frontend", env: "prod", age: "6h", pods: pods("web-frontend", "node-b", "Running", "Running")},
		{name: "worker-pool", env: "prod", age: "12m", pods: pods("worker-pool", "node-c", "Running", "Pending", "Pending", "Pending")},
		{name: "cache-redis", env: "prod", age: "3d", pods: pods("cache-redis", "node-a", "Running")},
		{name: "metrics-agent", env: "prod", age: "9d", pods: pods("metrics-agent", "node-b", "CrashLoopBackOff"), terminating: true},
		{name: "api-server", env: "staging", age: "2d", pods: pods("api-server", "node-s", "Running")},
		{name: "web-frontend", env: "staging", age: "5h", pods: pods("web-frontend", "node-s", "Running", "Pending")},
		{name: "batch-runner", env: "staging", age: "1d", pods: pods("batch-runner", "node-s", "Running", "Running")},
	}
}

// Cells are coloured with ansi.CellColor, which closes with a foreground-only
// reset, so the selected row's background runs unbroken across them (rule 19).

func healthCell(h string) string {
	switch h {
	case "Healthy":
		return ansi.CellColor(2, "✓ Healthy")
	case "Degraded":
		return ansi.CellColor(3, "● Degraded")
	}
	return ansi.CellColor(1, "✗ Down")
}

func phaseGlyph(phase string) string {
	switch phase {
	case "Running":
		return ansi.CellColor(2, "●")
	case "Pending":
		return ansi.CellColor(3, "●")
	}
	return ansi.CellColor(1, "●")
}

// healthLess sorts the rendered cell, which carries the CellColor escape
// around the word; strings.Contains reads through it without a strip.
func healthLess(a, b string) bool { return healthRank(a) < healthRank(b) }

func healthRank(s string) int {
	switch {
	case strings.Contains(s, "Down"):
		return 0
	case strings.Contains(s, "Degraded"):
		return 1
	}
	return 2
}

// node satisfies tree.Node over the cluster. kind is this example's own
// field: the tree neither knows nor needs it.
type node struct {
	label string
	kind  string // "root", "namespace", "workload", "pod"
	dep   *deployment
	pod   *pod
	kids  []*node
}

func (n *node) Label() string { return n.label }
func (n *node) Children() []tree.Node {
	out := make([]tree.Node, len(n.kids))
	for i, k := range n.kids {
		out[i] = k
	}
	return out
}

func clusterTree(deps []deployment) *node {
	root := &node{label: "cluster", kind: "root"}
	namespaces := map[string]*node{}
	for i := range deps {
		d := &deps[i]
		ns, ok := namespaces[d.env]
		if !ok {
			ns = &node{label: d.env, kind: "namespace"}
			namespaces[d.env] = ns
			root.kids = append(root.kids, ns)
		}
		w := &node{label: d.name, kind: "workload", dep: d}
		for j := range d.pods {
			p := &d.pods[j]
			w.kids = append(w.kids, &node{label: phaseGlyph(p.phase) + " " + p.name, kind: "pod", dep: d, pod: p})
		}
		ns.kids = append(ns.kids, w)
	}
	return root
}

// The inspector's records. inspector.FromMap turns the decoded-JSON shape a
// REST or kubectl payload arrives in into fields, which is the path most
// callers take.

func deploymentFields(d deployment) []inspector.Field {
	names := make([]any, len(d.pods))
	for i, p := range d.pods {
		names[i] = p.name
	}
	return inspector.FromMap(map[string]any{
		"metadata": map[string]any{"name": d.name, "namespace": d.env, "age": d.age},
		"spec":     map[string]any{"replicas": len(d.pods), "strategy": "RollingUpdate"},
		"status": map[string]any{
			"health":        d.health(),
			"readyReplicas": d.ready(),
			"terminating":   d.terminating,
		},
		"pods": names,
	})
}

func podFields(d deployment, p pod) []inspector.Field {
	return inspector.FromMap(map[string]any{
		"metadata": map[string]any{"name": p.name, "namespace": d.env, "ownerReference": "deployment/" + d.name},
		"spec":     map[string]any{"nodeName": p.node, "containers": []any{map[string]any{"name": d.name, "image": "registry.local/" + d.name + ":1.4.2"}}},
		"status":   map[string]any{"phase": p.phase, "restartCount": p.restarts},
	})
}

func namespaceFields(n *node) []inspector.Field {
	workloads := make([]any, len(n.kids))
	podCount := 0
	for i, w := range n.kids {
		workloads[i] = w.label
		podCount += len(w.kids)
	}
	return inspector.FromMap(map[string]any{
		"metadata":  map[string]any{"name": n.label, "kind": n.kind},
		"workloads": workloads,
		"pods":      fmt.Sprint(podCount),
	})
}

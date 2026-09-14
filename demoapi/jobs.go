package demoapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// describe is the detail document: nested several levels deep, with arrays of
// objects, so inspector.FromMap and pkg/tree have something worth rendering.
//
// A flat map would exercise neither — both components exist for the shape
// where a value is itself a document.
func describe(a App, jobs []Job) map[string]any {
	history := make([]any, 0, 5)
	for i, j := range jobs {
		if i == 5 {
			break
		}
		entry := map[string]any{
			"id":      j.ID,
			"kind":    j.Kind,
			"status":  j.Status,
			"started": j.Started.UTC().Format(time.RFC3339),
		}
		if !j.Finished.IsZero() {
			entry["finished"] = j.Finished.UTC().Format(time.RFC3339)
		}
		history = append(history, entry)
	}

	return map[string]any{
		"metadata": map[string]any{
			"id":     a.ID,
			"name":   a.Name,
			"region": a.Region,
			"labels": map[string]any{
				"app.kubernetes.io/name":       a.Name,
				"app.kubernetes.io/managed-by": "demoapi",
			},
		},
		"spec": map[string]any{
			"source": map[string]any{
				"repoURL":        "https://git.example.test/" + a.Name + ".git",
				"path":           "deploy/overlays/" + a.Region,
				"targetRevision": "main",
			},
			"destination": map[string]any{
				"server":    "https://k8s." + a.Region + ".example.test",
				"namespace": a.Name,
			},
			"syncPolicy": map[string]any{
				"automated": map[string]any{"prune": true, "selfHeal": false},
				"retry":     map[string]any{"limit": 3, "backoff": "5s"},
			},
		},
		"status": map[string]any{
			"sync":   map[string]any{"status": a.Sync, "revision": strconv.FormatInt(a.Rev, 10)},
			"health": map[string]any{"status": a.Health},
			"resources": []any{
				map[string]any{"kind": "Deployment", "name": a.Name, "status": a.Sync},
				map[string]any{"kind": "Service", "name": a.Name, "status": SyncSynced},
				map[string]any{"kind": "ConfigMap", "name": a.Name + "-config", "status": a.Sync},
			},
			"history": history,
		},
	}
}

// logLines is what a job writes. Enough of them that a screen has to scroll.
//
// The placeholder is a token rather than a fmt verb. With %s, every line that
// did not happen to take the app name picked up a trailing
// "%!(EXTRA string=app-00000)" — Sprintf complains about arguments a format
// string does not consume, and half of these do not. A token substitution
// cannot have that failure mode, so the two kinds of line stop being different
// kinds of line.
var logLines = []string{
	"comparing desired state to live state",
	"found 3 resources out of sync",
	"applying Deployment/{app}",
	"waiting for rollout to complete",
	"Deployment/{app} successfully rolled out",
	"applying Service/{app}",
	"Service/{app} unchanged",
	"applying ConfigMap/{app}-config",
	"ConfigMap/{app}-config configured",
	"pruning 0 resources",
	"sync operation complete",
}

// jobLog streams the job's output while it runs, then closes.
//
// Chunked and flushed per line, deliberately: it is the only endpoint whose
// point is partial arrival. A screen consuming it has to handle lines showing
// up over time, which is what logview's MaxLines cap and the chained-read
// pattern of rule 12 are for — and what a buffered response would silently let
// it skip.
func (s *server) jobLog(w http.ResponseWriter, r *http.Request) {
	j, ok := s.w.job(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "job "+strconv.Quote(r.PathValue("id"))+" not found")
		return
	}

	rate := duration(r.URL.Query().Get("rate"))
	if rate <= 0 {
		rate = 150 * time.Millisecond
	}

	flusher, canFlush := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	for i, line := range logLines {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(rate):
		}
		fmt.Fprintln(w, strings.ReplaceAll(line, "{app}", j.App))
		if canFlush {
			flusher.Flush()
		}
		if i == len(logLines)-1 && j.fails {
			fmt.Fprintf(w, "error: rpc error: application %q not found\n", j.App)
			if canFlush {
				flusher.Flush()
			}
		}
	}
}

package demoapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jsdrews/tuilib/demoapi/api"
)

// describe is the detail document: nested several levels deep, with arrays of
// objects, so inspector.FromMap and pkg/tree have something worth rendering.
//
// A flat map would exercise neither — both components exist for the shape
// where a value is itself a document.
func describe(a App, jobs []Job) api.AppDetail {
	history := make([]api.AppDetail_Status_History, 0, 5)
	for i, j := range jobs {
		if i == 5 {
			break
		}
		history = append(history, api.AppDetail_Status_History{
			Id:       j.ID,
			Kind:     api.JobKind(j.Kind),
			Status:   api.JobStatus(j.Status),
			Started:  j.Started.UTC().Truncate(time.Second),
			Finished: optionalTime(j.Finished.UTC().Truncate(time.Second)),
		})
	}

	var d api.AppDetail
	d.Metadata = api.AppDetail_Metadata{
		Id: a.ID, Name: a.Name, Region: a.Region,
		Labels: map[string]string{
			"app.kubernetes.io/name":       a.Name,
			"app.kubernetes.io/managed-by": "demoapi",
		},
	}
	d.Spec.Source = api.AppDetail_Spec_Source{
		RepoURL:        "https://git.example.test/" + a.Name + ".git",
		Path:           "deploy/overlays/" + a.Region,
		TargetRevision: "main",
	}
	d.Spec.Destination = api.AppDetail_Spec_Destination{
		Server:    "https://k8s." + a.Region + ".example.test",
		Namespace: a.Name,
	}
	d.Spec.SyncPolicy.Automated = api.AppDetail_Spec_SyncPolicy_Automated{Prune: true, SelfHeal: false}
	d.Spec.SyncPolicy.Retry = api.AppDetail_Spec_SyncPolicy_Retry{Limit: 3, Backoff: "5s"}
	d.Status.Sync = api.AppDetail_Status_Sync{Status: api.SyncStatus(a.Sync), Revision: strconv.FormatInt(a.Rev, 10)}
	d.Status.Health.Status = a.Health
	d.Status.OperationState.Phase = a.Phase
	d.Status.ReconciledAt = a.ReconciledAt.UTC().Truncate(time.Second)
	d.Status.Resources = []api.AppDetail_Status_Resources{
		{Kind: "Deployment", Name: a.Name, Status: api.SyncStatus(a.Sync)},
		{Kind: "Service", Name: a.Name, Status: SyncSynced},
		{Kind: "ConfigMap", Name: a.Name + "-config", Status: api.SyncStatus(a.Sync)},
	}
	d.Status.History = history
	return d
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

// GetJobLog streams the job's output while it runs, then closes.
//
// Chunked and flushed per line, deliberately: it is the only endpoint whose
// point is partial arrival. A screen consuming it has to handle lines showing
// up over time, which is what logview's MaxLines cap and the chained-read
// pattern of rule 12 are for — and what a buffered response would silently let
// it skip. The spec's generated text response is a whole string, so the stream
// is its own response type.
func (s *server) GetJobLog(ctx context.Context, r api.GetJobLogRequestObject) (api.GetJobLogResponseObject, error) {
	j, ok := s.w.job(r.Id)
	if !ok {
		return api.GetJobLogdefaultJSONResponse{StatusCode: http.StatusNotFound, Body: notFound("job", r.Id)}, nil
	}
	rate := duration(deref(r.Params.Rate))
	if rate <= 0 {
		rate = 150 * time.Millisecond
	}
	return logStream{ctx: ctx, job: j, rate: rate}, nil
}

type logStream struct {
	ctx  context.Context
	job  Job
	rate time.Duration
}

func (l logStream) VisitGetJobLogResponse(w http.ResponseWriter) error {
	flusher, canFlush := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	for i, line := range logLines {
		select {
		case <-l.ctx.Done():
			return nil
		case <-time.After(l.rate):
		}
		fmt.Fprintln(w, strings.ReplaceAll(line, "{app}", l.job.App))
		if canFlush {
			flusher.Flush()
		}
		if i == len(logLines)-1 && l.job.fails {
			fmt.Fprintf(w, "error: rpc error: application %q not found\n", l.job.App)
			if canFlush {
				flusher.Flush()
			}
		}
	}
	return nil
}

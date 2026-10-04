package demoapi

import (
	"time"

	"github.com/jsdrews/tuilib/demoapi/api"
)

// The world keeps its own types, because they carry what the wire must not:
// a job's finish time, an app's drift, the values a lagging replica would
// still report. These turn them into the spec's.

func (a App) wire() api.App {
	return api.App{
		Id:           a.ID,
		Name:         a.Name,
		Region:       a.Region,
		Sync:         api.SyncStatus(a.Sync),
		Health:       a.Health,
		Phase:        a.Phase,
		ReconciledAt: a.ReconciledAt,
		Rev:          a.Rev,
	}
}

func (j Job) wire() api.Job {
	return api.Job{
		Id:       j.ID,
		App:      j.App,
		Kind:     api.JobKind(j.Kind),
		Status:   api.JobStatus(j.Status),
		Started:  j.Started,
		Finished: optionalTime(j.Finished),
	}
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

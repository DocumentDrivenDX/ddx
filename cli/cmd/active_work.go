package cmd

import (
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/activework"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

func collectActiveWorkSnapshot(projectRoot string, store *bead.Store, now time.Time) (activework.Snapshot, error) {
	return activework.Collect(projectRoot, store, now)
}

func activeWorkRecordsForFocus(snapshot activework.Snapshot) []WorkFocusActiveWorker {
	out := make([]WorkFocusActiveWorker, 0, len(snapshot.Records))
	for _, rec := range snapshot.Records {
		out = append(out, WorkFocusActiveWorker{
			WorkerID:       rec.WorkerID,
			CurrentBead:    rec.BeadID,
			AttemptID:      rec.AttemptID,
			Phase:          rec.Phase,
			LastActivityAt: rec.LastActivityAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return out
}

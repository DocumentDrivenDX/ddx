package workerstatus

import "time"

const livenessStartedAtSkew = 10 * time.Second

// EnrichWithFreshLiveness overlays fresh worker sidecars onto live process
// rows. The process scan remains authoritative for liveness, pid, command,
// and project scope; the sidecar only fills execution-specific fields that a
// long-running `ddx work --watch` parent does not carry in argv/cwd.
func EnrichWithFreshLiveness(workers []LiveWorker, now time.Time) []LiveWorker {
	if len(workers) == 0 {
		return workers
	}

	freshByProject := make(map[string]map[int][]LivenessRecord)
	for _, w := range workers {
		key := canonicalPath(w.ProjectRoot)
		if key == "" {
			continue
		}
		if _, ok := freshByProject[key]; ok {
			continue
		}
		freshByProject[key] = freshLivenessByPID(key, now)
	}

	out := make([]LiveWorker, len(workers))
	copy(out, workers)
	for i := range out {
		projectKey := canonicalPath(out[i].ProjectRoot)
		if projectKey == "" || out[i].PID <= 0 {
			continue
		}
		rec, ok := matchingLivenessRecord(out[i], freshByProject[projectKey][out[i].PID])
		if !ok {
			continue
		}
		applyLivenessRecord(&out[i], rec)
	}
	return out
}

func freshLivenessByPID(projectRoot string, now time.Time) map[int][]LivenessRecord {
	records, err := ListLiveness(projectRoot)
	if err != nil {
		return nil
	}
	out := make(map[int][]LivenessRecord)
	for _, rec := range records {
		if rec.PID <= 0 || !rec.IsFresh(now) {
			continue
		}
		out[rec.PID] = append(out[rec.PID], rec)
	}
	return out
}

func matchingLivenessRecord(worker LiveWorker, records []LivenessRecord) (LivenessRecord, bool) {
	if len(records) == 0 {
		return LivenessRecord{}, false
	}
	for _, rec := range records {
		if startedAtCompatible(worker.StartedAt, rec.StartedAt) {
			return rec, true
		}
	}
	return LivenessRecord{}, false
}

func startedAtCompatible(workerStartedAt, sidecarStartedAt time.Time) bool {
	if workerStartedAt.IsZero() || sidecarStartedAt.IsZero() {
		return true
	}
	delta := workerStartedAt.Sub(sidecarStartedAt)
	if delta < 0 {
		delta = -delta
	}
	return delta <= livenessStartedAtSkew
}

func applyLivenessRecord(worker *LiveWorker, rec LivenessRecord) {
	if worker == nil {
		return
	}
	if worker.BeadID == "" && rec.CurrentBead != "" {
		worker.BeadID = rec.CurrentBead
	}
	if worker.AttemptID == "" && rec.AttemptID != "" {
		worker.AttemptID = rec.AttemptID
	}
	if worker.Phase == "" && rec.Phase != "" {
		worker.Phase = rec.Phase
	}
	if worker.ChildPID == 0 && rec.ChildPID > 0 {
		worker.ChildPID = rec.ChildPID
	}
	if worker.LastActivityAt.IsZero() && !rec.LastActivityAt.IsZero() {
		worker.LastActivityAt = rec.LastActivityAt.UTC()
	}
	if worker.ExecutionWorktree == "" && rec.ChildPID > 0 {
		childBead, childWorktree := inferExecutionFromPID(rec.ChildPID)
		if childWorktree != "" {
			worker.ExecutionWorktree = childWorktree
		}
		if worker.BeadID == "" && childBead != "" {
			worker.BeadID = childBead
		}
	}
}

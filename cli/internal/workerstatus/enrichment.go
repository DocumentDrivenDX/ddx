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
	// records are already filtered to fresh sidecars for this worker's project
	// root and PID. A single fresh same-PID sidecar is authoritative: PID +
	// freshness is sufficient, so a start-time skew between the process
	// scanner's reported start time and the sidecar's recorded started_at must
	// not discard it (ddx-f9b41107). The start-time check only disambiguates a
	// reused PID — i.e. when more than one fresh sidecar claims the same PID.
	if len(records) == 1 {
		return records[0], true
	}
	for _, rec := range records {
		if startedAtCompatible(worker.StartedAt, rec.StartedAt) {
			return rec, true
		}
	}
	// Multiple fresh same-PID sidecars but none start-time-compatible: surface
	// the most recently active one rather than nothing.
	best := records[0]
	for _, rec := range records[1:] {
		if rec.LastActivityAt.After(best.LastActivityAt) {
			best = rec
		}
	}
	return best, true
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
	if worker.Message == "" && rec.Message != "" {
		worker.Message = rec.Message
	}
	if worker.ChildPID == 0 && rec.ChildPID > 0 {
		worker.ChildPID = rec.ChildPID
	}
	if len(worker.ProviderChildren) == 0 && len(rec.ProviderChildren) > 0 {
		worker.ProviderChildren = rec.ProviderChildren
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

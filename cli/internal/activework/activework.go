package activework

import (
	"context"
	"sort"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
)

// Record describes one fresh source of active work for a bead.
//
// The snapshot reconciles claim-heartbeat leases, worker liveness sidecars,
// and fresh run-state records into one project-scoped view so operator-facing
// status surfaces can agree on what is actively running.
type Record struct {
	ProjectRoot    string    `json:"project_root,omitempty"`
	WorkerID       string    `json:"worker_id,omitempty"`
	BeadID         string    `json:"bead_id,omitempty"`
	AttemptID      string    `json:"attempt_id,omitempty"`
	Owner          string    `json:"owner,omitempty"`
	Phase          string    `json:"phase,omitempty"`
	Source         string    `json:"source,omitempty"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

// Snapshot is the shared active-work summary used by bead, work, and GraphQL
// status views.
type Snapshot struct {
	Count   int      `json:"count"`
	BeadIDs []string `json:"bead_ids"`
	Records []Record `json:"records"`
}

// Collect returns a fresh active-work snapshot for projectRoot.
//
// A record is considered active when it is fresh under the source-specific
// freshness window:
// - claim heartbeats: bead.HeartbeatTTL
// - worker sidecars: workerstatus.LivenessTTL
// - run-state files: their ExpiresAt timestamp
//
// Stale records are ignored.
func Collect(projectRoot string, store *bead.Store, now time.Time) (Snapshot, error) {
	byKey := make(map[string]Record)

	add := func(rec Record) {
		key := activeWorkKey(rec)
		if key == "" {
			return
		}
		if existing, ok := byKey[key]; ok {
			byKey[key] = mergeRecord(existing, rec)
			return
		}
		byKey[key] = rec
	}

	if store != nil {
		beads, err := store.ReadAll(context.Background())
		if err != nil {
			return Snapshot{}, err
		}
		for _, b := range beads {
			if b.Status != bead.StatusOpen && b.Status != bead.StatusInProgress {
				continue
			}
			lease, found, err := store.ClaimLease(b.ID)
			if err != nil || !found || lease.UpdatedAt.IsZero() {
				continue
			}
			if now.Sub(lease.UpdatedAt) > bead.HeartbeatTTL {
				continue
			}
			add(Record{
				ProjectRoot:    projectRoot,
				BeadID:         b.ID,
				WorkerID:       lease.Owner,
				Owner:          lease.Owner,
				Source:         "claim",
				LastActivityAt: lease.UpdatedAt.UTC(),
			})
		}
	}

	if projectRoot != "" {
		liveness, err := workerstatus.ListLiveness(projectRoot)
		if err != nil {
			return Snapshot{}, err
		}
		for _, rec := range liveness {
			if !rec.IsFresh(now) {
				continue
			}
			if rec.PID > 0 && !processAlive(rec.PID) {
				continue
			}
			add(Record{
				ProjectRoot:    projectRoot,
				WorkerID:       rec.WorkerID,
				BeadID:         rec.CurrentBead,
				AttemptID:      rec.AttemptID,
				Phase:          rec.Phase,
				Source:         "liveness",
				LastActivityAt: rec.LastActivityAt.UTC(),
			})
		}

		states, err := agent.ReadRunStates(projectRoot)
		if err != nil {
			return Snapshot{}, err
		}
		for _, state := range states {
			if !state.ExpiresAt.IsZero() && now.After(state.ExpiresAt) {
				continue
			}
			if state.PID > 0 && !processAlive(state.PID) {
				continue
			}
			add(Record{
				ProjectRoot:    projectRoot,
				BeadID:         state.BeadID,
				AttemptID:      state.AttemptID,
				Source:         "run-state",
				LastActivityAt: state.RefreshedAt.UTC(),
			})
		}
	}

	return snapshotFromRecords(byKey), nil
}

// Merge reconciles one or more project-scoped snapshots into one host-scoped
// view. ProjectRoot participates in the active-work key so equal bead IDs from
// different repositories do not collapse into one record.
func Merge(snapshots ...Snapshot) Snapshot {
	byKey := make(map[string]Record)
	for _, snap := range snapshots {
		for _, rec := range snap.Records {
			key := activeWorkKey(rec)
			if key == "" {
				continue
			}
			if existing, ok := byKey[key]; ok {
				byKey[key] = mergeRecord(existing, rec)
				continue
			}
			byKey[key] = rec
		}
	}
	return snapshotFromRecords(byKey)
}

func snapshotFromRecords(byKey map[string]Record) Snapshot {
	records := make([]Record, 0, len(byKey))
	beadIDs := make(map[string]struct{})
	for _, rec := range byKey {
		records = append(records, rec)
		if rec.BeadID != "" {
			beadIDs[rec.BeadID] = struct{}{}
		}
	}

	sort.SliceStable(records, func(i, j int) bool {
		if records[i].LastActivityAt.Equal(records[j].LastActivityAt) {
			if records[i].ProjectRoot == records[j].ProjectRoot {
				if records[i].BeadID == records[j].BeadID {
					if records[i].AttemptID == records[j].AttemptID {
						if records[i].WorkerID == records[j].WorkerID {
							return records[i].Source < records[j].Source
						}
						return records[i].WorkerID < records[j].WorkerID
					}
					return records[i].AttemptID < records[j].AttemptID
				}
				return records[i].BeadID < records[j].BeadID
			}
			return records[i].ProjectRoot < records[j].ProjectRoot
		}
		return records[i].LastActivityAt.After(records[j].LastActivityAt)
	})

	beadList := make([]string, 0, len(beadIDs))
	for id := range beadIDs {
		beadList = append(beadList, id)
	}
	sort.Strings(beadList)

	return Snapshot{
		Count:   len(records),
		BeadIDs: beadList,
		Records: records,
	}
}

func activeWorkKey(rec Record) string {
	prefix := rec.ProjectRoot + "|"
	switch {
	case rec.BeadID != "":
		return prefix + "bead:" + rec.BeadID
	case rec.AttemptID != "":
		return prefix + "attempt:" + rec.AttemptID
	case rec.WorkerID != "":
		return prefix + "worker:" + rec.WorkerID
	default:
		return ""
	}
}

func mergeRecord(dst, src Record) Record {
	if src.LastActivityAt.After(dst.LastActivityAt) {
		if src.ProjectRoot != "" {
			dst.ProjectRoot = src.ProjectRoot
		}
		if src.BeadID != "" {
			dst.BeadID = src.BeadID
		}
		if src.AttemptID != "" {
			dst.AttemptID = src.AttemptID
		}
		if src.WorkerID != "" {
			dst.WorkerID = src.WorkerID
		}
		if src.Owner != "" {
			dst.Owner = src.Owner
		}
		if src.Phase != "" {
			dst.Phase = src.Phase
		}
		if src.Source != "" {
			dst.Source = src.Source
		}
		dst.LastActivityAt = src.LastActivityAt
		return dst
	}
	if dst.ProjectRoot == "" {
		dst.ProjectRoot = src.ProjectRoot
	}
	if dst.BeadID == "" {
		dst.BeadID = src.BeadID
	}
	if dst.AttemptID == "" {
		dst.AttemptID = src.AttemptID
	}
	if dst.WorkerID == "" {
		dst.WorkerID = src.WorkerID
	}
	if dst.Owner == "" {
		dst.Owner = src.Owner
	}
	if dst.Phase == "" {
		dst.Phase = src.Phase
	}
	if dst.Source == "" {
		dst.Source = src.Source
	}
	if dst.LastActivityAt.IsZero() {
		dst.LastActivityAt = src.LastActivityAt
	}
	return dst
}

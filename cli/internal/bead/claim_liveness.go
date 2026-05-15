package bead

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const claimLivenessNamespace = "ddx-claim-heartbeats"

type ClaimLeaseRecord struct {
	BeadID    string    `json:"bead_id"`
	Owner     string    `json:"owner,omitempty"`
	Session   string    `json:"session,omitempty"`
	Worktree  string    `json:"worktree,omitempty"`
	Machine   string    `json:"machine,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	PID       int       `json:"pid,omitempty"`
}

func claimLivenessPath(ddxDir, id string) string {
	root := canonicalClaimRoot(ddxDir)
	sum := sha1.Sum([]byte(root))
	return filepath.Join(os.TempDir(), claimLivenessNamespace, hex.EncodeToString(sum[:]), id+".json")
}

func canonicalClaimRoot(ddxDir string) string {
	root := filepath.Clean(filepath.Dir(ddxDir))
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	return filepath.Clean(root)
}

func writeAtomicClaimFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("bead: mkdir claim liveness dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("bead: create claim liveness tmp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("bead: write claim liveness tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("bead: close claim liveness tmp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("bead: publish claim liveness: %w", err)
	}
	return nil
}

func (s *Store) writeClaimHeartbeat(rec ClaimLeaseRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("bead: marshal claim liveness: %w", err)
	}
	data = append(data, '\n')
	return writeAtomicClaimFile(claimLivenessPath(s.Dir, rec.BeadID), data)
}

func (s *Store) readClaimHeartbeat(id string) (ClaimLeaseRecord, bool, error) {
	data, err := os.ReadFile(claimLivenessPath(s.Dir, id))
	if os.IsNotExist(err) {
		return ClaimLeaseRecord{}, false, nil
	}
	if err != nil {
		return ClaimLeaseRecord{}, false, fmt.Errorf("bead: read claim liveness: %w", err)
	}
	var rec ClaimLeaseRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return ClaimLeaseRecord{}, true, fmt.Errorf("bead: parse claim liveness: %w", err)
	}
	return rec, true, nil
}

func (s *Store) upsertClaimHeartbeat(id string, mutate func(*ClaimLeaseRecord)) error {
	rec, found, err := s.readClaimHeartbeat(id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if !found {
		rec = ClaimLeaseRecord{
			BeadID:    id,
			StartedAt: now,
		}
	}
	if rec.BeadID == "" {
		rec.BeadID = id
	}
	if rec.StartedAt.IsZero() {
		rec.StartedAt = now
	}
	rec.UpdatedAt = now
	if rec.PID == 0 {
		rec.PID = os.Getpid()
	}
	if mutate != nil {
		mutate(&rec)
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("bead: marshal claim liveness: %w", err)
	}
	data = append(data, '\n')
	return writeAtomicClaimFile(claimLivenessPath(s.Dir, id), data)
}

func (s *Store) TouchClaimHeartbeat(id string) error {
	return s.upsertClaimHeartbeat(id, nil)
}

func (s *Store) RemoveClaimHeartbeat(id string) error {
	err := os.Remove(claimLivenessPath(s.Dir, id))
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("bead: remove claim liveness: %w", err)
}

func (s *Store) ClaimHeartbeatFresh(id string) (bool, bool, error) {
	rec, found, err := s.readClaimHeartbeat(id)
	if err != nil {
		return false, false, err
	}
	if !found {
		return false, false, nil
	}
	if rec.UpdatedAt.IsZero() {
		return false, true, nil
	}
	return time.Since(rec.UpdatedAt) <= HeartbeatTTL, true, nil
}

// ClaimLease returns the external worker/manual claim sidecar for id when one
// exists. The record is the operator-facing live claim surface for workers; the
// tracker still carries any explicit manual lifecycle status change.
func (s *Store) ClaimLease(id string) (ClaimLeaseRecord, bool, error) {
	return s.readClaimHeartbeat(id)
}

package agent

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

const (
	// ExecuteBeadSlotPrefix is the directory prefix for pooled reusable
	// attempt workspace slots. Slot paths are derived from
	// AttemptWorkspaceSlotKey, never from the attempt ID.
	ExecuteBeadSlotPrefix = ".execute-bead-slot-"
	// ExecuteBeadEphemeralPrefix marks non-pooled fallback workspaces
	// allocated when reuse is disabled or the bounded pool is exhausted.
	ExecuteBeadEphemeralPrefix = ".execute-bead-ephemeral-"
	// slotLockFileName is the exclusive lock file inside each slot directory.
	slotLockFileName = ".slot.lock"
	// slotStampFileName records last-use timestamp for age-based eviction.
	slotStampFileName = ".slot.stamp"
	// slotQuarantineSuffix marks quarantined slots so future allocations skip
	// them instead of reusing an unhealthy workspace.
	slotQuarantineSuffix = ".quarantine.json"
)

// AttemptWorkspaceSlotKey identifies a reusable workspace slot pool.
// Paths are derived from this key alone so sequential attempts that share
// project, backend, worker-slot, and trust-boundary identity resolve to
// the same slot directories regardless of attempt ID.
type AttemptWorkspaceSlotKey struct {
	ProjectRoot   string
	Backend       string
	WorkerSlot    string
	TrustBoundary string
}

// Fingerprint returns a stable path-safe digest of the key components.
// Distinct project roots, backends, worker slots, or trust boundaries
// produce distinct fingerprints; attempt ID is intentionally excluded.
func (k AttemptWorkspaceSlotKey) Fingerprint() string {
	project := filepath.Clean(strings.TrimSpace(k.ProjectRoot))
	backend := strings.ToLower(strings.TrimSpace(k.Backend))
	if backend == "" {
		backend = AttemptBackendLocalClone
	}
	worker := strings.TrimSpace(k.WorkerSlot)
	if worker == "" {
		worker = "default"
	}
	trust := strings.TrimSpace(k.TrustBoundary)
	if trust == "" {
		trust = "default"
	}
	payload := strings.Join([]string{project, backend, worker, trust}, "\x00")
	sum := sha1.Sum([]byte(payload))
	return hex.EncodeToString(sum[:])[:16]
}

// PoolRoot returns the directory that holds numbered slots for this key
// under the project's execution temp root.
func (k AttemptWorkspaceSlotKey) PoolRoot() string {
	return filepath.Join(config.ExecutionTempRoot(k.ProjectRoot), ExecuteBeadSlotPrefix+k.Fingerprint())
}

// SlotPath returns the workspace directory for the given slot index.
// The path does not include an attempt ID.
func (k AttemptWorkspaceSlotKey) SlotPath(index int) string {
	return filepath.Join(k.PoolRoot(), "slot-"+strconv.Itoa(index))
}

// AttemptWorkspaceSlot is one exclusively locked workspace allocation.
// When Pooled is false the Path is a fresh non-pooled directory that the
// caller should treat as single-use (pool bound exceeded or reuse disabled).
type AttemptWorkspaceSlot struct {
	Key    AttemptWorkspaceSlotKey
	Index  int
	Path   string
	Pooled bool
	// Reusable-workspace telemetry contract. These fields are additive and
	// default to zero until later execution telemetry populates them.
	SlotHitCount            int   `json:"slot_hit_count,omitempty"`
	SlotMissCount           int   `json:"slot_miss_count,omitempty"`
	ConservativeTimeSavedMS int64 `json:"conservative_time_saved_ms,omitempty"`
	ConservativeBytesSaved  int64 `json:"conservative_bytes_saved,omitempty"`
	lockFile                *os.File
}

// AttemptWorkspaceSlotPool hands out bounded, exclusively locked reusable
// workspace slots. Allocation never blocks on a held slot: when every
// slot in the bound is held, Allocate returns a fresh non-pooled path.
//
// This package owns keying, locking, bounds, and eviction only; backends
// are not wired here.
type AttemptWorkspaceSlotPool struct {
	policy *config.ReusableWorkspaceConfig
	// root overrides ExecutionTempRoot for tests when non-empty. Slot
	// directories are placed under root/ExecuteBeadSlotPrefix+fingerprint.
	root string
	// now is the clock used for age eviction; nil uses time.Now.
	now func() time.Time
}

// NewAttemptWorkspaceSlotPool builds a pool from the given policy.
// A nil policy applies documented ReusableWorkspaceConfig defaults.
func NewAttemptWorkspaceSlotPool(policy *config.ReusableWorkspaceConfig) *AttemptWorkspaceSlotPool {
	return &AttemptWorkspaceSlotPool{policy: policy.Clone()}
}

// withRoot returns a copy of the pool that stores slots under root.
// Intended for tests that must not touch the process execution temp root.
func (p *AttemptWorkspaceSlotPool) withRoot(root string) *AttemptWorkspaceSlotPool {
	cp := *p
	cp.root = root
	return &cp
}

// withNow returns a copy of the pool that uses now as its clock.
func (p *AttemptWorkspaceSlotPool) withNow(now func() time.Time) *AttemptWorkspaceSlotPool {
	cp := *p
	cp.now = now
	return &cp
}

func (p *AttemptWorkspaceSlotPool) clock() time.Time {
	if p != nil && p.now != nil {
		return p.now()
	}
	return time.Now()
}

func (p *AttemptWorkspaceSlotPool) poolRoot(key AttemptWorkspaceSlotKey) string {
	if p != nil && strings.TrimSpace(p.root) != "" {
		return filepath.Join(p.root, ExecuteBeadSlotPrefix+key.Fingerprint())
	}
	return key.PoolRoot()
}

func (p *AttemptWorkspaceSlotPool) slotPath(key AttemptWorkspaceSlotKey, index int) string {
	return filepath.Join(p.poolRoot(key), "slot-"+strconv.Itoa(index))
}

// Allocate grants an exclusively locked workspace for key.
// When reuse is disabled or every slot in the bound is held, it returns a
// fresh non-pooled workspace (Pooled=false) instead of blocking or sharing.
func (p *AttemptWorkspaceSlotPool) Allocate(key AttemptWorkspaceSlotKey) (*AttemptWorkspaceSlot, error) {
	if p == nil {
		p = NewAttemptWorkspaceSlotPool(nil)
	}
	policy := p.policy
	if !policy.ResolveEnabled() {
		return p.allocateEphemeral(key)
	}

	maxSlots := policy.ResolveMaxSlots()
	// Best-effort eviction before allocation so stale/oversize slots free up.
	_ = p.Evict(key)

	for i := 0; i < maxSlots; i++ {
		slot, err := p.tryAcquireSlot(key, i)
		if err != nil {
			return nil, err
		}
		if slot != nil {
			return slot, nil
		}
	}
	// Bound exhausted: fall back to a non-pooled workspace.
	return p.allocateEphemeral(key)
}

// Release drops the exclusive lock on a previously allocated slot.
// Pooled slots remain on disk for sequential reuse; non-pooled paths are
// removed. Callers that need destructive scrub of pooled content do so
// separately (backend integration is out of scope for this package surface).
func (p *AttemptWorkspaceSlotPool) Release(slot *AttemptWorkspaceSlot) error {
	if slot == nil {
		return nil
	}
	var firstErr error
	if slot.lockFile != nil {
		if err := releaseExclusiveLock(slot.lockFile); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := slot.lockFile.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		slot.lockFile = nil
	}
	if !slot.Pooled && slot.Path != "" {
		if err := os.RemoveAll(slot.Path); err != nil && firstErr == nil {
			firstErr = err
		}
	} else if slot.Pooled && slot.Path != "" {
		_ = touchSlotStamp(slot.Path, p.clock())
	}
	return firstErr
}

// Evict removes pooled slots for key that exceed the configured max age or
// that must be reclaimed to bring total disk usage under the high-water mark.
// Held (locked) slots are skipped so concurrent holders are not disrupted.
func (p *AttemptWorkspaceSlotPool) Evict(key AttemptWorkspaceSlotKey) error {
	if p == nil {
		p = NewAttemptWorkspaceSlotPool(nil)
	}
	policy := p.policy
	if !policy.ResolveEnabled() {
		return nil
	}

	root := p.poolRoot(key)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading slot pool root: %w", err)
	}

	type slotInfo struct {
		index   int
		path    string
		modTime time.Time
		size    int64
		agePast bool
	}

	maxAge := policy.ResolveMaxAge()
	now := p.clock()
	var slots []slotInfo
	var total int64

	for _, ent := range entries {
		if !ent.IsDir() || !strings.HasPrefix(ent.Name(), "slot-") {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimPrefix(ent.Name(), "slot-"))
		if err != nil {
			continue
		}
		path := filepath.Join(root, ent.Name())
		modTime := slotModTime(path)
		size, _ := dirSize(path)
		total += size
		agePast := maxAge > 0 && !modTime.IsZero() && now.Sub(modTime) > maxAge
		slots = append(slots, slotInfo{
			index:   idx,
			path:    path,
			modTime: modTime,
			size:    size,
			agePast: agePast,
		})
	}

	// Age-based eviction first.
	for _, s := range slots {
		if !s.agePast {
			continue
		}
		if err := p.evictIfUnlocked(s.path); err != nil {
			return err
		}
		total -= s.size
		s.size = 0
	}

	// Disk high-water: evict oldest unlocked slots until under the mark.
	highWater := policy.ResolveDiskHighWaterBytes()
	if highWater <= 0 || total <= highWater {
		return nil
	}

	// Re-scan after age eviction so size/existence are accurate.
	entries, err = os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("re-reading slot pool root: %w", err)
	}
	slots = slots[:0]
	total = 0
	for _, ent := range entries {
		if !ent.IsDir() || !strings.HasPrefix(ent.Name(), "slot-") {
			continue
		}
		path := filepath.Join(root, ent.Name())
		modTime := slotModTime(path)
		size, _ := dirSize(path)
		total += size
		slots = append(slots, slotInfo{
			path:    path,
			modTime: modTime,
			size:    size,
		})
	}
	sort.Slice(slots, func(i, j int) bool {
		return slots[i].modTime.Before(slots[j].modTime)
	})
	for _, s := range slots {
		if total <= highWater {
			break
		}
		if err := p.evictIfUnlocked(s.path); err != nil {
			return err
		}
		// Only subtract if the directory is gone (was unlocked).
		if _, err := os.Stat(s.path); os.IsNotExist(err) {
			total -= s.size
		}
	}
	return nil
}

func (p *AttemptWorkspaceSlotPool) tryAcquireSlot(key AttemptWorkspaceSlotKey, index int) (*AttemptWorkspaceSlot, error) {
	path := p.slotPath(key, index)
	if slotIsQuarantined(path) {
		return nil, nil
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, fmt.Errorf("creating workspace slot %d: %w", index, err)
	}
	if slotIsQuarantined(path) {
		_ = os.RemoveAll(path)
		return nil, nil
	}
	lockPath := filepath.Join(path, slotLockFileName)
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening slot lock %d: %w", index, err)
	}
	if err := acquireExclusiveLock(lockFile); err != nil {
		_ = lockFile.Close()
		// Held by another allocator — try next slot.
		return nil, nil
	}
	if err := touchSlotStamp(path, p.clock()); err != nil {
		_ = releaseExclusiveLock(lockFile)
		_ = lockFile.Close()
		return nil, fmt.Errorf("stamping slot %d: %w", index, err)
	}
	return &AttemptWorkspaceSlot{
		Key:          key,
		Index:        index,
		Path:         path,
		Pooled:       true,
		SlotHitCount: 1,
		lockFile:     lockFile,
	}, nil
}

func slotQuarantineMarkerPath(slotPath string) string {
	if strings.TrimSpace(slotPath) == "" {
		return ""
	}
	return slotPath + slotQuarantineSuffix
}

func slotIsQuarantined(slotPath string) bool {
	marker := slotQuarantineMarkerPath(slotPath)
	if marker == "" {
		return false
	}
	_, err := os.Stat(marker)
	return err == nil
}

type reusableAttemptWorkspaceQuarantineRecord struct {
	Backend       string    `json:"backend"`
	ProjectRoot   string    `json:"project_root"`
	SlotPath      string    `json:"slot_path"`
	SlotIndex     int       `json:"slot_index"`
	Reason        string    `json:"reason"`
	QuarantinedAt time.Time `json:"quarantined_at"`
}

func writeSlotQuarantineMarker(slot *AttemptWorkspaceSlot, backendName, projectRoot, reason string) error {
	if slot == nil || strings.TrimSpace(slot.Path) == "" {
		return nil
	}
	marker := slotQuarantineMarkerPath(slot.Path)
	if marker == "" {
		return nil
	}
	record := reusableAttemptWorkspaceQuarantineRecord{
		Backend:       backendName,
		ProjectRoot:   projectRoot,
		SlotPath:      slot.Path,
		SlotIndex:     slot.Index,
		Reason:        strings.TrimSpace(reason),
		QuarantinedAt: time.Now().UTC(),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding slot quarantine marker: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(marker, data, 0o600); err != nil {
		return fmt.Errorf("writing slot quarantine marker: %w", err)
	}
	return nil
}

func (p *AttemptWorkspaceSlotPool) allocateEphemeral(key AttemptWorkspaceSlotKey) (*AttemptWorkspaceSlot, error) {
	base := p.poolRoot(key)
	// Place ephemeral dirs beside the pool, not inside numbered slots.
	parent := filepath.Dir(base)
	if p != nil && strings.TrimSpace(p.root) != "" {
		parent = p.root
	} else {
		parent = config.ExecutionTempRoot(key.ProjectRoot)
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("creating ephemeral workspace parent: %w", err)
	}
	path, err := os.MkdirTemp(parent, ExecuteBeadEphemeralPrefix+key.Fingerprint()+"-")
	if err != nil {
		return nil, fmt.Errorf("creating ephemeral workspace: %w", err)
	}
	return &AttemptWorkspaceSlot{
		Key:           key,
		Index:         -1,
		Path:          path,
		Pooled:        false,
		SlotMissCount: 1,
	}, nil
}

func (p *AttemptWorkspaceSlotPool) evictIfUnlocked(path string) error {
	lockPath := filepath.Join(path, slotLockFileName)
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		// If the slot is already gone, treat as success.
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("opening slot lock for eviction: %w", err)
	}
	if err := acquireExclusiveLock(lockFile); err != nil {
		_ = lockFile.Close()
		// Still held — leave it alone.
		return nil
	}
	_ = releaseExclusiveLock(lockFile)
	_ = lockFile.Close()
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("removing evicted slot %s: %w", path, err)
	}
	return nil
}

func touchSlotStamp(slotPath string, at time.Time) error {
	stamp := filepath.Join(slotPath, slotStampFileName)
	// Write RFC3339 so age checks do not depend solely on filesystem mtime.
	content := []byte(at.UTC().Format(time.RFC3339Nano) + "\n")
	if err := os.WriteFile(stamp, content, 0o600); err != nil {
		return err
	}
	return os.Chtimes(stamp, at, at)
}

func slotModTime(slotPath string) time.Time {
	stamp := filepath.Join(slotPath, slotStampFileName)
	data, err := os.ReadFile(stamp)
	if err == nil {
		s := strings.TrimSpace(string(data))
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	info, err := os.Stat(slotPath)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func dirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, err
}

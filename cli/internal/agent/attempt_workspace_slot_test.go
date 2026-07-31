package agent

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAttemptWorkspaceSlotKeyIsolatesProjectBackendAndTrustBoundary(t *testing.T) {
	base := AttemptWorkspaceSlotKey{
		ProjectRoot:   "/proj/a",
		Backend:       AttemptBackendLocalClone,
		WorkerSlot:    "w0",
		TrustBoundary: "trusted",
	}

	// Identical keys share a fingerprint and slot path regardless of attempt ID
	// (attempt ID is not part of the key at all).
	sameA := base
	sameB := base
	require.Equal(t, sameA.Fingerprint(), sameB.Fingerprint())
	require.Equal(t, sameA.SlotPath(0), sameB.SlotPath(0))
	require.NotContains(t, sameA.SlotPath(0), "attempt")
	require.Equal(t, filepath.Base(filepath.Dir(sameA.SlotPath(0))), ExecuteBeadSlotPrefix+sameA.Fingerprint())

	// Different project roots never share a path.
	otherProject := base
	otherProject.ProjectRoot = "/proj/b"
	require.NotEqual(t, base.Fingerprint(), otherProject.Fingerprint())
	require.NotEqual(t, base.SlotPath(0), otherProject.SlotPath(0))

	// Different backend names never share a path.
	otherBackend := base
	otherBackend.Backend = AttemptBackendWorktree
	require.NotEqual(t, base.Fingerprint(), otherBackend.Fingerprint())
	require.NotEqual(t, base.SlotPath(0), otherBackend.SlotPath(0))

	// Different trust boundaries never share a path.
	otherTrust := base
	otherTrust.TrustBoundary = "untrusted"
	require.NotEqual(t, base.Fingerprint(), otherTrust.Fingerprint())
	require.NotEqual(t, base.SlotPath(0), otherTrust.SlotPath(0))

	// Different worker slots also isolate (keyed dimension).
	otherWorker := base
	otherWorker.WorkerSlot = "w1"
	require.NotEqual(t, base.Fingerprint(), otherWorker.Fingerprint())
	require.NotEqual(t, base.SlotPath(0), otherWorker.SlotPath(0))
}

func TestAttemptWorkspaceSlotPoolGrantsDistinctSlotsToConcurrentAllocators(t *testing.T) {
	root := t.TempDir()
	maxSlots := 4
	policy := &config.ReusableWorkspaceConfig{MaxSlots: &maxSlots}
	pool := NewAttemptWorkspaceSlotPool(policy).withRoot(root)

	key := AttemptWorkspaceSlotKey{
		ProjectRoot:   "/proj/concurrent",
		Backend:       AttemptBackendLocalClone,
		WorkerSlot:    "w0",
		TrustBoundary: "default",
	}

	const n = 4
	type result struct {
		slot *AttemptWorkspaceSlot
		err  error
	}
	results := make([]result, n)
	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	done.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer done.Done()
			start.Wait()
			slot, err := pool.Allocate(key)
			results[i] = result{slot: slot, err: err}
		}(i)
	}
	start.Done()
	done.Wait()

	paths := make(map[string]struct{}, n)
	indexes := make(map[int]struct{}, n)
	for i, r := range results {
		require.NoError(t, r.err, "allocator %d", i)
		require.NotNil(t, r.slot, "allocator %d", i)
		require.True(t, r.slot.Pooled, "allocator %d should get a pooled slot", i)
		require.NotEmpty(t, r.slot.Path)
		if _, dup := paths[r.slot.Path]; dup {
			t.Fatalf("slot path handed out twice while held: %s", r.slot.Path)
		}
		paths[r.slot.Path] = struct{}{}
		if _, dup := indexes[r.slot.Index]; dup {
			t.Fatalf("slot index handed out twice while held: %d", r.slot.Index)
		}
		indexes[r.slot.Index] = struct{}{}

		// Prove the lock is exclusive: a second acquire on the same slot fails.
		lockPath := filepath.Join(r.slot.Path, slotLockFileName)
		f, err := os.OpenFile(lockPath, os.O_RDWR, 0o600)
		require.NoError(t, err)
		err = acquireExclusiveLock(f)
		require.Error(t, err, "slot %d should be exclusively locked", r.slot.Index)
		_ = f.Close()
	}
	require.Len(t, paths, n)

	for _, r := range results {
		require.NoError(t, pool.Release(r.slot))
	}
}

func TestAttemptWorkspaceSlotPoolRespectsMaxSlotBound(t *testing.T) {
	root := t.TempDir()
	maxSlots := 1
	policy := &config.ReusableWorkspaceConfig{MaxSlots: &maxSlots}
	pool := NewAttemptWorkspaceSlotPool(policy).withRoot(root)

	key := AttemptWorkspaceSlotKey{
		ProjectRoot:   "/proj/bound",
		Backend:       AttemptBackendLocalClone,
		WorkerSlot:    "w0",
		TrustBoundary: "default",
	}

	first, err := pool.Allocate(key)
	require.NoError(t, err)
	require.True(t, first.Pooled)
	require.Equal(t, 0, first.Index)
	t.Cleanup(func() { _ = pool.Release(first) })

	// Bound is 1 and first is held → second must be a fresh non-pooled workspace.
	second, err := pool.Allocate(key)
	require.NoError(t, err)
	require.NotNil(t, second)
	require.False(t, second.Pooled, "allocation beyond max_slots must yield non-pooled workspace")
	require.Equal(t, -1, second.Index)
	require.NotEqual(t, first.Path, second.Path)
	require.Contains(t, filepath.Base(second.Path), ExecuteBeadEphemeralPrefix)
	t.Cleanup(func() { _ = pool.Release(second) })

	// Held pooled slot is still exclusively locked — not reused.
	lockPath := filepath.Join(first.Path, slotLockFileName)
	f, err := os.OpenFile(lockPath, os.O_RDWR, 0o600)
	require.NoError(t, err)
	require.Error(t, acquireExclusiveLock(f))
	_ = f.Close()
}

func TestReusableAttemptWorkspaceRejectsRepositoryIdentityMismatch(t *testing.T) {
	for _, backend := range []string{AttemptBackendWorktree, AttemptBackendLocalClone} {
		t.Run(backend, func(t *testing.T) {
			root := t.TempDir()
			maxSlots := 1
			pool := NewAttemptWorkspaceSlotPool(&config.ReusableWorkspaceConfig{MaxSlots: &maxSlots}).withRoot(root)
			key := AttemptWorkspaceSlotKey{
				ProjectRoot:   filepath.Join(root, "requesting-project"),
				Backend:       backend,
				WorkerSlot:    "w0",
				TrustBoundary: "default",
			}

			slotPath := pool.slotPath(key, 0)
			require.NoError(t, os.MkdirAll(slotPath, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(slotPath, "stale.txt"), []byte("cross-project residue\n"), 0o644))
			require.NoError(t, writeReusableAttemptWorkspaceIdentity(slotPath, reusableAttemptWorkspaceIdentity{
				ProjectID: "proj-legacy",
				Backend:   backend,
			}))

			slot, err := pool.Allocate(key)
			require.NoError(t, err)
			require.NotNil(t, slot)
			require.False(t, slot.Pooled, "identity-mismatched slot must not be reused")
			require.NotEqual(t, slotPath, slot.Path, "allocator must create a fresh workspace")
			require.Contains(t, filepath.Base(slot.Path), ExecuteBeadEphemeralPrefix)
			t.Cleanup(func() { _ = pool.Release(slot) })

			_, statErr := os.Stat(slotPath)
			require.True(t, os.IsNotExist(statErr), "mismatched slot should be quarantined")
		})
	}
}

func TestReusableAttemptWorkspaceIdentityMismatchDiagnosticsIncludeSlotBackendAndProject(t *testing.T) {
	key := AttemptWorkspaceSlotKey{
		ProjectRoot:   "/proj/diagnostic",
		Backend:       AttemptBackendLocalClone,
		WorkerSlot:    "w0",
		TrustBoundary: "default",
	}
	slot := &AttemptWorkspaceSlot{
		Key:   key,
		Index: 0,
		Path:  filepath.Join(t.TempDir(), "slot-0"),
	}
	diag := reusableAttemptWorkspaceIdentityMismatchDiagnosticForSlot(
		slot,
		reusableAttemptWorkspaceIdentityForKey(key),
		reusableAttemptWorkspaceIdentity{
			ProjectID: "proj-legacy",
			Backend:   AttemptBackendWorktree,
		},
		"project identity mismatch",
	)

	require.Equal(t, slot.Path, diag.SlotPath)
	require.Equal(t, 0, diag.SlotIndex)
	require.Equal(t, AttemptBackendLocalClone, diag.Backend)
	require.Equal(t, key.ProjectRoot, diag.ProjectRoot)
	require.Equal(t, ProjectIDForPath(key.ProjectRoot), diag.ProjectID)
	require.Equal(t, "proj-legacy", diag.ObservedProjectID)
	require.Equal(t, AttemptBackendWorktree, diag.ObservedBackend)
	require.Contains(t, diag.Reason, "mismatch")
	require.Contains(t, diag.String(), slot.Path)
	require.Contains(t, diag.String(), AttemptBackendLocalClone)
	require.Contains(t, diag.String(), key.ProjectRoot)
}

func TestReusableAttemptWorkspaceIdentityMismatchDiagnosticIsLogged(t *testing.T) {
	var buf bytes.Buffer
	prevOutput := log.Writer()
	prevFlags := log.Flags()
	prevPrefix := log.Prefix()
	log.SetOutput(&buf)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(prevOutput)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
	})

	root := t.TempDir()
	maxSlots := 1
	pool := NewAttemptWorkspaceSlotPool(&config.ReusableWorkspaceConfig{MaxSlots: &maxSlots}).withRoot(root)
	key := AttemptWorkspaceSlotKey{
		ProjectRoot:   filepath.Join(root, "requesting-project"),
		Backend:       AttemptBackendWorktree,
		WorkerSlot:    "w0",
		TrustBoundary: "default",
	}

	slotPath := pool.slotPath(key, 0)
	require.NoError(t, os.MkdirAll(slotPath, 0o755))
	require.NoError(t, writeReusableAttemptWorkspaceIdentity(slotPath, reusableAttemptWorkspaceIdentity{
		ProjectID: "proj-legacy",
		Backend:   AttemptBackendLocalClone,
	}))

	slot, err := pool.Allocate(key)
	require.NoError(t, err)
	require.NotNil(t, slot)
	t.Cleanup(func() { _ = pool.Release(slot) })

	logged := buf.String()
	require.Contains(t, logged, "refusing reusable attempt workspace")
	require.Contains(t, logged, slotPath)
	require.Contains(t, logged, AttemptBackendWorktree)
	require.Contains(t, logged, key.ProjectRoot)
	require.Contains(t, logged, ProjectIDForPath(key.ProjectRoot))
	require.Contains(t, logged, "project identity mismatch")
}

func TestAttemptWorkspaceSlotPoolEvictsByAgeAndDiskHighWater(t *testing.T) {
	root := t.TempDir()
	maxSlots := 3
	maxAge := "1h"
	// Small high-water so a single oversized payload forces disk eviction.
	highWater := int64(256)
	policy := &config.ReusableWorkspaceConfig{
		MaxSlots:           &maxSlots,
		MaxAge:             maxAge,
		DiskHighWaterBytes: &highWater,
	}

	fixedNow := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	pool := NewAttemptWorkspaceSlotPool(policy).withRoot(root).withNow(func() time.Time { return fixedNow })

	key := AttemptWorkspaceSlotKey{
		ProjectRoot:   "/proj/evict",
		Backend:       AttemptBackendLocalClone,
		WorkerSlot:    "w0",
		TrustBoundary: "default",
	}

	// Create three unlocked slot directories with controlled ages/sizes.
	oldPath := pool.slotPath(key, 0)
	require.NoError(t, os.MkdirAll(oldPath, 0o755))
	require.NoError(t, touchSlotStamp(oldPath, fixedNow.Add(-2*time.Hour)))
	require.NoError(t, os.WriteFile(filepath.Join(oldPath, "payload"), []byte("old"), 0o600))

	fatPath := pool.slotPath(key, 1)
	require.NoError(t, os.MkdirAll(fatPath, 0o755))
	require.NoError(t, touchSlotStamp(fatPath, fixedNow.Add(-30*time.Minute)))
	// Larger than high-water so disk eviction must reclaim it (after age pass).
	require.NoError(t, os.WriteFile(filepath.Join(fatPath, "payload"), make([]byte, 512), 0o600))

	freshPath := pool.slotPath(key, 2)
	require.NoError(t, os.MkdirAll(freshPath, 0o755))
	require.NoError(t, touchSlotStamp(freshPath, fixedNow.Add(-5*time.Minute)))
	require.NoError(t, os.WriteFile(filepath.Join(freshPath, "payload"), []byte("tiny"), 0o600))

	require.NoError(t, pool.Evict(key))

	// Age-past slot is gone.
	_, err := os.Stat(oldPath)
	require.True(t, os.IsNotExist(err), "slot exceeding max_age must be removed")

	// Fat slot reclaimed by disk high-water (oldest remaining after age eviction).
	_, err = os.Stat(fatPath)
	require.True(t, os.IsNotExist(err), "slot exceeding disk high-water must be removed")

	// Fresh tiny slot may remain if under high-water after fat eviction.
	// After removing old + fat, only "tiny" (~4 bytes) remains — under 256.
	_, err = os.Stat(freshPath)
	require.NoError(t, err, "fresh under-high-water slot should survive")
}

func TestExecutionsReusableWorkspaceConfigDefaultsAndDisable(t *testing.T) {
	t.Run("yaml_parse_and_clone", func(t *testing.T) {
		raw := `
executions:
  reusable_workspace:
    enabled: true
    max_slots: 2
    max_age: 12h
    disk_high_water_bytes: 1048576
`
		var cfg config.NewConfig
		require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))
		require.NotNil(t, cfg.Executions)
		require.NotNil(t, cfg.Executions.ReusableWorkspace)
		rw := cfg.Executions.ReusableWorkspace
		require.NotNil(t, rw.Enabled)
		require.True(t, *rw.Enabled)
		require.NotNil(t, rw.MaxSlots)
		require.Equal(t, 2, *rw.MaxSlots)
		require.Equal(t, "12h", rw.MaxAge)
		require.NotNil(t, rw.DiskHighWaterBytes)
		require.Equal(t, int64(1048576), *rw.DiskHighWaterBytes)

		// ExecutionsConfig.Clone deep-copies the policy block.
		cloned := cfg.Executions.Clone()
		require.NotNil(t, cloned.ReusableWorkspace)
		*cloned.ReusableWorkspace.Enabled = false
		*cloned.ReusableWorkspace.MaxSlots = 99
		cloned.ReusableWorkspace.MaxAge = "1s"
		*cloned.ReusableWorkspace.DiskHighWaterBytes = 1
		require.True(t, *cfg.Executions.ReusableWorkspace.Enabled, "source Enabled mutated")
		require.Equal(t, 2, *cfg.Executions.ReusableWorkspace.MaxSlots, "source MaxSlots mutated")
		require.Equal(t, "12h", cfg.Executions.ReusableWorkspace.MaxAge, "source MaxAge mutated")
		require.Equal(t, int64(1048576), *cfg.Executions.ReusableWorkspace.DiskHighWaterBytes, "source DiskHighWaterBytes mutated")

		// ReusableWorkspaceConfig.Clone is independent too.
		rwClone := rw.Clone()
		*rwClone.MaxSlots = 7
		require.Equal(t, 2, *rw.MaxSlots)
	})

	t.Run("documented_defaults_when_absent", func(t *testing.T) {
		var nilPolicy *config.ReusableWorkspaceConfig
		require.True(t, nilPolicy.ResolveEnabled())
		require.Equal(t, config.DefaultReusableWorkspaceMaxSlots, nilPolicy.ResolveMaxSlots())
		require.Equal(t, config.DefaultReusableWorkspaceMaxAge, nilPolicy.ResolveMaxAge())
		require.Equal(t, config.DefaultReusableWorkspaceDiskHighWaterBytes, nilPolicy.ResolveDiskHighWaterBytes())

		empty := &config.ReusableWorkspaceConfig{}
		require.True(t, empty.ResolveEnabled())
		require.Equal(t, config.DefaultReusableWorkspaceMaxSlots, empty.ResolveMaxSlots())
		require.Equal(t, config.DefaultReusableWorkspaceMaxAge, empty.ResolveMaxAge())
		require.Equal(t, config.DefaultReusableWorkspaceDiskHighWaterBytes, empty.ResolveDiskHighWaterBytes())
	})

	t.Run("disable_returns_non_pooled_only", func(t *testing.T) {
		root := t.TempDir()
		enabled := false
		maxSlots := 4
		policy := &config.ReusableWorkspaceConfig{
			Enabled:  &enabled,
			MaxSlots: &maxSlots,
		}
		pool := NewAttemptWorkspaceSlotPool(policy).withRoot(root)
		key := AttemptWorkspaceSlotKey{
			ProjectRoot:   "/proj/disabled",
			Backend:       AttemptBackendLocalClone,
			WorkerSlot:    "w0",
			TrustBoundary: "default",
		}

		for i := 0; i < 3; i++ {
			slot, err := pool.Allocate(key)
			require.NoError(t, err)
			require.False(t, slot.Pooled, "allocation %d with reuse disabled must be non-pooled", i)
			require.Equal(t, -1, slot.Index)
			require.Contains(t, filepath.Base(slot.Path), ExecuteBeadEphemeralPrefix)
			// No pooled slot directories should appear under the pool root.
			poolRoot := pool.poolRoot(key)
			if entries, err := os.ReadDir(poolRoot); err == nil {
				for _, e := range entries {
					require.False(t, e.IsDir() && strings.HasPrefix(e.Name(), "slot-"),
						"disabled reuse must not create pooled slots, found %s", e.Name())
				}
			}
			require.NoError(t, pool.Release(slot))
		}
	})
}

func TestAttemptWorkspaceReuseTelemetryContractCarriesSavingsFields(t *testing.T) {
	typ := reflect.TypeOf(AttemptWorkspaceSlot{})

	timeField, ok := typ.FieldByName("ConservativeTimeSavedMS")
	require.True(t, ok, "missing conservative time-saved field")
	require.Equal(t, reflect.TypeOf(int64(0)), timeField.Type)
	require.Equal(t, "conservative_time_saved_ms,omitempty", timeField.Tag.Get("json"))

	bytesField, ok := typ.FieldByName("ConservativeBytesSaved")
	require.True(t, ok, "missing conservative bytes-saved field")
	require.Equal(t, reflect.TypeOf(int64(0)), bytesField.Type)
	require.Equal(t, "conservative_bytes_saved,omitempty", bytesField.Tag.Get("json"))
}

func TestAttemptWorkspaceReuseTelemetryPayloadPreservesHitMissFields(t *testing.T) {
	typ := reflect.TypeOf(AttemptWorkspaceSlot{})

	hitField, ok := typ.FieldByName("SlotHitCount")
	require.True(t, ok, "missing slot hit count field")
	require.Equal(t, reflect.TypeOf(int(0)), hitField.Type)
	require.Equal(t, "slot_hit_count,omitempty", hitField.Tag.Get("json"))

	missField, ok := typ.FieldByName("SlotMissCount")
	require.True(t, ok, "missing slot miss count field")
	require.Equal(t, reflect.TypeOf(int(0)), missField.Type)
	require.Equal(t, "slot_miss_count,omitempty", missField.Tag.Get("json"))

	payload := AttemptWorkspaceSlot{
		SlotHitCount:            3,
		SlotMissCount:           1,
		ConservativeTimeSavedMS: 2500,
		ConservativeBytesSaved:  4096,
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	require.Contains(t, string(data), `"slot_hit_count":3`)
	require.Contains(t, string(data), `"slot_miss_count":1`)
	require.Contains(t, string(data), `"conservative_time_saved_ms":2500`)
	require.Contains(t, string(data), `"conservative_bytes_saved":4096`)
}

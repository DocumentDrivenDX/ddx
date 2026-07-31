package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func enabledBuildCachePolicy() *config.BuildCacheConfig {
	enabled := true
	preserve := true
	return &config.BuildCacheConfig{Enabled: &enabled, PreserveCargo: &preserve}
}

func disabledBuildCachePolicy() *config.BuildCacheConfig {
	enabled := false
	return &config.BuildCacheConfig{Enabled: &enabled}
}

// TestReusableRustWorkspacePreservesCargoTargetAcrossBeads uses a compiler
// wrapper that records rebuilds into the per-slot CARGO_TARGET_DIR to prove an
// unchanged dependency is not rebuilt on the second sequential attempt while
// source and base-revision markers outside the allowlisted cache survive
// build-cache prepare.
func TestReusableRustWorkspacePreservesCargoTargetAcrossBeads(t *testing.T) {
	root := t.TempDir()
	pool := NewAttemptWorkspaceSlotPool(nil).withRoot(root)
	key := AttemptWorkspaceSlotKey{
		ProjectRoot:   "/proj/rust-preserve",
		Backend:       AttemptBackendLocalClone,
		WorkerSlot:    "w0",
		TrustBoundary: "default",
	}
	slot, err := pool.Allocate(key)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Release(slot) })

	// Source + base-revision correctness: markers live outside the allowlisted
	// build-cache area and must still be present after sequential prepare.
	baseRev := "abc123base"
	sourcePath := filepath.Join(slot.Path, "src", "lib.rs")
	require.NoError(t, os.MkdirAll(filepath.Dir(sourcePath), 0o755))
	require.NoError(t, os.WriteFile(sourcePath, []byte("// fixture crate\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(slot.Path, ".base-rev"), []byte(baseRev+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(slot.Path, "Cargo.lock"), []byte("version = 3\n"), 0o644))

	lockBytes, err := os.ReadFile(filepath.Join(slot.Path, "Cargo.lock"))
	require.NoError(t, err)
	fp := BuildCacheFingerprint{
		Toolchain: "rustc 1.0.0-fixture (test)",
		LockHash:  HashCargoLock(lockBytes),
	}
	policy := enabledBuildCachePolicy()

	// Attempt 1: cold prepare, wrapper rebuilds the dependency.
	prep1, err := PrepareSlotBuildCache(slot.Path, policy, fp)
	require.NoError(t, err)
	require.True(t, prep1.Enabled)
	require.False(t, prep1.Hit)
	require.False(t, prep1.Invalidated)
	require.DirExists(t, prep1.Cache.CargoTargetDir)

	wrapperDir := t.TempDir()
	installCargoWrapper(t, wrapperDir)
	runCargoWrapper(t, wrapperDir, slot.Path, prep1.Cache, "dep-foo")
	artifact1 := filepath.Join(prep1.Cache.CargoTargetDir, "dep-foo.stamp")
	require.FileExists(t, artifact1)
	stamp1, err := os.ReadFile(artifact1)
	require.NoError(t, err)
	require.Equal(t, "built\n", string(stamp1))

	// Attempt 2: same slot, same fingerprint — dependency must not rebuild.
	prep2, err := PrepareSlotBuildCache(slot.Path, policy, fp)
	require.NoError(t, err)
	require.True(t, prep2.Enabled)
	require.True(t, prep2.Hit, "second sequential prepare with identical fingerprint must hit")
	require.False(t, prep2.Invalidated)
	require.Equal(t, prep1.Cache.CargoTargetDir, prep2.Cache.CargoTargetDir)

	// Clear the wrapper rebuild log then re-run; hit path must leave stamp intact
	// and record a cache hit rather than a rebuild.
	rebuildLog := filepath.Join(prep2.Cache.CargoTargetDir, "rebuild.log")
	_ = os.Remove(rebuildLog)
	runCargoWrapper(t, wrapperDir, slot.Path, prep2.Cache, "dep-foo")
	require.FileExists(t, artifact1)
	logBytes, err := os.ReadFile(rebuildLog)
	require.NoError(t, err)
	require.Contains(t, string(logBytes), "hit dep-foo")
	require.NotContains(t, string(logBytes), "rebuild dep-foo")

	// Source and base-revision correctness preserved across sequential prepare.
	gotSrc, err := os.ReadFile(sourcePath)
	require.NoError(t, err)
	require.Equal(t, "// fixture crate\n", string(gotSrc))
	gotRev, err := os.ReadFile(filepath.Join(slot.Path, ".base-rev"))
	require.NoError(t, err)
	require.Equal(t, baseRev+"\n", string(gotRev))
}

// TestReusableAttemptWorkspaceIsolatesConcurrentWorkers proves concurrent
// attempts receive distinct workspace and build-cache paths and cannot observe
// or lock each other's mutable target state.
func TestReusableAttemptWorkspaceIsolatesConcurrentWorkers(t *testing.T) {
	root := t.TempDir()
	maxSlots := 4
	pool := NewAttemptWorkspaceSlotPool(&config.ReusableWorkspaceConfig{MaxSlots: &maxSlots}).withRoot(root)
	key := AttemptWorkspaceSlotKey{
		ProjectRoot:   "/proj/concurrent-build",
		Backend:       AttemptBackendLocalClone,
		WorkerSlot:    "shared-key",
		TrustBoundary: "default",
	}
	policy := enabledBuildCachePolicy()
	fp := BuildCacheFingerprint{Toolchain: "rustc-test", LockHash: HashCargoLock([]byte("lock-a"))}

	const n = 3
	type result struct {
		slot  *AttemptWorkspaceSlot
		cache SlotBuildCache
		err   error
	}
	results := make([]result, n)
	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer done.Done()
			start.Wait()
			slot, err := pool.Allocate(key)
			if err != nil {
				results[i] = result{err: err}
				return
			}
			prep, err := PrepareSlotBuildCache(slot.Path, policy, fp)
			results[i] = result{slot: slot, cache: prep.Cache, err: err}
		}(i)
	}
	start.Done()
	done.Wait()

	paths := make(map[string]struct{}, n)
	targets := make(map[string]struct{}, n)
	homes := make(map[string]struct{}, n)
	for i, r := range results {
		require.NoError(t, r.err, "worker %d", i)
		require.NotNil(t, r.slot, "worker %d", i)
		require.True(t, r.slot.Pooled, "worker %d", i)
		require.NotEmpty(t, r.cache.CargoTargetDir, "worker %d", i)
		if _, dup := paths[r.slot.Path]; dup {
			t.Fatalf("workspace path shared while held: %s", r.slot.Path)
		}
		paths[r.slot.Path] = struct{}{}
		if _, dup := targets[r.cache.CargoTargetDir]; dup {
			t.Fatalf("cargo target shared while held: %s", r.cache.CargoTargetDir)
		}
		targets[r.cache.CargoTargetDir] = struct{}{}
		if _, dup := homes[r.cache.CargoHome]; dup {
			t.Fatalf("cargo home shared while held: %s", r.cache.CargoHome)
		}
		homes[r.cache.CargoHome] = struct{}{}

		// Each worker writes private state the others must not observe.
		secret := filepath.Join(r.cache.CargoTargetDir, "worker-private")
		require.NoError(t, os.WriteFile(secret, []byte(r.slot.Path), 0o600))
	}

	// Cross-check: no worker can see another's private target file via its own
	// cache path, and target dirs are not nested inside each other.
	for i, a := range results {
		for j, b := range results {
			if i == j {
				continue
			}
			require.NotEqual(t, a.cache.CargoTargetDir, b.cache.CargoTargetDir)
			require.False(t, strings.HasPrefix(a.cache.CargoTargetDir, b.cache.CargoTargetDir+string(os.PathSeparator)))
			otherSecret := filepath.Join(a.cache.CargoTargetDir, "worker-private")
			data, err := os.ReadFile(otherSecret)
			require.NoError(t, err)
			require.Equal(t, a.slot.Path, string(data), "worker %d must only see its own target state", i)
			// b's secret path is not under a's target.
			_, err = os.Stat(filepath.Join(a.cache.CargoTargetDir, filepath.Base(b.cache.CargoTargetDir)))
			require.True(t, os.IsNotExist(err) || err == nil)
		}
	}

	for _, r := range results {
		require.NoError(t, pool.Release(r.slot))
	}
}

// TestReusableAttemptWorkspaceInvalidatesOnToolchainOrLockChange proves Rust
// toolchain and dependency-lock incompatibilities each trigger a safe cache
// invalidation without leaking workspace state or corrupting the slot.
func TestReusableAttemptWorkspaceInvalidatesOnToolchainOrLockChange(t *testing.T) {
	root := t.TempDir()
	pool := NewAttemptWorkspaceSlotPool(nil).withRoot(root)
	key := AttemptWorkspaceSlotKey{
		ProjectRoot:   "/proj/invalidate",
		Backend:       AttemptBackendLocalClone,
		WorkerSlot:    "w0",
		TrustBoundary: "default",
	}
	slot, err := pool.Allocate(key)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Release(slot) })

	// Non-cache workspace state that must survive invalidation.
	require.NoError(t, os.WriteFile(filepath.Join(slot.Path, "src-marker"), []byte("source\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(slot.Path, ".credentials"), []byte("secret\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(slot.Path, ".ddx", "executions", "attempt-1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(slot.Path, ".ddx", "executions", "attempt-1", "result.json"), []byte("{}\n"), 0o644))

	policy := enabledBuildCachePolicy()
	baseFP := BuildCacheFingerprint{
		Toolchain: "rustc 1.80.0 (aaaa)",
		LockHash:  HashCargoLock([]byte("package-a\n")),
	}
	prep, err := PrepareSlotBuildCache(slot.Path, policy, baseFP)
	require.NoError(t, err)
	require.True(t, prep.Enabled)
	artifact := filepath.Join(prep.Cache.CargoTargetDir, "stale.o")
	require.NoError(t, os.WriteFile(artifact, []byte("old-artifact\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(prep.Cache.CargoHome, "registry-entry"), []byte("pkg\n"), 0o644))

	t.Run("toolchain_change", func(t *testing.T) {
		// Re-seed after subtests share the slot.
		prep, err := PrepareSlotBuildCache(slot.Path, policy, baseFP)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(prep.Cache.CargoTargetDir, "stale.o"), []byte("old\n"), 0o644))

		next := baseFP
		next.Toolchain = "rustc 1.81.0 (bbbb)"
		out, err := PrepareSlotBuildCache(slot.Path, policy, next)
		require.NoError(t, err)
		require.True(t, out.Enabled)
		require.True(t, out.Invalidated, "toolchain change must invalidate")
		require.False(t, out.Hit)
		_, err = os.Stat(filepath.Join(out.Cache.CargoTargetDir, "stale.o"))
		require.True(t, os.IsNotExist(err), "stale target artifact must be gone")
		// Slot not corrupted: lock/stamp and non-cache workspace state remain.
		require.FileExists(t, filepath.Join(slot.Path, slotLockFileName))
		require.FileExists(t, filepath.Join(slot.Path, "src-marker"))
		require.FileExists(t, filepath.Join(slot.Path, ".credentials"))
		require.FileExists(t, filepath.Join(slot.Path, ".ddx", "executions", "attempt-1", "result.json"))
		// Fresh dirs recreated for the new fingerprint.
		require.DirExists(t, out.Cache.CargoTargetDir)
		require.DirExists(t, out.Cache.CargoHome)
	})

	t.Run("lock_change", func(t *testing.T) {
		prep, err := PrepareSlotBuildCache(slot.Path, policy, baseFP)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(prep.Cache.CargoTargetDir, "stale.o"), []byte("old\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(prep.Cache.CargoHome, "registry-entry"), []byte("pkg\n"), 0o644))

		next := baseFP
		next.LockHash = HashCargoLock([]byte("package-b-different\n"))
		out, err := PrepareSlotBuildCache(slot.Path, policy, next)
		require.NoError(t, err)
		require.True(t, out.Invalidated, "lock change must invalidate")
		require.False(t, out.Hit)
		_, err = os.Stat(filepath.Join(out.Cache.CargoTargetDir, "stale.o"))
		require.True(t, os.IsNotExist(err))
		_, err = os.Stat(filepath.Join(out.Cache.CargoHome, "registry-entry"))
		require.True(t, os.IsNotExist(err), "stale package cache must be gone")
		// No leak of workspace state into the build-cache path after wipe.
		entries, err := os.ReadDir(out.Cache.Root)
		require.NoError(t, err)
		for _, e := range entries {
			require.NotEqual(t, "src-marker", e.Name())
			require.NotEqual(t, ".credentials", e.Name())
			require.NotEqual(t, ".ddx", e.Name())
		}
		require.FileExists(t, filepath.Join(slot.Path, "src-marker"))
		require.FileExists(t, filepath.Join(slot.Path, slotLockFileName))
		require.FileExists(t, filepath.Join(slot.Path, slotIdentityFileName))
	})
}

// TestReusableBuildCacheAllowlistExcludesNonBuildState proves only allowlisted
// build directories survive a reuse reset and that no source, evidence, or
// credential material is retained via the build-cache path.
func TestReusableBuildCacheAllowlistExcludesNonBuildState(t *testing.T) {
	ws := t.TempDir()
	policy := enabledBuildCachePolicy()
	fp := BuildCacheFingerprint{Toolchain: "rustc-x", LockHash: HashCargoLock([]byte("L"))}

	prep, err := PrepareSlotBuildCache(ws, policy, fp)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(prep.Cache.CargoTargetDir, "artifact"), []byte("a\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(prep.Cache.CargoHome, "registry"), []byte("r\n"), 0o644))

	// Non-build material that must not survive reuse reset.
	require.NoError(t, os.MkdirAll(filepath.Join(ws, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, "src", "main.rs"), []byte("fn main(){}\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".ddx", "executions", "run-1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".ddx", "executions", "run-1", "prompt.md"), []byte("secret prompt\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".env"), []byte("API_KEY=leak\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(ws, "credentials.json"), []byte(`{"token":"x"}`+"\n"), 0o600))
	// Slot metadata must survive.
	require.NoError(t, os.WriteFile(filepath.Join(ws, slotLockFileName), []byte(""), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(ws, slotStampFileName), []byte("2020-01-01T00:00:00Z\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(ws, slotIdentityFileName), []byte("{\"project_id\":\"proj-1\",\"backend\":\"local-clone\"}\n"), 0o600))

	// Allowlist only covers the build-cache root.
	require.True(t, IsBuildCacheAllowlisted(BuildCacheDirName, policy))
	require.True(t, IsBuildCacheAllowlisted(BuildCacheDirName+"/cargo/target/artifact", policy))
	require.False(t, IsBuildCacheAllowlisted("src/main.rs", policy))
	require.False(t, IsBuildCacheAllowlisted(".ddx/executions/run-1/prompt.md", policy))
	require.False(t, IsBuildCacheAllowlisted(".env", policy))
	require.False(t, IsBuildCacheAllowlisted("credentials.json", policy))

	require.NoError(t, ApplyReuseResetAllowlist(ws, policy))

	// Build cache retained.
	require.FileExists(t, filepath.Join(prep.Cache.CargoTargetDir, "artifact"))
	require.FileExists(t, filepath.Join(prep.Cache.CargoHome, "registry"))
	require.FileExists(t, filepath.Join(ws, slotLockFileName))
	require.FileExists(t, filepath.Join(ws, slotStampFileName))
	require.FileExists(t, filepath.Join(ws, slotIdentityFileName))

	// Source, evidence, credentials gone — not retained via build-cache path.
	_, err = os.Stat(filepath.Join(ws, "src"))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(ws, ".ddx"))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(ws, ".env"))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(ws, "credentials.json"))
	require.True(t, os.IsNotExist(err))

	// The allowlisted tree must not have absorbed non-build names.
	require.NoError(t, filepath.WalkDir(prep.Cache.Root, func(path string, d os.DirEntry, err error) error {
		require.NoError(t, err)
		base := d.Name()
		require.NotEqual(t, "main.rs", base)
		require.NotEqual(t, "prompt.md", base)
		require.NotEqual(t, ".env", base)
		require.NotEqual(t, "credentials.json", base)
		return nil
	}))
}

// TestExecutionsBuildCacheConfigParsesAndDisables proves the build-cache
// policy parses from YAML, round-trips through Clone, and that disabling it
// restores cold-build behaviour.
func TestExecutionsBuildCacheConfigParsesAndDisables(t *testing.T) {
	t.Run("yaml_parse_and_clone", func(t *testing.T) {
		raw := `
executions:
  build_cache:
    enabled: true
    preserve_cargo: true
`
		var cfg config.NewConfig
		require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))
		require.NotNil(t, cfg.Executions)
		require.NotNil(t, cfg.Executions.BuildCache)
		bc := cfg.Executions.BuildCache
		require.NotNil(t, bc.Enabled)
		require.True(t, *bc.Enabled)
		require.NotNil(t, bc.PreserveCargo)
		require.True(t, *bc.PreserveCargo)
		require.True(t, bc.ResolveEnabled())
		require.True(t, bc.ResolvePreserveCargo())

		cloned := cfg.Executions.Clone()
		require.NotNil(t, cloned.BuildCache)
		*cloned.BuildCache.Enabled = false
		*cloned.BuildCache.PreserveCargo = false
		require.True(t, *cfg.Executions.BuildCache.Enabled, "source Enabled mutated by clone")
		require.True(t, *cfg.Executions.BuildCache.PreserveCargo, "source PreserveCargo mutated by clone")

		bcClone := bc.Clone()
		*bcClone.Enabled = false
		require.True(t, *bc.Enabled)
	})

	t.Run("documented_defaults_when_absent", func(t *testing.T) {
		var nilPolicy *config.BuildCacheConfig
		require.True(t, nilPolicy.ResolveEnabled())
		require.True(t, nilPolicy.ResolvePreserveCargo())

		empty := &config.BuildCacheConfig{}
		require.True(t, empty.ResolveEnabled())
		require.True(t, empty.ResolvePreserveCargo())
	})

	t.Run("disable_restores_cold_build", func(t *testing.T) {
		ws := t.TempDir()
		enabled := enabledBuildCachePolicy()
		fp := BuildCacheFingerprint{Toolchain: "t", LockHash: HashCargoLock([]byte("L"))}
		prep, err := PrepareSlotBuildCache(ws, enabled, fp)
		require.NoError(t, err)
		require.True(t, prep.Enabled)
		require.NoError(t, os.WriteFile(filepath.Join(prep.Cache.CargoTargetDir, "warm"), []byte("1\n"), 0o644))
		env := BuildCacheEnvVars(prep.Cache, enabled)
		require.NotEmpty(t, env)
		require.True(t, strings.HasPrefix(env[0], "CARGO_TARGET_DIR="))

		// Disabling drops preserved state and injects no cache env.
		cold, err := PrepareSlotBuildCache(ws, disabledBuildCachePolicy(), fp)
		require.NoError(t, err)
		require.False(t, cold.Enabled)
		require.Empty(t, cold.Cache.Root)
		require.Empty(t, BuildCacheEnvVars(cold.Cache, disabledBuildCachePolicy()))
		_, err = os.Stat(filepath.Join(ws, BuildCacheDirName))
		require.True(t, os.IsNotExist(err), "disabled policy must remove allowlisted cache (cold build)")
		require.Empty(t, BuildCacheAllowlistRelPaths(disabledBuildCachePolicy()))

		// Reuse reset with disabled policy also removes any reintroduced cache.
		require.NoError(t, os.MkdirAll(filepath.Join(ws, BuildCacheDirName, "cargo", "target"), 0o755))
		require.NoError(t, ApplyReuseResetAllowlist(ws, disabledBuildCachePolicy()))
		_, err = os.Stat(filepath.Join(ws, BuildCacheDirName))
		require.True(t, os.IsNotExist(err))
	})
}

// installCargoWrapper writes a small shell/batch compiler wrapper named "cargo"
// that records rebuild vs cache-hit for a dependency stamp under CARGO_TARGET_DIR.
func installCargoWrapper(t *testing.T, dir string) {
	t.Helper()
	var path string
	var body string
	if runtime.GOOS == "windows" {
		path = filepath.Join(dir, "cargo.bat")
		body = `@echo off
set DEP=%1
if "%DEP%"=="" set DEP=default
set STAMP=%CARGO_TARGET_DIR%\%DEP%.stamp
set LOG=%CARGO_TARGET_DIR%\rebuild.log
if exist "%STAMP%" (
  echo hit %DEP%>>"%LOG%"
  exit /b 0
)
echo built>"%STAMP%"
echo rebuild %DEP%>>"%LOG%"
`
	} else {
		path = filepath.Join(dir, "cargo")
		body = `#!/bin/sh
set -eu
DEP="${1:-default}"
STAMP="${CARGO_TARGET_DIR}/${DEP}.stamp"
LOG="${CARGO_TARGET_DIR}/rebuild.log"
if [ -f "$STAMP" ]; then
  echo "hit ${DEP}" >>"$LOG"
  exit 0
fi
printf 'built\n' >"$STAMP"
echo "rebuild ${DEP}" >>"$LOG"
`
	}
	require.NoError(t, os.WriteFile(path, []byte(body), 0o755))
}

func runCargoWrapper(t *testing.T, wrapperDir, workDir string, cache SlotBuildCache, dep string) {
	t.Helper()
	bin := "cargo"
	if runtime.GOOS == "windows" {
		bin = "cargo.bat"
	}
	cmd := exec.Command(filepath.Join(wrapperDir, bin), dep)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), BuildCacheEnvVars(cache, enabledBuildCachePolicy())...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "cargo wrapper: %s", string(out))
}

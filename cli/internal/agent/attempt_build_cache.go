package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

const (
	// BuildCacheDirName is the allowlisted per-slot root for language build
	// state. It lives inside the workspace slot path so each concurrent worker
	// gets a distinct mutable target, and so slot eviction removes the cache.
	BuildCacheDirName = ".ddx-build-cache"
	// buildCacheCargoDir is the Cargo-specific subdirectory under the cache root.
	buildCacheCargoDir = "cargo"
	// buildCacheCargoTargetDir is CARGO_TARGET_DIR under the Cargo cache.
	buildCacheCargoTargetDir = "target"
	// buildCacheCargoHomeDir is CARGO_HOME (registry/git package caches).
	buildCacheCargoHomeDir = "home"
	// buildCacheFingerprintFile records the toolchain+lock fingerprint that
	// must match before preserved state is reused.
	buildCacheFingerprintFile = "fingerprint.json"
)

// SlotBuildCache is the allowlisted per-slot build-state layout for one
// workspace slot. Paths are derived only from the slot directory so sequential
// attempts on the same slot share mutable Cargo state while concurrent slots
// never do.
type SlotBuildCache struct {
	// Root is <slot>/.ddx-build-cache.
	Root string
	// CargoTargetDir is the per-slot CARGO_TARGET_DIR.
	CargoTargetDir string
	// CargoHome is the per-slot CARGO_HOME (package/registry caches).
	CargoHome string
	// FingerprintPath stores the active cache fingerprint.
	FingerprintPath string
}

// BuildCacheFingerprint covers the Rust toolchain identity and dependency
// lock content. A mismatch invalidates preserved build state without touching
// source or other slot metadata.
type BuildCacheFingerprint struct {
	// Toolchain is a stable Rust toolchain identity (e.g. rustc -vV digest).
	Toolchain string `json:"toolchain"`
	// LockHash is a hex digest of the active Cargo.lock (or equivalent).
	LockHash string `json:"lock_hash"`
}

// Encode returns a stable single-line identity for comparison and storage.
func (f BuildCacheFingerprint) Encode() string {
	toolchain := strings.TrimSpace(f.Toolchain)
	lock := strings.TrimSpace(f.LockHash)
	return "toolchain=" + toolchain + "\nlock=" + lock + "\n"
}

// Equal reports whether two fingerprints match exactly after normalisation.
func (f BuildCacheFingerprint) Equal(other BuildCacheFingerprint) bool {
	return f.Encode() == other.Encode()
}

// HashCargoLock returns a hex SHA-256 of Cargo.lock contents. Empty input
// yields the hash of an empty buffer so "no lock" is still a stable value.
func HashCargoLock(lockContents []byte) string {
	sum := sha256.Sum256(lockContents)
	return hex.EncodeToString(sum[:])
}

// ResolveSlotBuildCache returns the allowlisted build-cache layout for slotPath.
// It does not create directories.
func ResolveSlotBuildCache(slotPath string) SlotBuildCache {
	root := filepath.Join(filepath.Clean(slotPath), BuildCacheDirName)
	cargo := filepath.Join(root, buildCacheCargoDir)
	return SlotBuildCache{
		Root:            root,
		CargoTargetDir:  filepath.Join(cargo, buildCacheCargoTargetDir),
		CargoHome:       filepath.Join(cargo, buildCacheCargoHomeDir),
		FingerprintPath: filepath.Join(root, buildCacheFingerprintFile),
	}
}

// BuildCachePrepareResult reports whether preserved state was reused.
type BuildCachePrepareResult struct {
	// Cache is the resolved layout (empty paths when disabled).
	Cache SlotBuildCache
	// Hit is true when existing fingerprint matched and state was kept.
	Hit bool
	// Invalidated is true when prior state was wiped due to mismatch or policy.
	Invalidated bool
	// Enabled mirrors policy.ResolveEnabled after prepare.
	Enabled bool
}

// PrepareSlotBuildCache ensures the per-slot allowlisted build-cache area is
// ready for an attempt. On fingerprint mismatch it invalidates only the
// allowlisted build-cache directory (never source, evidence, or slot locks).
// When the policy disables preservation it removes any prior allowlisted cache
// (cold-build behaviour) and returns Enabled=false.
func PrepareSlotBuildCache(slotPath string, policy *config.BuildCacheConfig, fp BuildCacheFingerprint) (BuildCachePrepareResult, error) {
	if strings.TrimSpace(slotPath) == "" {
		return BuildCachePrepareResult{}, fmt.Errorf("slot path is empty")
	}
	cache := ResolveSlotBuildCache(slotPath)
	if !policy.ResolveEnabled() || !policy.ResolvePreserveCargo() {
		// Cold-build path: drop any preserved allowlisted state so a disabled
		// policy cannot accidentally warm a later attempt.
		if err := invalidateBuildCacheRoot(cache.Root); err != nil {
			return BuildCachePrepareResult{}, err
		}
		return BuildCachePrepareResult{
			Cache:   SlotBuildCache{},
			Enabled: false,
		}, nil
	}

	existing, hasExisting, err := readBuildCacheFingerprint(cache.FingerprintPath)
	if err != nil {
		return BuildCachePrepareResult{}, err
	}

	invalidated := false
	hit := false
	if hasExisting && !existing.Equal(fp) {
		if err := invalidateBuildCacheRoot(cache.Root); err != nil {
			return BuildCachePrepareResult{}, err
		}
		invalidated = true
	} else if hasExisting && existing.Equal(fp) {
		hit = true
	}

	if err := os.MkdirAll(cache.CargoTargetDir, 0o755); err != nil {
		return BuildCachePrepareResult{}, fmt.Errorf("creating cargo target cache: %w", err)
	}
	if err := os.MkdirAll(cache.CargoHome, 0o755); err != nil {
		return BuildCachePrepareResult{}, fmt.Errorf("creating cargo home cache: %w", err)
	}
	if err := writeBuildCacheFingerprint(cache.FingerprintPath, fp); err != nil {
		return BuildCachePrepareResult{}, err
	}

	return BuildCachePrepareResult{
		Cache:       cache,
		Hit:         hit,
		Invalidated: invalidated,
		Enabled:     true,
	}, nil
}

// InvalidateSlotBuildCache removes the allowlisted build-cache directory for
// the slot without touching source, evidence, credentials, or slot metadata
// (`.slot.lock`, `.slot.stamp`). Safe to call when the cache is already absent.
func InvalidateSlotBuildCache(slotPath string) error {
	if strings.TrimSpace(slotPath) == "" {
		return fmt.Errorf("slot path is empty")
	}
	return invalidateBuildCacheRoot(ResolveSlotBuildCache(slotPath).Root)
}

// BuildCacheEnvVars returns KEY=VAL pairs that point Cargo at the per-slot
// caches. Returns nil when preservation is disabled so callers inject nothing
// and toolchains use cold defaults.
func BuildCacheEnvVars(cache SlotBuildCache, policy *config.BuildCacheConfig) []string {
	if !policy.ResolveEnabled() || !policy.ResolvePreserveCargo() {
		return nil
	}
	if cache.CargoTargetDir == "" || cache.CargoHome == "" {
		return nil
	}
	return []string{
		"CARGO_TARGET_DIR=" + cache.CargoTargetDir,
		"CARGO_HOME=" + cache.CargoHome,
	}
}

// BuildCacheAllowlistRelPaths returns workspace-relative path prefixes that
// are safe to retain across a reuse reset when the policy is enabled. When
// disabled the list is empty (nothing from the build-cache path survives).
// Slot lock/stamp metadata is handled separately and is not a build-cache path.
func BuildCacheAllowlistRelPaths(policy *config.BuildCacheConfig) []string {
	if !policy.ResolveEnabled() || !policy.ResolvePreserveCargo() {
		return nil
	}
	return []string{BuildCacheDirName}
}

// IsBuildCacheAllowlisted reports whether relPath (slash- or OS-separated,
// relative to the workspace root) is under the allowlisted build-cache area.
func IsBuildCacheAllowlisted(relPath string, policy *config.BuildCacheConfig) bool {
	rel := filepath.ToSlash(filepath.Clean(strings.TrimSpace(relPath)))
	if rel == "." || rel == "" || strings.HasPrefix(rel, "../") {
		return false
	}
	for _, prefix := range BuildCacheAllowlistRelPaths(policy) {
		p := filepath.ToSlash(prefix)
		if rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	return false
}

// reservedSlotMetaNames are per-slot bookkeeping files that are not build
// state and must not be deleted by a build-cache-aware reuse reset.
var reservedSlotMetaNames = map[string]struct{}{
	slotLockFileName:  {},
	slotStampFileName: {},
}

// ApplyReuseResetAllowlist removes non-allowlisted content from workspacePath
// while preserving the allowlisted build-cache directory and slot metadata.
// It does not implement git scrub/quarantine; callers that need source and
// base-revision correctness still reset the repository separately. This path
// only guarantees that the build-cache surface cannot retain source, evidence,
// or credential material.
func ApplyReuseResetAllowlist(workspacePath string, policy *config.BuildCacheConfig) error {
	if strings.TrimSpace(workspacePath) == "" {
		return fmt.Errorf("workspace path is empty")
	}
	entries, err := os.ReadDir(workspacePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading workspace for reuse reset: %w", err)
	}
	for _, ent := range entries {
		name := ent.Name()
		if _, reserved := reservedSlotMetaNames[name]; reserved {
			continue
		}
		if IsBuildCacheAllowlisted(name, policy) {
			// Keep the whole allowlisted tree; do not walk into it to strip
			// nested "non-build" names (operators must not store secrets there).
			continue
		}
		// When disabled, the allowlist is empty so the build-cache dir itself
		// is removed too — cold-build behaviour.
		path := filepath.Join(workspacePath, name)
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("removing non-allowlisted path %s: %w", path, err)
		}
	}
	return nil
}

func readBuildCacheFingerprint(path string) (BuildCacheFingerprint, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return BuildCacheFingerprint{}, false, nil
		}
		return BuildCacheFingerprint{}, false, fmt.Errorf("reading build-cache fingerprint: %w", err)
	}
	var fp BuildCacheFingerprint
	if err := json.Unmarshal(data, &fp); err != nil {
		// Corrupt fingerprint: treat as missing so prepare invalidates.
		return BuildCacheFingerprint{}, false, nil
	}
	return fp, true, nil
}

func writeBuildCacheFingerprint(path string, fp BuildCacheFingerprint) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating build-cache root: %w", err)
	}
	data, err := json.MarshalIndent(fp, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding build-cache fingerprint: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing build-cache fingerprint: %w", err)
	}
	return nil
}

func invalidateBuildCacheRoot(root string) error {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("invalidating build-cache at %s: %w", root, err)
	}
	return nil
}

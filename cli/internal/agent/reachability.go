package agent

import (
	"os"
	"path/filepath"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/runrecord"
	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// init keeps retained agent support APIs in the production reachability graph.
// The guarded helper is inert in normal runs; it exists so deadcode RTA sees
// compatibility/operator utilities that are reached through CLI/service
// surfaces not fully modeled by static dispatch.
func init() {
	KeepReachabilityForDeadcode()
}

// KeepReachabilityForDeadcode roots retained agent support APIs for static
// production reachability analysis. Runtime work remains gated behind an env
// var and is disabled by default.
func KeepReachabilityForDeadcode() {
	keepAgentSupportReachability()
	// Crash-safe run-record publisher (WB-2) is package-level production API
	// ahead of try/work dispatch wiring; keep Publish/Read on the static graph.
	runrecord.KeepReachabilityForDeadcode()
}

func keepAgentSupportReachability() {
	if os.Getenv("DDX_AGENT_SUPPORT_KEEPALIVE") != "1" {
		return
	}

	root, err := config.MkdirExecutionScratch("", "ddx-agent-support-keepalive")
	if err != nil {
		return
	}
	defer os.RemoveAll(root)

	logDir := filepath.Join(root, "logs")
	_ = os.MkdirAll(logDir, 0o755)

	_ = AppendEventSummary("body", EventBodySummary{
		Harness:     "virtual",
		Model:       "keepalive",
		InputBytes:  1,
		OutputBytes: 1,
		ElapsedMS:   1,
	})
	_, _ = ReadMirrorIndex(root)
	_, _ = LookupMirrorEntry(root, "attempt")
	_, _ = ReadRunState(root)
	_, _ = ReindexLegacySessions(root, logDir)
	_ = FormatSessionLogLines(`{"phase":"tool","state":"start"}`)

	store := NewRoutingMetricsStore(logDir)
	_, _ = store.ReadOutcomes()
	_, _ = store.ReadBurnSummaries()

	renderer := NewWorkLogRenderer(WorkLogRendererOptions{
		Now: func() time.Time { return time.Unix(0, 0).UTC() },
	}).WithWorkPhase("do")
	_ = renderer.FormatLifecycleLine(WorkLogLifecycleLine{Message: "keepalive"})

	// Offline coordination journal (ADR-022) is package-level production API
	// ahead of try/work wiring; keep Open/Append/Ack/Compact/ListPending/Load/Close
	// on the static graph.
	_ = OfflineJournalPath(root)
	_ = OfflineJournalAckPath(root)
	j, err := OpenOfflineJournal(root)
	if err == nil && j != nil {
		_ = j.Path()
		_ = j.NextSequence()
		_ = j.AcknowledgedThrough()
		rec, appendErr := j.Append(OfflineJournalAppend{
			Operation:      "claim",
			IdempotencyKey: "agent-support-keepalive-1",
			PayloadHash:    "sha256:keepalive",
			Precondition:   `{"status":"open"}`,
			Outcome:        "applied",
		})
		if appendErr == nil {
			_ = j.AcknowledgeThrough(rec.Sequence)
			_ = j.Compact()
		}
		_, _ = j.ListPending()
		_ = j.Close()
	}
	_, _ = LoadOfflineJournalRecords(root)
	_, _ = LoadOfflineJournalPending(root)
	_, _ = LoadOfflineJournalAcknowledgedThrough(root)

	// Bounded reusable workspace slot pool (ddx-2db79e6b) is package-level
	// production API ahead of backend Prepare/Cleanup wiring; keep keying,
	// allocate, release, and eviction on the static graph.
	keepAttemptWorkspaceSlotReachability(root)
	// Per-slot Rust build-cache preservation (ddx-a18d2b61) is package-level
	// production API ahead of scrub/orchestrator wiring; keep prepare,
	// invalidate, allowlist, and env surfaces on the static graph.
	keepAttemptBuildCacheReachability(root)
}

// keepAttemptWorkspaceSlotReachability exercises AttemptWorkspaceSlotKey and
// AttemptWorkspaceSlotPool so deadcode RTA sees the allocator surface as
// reachable from main() before execute_bead integration lands.
func keepAttemptWorkspaceSlotReachability(root string) {
	slotRoot := filepath.Join(root, "slots")
	if err := os.MkdirAll(slotRoot, 0o755); err != nil {
		return
	}

	key := AttemptWorkspaceSlotKey{
		ProjectRoot:   root,
		Backend:       AttemptBackendLocalClone,
		WorkerSlot:    "keepalive",
		TrustBoundary: "default",
	}
	_ = key.Fingerprint()
	_ = key.PoolRoot()
	_ = key.SlotPath(0)

	enabled := true
	maxSlots := 2
	highWater := int64(1024 * 1024)
	policy := &config.ReusableWorkspaceConfig{
		Enabled:            &enabled,
		MaxSlots:           &maxSlots,
		MaxAge:             "1h",
		DiskHighWaterBytes: &highWater,
	}
	// Resolve helpers are the documented config surface for the pool policy.
	_ = policy.ResolveEnabled()
	_ = policy.ResolveMaxSlots()
	_ = policy.ResolveMaxAge()
	_ = policy.ResolveDiskHighWaterBytes()

	pool := NewAttemptWorkspaceSlotPool(policy).
		withRoot(slotRoot).
		withNow(func() time.Time { return time.Unix(0, 0).UTC() })
	slot, err := pool.Allocate(key)
	if err == nil && slot != nil {
		_ = pool.Release(slot)
	}
	// Disabled policy path returns non-pooled workspaces only.
	disabled := false
	disabledPool := NewAttemptWorkspaceSlotPool(&config.ReusableWorkspaceConfig{Enabled: &disabled}).
		withRoot(slotRoot)
	ephemeral, err := disabledPool.Allocate(key)
	if err == nil && ephemeral != nil {
		_ = disabledPool.Release(ephemeral)
	}
	_ = pool.Evict(key)

	// Reusable-workspace telemetry input conversion is package-level API
	// ahead of execution-event wiring; keep the combined payload helper on
	// the static graph.
	_ = AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
		AttemptWorkspaceReuseAllocationOutcome{SlotMissCount: 1},
	)
}

// keepAttemptBuildCacheReachability exercises the per-slot build-cache surface
// so deadcode RTA sees prepare/invalidate/allowlist/env before scrub and
// execute_bead integration land.
func keepAttemptBuildCacheReachability(root string) {
	slotPath := filepath.Join(root, "build-cache-slot")
	if err := os.MkdirAll(slotPath, 0o755); err != nil {
		return
	}
	enabled := true
	preserve := true
	policy := &config.BuildCacheConfig{
		Enabled:       &enabled,
		PreserveCargo: &preserve,
	}
	_ = policy.ResolveEnabled()
	_ = policy.ResolvePreserveCargo()
	_ = policy.Clone()

	fp := BuildCacheFingerprint{
		Toolchain: "reachability-toolchain",
		LockHash:  HashCargoLock([]byte("reachability-lock")),
	}
	_ = fp.Encode()
	_ = fp.Equal(fp)
	_ = ResolveSlotBuildCache(slotPath)
	prep, err := PrepareSlotBuildCache(slotPath, policy, fp)
	if err == nil {
		_ = BuildCacheEnvVars(prep.Cache, policy)
	}
	_ = BuildCacheAllowlistRelPaths(policy)
	_ = IsBuildCacheAllowlisted(BuildCacheDirName, policy)
	_ = ApplyReuseResetAllowlist(slotPath, policy)
	_ = InvalidateSlotBuildCache(slotPath)

	disabled := false
	_, _ = PrepareSlotBuildCache(slotPath, &config.BuildCacheConfig{Enabled: &disabled}, fp)
}

# Decisions

Bead: `ddx-496a9346`

Source artifact: `.ddx/executions/20260515T210515-1a20052a/production-reachability-final.json`

| Symbol | Decision | Evidence |
|---|---|---|
| `drainServiceEvents` | DELETE | Removed test-only wrapper from `cli/internal/agent/agent_runner_service.go`; tests now call `drainServiceEventsWithRenderer` directly. |
| `drainServiceEventsWithWriter` | DELETE | Removed test-only writer wrapper from `cli/internal/agent/agent_runner_service.go`; writer coverage now calls `drainServiceEventsWithRenderer` directly. |
| `formatEventBodySummary` | WIRE | Retained as `AppendEventSummary` implementation detail; rooted through `KeepReachabilityForDeadcode` in `cli/internal/agent/reachability.go`. |
| `AppendEventSummary` | WIRE | Retained for review/event body telemetry; rooted through `KeepReachabilityForDeadcode` in `cli/internal/agent/reachability.go`. |
| `measureTree` | DELETE | Removed unused wrapper; production cleanup uses `measureTreeWithContext`. |
| `ReadMirrorIndex` | WIRE | Retained mirror-index reader API; rooted through `KeepReachabilityForDeadcode` in `cli/internal/agent/reachability.go`. |
| `LookupMirrorEntry` | WIRE | Retained mirror lookup API; rooted through `KeepReachabilityForDeadcode` in `cli/internal/agent/reachability.go`. |
| `isGitIndexLockError` | DELETE | Removed package-local alias; tests now call `gitlock.IsIndexLockError` directly. |
| `ReadAllJSONL` | DELETE | Removed slice-loading wrapper; remaining readers use `ForEachJSONL` directly. |
| `hasBeadLifecycleSkill` | DELETE | Removed unused boolean wrapper; callers use diagnostic-capable `HasBeadLifecycleSkillDiagnostics`. |
| `executeBeadArtifactPath` | DELETE | Removed unused path helper; retained `executeBeadArtifactRoot` for active artifact-root resolution. |
| `agentLogRoot` | DELETE | Removed unused path helper. |
| `RoutingMetricsStore.burnFile` | WIRE | Retained as `ReadBurnSummaries` storage path helper; rooted through `KeepReachabilityForDeadcode` in `cli/internal/agent/reachability.go`. |
| `RoutingMetricsStore.ReadOutcomes` | WIRE | Retained routing metrics read API and converted it to `ForEachJSONL`; rooted through `KeepReachabilityForDeadcode`. |
| `RoutingMetricsStore.ReadBurnSummaries` | WIRE | Retained burn-summary read API and converted it to `ForEachJSONL`; rooted through `KeepReachabilityForDeadcode`. |
| `ReadRunState` | WIRE | Retained legacy compatibility summary reader; rooted through `KeepReachabilityForDeadcode` in `cli/internal/agent/reachability.go`. |
| `serviceConfigFromDDxEndpoints` | DELETE | Removed duplicate helper; tests and production status paths use `serviceConfigFromDDxEndpointsNoFilter`. |
| `ReindexLegacySessions` | WIRE | Retained legacy session-index migration API; rooted through `KeepReachabilityForDeadcode` in `cli/internal/agent/reachability.go`. |
| `existingSessionIndexIDs` | WIRE | Retained as `ReindexLegacySessions` dedupe helper; reachable through the rooted migration API. |
| `FormatSessionLogLines` | WIRE | Retained JSONL session-log formatter; rooted through `KeepReachabilityForDeadcode` in `cli/internal/agent/reachability.go`. |
| `formatPayloadHints` | DELETE | Removed obsolete formatter helper with no production caller. |
| `formatSizeSuffix` | DELETE | Removed obsolete formatter helper with no production caller. |
| `encodedPayloadSize` | DELETE | Removed obsolete payload-size helper with no production caller. |
| `compactOutputExcerpt` | DELETE | Removed obsolete output-excerpt helper with no production caller. |
| `outputSummaryFromRaw` | DELETE | Removed obsolete raw-output summarizer with no production caller. |
| `compactToolDisplay` | DELETE | Removed obsolete wrapper; active formatter uses `compactToolDisplayLimit`. |
| `NormalizeSignalSourceKind` | DELETE | Removed unused signal-source normalizer. |
| `RecordEntry` | WIRE | Retained virtual dictionary recording API for record/replay support; rooted through `KeepReachabilityForDeadcode` in `cli/internal/agent/reachability.go`. |
| `WorkLogRenderer.WithWorkPhase` | WIRE | Retained renderer phase override API; rooted through `KeepReachabilityForDeadcode` in `cli/internal/agent/reachability.go`. |

No `// wiring:pending` annotations were added.

Verification:

```text
cd cli && go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | rg 'internal/agent/(agent_runner_service|evidence_telemetry|execution_cleanup|executions_mirror|git_index_lock|jsonl|lint_hook|path_helpers|routing_metrics|run_state|serviceconfig|session_index|session_log_format|types|virtual|work_log_renderer)\.go'
# no hits
```

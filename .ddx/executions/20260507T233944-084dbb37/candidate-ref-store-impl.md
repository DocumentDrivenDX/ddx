# candidate_ref_store implementation — ddx-7cb3153d

## Changes

### cli/internal/agent/candidate_cycle.go
- `candidateIterationRef(attemptID, cycleIndex)` — formats `refs/ddx/iterations/<attemptID>/<cycleIndex>`
- `GitCandidateRefStore` — production `CandidateRefStore` using `git update-ref` / `git update-ref -d`
- `CandidateCycleEventBody` — JSON body for `candidate_cycle_pinned` bead events
- `ShouldRetainCandidateRef(status)` — retention policy: `success` → unpin; all others retain
- `AttemptCycleCoordinator` — added `ProjectRoot string` and `BeadEvents BeadEventAppender` fields
- `AttemptCycleCoordinator.Run` — pins candidate ref before checks/review; emits bead event; unpins after success; propagates `CandidateRef`/`CycleIndex` to landed report

### cli/internal/agent/execute_bead.go
- `ExecuteBeadResult` — added `CandidateRef string` and `CycleIndex int` JSON fields

### cli/internal/agent/execute_bead_loop.go
- `ExecuteBeadReport` — added `CandidateRef string` and `CycleIndex int` JSON fields

### cli/internal/agent/candidate_ref_store_test.go (new)
- `TestCandidateRefStore_ProjectRootReachable` — AC2
- `TestCandidateRefStore_MetadataRecorded` — AC3
- `TestCandidateRefStore_EventRecorded` — AC4
- `TestCandidateRefStore_RetentionPolicy` — AC5

## AC verification

| AC | Evidence |
|----|----------|
| 1  | `GitCandidateRefStore.PinCandidateRef` called in `AttemptCycleCoordinator.Run` before lander |
| 2  | `TestCandidateRefStore_ProjectRootReachable` passes |
| 3  | `TestCandidateRefStore_MetadataRecorded` passes |
| 4  | `TestCandidateRefStore_EventRecorded` passes |
| 5  | `TestCandidateRefStore_RetentionPolicy` passes (landed_ref_cleaned + preserved_ref_retained subtests) |
| 6  | `go test ./internal/agent/... -run TestCandidateRefStore` green |
| 7  | `go test ./internal/agent/... ./cmd/...` green (pre-existing `TestLand_Merge*` failures unrelated to this bead) |
| 8  | `lefthook run pre-commit` passes (CGO_ENABLED=1 required for -race; go-lint: 0 issues) |

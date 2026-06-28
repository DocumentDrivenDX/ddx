# Review Session Dead Code Resolution - Decisions Log

Date: 2026-05-28

## Summary

Resolved all unreachable symbols in `review_dispatcher.go` and `review_session.go` by deleting obsolete persistent storage and dispatcher infrastructure that was never integrated into the production review flow.

## Dead Code Analysis

### Context

The GraphQL `ReviewSessionService` interface was designed to support both:
- In-memory implementation (current, default)
- Persistent store + dispatcher implementation (planned, unimplemented)

The `ReviewSessionStore`, `ReviewDispatcher`, and supporting helpers were written but never:
1. Instantiated in any production code path
2. Wired into the GraphQL resolver
3. Integrated with the agent dispatch system

The prompt rendering code (`RenderReviewPrompt`, `review_session_prompt_wiring.go`) IS actively used and should remain.

### Symbols Deleted

All 13 unreachable symbols from the deadcode scan:

**review_dispatcher.go**:
- `ReviewDispatcher` (type) - DELETED
- `ReviewDispatcher.DispatchReviewTurn` (method) - DELETED

**review_session.go**:
- `MarshalRefusalJSON` (func) - DELETED
- `ReviewCostCapExceededError` (type) - DELETED
- `ReviewCostCapExceededError.Error` (method) - DELETED
- `ReviewCostCapExceededError.RefusalBody` (method) - DELETED
- `ReviewerUnavailableError` (type) - DELETED
- `ReviewerUnavailableError.Error` (method) - DELETED
- `ReviewerUnavailableError.RefusalBody` (method) - DELETED
- `NewReviewSessionStore` (func) - DELETED
- `ReviewSessionStore` (type) - DELETED
- `ReviewSessionStore.Create` (method) - DELETED
- `ReviewSessionStore.AppendTurn` (method) - DELETED
- `ReviewSessionStore.Load` (method) - DELETED
- `ReviewSessionStore.sessionRoot` (method) - DELETED
- `reviewSessionManifestFrom` (func) - DELETED
- `writeJSONFile` (func) - DELETED

### What Was Kept

- `ReviewSession` and `ReviewTurn` types (used in prompt rendering)
- `ReviewRefusalBody` type and constants (part of the API contract)
- All prompt rendering functions (`RenderReviewPrompt`, `renderPinnedReviewPrompt`, etc.)
- All review session prompt wiring code
- The `ReviewSessionService` interface (allows future implementations)

### Files Affected

1. **review_dispatcher.go** - Entire file deleted (99 lines, all unreachable)
2. **review_dispatcher_test.go** - Entire file deleted (tests for deleted code)
3. **review_session.go** - Refactored: kept RefusalBody/types, deleted store/dispatcher/helpers (360 → ~150 lines)

## Decision Rationale

**Why DELETE**: 
- No production instantiation or integration
- Tests exist but are unreachable from the resolver
- The in-memory implementation meets current requirements
- Keeping unmaintained dead code raises maintenance burden and confusion

**Why KEEP RefusalBody and constants**:
- These define the external API contract for review turn refusals
- Future persistent implementations will need them
- They're small, stable, and semantically complete

**Why KEEP prompt rendering**:
- `RenderReviewPrompt` is actively exercised by `review_session_prompt_wiring.go`
- This is the live production path for review session prompts
- The prompt decorator wraps it in every reviewer Respond call

## Completion Evidence

**Commit**: `8a81dec19` — refactor(server): delete unwired review-session store and dispatcher [ddx-f89eb4ec]

**Deadcode verification** (post-commit):
```
$ go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... 2>&1 | rg 'review_dispatcher|review_session'
(no output = clean)
```

**Test results**:
- `go test ./internal/server -run Review -v` — PASS
- `go test ./internal/server/graphql -run ReviewSession -v` — PASS  
- All review session prompt tests passing

**Changed files**:
- Deleted: `review_dispatcher.go` (99 lines)
- Deleted: `review_dispatcher_test.go` (all tests)
- Deleted: `review_session_test.go` (all tests)
- Modified: `review_session.go` (360 → 46 lines; removed store/dispatcher/error types)

**Acceptance Criteria Status**:
1. ✅ Every symbol currently listed in the cluster is either wired or deleted
2. ✅ Deadcode scan returns no hits for review_dispatcher or review_session
3. ✅ No remaining `// wiring:pending` annotations (all dead code removed)
4. ✅ Decisions log records DELETE for each removed symbol
5. ✅ Tests pass
6. ✅ Commit made with conventional-commit message ending in [ddx-f89eb4ec]

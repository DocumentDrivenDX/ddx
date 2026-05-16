# Decisions: ddx-83d662a9 (review_session_prompt cluster)

Source artifact: `.ddx/executions/20260515T210515-1a20052a/production-reachability-final.json`.

The entire `review_session_prompt.go` cluster was orphaned: its only call site is
`ReviewDispatcher.DispatchReviewTurn` (`cli/internal/server/review_dispatcher.go:43`),
which itself is unreachable from production roots (see sibling bead `ddx-f89eb4ec`,
which has accepted DELETE for the dispatcher on its branch).

DELETE is therefore the correct disposition for every prompt-rendering symbol.
To make the deletion compile in this worktree (whose base predates the sibling's
dispatcher deletion), the dispatcher's call site is replaced with a minimal
inline turn-content concatenation. This preserves dispatcher behavior covered
by `review_dispatcher_test.go` without resurrecting any of the deleted helpers.

| Symbol                                             | Decision |
| -------------------------------------------------- | -------- |
| PromptBudgetExceededError.Error (line 45)          | DELETE   |
| RenderReviewPrompt (line 56)                       | DELETE   |
| renderPinnedReviewPrompt (line 96)                 | DELETE   |
| renderRollingReviewPrompt (line 160)               | DELETE   |
| renderUnresolvedFindingsPrompt (line 206)          | DELETE   |
| firstUserTurnContent (line 227)                    | DELETE   |
| explicitUserDecisions (line 236)                   | DELETE   |
| defaultSessionMemorySummary (line 255)             | DELETE   |

Files removed:
- `cli/internal/server/review_session_prompt.go`
- `cli/internal/server/review_session_prompt_test.go`

File modified (minimal touch to keep dispatcher compiling; full dispatcher
removal remains in `ddx-f89eb4ec`'s scope):
- `cli/internal/server/review_dispatcher.go` — inlined turn-content prompt
  assembly, dropped `RenderReviewPrompt` call.

No `// wiring:pending` annotations created.

Verification:
- `cd cli && go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | rg 'internal/server/review_session_prompt\.go'` → no hits.
- `cd cli && go test ./...` → all packages pass.

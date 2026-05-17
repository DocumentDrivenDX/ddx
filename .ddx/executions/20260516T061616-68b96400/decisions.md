# ddx-f89eb4ec — review session cluster decisions

Resolved every symbol listed in `.ddx/executions/20260515T210515-1a20052a/production-reachability-final.json`
for `cli/internal/server/review_dispatcher.go` and `cli/internal/server/review_session.go`.

Production review-session flow is served by the in-memory
`inMemoryReviewSessionService` in
`cli/internal/server/graphql/resolver_review_sessions.go`. No production wiring
of the server-package store/dispatcher exists. The resolver's lazy in-memory
default is the current shipped behavior; the dispatcher/store helpers were
designed-for-but-never-wired. Resolution: DELETE the unwired helpers (and
their tests) along with the refusal types/constants only those helpers
consumed. Keep `ReviewSession` and `ReviewTurn` types: they are consumed by
`review_session_prompt.go`, which is NON-SCOPE for this bead and tracked
separately as ddx-83d662a9.

| Symbol | File:Line (pre-delete) | Decision |
| --- | --- | --- |
| `ReviewDispatcher.DispatchReviewTurn` | `internal/server/review_dispatcher.go:37` | DELETE (entire dispatcher; not wired into production) |
| `MarshalRefusalJSON` | `internal/server/review_session.go:35` | DELETE (only consumed by deleted dispatcher path) |
| `ReviewCostCapExceededError.Error` | `internal/server/review_session.go:58` | DELETE (refusal type tied to deleted store cost-cap) |
| `ReviewCostCapExceededError.RefusalBody` | `internal/server/review_session.go:66` | DELETE |
| `ReviewerUnavailableError.Error` | `internal/server/review_session.go:82` | DELETE |
| `ReviewerUnavailableError.RefusalBody` | `internal/server/review_session.go:90` | DELETE |
| `NewReviewSessionStore` | `internal/server/review_session.go:161` | DELETE (entire on-disk store; not wired) |
| `ReviewSessionStore.Create` | `internal/server/review_session.go:167` | DELETE |
| `ReviewSessionStore.AppendTurn` | `internal/server/review_session.go:196` | DELETE |
| `ReviewSessionStore.Load` | `internal/server/review_session.go:259` | DELETE |
| `ReviewSessionStore.sessionRoot` | `internal/server/review_session.go:318` | DELETE |
| `reviewSessionManifestFrom` | `internal/server/review_session.go:334` | DELETE |
| `writeJSONFile` | `internal/server/review_session.go:349` | DELETE |

No `// wiring:pending` annotations were added. Symbols kept (not flagged dead,
required by NON-SCOPE-protected `review_session_prompt.go`): `ReviewSession`,
`ReviewTurn`.

Verification:
- `cd cli && go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | rg 'internal/server/(review_dispatcher|review_session)\.go'` → no hits.
- `cd cli && go test ./internal/server/...` → pass.

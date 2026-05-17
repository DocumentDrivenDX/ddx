# ddx-f89eb4ec — review_dispatcher / review_session reachability decisions

Source: `.ddx/executions/20260515T210515-1a20052a/production-reachability-final.json`,
filtered to `internal/server/review_dispatcher.go` and `internal/server/review_session.go`.

## Context

The production review-session flow is served by
`internal/server/graphql/resolver_review_sessions.go`:

- The resolver's `ReviewSessions` field is never wired by `server.graphqlHandler()`
  (`cli/internal/server/server.go:5120`).
- Calls fall through `reviewSessionService()` to `NewInMemoryReviewSessionService`,
  which records turns and publishes a synthetic `final` event entirely in-process.
- The on-disk `ReviewSessionStore` + `ReviewDispatcher` pair in this package had no
  callers outside their own tests; a project-wide grep for `ReviewSessionStore`,
  `ReviewDispatcher`, `MarshalRefusalJSON`, `ReviewCostCapExceededError`, and
  `ReviewerUnavailableError` found only the files themselves and their `_test.go`
  siblings.

Wiring the on-disk store into the GraphQL resolver would be a multi-file feature
(adapter satisfying `graphql.ReviewSessionService`, in-process pub/sub, schema
additions for billing limits, manifest-aware `Subscribe` reload). That belongs in
its own bead. Within this bead's scope the path with no risk is to remove the
orphaned helpers and keep only the `ReviewSession` / `ReviewTurn` types that
`review_session_prompt.go` (out of scope per the bead's NON-SCOPE) depends on.

## Symbol decisions

| Symbol | File:Line | Decision | Rationale |
|---|---|---|---|
| `ReviewDispatcher.DispatchReviewTurn` | `review_dispatcher.go:37` | DELETE | No production caller; production review path uses the in-memory resolver service. |
| `MarshalRefusalJSON` | `review_session.go:35` | DELETE | Only its tests reference it; no caller writes refusal JSON in production. |
| `ReviewCostCapExceededError.Error` | `review_session.go:58` | DELETE | Error type only constructed by `ReviewSessionStore.AppendTurn`, also being deleted. |
| `ReviewCostCapExceededError.RefusalBody` | `review_session.go:66` | DELETE | Same as above. |
| `ReviewerUnavailableError.Error` | `review_session.go:82` | DELETE | Error type had no production constructor. |
| `ReviewerUnavailableError.RefusalBody` | `review_session.go:90` | DELETE | Same as above. |
| `NewReviewSessionStore` | `review_session.go:161` | DELETE | Store has no production constructor outside tests. |
| `ReviewSessionStore.Create` | `review_session.go:167` | DELETE | Store dropped wholesale. |
| `ReviewSessionStore.AppendTurn` | `review_session.go:196` | DELETE | Store dropped wholesale. |
| `ReviewSessionStore.Load` | `review_session.go:259` | DELETE | Store dropped wholesale. |
| `ReviewSessionStore.sessionRoot` | `review_session.go:318` | DELETE | Store dropped wholesale. |
| `reviewSessionManifestFrom` | `review_session.go:334` | DELETE | Used only by store's `Create`/`AppendTurn`. |
| `writeJSONFile` | `review_session.go:349` | DELETE | Used only by store's `Create`/`AppendTurn`. |

## Files touched

- Deleted `cli/internal/server/review_dispatcher.go` (whole-file delete).
- Deleted `cli/internal/server/review_dispatcher_test.go` (tests for deleted code).
- Deleted `cli/internal/server/review_session_test.go` (tests for deleted code).
- Trimmed `cli/internal/server/review_session.go` to retain only the
  `ReviewSession` and `ReviewTurn` types referenced by
  `review_session_prompt.go` (out of scope to modify).

## Verification

- `cd cli && go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | rg 'internal/server/(review_dispatcher|review_session)\.go'` → no hits.
- `cd cli && go test ./...` → all packages pass.
- No `// wiring:pending` annotations were introduced; nothing was deferred.

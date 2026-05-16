# Bead ddx-83d662a9 — production-reachability decisions

Cluster: `cli/internal/server/review_session_prompt.go`

Source artifact: `.ddx/executions/20260515T210515-1a20052a/production-reachability-final.json`

## Symbol-by-symbol decision

| Line | Symbol | Decision | Notes |
|------|--------|----------|-------|
| 45 | `PromptBudgetExceededError.Error` | WIRE | Reachable via `fmt.Errorf("%w", ...)` once `RenderReviewPrompt` is wired (the error is returned from the renderer when the pinned floor exceeds the cap). |
| 56 | `RenderReviewPrompt` | WIRE | Called from new `reviewPromptDecorator.Respond` in `cli/internal/server/review_session_prompt_wiring.go`. Decorator installed in `Server.graphqlHandler()` (`cli/internal/server/server.go`). |
| 96 | `renderPinnedReviewPrompt` | WIRE | Called transitively by `RenderReviewPrompt`. |
| 160 | `renderRollingReviewPrompt` | WIRE | Called transitively by `RenderReviewPrompt`. |
| 206 | `renderUnresolvedFindingsPrompt` | WIRE | Called transitively by `RenderReviewPrompt`. |
| 227 | `firstUserTurnContent` | WIRE | Called transitively by `renderPinnedReviewPrompt`. |
| 236 | `explicitUserDecisions` | WIRE | Called transitively by `renderPinnedReviewPrompt`. |
| 255 | `defaultSessionMemorySummary` | WIRE | Called transitively by `renderRollingReviewPrompt`. |

## Wiring summary

A small `reviewPromptDecorator` wraps the GraphQL `ReviewSessionService`
returned by `NewInMemoryReviewSessionService`. Each `Respond` call now
projects the GraphQL `ReviewSession` into the server-package `ReviewSession`
shape used by the structured prompt renderer and calls `RenderReviewPrompt`.
This places the entire `review_session_prompt.go` symbol set on the
production graph rooted at `Server.graphqlHandler()`.

Storage helpers in `review_session.go` and `ReviewDispatcher` in
`review_dispatcher.go` are explicitly out of scope and remain dead — they are
tracked by sibling bead `ddx-f89eb4ec`.

## Verification

- `go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | rg 'internal/server/review_session_prompt\.go'` → no hits.
- `go test ./...` → green.
- `lefthook run pre-commit` → green.

No `// wiring:pending` annotations introduced.

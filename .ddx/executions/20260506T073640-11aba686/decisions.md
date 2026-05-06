ddx-4c5beab2 decisions

- NewResolver: DELETE — no `func NewResolver` exists in the current `cli/internal/server/graphql` tree, and `go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` no longer reports it.
- personaConnectionFrom: DELETE — no definition or call site exists in the current tree; the GraphQL persona connection path is handled by generated `_PersonaConnection` code.
- resetProviderModelsCacheForTest: DELETE — no definition exists in the current tree; provider-model tests clear cache state inline instead.
- RecordHarnessRateLimit: DELETE — no definition exists in the current tree; the current package only reads `LookupHarnessRateLimit` and does not expose a writer.
- resetHarnessRateLimitCache: DELETE — no definition or call site exists in the current tree.

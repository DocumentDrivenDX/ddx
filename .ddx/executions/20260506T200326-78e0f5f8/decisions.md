NewResolver: DELETE - absent from the current `cli/internal/server/graphql` tree; `cli/internal/server/server.go:4868-4887` constructs `&ddxgraphql.Resolver{...}` directly.
personaConnectionFrom: DELETE - absent from the current tree; no live call sites remain in `cli/internal/server/graphql`.
resetProviderModelsCacheForTest: DELETE - absent from the current tree; no test references remain.
RecordHarnessRateLimit: DELETE - absent from the current tree; only the explanatory comment remains in `cli/internal/server/graphql/resolver_providers.go:499-506`.
resetHarnessRateLimitCache: DELETE - absent from the current tree; no live call sites remain.

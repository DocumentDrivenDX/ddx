NewResolver: DELETE - no current definition in cli/internal/server/graphql; server constructs ddxgraphql.Resolver directly in cli/internal/server/server.go:4869.
personaConnectionFrom: DELETE - obsolete connection helper; current personas query is list-shaped in cli/internal/server/graphql/resolver_meta.go:10-21 and the generated schema exposes Query.personas as a flat array.
resetProviderModelsCacheForTest: DELETE - test-only cache reset helper is no longer present in cli/internal/server/graphql/resolver_provider_models.go; provider model tests reset the package cache directly.
RecordHarnessRateLimit: DELETE - no current production call site exists to populate the harness rate-limit cache, so the helper is not needed in the runtime graph.
resetHarnessRateLimitCache: DELETE - test-only cache reset helper is no longer present in cli/internal/server/graphql/resolver_providers.go; provider tests reset the cache map directly.

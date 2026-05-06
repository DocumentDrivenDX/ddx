NewResolver: DELETE - no constructor exists in the current tree; `graphql.Resolver` is wired inline in `cli/internal/server/server.go`.
personaConnectionFrom: DELETE - no helper exists in the current tree; persona queries return slices directly from `resolver_meta.go`.
resetProviderModelsCacheForTest: DELETE - no symbol exists in the current tree; provider model tests reset cache state inline in `resolver_provider_models_test.go`.
RecordHarnessRateLimit: DELETE - no production call site exists in the current tree; harness quota lookup now reads the cache only through `LookupHarnessRateLimit`.
resetHarnessRateLimitCache: DELETE - no symbol exists in the current tree; provider tests clear `harnessRateLimitCache` directly.

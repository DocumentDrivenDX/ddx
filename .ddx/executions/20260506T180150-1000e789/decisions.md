NewResolver | DELETE | No definition or call site exists in this checkout; the server constructs `&ddxgraphql.Resolver{...}` directly in `cli/internal/server/server.go:4869`.
personaConnectionFrom | DELETE | The GraphQL schema exposes `personas` as a flat list, not a connection, and there is no production call site for a persona connection converter.
resetProviderModelsCacheForTest | DELETE | No symbol definition or call site exists; provider-model cache tests reset `providerModelsCache` directly in `cli/internal/server/graphql/resolver_provider_models_test.go:180-187`.
RecordHarnessRateLimit | DELETE | No writer exists in the current tree; `quotaFromHarnessInfo` only reads the cache through `LookupHarnessRateLimit` in `cli/internal/server/graphql/resolver_providers.go:499-516`.
resetHarnessRateLimitCache | DELETE | No symbol definition or call site exists; harness rate-limit cache resets are performed inline in `cli/internal/server/graphql/providers_unified_test.go:394-400`.

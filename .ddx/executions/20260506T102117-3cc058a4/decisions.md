DELETE NewResolver - no symbol remains in `cli/internal/server/graphql/resolver.go`; server now constructs `ddxgraphql.Resolver` inline in `cli/internal/server/server.go`.
DELETE personaConnectionFrom - no symbol remains in the current GraphQL package, and no production caller exists in `cli/internal/server/graphql`.
DELETE resetProviderModelsCacheForTest - no symbol remains; the provider model tests reset `providerModelsCache` directly in `cli/internal/server/graphql/resolver_provider_models_test.go`.
DELETE RecordHarnessRateLimit - no symbol remains as a callable function; current production code reads harness limits through `LookupHarnessRateLimit` in `cli/internal/server/graphql/resolver_providers.go`.
DELETE resetHarnessRateLimitCache - no symbol remains; the unified provider tests reset `harnessRateLimitCache.byName` directly in `cli/internal/server/graphql/providers_unified_test.go`.

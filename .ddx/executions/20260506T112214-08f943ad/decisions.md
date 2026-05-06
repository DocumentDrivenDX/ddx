# Decisions

- `internal/server/graphql/resolver.go:21 NewResolver` - DELETE: no `NewResolver` symbol exists in the current tree; the GraphQL resolver singleton is constructed in `cli/internal/server/server.go:4862-4888`.
- `internal/server/graphql/resolver_meta.go:90 personaConnectionFrom` - DELETE: no current definition or reference exists; persona GraphQL now routes through `personaToGQL` in `cli/internal/server/graphql/resolver_persona.go:10-87`.
- `internal/server/graphql/resolver_provider_models.go:292 resetProviderModelsCacheForTest` - DELETE: no current definition or reference exists; the provider-model cache is exercised through `providerModelsCache` and `providerModelsFetcher` in `cli/internal/server/graphql/resolver_provider_models.go:15-23,30-37`.
- `internal/server/graphql/resolver_providers.go:35 RecordHarnessRateLimit` - DELETE: no current definition or reference exists; harness quota lookup uses `LookupHarnessRateLimit` in `cli/internal/server/graphql/resolver_providers.go:32-39`.
- `internal/server/graphql/resolver_providers.go:55 resetHarnessRateLimitCache` - DELETE: no current definition or reference exists; the cache is only observed through `LookupHarnessRateLimit` in `cli/internal/server/graphql/resolver_providers.go:32-39`.

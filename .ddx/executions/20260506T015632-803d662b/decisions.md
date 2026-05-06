ddx-4c5beab2 decisions log

1. `internal/server/graphql/resolver.go:21 NewResolver` - DELETE: no longer present in the current tree; the gqlgen resolver root is constructed through the existing server wiring and deadcode no longer reports this symbol.
2. `internal/server/graphql/resolver_meta.go:90 personaConnectionFrom` - DELETE: no longer present in the current tree; persona GraphQL resolution now routes through the active `personas` / `persona` resolver path.
3. `internal/server/graphql/resolver_provider_models.go:292 resetProviderModelsCacheForTest` - DELETE: test-only cache reset helper is absent from the current tree, and the package cache is initialized entirely at package scope.
4. `internal/server/graphql/resolver_providers.go:35 RecordHarnessRateLimit` - DELETE: no current production caller remains in this tree; harness quota lookup now reads from the cache when data is recorded elsewhere, but this symbol itself is gone.
5. `internal/server/graphql/resolver_providers.go:55 resetHarnessRateLimitCache` - DELETE: test-only cache reset helper is absent from the current tree, and the harness rate-limit cache is initialized at package scope.

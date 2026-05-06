# Reachability decisions for ddx-4c5beab2

- DELETE `internal/server/graphql/resolver.go:21` `NewResolver` — no definition exists in the current tree; the current GraphQL root is constructed via `ddxgraphql.NewExecutableSchema(...)` in `cli/internal/server/server.go`.
- DELETE `internal/server/graphql/resolver_meta.go:90` `personaConnectionFrom` — no definition exists in the current tree; `resolver_meta.go` only contains persona query resolvers and helpers.
- DELETE `internal/server/graphql/resolver_provider_models.go:292` `resetProviderModelsCacheForTest` — no definition exists in the current tree; the provider-model cache tests reset state inline.
- DELETE `internal/server/graphql/resolver_providers.go:35` `RecordHarnessRateLimit` — no definition exists in the current tree; the file only retains `LookupHarnessRateLimit` plus quota helpers.
- DELETE `internal/server/graphql/resolver_providers.go:55` `resetHarnessRateLimitCache` — no definition exists in the current tree; the harness rate-limit cache is only cleared by tests that reinitialize package state directly.

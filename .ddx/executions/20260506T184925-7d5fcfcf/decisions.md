DELETE internal/server/graphql/resolver.go:21 NewResolver — no remaining production caller; graphql resolvers are constructed inline in internal/server/server.go.
DELETE internal/server/graphql/resolver_meta.go:90 personaConnectionFrom — no definition or production caller remains in the current tree.
DELETE internal/server/graphql/resolver_provider_models.go:292 resetProviderModelsCacheForTest — test-only cache reset helper is not referenced by production code.
DELETE internal/server/graphql/resolver_providers.go:35 RecordHarnessRateLimit — no write-side production caller remains; harness quota lookup uses LookupHarnessRateLimit only.
DELETE internal/server/graphql/resolver_providers.go:55 resetHarnessRateLimitCache — test-only cache reset helper is not referenced by production code.

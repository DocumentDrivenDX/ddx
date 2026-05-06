WIRE internal/server/graphql/resolver.go:21 NewResolver — server.graphqlHandler now constructs the root resolver through the constructor, making it reachable from main().
DELETE internal/server/graphql/resolver_meta.go:90 personaConnectionFrom — no surviving production reference in the current tree; the helper is already absent from source.
DELETE internal/server/graphql/resolver_provider_models.go:292 resetProviderModelsCacheForTest — no surviving production reference in the current tree; the test cache-reset helper is already absent from source.
DELETE internal/server/graphql/resolver_providers.go:35 RecordHarnessRateLimit — no surviving production reference in the current tree; harness quota lookup uses LookupHarnessRateLimit instead.
DELETE internal/server/graphql/resolver_providers.go:55 resetHarnessRateLimitCache — no surviving production reference in the current tree; the test cache-reset helper is already absent from source.

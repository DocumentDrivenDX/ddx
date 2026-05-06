NewResolver: DELETE - no constructor remains in `cli/internal/server/graphql`; `cli/internal/server/server.go:4869-4887` wires `&ddxgraphql.Resolver{...}` directly into gqlgen.
personaConnectionFrom: DELETE - no declaration remains in `cli/internal/server/graphql`; persona routing is handled through `personaProjectRoot` and the direct `Persona`/`Personas` resolvers in `resolver_meta.go:9-38`.
resetProviderModelsCacheForTest: DELETE - tests reset `providerModelsCache` directly in `resolver_provider_models_test.go:35-43` and similar setup blocks, so no exported reset helper is needed.
RecordHarnessRateLimit: DELETE - the harness rate-limit path now uses `harnessRateLimitCache` directly in `providers_unified_test.go:394-426` and `LookupHarnessRateLimit` in `resolver_providers.go:31-38`.
resetHarnessRateLimitCache: DELETE - the cache reset is handled inline by tests that clear `harnessRateLimitCache.byName` directly in `providers_unified_test.go:394-400`.

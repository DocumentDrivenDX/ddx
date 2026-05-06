NewResolver: WIRE via cli/internal/server/server.go:4868, where the GraphQL handler constructs the resolver root for the production server.
personaConnectionFrom: DELETE; no current definition exists in cli/internal/server/graphql, so the stale reachability entry has already been removed.
resetProviderModelsCacheForTest: DELETE; no current definition exists in cli/internal/server/graphql, and the provider-model cache helpers are now exercised through the live resolver paths and tests.
RecordHarnessRateLimit: DELETE; no current definition exists in cli/internal/server/graphql, and quota lookup now reads from the harness rate-limit cache only.
resetHarnessRateLimitCache: DELETE; no current definition exists in cli/internal/server/graphql, and the cache is reset only through test-local setup.

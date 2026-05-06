WIRE `internal/server/graphql/resolver.go:158` NewResolver is instantiated by `cli/internal/server/server.go:4868` in the singleton GraphQL handler path.
DELETE `internal/server/graphql/resolver_meta.go:90` personaConnectionFrom is not present in the current tree; the persona resolvers use direct list/lookup helpers instead.
DELETE `internal/server/graphql/resolver_provider_models.go:292` resetProviderModelsCacheForTest is not present in the current tree; the provider-model tests reset the cache inline.
DELETE `internal/server/graphql/resolver_providers.go:35` RecordHarnessRateLimit is not present in the current tree; the package only exposes LookupHarnessRateLimit and reads the cache in quotaFromHarnessInfo.
DELETE `internal/server/graphql/resolver_providers.go:55` resetHarnessRateLimitCache is not present in the current tree; harness-rate-limit tests reset harnessRateLimitCache inline.

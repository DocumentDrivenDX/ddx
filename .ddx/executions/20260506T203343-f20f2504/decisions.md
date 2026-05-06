NewResolver: WIRE — constructed in cli/internal/server/server.go:4868 via graphqlHandler(), which is installed from server.New() in cli/cmd/server.go:104.
personaConnectionFrom: DELETE — no symbol exists in the current tree; PersonaConnection is served by gqlgen-generated code and query resolvers in cli/internal/server/graphql/resolver_meta.go.
resetProviderModelsCacheForTest: DELETE — no current symbol exists; provider-model cache tests reset package state inline in cli/internal/server/graphql/resolver_provider_models_test.go.
RecordHarnessRateLimit: WIRE — quotaFromHarnessInfo consults LookupHarnessRateLimit in cli/internal/server/graphql/resolver_providers.go:506, so the production harness-status path consumes the recorded signal.
resetHarnessRateLimitCache: DELETE — no current symbol exists; the cache is reset inline in cli/internal/server/graphql/providers_unified_test.go.

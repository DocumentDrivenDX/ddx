NewResolver: DELETE - no constructor symbol exists in cli/internal/server/graphql; current server wiring uses ddxgraphql.NewExecutableSchema with an inline Resolver in cli/internal/server/server.go.
personaConnectionFrom: DELETE - no function exists in the current tree; persona resolvers are wired directly through Persona(s)/PersonaByRole in cli/internal/server/graphql/resolver_meta.go.
resetProviderModelsCacheForTest: DELETE - no reset helper exists in the current tree; provider-model cache is exercised through public resolvers/tests in cli/internal/server/graphql/resolver_provider_models.go.
RecordHarnessRateLimit: DELETE - no write helper exists in the current tree; harness rate-limit lookup is read via LookupHarnessRateLimit in cli/internal/server/graphql/resolver_providers.go.
resetHarnessRateLimitCache: DELETE - no reset helper exists in the current tree; harness rate-limit state is only exposed through LookupHarnessRateLimit in cli/internal/server/graphql/resolver_providers.go.

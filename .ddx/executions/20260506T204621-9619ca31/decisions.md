WIRE NewResolver — wired from cli/internal/server/server.go:4868 in Server.graphqlHandler, which constructs the live GraphQL resolver root.
DELETE personaConnectionFrom — no definition or production call site exists in cli/internal/server/graphql; persona resolution now goes directly through persona loader helpers in resolver_meta.go:10-52.
DELETE resetProviderModelsCacheForTest — no definition or production call site exists in cli/internal/server/graphql; provider-model cache tests reset state inline.
DELETE RecordHarnessRateLimit — no function definition remains in cli/internal/server/graphql; only explanatory comments reference the former helper in resolver_providers.go:23-26 and 499-505.
DELETE resetHarnessRateLimitCache — no function definition or production/test caller remains in cli/internal/server/graphql.

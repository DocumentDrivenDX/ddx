DELETE NewResolver - no definition exists in the current cli/internal/server/graphql package; the server constructs ddxgraphql.Resolver directly in cli/internal/server/server.go.
DELETE personaConnectionFrom - no definition exists in the current cli/internal/server/graphql package; no production call site remains.
DELETE resetProviderModelsCacheForTest - no definition exists in the current cli/internal/server/graphql package; provider-model tests reset cache state directly instead.
DELETE RecordHarnessRateLimit - no definition exists in the current cli/internal/server/graphql package; quota lookup uses LookupHarnessRateLimit instead.
DELETE resetHarnessRateLimitCache - no definition exists in the current cli/internal/server/graphql package; no production call site remains.

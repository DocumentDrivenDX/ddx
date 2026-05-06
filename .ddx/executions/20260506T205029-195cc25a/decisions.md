WIRE NewResolver - wired by cli/internal/server/server.go:4868 into graphqlHandler, which constructs the gqlgen schema used by the /graphql and scoped project routes.
DELETE personaConnectionFrom - no current definition or call site remains in cli/internal/server/graphql; the helper is absent from the tree and no deadcode hit remains.
DELETE resetProviderModelsCacheForTest - no current definition or call site remains; provider model tests reset the cache inline instead of via a helper.
DELETE RecordHarnessRateLimit - no current definition or call site remains; harness rate-limit tests write the cache directly.
DELETE resetHarnessRateLimitCache - no current definition or call site remains; harness rate-limit tests reset the cache inline instead of via a helper.

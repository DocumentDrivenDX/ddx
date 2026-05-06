NewResolver: DELETE - no current production or test call sites remain in cli/internal/server/graphql; server constructs &ddxgraphql.Resolver directly in cli/internal/server/server.go.
personaConnectionFrom: DELETE - no symbol definition or call site remains in cli/internal/server/graphql; generated PersonaConnection handling is in cli/internal/server/graphql/generated.go.
resetProviderModelsCacheForTest: DELETE - test-only cache reset helper is not present in the current tree; tests clear providerModelsCache directly.
RecordHarnessRateLimit: DELETE - no production caller exists; quotaFromHarnessInfo only reads LookupHarnessRateLimit and no server path records a signal.
resetHarnessRateLimitCache: DELETE - test-only cache reset helper is not present in the current tree; tests reset harnessRateLimitCache directly.

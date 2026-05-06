NewResolver: WIRE - constructed in cli/internal/server/server.go:4868 and used to build the gqlgen handler for the live GraphQL server path.
personaConnectionFrom: DELETE - no longer present in the current tree; the personas query is resolved directly by resolver_meta.go's Personas method and the generated dispatcher routes to Query.personas.
resetProviderModelsCacheForTest: DELETE - no longer present in the current tree; provider model tests clear providerModelsCache directly.
RecordHarnessRateLimit: DELETE - no longer present in the current tree; quotaFromHarnessInfo only reads LookupHarnessRateLimit.
resetHarnessRateLimitCache: DELETE - no longer present in the current tree; provider status tests clear harnessRateLimitCache directly.

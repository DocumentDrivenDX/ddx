NewResolver: WIRE - retained and reached via `cli/internal/server/server.go:4868` from the CLI server command path (`cli/cmd/server.go:104`), so the GraphQL resolver root is installed in production.
personaConnectionFrom: DELETE - no definition or call site remains in the current `cli/internal/server/graphql` tree; the persona queries now resolve directly through the generated schema and `personaToGQL`.
resetProviderModelsCacheForTest: DELETE - no definition or call site remains in the current tree; the provider-model tests clear `providerModelsCache` directly instead.
RecordHarnessRateLimit: DELETE - no definition or call site remains in the current tree; `quotaFromHarnessInfo` still reads `LookupHarnessRateLimit`, but there is no production writer left to keep.
resetHarnessRateLimitCache: DELETE - no definition or call site remains in the current tree; the rate-limit cache is reset directly in the provider-status tests.

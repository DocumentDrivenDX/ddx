NewResolver: DELETE - not present in the current tree; GraphQL resolvers are constructed inline in cli/internal/server/server.go.
personaConnectionFrom: DELETE - no current definition or call site in cli/internal/server/graphql; the persona query path now returns direct Persona nodes.
resetProviderModelsCacheForTest: DELETE - no current definition; the provider model tests reset package caches inline where needed.
RecordHarnessRateLimit: DELETE - no current definition; harness quota lookup now reads the cache through LookupHarnessRateLimit only.
resetHarnessRateLimitCache: DELETE - no current definition; no production or test call site remains in the current tree.

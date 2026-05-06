NewResolver: DELETE constructor not referenced by production bootstrap; server wires graphql.Resolver inline in cli/internal/server/server.go:4868-4889.
personaConnectionFrom: DELETE helper not present in current source and no in-tree callers remain.
resetProviderModelsCacheForTest: DELETE test-only cache helper not present in current source and not reachable from production roots.
RecordHarnessRateLimit: WIRE quotaFromHarnessInfo consumes LookupHarnessRateLimit in production path; keep public helper for server harness-dispatch path.
resetHarnessRateLimitCache: DELETE test-only cache helper not present in current source and not reachable from production roots.

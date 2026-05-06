DELETE NewResolver: gqlgen server startup now wires `&Resolver{...}` directly; the constructor is absent from the current tree.
DELETE personaConnectionFrom: persona GraphQL shape is built inline by the current resolvers; the helper is absent from the current tree.
DELETE resetProviderModelsCacheForTest: tests reset `providerModelsCache` inline now; the helper is absent from the current tree.
DELETE RecordHarnessRateLimit: the harness quota path now reads `LookupHarnessRateLimit`/`QuotaFromRateLimitSignal`; the mutator helper is absent from the current tree.
DELETE resetHarnessRateLimitCache: tests reset `harnessRateLimitCache` inline now; the helper is absent from the current tree.

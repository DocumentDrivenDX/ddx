DELETE NewResolver: no definition remains in the current graphql package; server wiring instantiates `&ddxgraphql.Resolver{...}` directly.
DELETE personaConnectionFrom: no definition remains in the current graphql package; persona resolvers use direct loader-to-GQL conversions instead.
DELETE resetProviderModelsCacheForTest: no definition remains in the current graphql package; provider-model cache tests reset package state inline.
DELETE RecordHarnessRateLimit: no definition remains in the current graphql package; the rate-limit cache is only read through `LookupHarnessRateLimit`.
DELETE resetHarnessRateLimitCache: no definition remains in the current graphql package; no production caller exists and the cache is only touched by tests/helpers.

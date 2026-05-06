DELETE NewResolver - already removed from the current tree; server.go now constructs `&ddxgraphql.Resolver{...}` inline instead of calling a constructor.
DELETE personaConnectionFrom - already removed; the personas path is implemented as a flat query in resolver_meta.go and no connection helper is wired.
DELETE resetProviderModelsCacheForTest - already removed; provider-model tests reset `providerModelsCache` directly in resolver_provider_models_test.go.
DELETE RecordHarnessRateLimit - already removed; the runtime quota path reads `LookupHarnessRateLimit` in quotaFromHarnessInfo instead.
DELETE resetHarnessRateLimitCache - already removed; provider tests clear `harnessRateLimitCache` directly in providers_unified_test.go.

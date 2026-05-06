NewResolver | DELETE | no live definition in cli/internal/server/graphql; schema construction now flows through generated NewExecutableSchema and Resolver.Query/Mutation/Subscription.
personaConnectionFrom | DELETE | no live definition in cli/internal/server/graphql; persona query resolution now uses resolver_meta.go helpers directly.
resetProviderModelsCacheForTest | DELETE | no live definition in cli/internal/server/graphql; provider model tests reset providerModelsCache in place.
RecordHarnessRateLimit | DELETE | no live definition in cli/internal/server/graphql; quotaFromHarnessInfo already reads LookupHarnessRateLimit and tests mutate harnessRateLimitCache directly.
resetHarnessRateLimitCache | DELETE | no live definition in cli/internal/server/graphql; harness rate-limit tests reset harnessRateLimitCache in place.

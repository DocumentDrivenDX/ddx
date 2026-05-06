NewResolver: DELETE no func NewResolver exists in cli/internal/server/graphql; server and tests construct Resolver directly with struct literals and Mutation()/Query()/Subscription() adapters.
personaConnectionFrom: DELETE no definition or call site exists in the live tree; persona GraphQL wiring uses personaProjectRoot, Persona, PersonaByRole, and personaToGQL instead.
resetProviderModelsCacheForTest: DELETE no symbol exists in the live tree; provider-model cache tests reset providerModelsCache inline instead of calling a helper.
RecordHarnessRateLimit: DELETE no exported or package-local function exists in the live tree; the package only retains LookupHarnessRateLimit and quotaFromHarnessInfo reads from that cache.
resetHarnessRateLimitCache: DELETE no symbol exists in the live tree; no production or test call site remains.

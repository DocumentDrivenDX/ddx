# Reachability decisions

- `WIRE` `NewResolver` - `cli/internal/server/server.go:4868` constructs the singleton GraphQL resolver in `graphqlHandler`, and `cli/internal/server/graphql/resolver.go:156-158` defines the constructor.
- `DELETE` `personaConnectionFrom` - no symbol with that name exists in the current `cli/internal/server/graphql` tree; personas are resolved by the flat-array query in `cli/internal/server/graphql/resolver_meta.go:10-38`, and the schema query does not route through a connection helper.
- `DELETE` `resetProviderModelsCacheForTest` - no symbol with that name exists in the current tree; provider-model tests rebind the package variables directly instead of calling a reset helper.
- `DELETE` `RecordHarnessRateLimit` - no symbol with that name exists in the current tree; harness quota lookup only reads `LookupHarnessRateLimit` from the package cache in `cli/internal/server/graphql/resolver_providers.go:19-34` and `cli/internal/server/graphql/resolver_providers.go:506-515`.
- `DELETE` `resetHarnessRateLimitCache` - no symbol with that name exists in the current tree; the harness rate-limit cache is the package-level map in `cli/internal/server/graphql/resolver_providers.go:19-29` and there is no reset helper left to wire.

Verification:

- `cd cli && go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... 2>&1 | rg 'internal/server/graphql'` produced no output.

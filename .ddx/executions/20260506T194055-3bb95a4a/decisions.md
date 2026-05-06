DELETE NewResolver — no `func NewResolver` exists in `cli/internal/server/graphql`, and a fresh `go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` scan does not report any `internal/server/graphql` reachability.
DELETE personaConnectionFrom — no current definition or production call site exists in `cli/internal/server/graphql`; refreshed deadcode scan is clean for that package.
DELETE resetProviderModelsCacheForTest — no current definition or production call site exists in `cli/internal/server/graphql`; provider-model tests now reset cache state inline.
DELETE RecordHarnessRateLimit — only documentation references remain in `cli/internal/server/graphql/resolver_providers.go`; there is no current function definition or production caller.
DELETE resetHarnessRateLimitCache — only test-setup code references remain in `cli/internal/server/graphql`; there is no current function definition or production caller.

# Decisions: ddx-bd927304 internal/bead core residual symbols

Each symbol from the cluster in `production-reachability-final.json` is dispositioned below.

| File | Symbol | Disposition | Notes |
|------|--------|-------------|-------|
| archive.go:37 | `DefaultArchivePolicy` | DELETE | Only used by tests (archive_test.go × 6, benchmark_backends_test.go × 1); no production caller uses it (cmd/bead_archive.go constructs its own ArchivePolicy literal). Replaced in test files with a private `defaultArchivePolicy()` helper local to `archive_test.go` so the policy presets stay co-located with the test suite. |
| axon_backend.go:112 | `WithAxonGraphQLTransport` | DELETE | Test-only option; no production caller wires a GraphQL transport. Tests now assign the `GraphQLTransport` field directly. |
| axon_backend.go:120 | `WithAxonGraphQLClient` | DELETE | Same rationale as above. Tests assign `GraphQLClient` directly. The `AxonBackendOption` type and the variadic `opts ...AxonBackendOption` parameter on `NewAxonBackend` are deleted along with them since they have no remaining producers. |
| axon_backend.go:134 | `AxonExperimentalEnabled` | DELETE | Doc string already noted the legacy helper "no longer gates store selection". No production reader. `AxonExperimentalEnv` constant and `TestAxonExperimentalEnabledTruthyValues` removed; the lingering `t.Setenv(AxonExperimentalEnv, "0")` in `store_test.go` is also dropped (it was guarding against a flag that no longer exists). |
| context.go:18 | `WithIdentity` | DELETE | No callers (production or test). Identity context plumbing was never adopted. |
| context.go:26 | `IdentityFromContext` | DELETE | Same; no callers. |
| context.go:35 | `WithTrace` | DELETE | Same; no callers. The `go.opentelemetry.io/otel/trace` import goes with it. |
| context.go:43 | `TraceFromContext` | DELETE | Same; no callers. The `Identity` struct and key types are removed because the entire context.go API is unused. |
| id.go:77 | `SequentialIDGenerator.GenID` | DELETE | Test-only. The `SequentialIDGenerator` type and its `GenID` method are moved to `id_test.go` so the test suite still exercises monotonic-id behaviour without leaking production-graph dead weight. |
| id.go:102 | `NewIDGenerator` | DELETE | No callers. Production code uses `RandomHexIDGenerator{...}.GenID(ctx)` directly (store.go:178). The `IDGenerator` interface, whose only producer was `NewIDGenerator`, is deleted too. |
| lifecycle.go:133 | `CanLifecycleTransition` | DELETE | Convenience predicate used only by `lifecycle_test.go`. Tests now invoke `ValidateLifecycleTransition(...) == nil` inline. |
| operation.go:12 | `MutateFunc.Apply` | DELETE | Test-only adapter. `MutateFunc` and its `Apply` method move to a test helper file (`mutate_func_helper_test.go`); the production `Operation` interface stays put because backend/store production signatures still reference it. |
| reconcile.go:307 | `isNoViableProviderEvent` | DELETE | No callers (production or test). Speculative helper. |
| reconcile.go:346 | `hasRetryAfter` | DELETE | No callers (production or test). Speculative helper. |

## Acceptance evidence

- AC1: every listed symbol either deleted from production code or relocated into a `_test.go` file (test-only definitions are invisible to `golang.org/x/tools/cmd/deadcode`).
- AC2: `cd cli && go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | rg 'internal/bead/(archive|axon_backend|context|id|lifecycle|operation|reconcile)\.go'` returns no hits (see commit-time output below).
- AC3: no `// wiring:pending` annotations were needed; every cluster symbol resolved within scope.
- AC5: `cd cli && go test ./...` passes.
- AC6: `lefthook run pre-commit` passes.

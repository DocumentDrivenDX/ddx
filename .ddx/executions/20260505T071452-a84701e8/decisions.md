DELETE WithSpokeHeartbeatInterval - test-only SpokeOption moved to cli/internal/server/federation_spoke_test.go.
DELETE WithSpokeHeartbeatJitter - test-only SpokeOption moved to cli/internal/server/federation_spoke_test.go.
DELETE WithSpokeStatePath - test-only SpokeOption moved to cli/internal/server/federation_spoke_test.go.
DELETE WithSpokeHTTPClient - test-only SpokeOption moved to cli/internal/server/federation_spoke_test.go.
DELETE WithSpokeSelfURL - test-only SpokeOption moved to cli/internal/server/federation_spoke_test.go.
DELETE WithSpokeNodeID - test-only SpokeOption moved to cli/internal/server/federation_spoke_test.go.
DELETE reportedWorkersAdapter.setNow - test-only clock override moved to cli/internal/server/workers_panel_live_transitions_test.go.
DELETE Server.execStore - unused wrapper removed; request-scoped execStoreForRequest remains.
DELETE Server.loadSessions - unused wrapper removed; loadSessionsFor remains.
DELETE readAddrFilePID - unused singleton helper removed from cli/internal/server/singleton.go.
DELETE workerIngestRegistry.close - test-only cleanup helper moved to cli/internal/server/graphql_reported_workers_test.go.
DELETE workerHarnessHealthy - unused harness-health helper removed from cli/internal/server/workers.go.
DELETE NewReviewSessionStore - review-session persistence moved to cli/internal/server/review_session_test.go.
DELETE ReviewSessionStore.Create - review-session persistence moved to cli/internal/server/review_session_test.go.
DELETE ReviewSessionStore.AppendTurn - review-session persistence moved to cli/internal/server/review_session_test.go.
DELETE ReviewSessionStore.Load - review-session persistence moved to cli/internal/server/review_session_test.go.
DELETE ReviewSessionStore.sessionRoot - review-session persistence moved to cli/internal/server/review_session_test.go.
DELETE reviewSessionManifestFrom - review-session helper moved to cli/internal/server/review_session_test.go.
DELETE writeJSONFile - review-session helper moved to cli/internal/server/review_session_test.go.
DELETE NewResolver - GraphQL test helper moved to cli/internal/server/graphql/test_helpers_test.go.
DELETE personaConnectionFrom - unused GraphQL helper removed from cli/internal/server/graphql/resolver_meta.go.
DELETE resetProviderModelsCacheForTest - GraphQL test seam moved to cli/internal/server/graphql/test_helpers_test.go.
DELETE RecordHarnessRateLimit - GraphQL test seam moved to cli/internal/server/graphql/test_helpers_test.go.
DELETE resetHarnessRateLimitCache - GraphQL test seam moved to cli/internal/server/graphql/test_helpers_test.go.
DELETE DefaultBeadFixtureSpec - perf harness moved to cli/internal/server/perf/fixtures_test.go.
DELETE BeadFixture.TotalBeads - perf harness moved to cli/internal/server/perf/fixtures_test.go.
DELETE BuildBeadFixture - perf harness moved to cli/internal/server/perf/fixtures_test.go.
DELETE seedProjectBeads - perf harness moved to cli/internal/server/perf/fixtures_test.go.
DELETE seedProjectDocGraph - perf harness moved to cli/internal/server/perf/fixtures_test.go.
DELETE seedProjectSessions - perf harness moved to cli/internal/server/perf/fixtures_test.go.
DELETE Environment - perf harness moved to cli/internal/server/perf/fixtures_test.go.
DELETE Targets - perf harness moved to cli/internal/server/perf/harness_test.go.
DELETE RunMatrix - perf harness moved to cli/internal/server/perf/harness_test.go.
DELETE variablesFor - perf harness moved to cli/internal/server/perf/harness_test.go.
DELETE postGraphQL - perf harness moved to cli/internal/server/perf/harness_test.go.
DELETE PostGraphQL - perf harness moved to cli/internal/server/perf/harness_test.go.
DELETE percentileMillis - perf harness moved to cli/internal/server/perf/harness_test.go.
DELETE percentile - perf harness moved to cli/internal/server/perf/harness_test.go.
DELETE toMillis - perf harness moved to cli/internal/server/perf/harness_test.go.
DELETE WriteReports - perf harness moved to cli/internal/server/perf/report_test.go.
DELETE renderMarkdown - perf harness moved to cli/internal/server/perf/report_test.go.
DELETE flatted.go package - orphan Go package under cli/internal/server/frontend/node_modules removed from deadcode scope.

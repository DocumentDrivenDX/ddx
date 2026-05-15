# ddx-b6859802 decisions

Moved the test-helper cluster from `cli/cmd/run_test_helpers.go` to `cli/cmd/run_test_helpers_test.go`. Every residual symbol was `DELETE` from the production reachability graph because each caller is in `cli/cmd/*_test.go`; no symbol required runtime wiring.

| Symbol | Decision | Note |
| --- | --- | --- |
| `stubAgentService.Execute` | DELETE | Test-only helper moved to `_test.go`. |
| `stubAgentService.ResolveRoute` | DELETE | Test-only helper moved to `_test.go`. |
| `stubAgentService.TailSessionLog` | DELETE | Test-only helper moved to `_test.go`. |
| `stubAgentService.ListHarnesses` | DELETE | Test-only helper moved to `_test.go`. |
| `stubAgentService.ListProviders` | DELETE | Test-only helper moved to `_test.go`. |
| `stubAgentService.ListModels` | DELETE | Test-only helper moved to `_test.go`. |
| `stubAgentService.HealthCheck` | DELETE | Test-only helper moved to `_test.go`. |
| `stubAgentService.ListPolicies` | DELETE | Test-only helper moved to `_test.go`. |
| `stubAgentService.RecordRouteAttempt` | DELETE | Test-only helper moved to `_test.go`. |
| `stubAgentService.RouteStatus` | DELETE | Test-only helper moved to `_test.go`. |
| `stubAgentService.ListSessionLogs` | DELETE | Test-only helper moved to `_test.go`. |
| `stubAgentService.WriteSessionLog` | DELETE | Test-only helper moved to `_test.go`. |
| `stubAgentService.ReplaySession` | DELETE | Test-only helper moved to `_test.go`. |
| `stubAgentService.UsageReport` | DELETE | Test-only helper moved to `_test.go`. |
| `executeCapturingStub.Execute` | DELETE | Test-only helper moved to `_test.go`. |
| `executeCapturingStub.ResolveRoute` | DELETE | Test-only helper moved to `_test.go`. |
| `executeCapturingStub.TailSessionLog` | DELETE | Test-only helper moved to `_test.go`. |
| `executeCapturingStub.ListHarnesses` | DELETE | Test-only helper moved to `_test.go`. |
| `executeCapturingStub.ListProviders` | DELETE | Test-only helper moved to `_test.go`. |
| `executeCapturingStub.ListModels` | DELETE | Test-only helper moved to `_test.go`. |
| `executeCapturingStub.ListPolicies` | DELETE | Test-only helper moved to `_test.go`. |
| `executeCapturingStub.HealthCheck` | DELETE | Test-only helper moved to `_test.go`. |
| `executeCapturingStub.RecordRouteAttempt` | DELETE | Test-only helper moved to `_test.go`. |
| `executeCapturingStub.RouteStatus` | DELETE | Test-only helper moved to `_test.go`. |
| `executeCapturingStub.ListSessionLogs` | DELETE | Test-only helper moved to `_test.go`. |
| `executeCapturingStub.WriteSessionLog` | DELETE | Test-only helper moved to `_test.go`. |
| `executeCapturingStub.ReplaySession` | DELETE | Test-only helper moved to `_test.go`. |
| `executeCapturingStub.UsageReport` | DELETE | Test-only helper moved to `_test.go`. |
| `installExecuteCapturingStub` | DELETE | Test-only helper moved to `_test.go`. |
| `canonicalFizeauPolicyFixture` | DELETE | Test-only helper moved to `_test.go`. |
| `capturedImplementationRequests` | DELETE | Test-only helper moved to `_test.go`. |
| `minimalProjectDir` | DELETE | Test-only helper moved to `_test.go`. |
| `appendTestRoutingEvidence` | DELETE | Test-only helper moved to `_test.go`. |
| `setupWorkIntakeFixture` | DELETE | Test-only helper moved to `_test.go`. |

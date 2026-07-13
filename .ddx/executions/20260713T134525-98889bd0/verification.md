# ddx-93d8b7c8 Verification

Scope checked:
- `cli/internal/agent/model_discovery_containment.go`
- `cli/cmd/lifecycle_model_discovery_test.go`
- `cli/cmd/execute_loop_shared.go`

Evidence:
- `ListModelsWithProbeContainment` captures a pre-call provider baseline, calls `svc.ListModels`, and reaps new provider children before returning.
- `TestLifecycleModelDiscovery_CancellationReapsPTYProbe` covers caller cancellation and asserts the probe PID is reaped.
- `TestLifecycleModelDiscovery_ReturnDoesNotLeaveNoAltScreenChild` covers success, upstream error, caller timeout, and worker cancellation while preserving the pre-call baseline provider.

Validation:
- `cd cli && go test ./cmd/... -run '^(TestLifecycleModelDiscovery_CancellationReapsPTYProbe|TestLifecycleModelDiscovery_ReturnDoesNotLeaveNoAltScreenChild)$' -count=1 -timeout 60s`
- `lefthook run pre-commit`

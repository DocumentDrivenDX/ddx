# Fizeau Execute Routing Investigation — ddx-1e516bc9

**Outcome: B — gap does not exist in the current fizeau version (v0.12.0)**

## Hypothesis to test

The bead claimed that fizeau v0.10.3 `service_execute.go:193-224` skips the routing engine entirely when `req.Harness != ""`, resulting in an empty resolved Model when `Model == ""` even when `Profile`/`MinPower` routing inputs are present.

## Finding

`cli/go.mod` pins `github.com/easel/fizeau v0.12.0`. The routing gap was present in v0.10.3 but has been fully resolved in v0.12.0.

### Evidence — fizeau v0.12.0 `service_execute.go`

`~/go/pkg/mod/github.com/easel/fizeau@v0.12.0/service_execute.go`, function `resolveExecuteRoute`:

**Function docstring (lines ~155-163)**:
> The request is dispatched through the routing engine (internal/routing.Resolve)
> when under-specified (Harness == ""), or when Harness is set but Model is empty
> and routing inputs (Policy or MinPower) are present (engine runs within the
> harness's eligible models). When Harness is set and Model is also set, the
> decision is accepted verbatim.

**Empty-model routing guard (lines ~193-224)**:

```go
if !cfg.TestOnly && !cfg.IsHTTPProvider && canonical != "fiz" && req.Model == "" {
    if req.Policy == "" && req.MinPower == 0 {
        // Under-specified: no model, no routing inputs → fail early
        return nil, fmt.Errorf("under-specified routing for harness=%q: "+
            "supply --model, --policy, or --min-power", canonical)
    }
    // Policy or MinPower present: route within harness's eligible models.
    // Class 2 harnesses (AutoRoutingEligible=false: gemini, opencode, pi)
    // require explicit --model.
    if !cfg.AutoRoutingEligible {
        return nil, fmt.Errorf("no auto-resolution available for harness=%q: "+
            "harness does not support auto-routing; supply an explicit --model", canonical)
    }
    return s.resolveExecuteRouteWithEngine(req)
}
```

Three cases are covered:
1. **Harness set, Model empty, no routing inputs**: hard error "under-specified routing for harness=X" (AC5).
2. **Harness set, Model empty, Profile/MinPower present, Class 2 (AutoRoutingEligible=false)**: hard error "no auto-resolution available for harness=X" (AC2 Class 2 requirement).
3. **Harness set, Model empty, Profile/MinPower present, Class 1 (AutoRoutingEligible=true)**: calls `resolveExecuteRouteWithEngine(req)` which runs the routing engine within the harness's eligible models (AC2 + AC4).

### Evidence — upstream tests in fizeau v0.12.0

`~/go/pkg/mod/github.com/easel/fizeau@v0.12.0/service_execute_harness_pin_test.go`:

| AC | Test name | Line |
|----|-----------|------|
| AC4 | `TestExecute_ExplicitHarnessEmptyModelWithProfile_RoutesWithinHarness` | 270 |
| AC5 | `TestExecute_ExplicitHarnessEmptyModelNoProfile_FailsClearly` | 331 |
| AC2 Class 2 | `TestExecute_Class2HarnessEmptyModelWithProfile_FailsClearly` | 351 |

Test docstring for AC4 explicitly references this bead:
> "asserts AC4 from ddx-1e516bc9"

### Evidence — DDx call site documentation (AC10)

`cli/internal/agent/service_run.go:247-251`:
```go
// When Harness is set and Model is empty, fizeau routes within the harness's
// eligible models using Profile/MinPower as the routing constraint. This is the
// well-formed request shape for profile-driven dispatch without an explicit model.
// fizeau returns ErrHarnessModelIncompatible (typed, non-nil) on pre-dispatch
// errors and a failed final event for post-dispatch errors.
```

### AC3 — go.mod bump

`cli/go.mod` already pins `github.com/easel/fizeau v0.12.0`. No bump needed.

### AC9 — DDx tests

`cd cli && go test ./internal/agent/... ./cmd/...` passes green on this commit.

### AC6 — End-to-end verification

Blocked on ddx-cfedee8e / ddx-9228a484 (C9) landing first; those beads supply the
ladder-driven MinPower to the Execute call. Not attempted here.

## Summary

All ACs in scope for Outcome B (AC1–AC5, AC7–AC10) are satisfied by fizeau v0.12.0
(already pinned) and the existing DDx code. The only remaining AC (AC6 E2E) is
gated on cfedee8e/C9 as the bead description states.

**Bead status: close as already-resolved.**

# fizeau Execute Routing Gap — Investigation Report

**Bead:** ddx-1e516bc9  
**Outcome:** B — gap does not exist in currently-pinned fizeau version; bead is already-resolved.

## Hypothesis under investigation

fizeau v0.10.3 `service_execute.go:193-224` only invokes the routing engine when `req.Harness == ""`. When `Harness` is set and `Model` is empty but `Profile`/`MinPower` are non-empty, the old code accepted the route verbatim and returned `RouteDecision{Model:""}` — a silent misconfiguration.

## Finding: Outcome B — fixed in v0.10.15

The DDx `cli/go.mod` pins `github.com/DocumentDrivenDX/fizeau v0.10.15`. That version already contains the full fix for every AC in this bead.

### Code evidence

**`fizeau@v0.10.15/service_execute.go` — `resolveExecuteRoute` (lines 163–228):**

```
// Lines 194-213 (module cache: /home/erik/go/pkg/mod/.../fizeau@v0.10.15/service_execute.go)
if !cfg.TestOnly && !cfg.IsHTTPProvider && canonical != "fiz" && req.Model == "" {
    if req.Profile == "" && req.MinPower == 0 {
        // Under-specified: no model, no routing inputs → clear error (AC5)
        return nil, fmt.Errorf("under-specified routing for harness=%q: "+
            "supply --model, --profile, or --min-power", canonical)
    }
    // Profile or MinPower present: route within harness's eligible models
    if !cfg.AutoRoutingEligible {
        // Class 2 harness (gemini/opencode/pi) — clear error, not silent empty (AC2)
        return nil, fmt.Errorf("no auto-resolution available for harness=%q: "+
            "harness does not support auto-routing; supply an explicit --model", canonical)
    }
    return s.resolveExecuteRouteWithEngine(req)  // routing engine runs within harness
}
```

The old "silent-empty-model" path is gone. The doc comment on `resolveExecuteRoute` (lines 157–162) explicitly states:

> "when Harness is set but Model is empty and routing inputs (Profile or MinPower) are present (engine runs within the harness's eligible models)"

### Test evidence (AC4 and AC5 already in fizeau v0.10.15)

**`fizeau@v0.10.15/service_execute_harness_pin_test.go`:**

| Test | Lines | AC |
|---|---|---|
| `TestExecute_ExplicitHarnessEmptyModelWithProfile_RoutesWithinHarness` | 270–325 | AC4 |
| `TestExecute_ExplicitHarnessEmptyModelNoProfile_FailsClearly` | 331–345 | AC5 |
| `TestExecute_Class2HarnessEmptyModelWithProfile_FailsClearly` | 351–368 | AC2 |

Test comments explicitly cite `ddx-1e516bc9` and each AC. Test `AC4` uses `sonnet-4.6` catalog entries (not stale Haiku/Sonnet/Opus literals), satisfying AC7.

### DDx call-site comment (AC10)

**`cli/internal/agent/service_run.go:243-248`:**

```go
// When Harness is set and Model is empty, fizeau routes within the harness's
// eligible models using Profile/MinPower as the routing constraint. This is the
// well-formed request shape for profile-driven dispatch without an explicit model.
// fizeau returns ErrHarnessModelIncompatible (typed, non-nil) on pre-dispatch
// errors and a failed final event for post-dispatch errors.
events, err := svc.Execute(cancelCtx, req)
```

This satisfies AC10.

## AC status summary

| AC | Status | Evidence |
|---|---|---|
| AC1 — investigation | DONE | This report; Outcome B |
| AC2 — upstream Class 2 error | ALREADY DONE | `service_execute.go:208-211`; test line 351 |
| AC3 — bump go.mod | N/A | go.mod already pins v0.10.15 |
| AC4 — positive routing test | ALREADY DONE | test line 270 |
| AC5 — negative under-spec test | ALREADY DONE | test line 331 |
| AC6 — e2e verification | DEFERRED | Depends on ddx-cfedee8e/C9 (ladder wiring) |
| AC7 — current catalog names | ALREADY DONE | test uses `sonnet-4.6` |
| AC8 — no empty-fizeau fallback | ALREADY DONE | clear errors replace silent path |
| AC9 — go test green | DONE | `cd cli && go test ./internal/agent/... ./cmd/...` passes |
| AC10 — call-site comment | ALREADY DONE | service_run.go:243-248 |

## Conclusion

All implementable ACs are satisfied by fizeau v0.10.15 + the existing DDx call-site comment. AC6 (end-to-end verification) remains deferred on `ddx-cfedee8e`/C9 landing the ladder wiring so that `MinPower` is actually supplied in `ServiceExecuteRequest`. This bead is closed as already-resolved.

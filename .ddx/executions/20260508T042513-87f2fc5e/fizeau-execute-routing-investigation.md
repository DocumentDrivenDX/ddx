# fizeau Execute Routing Investigation — ddx-1e516bc9

**Date:** 2026-05-08
**fizeau version investigated:** v0.10.14 (go.mod pin at time of investigation)
**Bead description says v0.10.3; actual pin is v0.10.14 — gap persists in both.**

## Outcome: A — Gap Confirmed

The hypothesis is correct. `service_execute.go:resolveExecuteRoute` skips the routing engine entirely when `Harness` is non-empty, regardless of whether `Model` is empty and `Profile`/`MinPower` is non-empty.

---

## Evidence: Code Path Analysis

### `service_execute.go:161–201` — `resolveExecuteRoute`

```go
func (s *service) resolveExecuteRoute(req ServiceExecuteRequest) (*RouteDecision, error) {
    // Line 165: routing engine only fires when Harness == ""
    if req.Harness == "" {
        return s.resolveExecuteRouteWithEngine(req)
    }
    // All code below is the explicit-harness path — engine NOT called
    canonical := harnesses.ResolveHarnessAlias(req.Harness)
    ...
    resolvedModel := resolveSubprocessModelAlias(canonical, req.Model) // line 188
    decision := &RouteDecision{
        Harness: canonical,
        Model:   resolvedModel, // line 191 — empty when req.Model == ""
        Reason:  "explicit",
    }
    return decision, nil
}
```

**File:** `/home/erik/go/pkg/mod/github.com/!document!driven!d!x/fizeau@v0.10.14/service_execute.go:161–201`

### `service_execute.go:325–338` — `validateExplicitHarnessModel`

```go
func validateExplicitHarnessModel(name string, cfg harnesses.HarnessConfig, model, provider string) error {
    if model == "" || cfg.TestOnly || cfg.IsHTTPProvider || name == "fiz" {
        return nil  // ← silent pass when model is empty
    }
    ...
}
```

**File:** `service_execute.go:325–338`

Empty model passes validation without error — no routing fallback, no error.

### `service_models.go:166–191` — `resolveSubprocessModelAlias` + `claudeCLIExecutableModel`

```go
func resolveSubprocessModelAlias(harness, model string) string {
    switch harness {
    case "claude":
        return claudeCLIExecutableModel(model)
    ...
    }
}

func claudeCLIExecutableModel(model string) string {
    normalized := strings.ToLower(strings.TrimSpace(model))
    switch {
    case normalized == "sonnet" || ...: return "sonnet"
    case normalized == "opus" || ...:   return "opus"
    case normalized == "haiku" || ...:  return "haiku"
    default:
        return model  // ← returns "" when model == ""
    }
}
```

**File:** `service_models.go:166–191`

`resolveSubprocessModelAlias("claude", "")` → `claudeCLIExecutableModel("")` → `""` (default case returns the input unchanged).

### `service_routing.go:1066–1088` — `resolveExecuteRouteWithEngine`

```go
func (s *service) resolveExecuteRouteWithEngine(req ServiceExecuteRequest) (*RouteDecision, error) {
    rr := RouteRequest{
        Profile:  req.Profile,
        Model:    req.Model,
        Harness:  req.Harness,   // ← Harness IS passed to the engine
        MinPower: req.MinPower,
        ...
    }
    dec, err := s.ResolveRoute(context.Background(), rr)
    ...
    return dec, nil
}
```

**File:** `service_routing.go:1066–1088`

The engine already accepts `Harness` as a constraint (`routing/engine.go:21: Harness string // hard preference; constrains routing to one harness`). So calling `resolveExecuteRouteWithEngine` with `req.Harness` set routes within that harness's eligible models.

---

## Harness `AutoRoutingEligible` Values

From `internal/harnesses/registry.go`:

| Harness    | AutoRoutingEligible | Notes                         |
|------------|---------------------|-------------------------------|
| codex      | true                | Class 1 — eligible            |
| claude     | true                | Class 1 — eligible            |
| fiz        | true                | Class 1 — eligible (local)    |
| gemini     | false               | Class 2 — NOT eligible        |
| opencode   | false (unset)       | Class 2 — NOT eligible        |
| pi         | false (unset)       | Class 2 — NOT eligible        |

---

## Root Cause Summary

When `ServiceExecuteRequest{Harness: "claude", Model: "", Profile: "code-economy"}` is submitted:

1. `resolveExecuteRoute` takes the `req.Harness != ""` branch (line 165)
2. `validateExplicitHarnessModel` returns `nil` because `model == ""` (line 326)
3. `resolveSubprocessModelAlias("claude", "")` returns `""` (line 188)
4. `RouteDecision{Model: ""}` is returned — empty model, no routing engine, no error

DDx then calls `runSubprocess` with `decision.Model = ""`, which invokes `claude --print -p --verbose --output-format stream-json <prompt>` — **no `--model` flag** — so Anthropic CLI uses the user's session default.

---

## Fix Design

In `service_execute.go:resolveExecuteRoute`, add an early exit after resolving the canonical harness:

```go
// When Harness is set but Model is empty AND routing inputs (Profile or MinPower)
// are present, run the routing engine within the harness's eligible models
// instead of returning a silent empty-model decision.
if req.Model == "" && (req.Profile != "" || req.MinPower != 0) {
    if !cfg.AutoRoutingEligible {
        return nil, fmt.Errorf("no auto-resolution available for harness=%q: "+
            "harness does not support auto-routing; supply an explicit --model", canonical)
    }
    return s.resolveExecuteRouteWithEngine(req)
}
```

This inserts between the `cfg, _ := s.registry.Get(canonical)` line and the `validateExplicitHarnessProfile` call. The engine already respects `req.Harness` as a hard constraint via `RouteRequest.Harness`, so it will only pick models within the claude harness.

For Class 2 harnesses (AutoRoutingEligible=false): the returned error is an explicit, human-readable message rather than silent empty Model.

---

## Tests Required (AC4/AC5)

**AC4 — positive:** `TestExecute_ExplicitHarnessEmptyModelWithProfile_RoutesWithinHarness`
- Input: `ServiceExecuteRequest{Harness: "claude", Model: "", Profile: "code-economy"}`
- Assert: `decision.Model` is non-empty AND is a claude-family model

**AC5 — negative:** `TestExecute_ExplicitHarnessEmptyModelNoProfile_FailsClearly`
- Input: `ServiceExecuteRequest{Harness: "claude", Model: "", Profile: "", MinPower: 0}`
- Assert: returns clear error (not silent empty model)

These tests are written in `~/Projects/fizeau/service_execute_harness_pin_test.go` as part of the upstream fizeau fix.

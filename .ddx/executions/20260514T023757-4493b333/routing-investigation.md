# ddx-e0b95b4a — default-policy routing resolving ordinary work to Opus

## Verdict: DDx-side bug in implementation routing; readiness/intake is by design

The implementation request envelope sent by `ddx work` from
`/home/erik/Projects/ddx` was empty in every meaningful routing field:
`Policy=""`, `Harness=""`, `Provider=""`, `Model=""`, `MinPower=0`.
Fizeau then resolved an empty `Policy` to its default policy band and scored
Claude Opus on ordinary implementation work. The DDx side of the request was
wrong; Fizeau's default-policy behavior is the documented fallback when the
caller declines to pick a policy. The fix is on the DDx side.

Readiness/intake routing is a separate path that intentionally requests the
strongest available policy (see `NewPreClaimIntakeHook` /
`SelectStrongestProfile`). That is a design choice for splitter quality, not a
bug — but it is the reason readiness dispatches show up as Opus too. It is in
scope of this bead only as a diagnostic separation; the actual readiness
routing strategy is not changed here.

## Evidence

Two attempts captured in `.ddx/attachments/`:

`.ddx/attachments/ddx-c3219628/events.jsonl` (worker-status bug, $15.0776):

```json
{"kind":"routing","summary":"provider=claude model=opus",
 "body":{"requested_profile":"","requested_tier":"",
         "resolved_model":"opus","resolved_provider":"claude"}}
{"kind":"execution-routing-intent",
 "body":{"actual_model":"opus","actual_harness":"claude",
         "requested_profile":"","requested_tier":"standard",
         "routing_intent_source":"heuristic"}}
{"kind":"cost",
 "body":{"harness":"claude","model":"opus","cost_usd":15.077649250000004}}
```

`.ddx/attachments/ddx-6cde5ffd/events.jsonl` (embedded registry, $6.83):

```json
{"summary":"do route claude/opus reason=policy=default; score=229.5"}
```

Both runs are no-flag `ddx work` invocations. Both carry
`requested_profile: ""` — DDx-side. Fizeau's `policy=default` is what shows
up because the request lacks any DDx-supplied policy.

## Root cause (file:line)

`cli/cmd/execute_loop_shared.go:146`

```go
autoInferTier := noRoutingFlags && !projectHasRoutingConfig(projectRoot)
```

When `autoInferTier` is false, the per-bead profile selection block at
`execute_loop_shared.go:397-415` is skipped: `initialProfile := spec.Profile`
(empty for no-flag) is forwarded to `singleTierAttempt` and ends up as
`ServiceExecuteRequest.Policy = ""` in `service_run.go:244`.

The bug was in `projectHasRoutingConfig` itself
(`execute_loop_shared.go:536-556` before this commit):

```go
if strings.TrimSpace(a.Model) != "" { return true }
if len(a.Endpoints) > 0           { return true }   // ← over-broad
return false
```

`agent.endpoints` is transport configuration: it lists where local-inference
providers live (`lmstudio`, `omlx`, etc.). It does NOT pin a routing decision
the way `agent.model` does. The schema's own description is explicit:

```
"Endpoint-first native agent provider targets. These blocks intentionally
 omit provider profile names and model pins; routing discovers live models
 from /v1/models."
```

`/home/erik/Projects/ddx/.ddx/config.yaml` has four `agent.endpoints` and no
`agent.model`. Pre-fix, that made `projectHasRoutingConfig` return true, which
suppressed zero-config tier inference, which sent `Policy=""`. Post-fix, only
an explicit `agent.model` suppresses inference.

## Fix

`cli/cmd/execute_loop_shared.go:536-556`: drop the `len(a.Endpoints) > 0`
branch. Only `strings.TrimSpace(a.Model) != ""` suppresses zero-config tier
inference now.

The fix is policy/tier-name-free — it does not hardcode any provider, model,
or tier name (AC4). It deletes one over-broad check; everything else continues
to work via the existing `profileSelector` / `InferTier` /
`SelectImplementationProfile` chain that picks a policy by metadata from
Fizeau's `ListPolicies` snapshot.

## Regression coverage

`cli/cmd/work_test.go:177-274` —
`TestProjectHasRoutingConfig_EndpointsAreTransportNotRoutingPin` (5
sub-cases):

- `no .ddx/config.yaml` → false
- `library-only config` → false
- `endpoints only, no model pin (production scenario)` → false (the
  regression case)
- `model pin counts as routing config` → true
- `model pin coexists with endpoints` → true

`agent.endpoints` alone is now classified as transport, not routing, so the
implementation path runs the same zero-config inference it does for a fully
empty config — which is the path already covered by
`TestWorkZeroConfigInferredTaskSelectsFizeauPolicyWithoutInitialMinPower`
(`cli/cmd/work_test.go:130-175`) asserting:

- `lastReq.Policy == "default"` (selected by metadata)
- `lastReq.Harness/Provider/Model` empty
- `lastReq.MinPower == 0`

That existing test exercises the production envelope shape AC1 wants; the new
test exercises the regression-specific gate.

## Readiness/intake routing (AC2)

Covered by existing tests in `cli/internal/agent/preclaim_intake_hook_test.go`:

- `TestPreClaimIntakeHook_DispatchesWithStrongestProfileNoStrongPowerTrick`
  (lines 135-164) — asserts intake dispatch sends `Policy="smart"` (the
  strongest band from the live snapshot), no Harness/Provider/Model pins.
- `TestLifecycleHooks_UnpinnedWorkersStillUseProfileSelection` (lines 251-298)
  — asserts intake → `"smart"`, lint → `"cheap"`. The contrast documents the
  design: lint stays cheap, intake/splitter goes strong.

So readiness/intake routing is wired through a different selector
(`SelectStrongestProfile`, `NewPreClaimIntakeHook:186`) and is exercised by a
different test surface. An operator can diagnose an expensive readiness
default independently of implementation routing by looking at intake-hook
tests (and at the `MODE: intake` prompt in session logs / attempt
`PromptSource == "bead-lifecycle-intake"`).

The readiness/intake strong-policy choice is intentional (see commit
`565f04575 feat: wire strong-model intake decomposition into ddx work
[ddx-f58797f1]`). Changing it would break the splitter tests above and is
out of scope for this bead — it is a separate operator-economics decision.

## What's NOT covered by this fix

- Readiness/intake will still route to the strongest available policy band.
  If `claude/opus` is the strongest band in the operator's Fizeau snapshot,
  readiness dispatches will still hit Opus. The fix here does not touch that
  path. If the operator wants readiness on a cheaper band, that is a
  follow-up bead targeting `NewPreClaimIntakeHook` / `SelectStrongestProfile`
  policy.
- The 30s readiness timeout the operator observed is governed by
  `--preclaim-timeout` and is not changed here.
- Fizeau's default-policy behavior (when DDx genuinely supplies an empty
  Policy) is also unchanged: that is the correct Fizeau contract and the
  bug was that DDx was reaching that fallback for projects with endpoint
  config.

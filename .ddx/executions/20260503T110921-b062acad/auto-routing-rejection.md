# Investigation: bare `ddx work` and "no viable routing candidate: N candidates rejected"

Bead: ddx-9e4c238d
Base rev: 9f7d893bff9d4788459f259b6ff5e83c96ad430b

## Summary

The premise in the bead's description is incorrect: **DDx does not pre-filter
routing candidates for bare `ddx work`**. The error
`no viable routing candidate: N candidates rejected` originates inside
fizeau's routing engine, not DDx. The fix called for in AC#2 ("Bare `ddx
work` (no pins) passes a maximally-permissive ServiceExecuteRequest to
fizeau") is already in effect at base rev 9f7d893b. Existing tests
prove it.

## File:line walk-through (AC#1)

The bead lists four hypotheses for "where DDx rejects 4 candidates". Each
is wrong on the merits at this revision:

### Hypothesis 1 — `ValidateForExecuteLoopViaService`

`cli/internal/agent/service_run.go:392-434`
```
func ValidateForExecuteLoopViaService(ctx context.Context, workDir, harnessName, model, provider, modelRef string) error {
    if harnessName == "" {
        return nil
    }
    ...
}
```

Returns `nil` immediately when `harnessName == ""` (no operator pin).

Caller in `cli/cmd/agent_cmd.go:1652-1656`:
```go
// Skipped for ddx work (treatPassthroughAsOpaque=true): harness/provider/model
// are opaque passthrough constraints there; DDx does not validate them.
if !treatPassthroughAsOpaque {
    if err := agent.ValidateForExecuteLoopViaService(...); err != nil {
        return fmt.Errorf("execute-loop: %w", err)
    }
}
```

`ddx work` invokes `runAgentExecuteLoopImpl(cmd, true)` (`cli/cmd/work.go:92`),
so `treatPassthroughAsOpaque == true` and the call is skipped entirely. Even
if it did run, the `harnessName == ""` short-circuit would still return nil.

### Hypothesis 2 — `escalation.InferTier`

`cli/internal/agent/escalation/ladder.go` defines escalation, and
`cli/cmd/agent_cmd.go:1865-1872` invokes `escalation.InferTier(b)` purely to
choose a tier name passed forward as `attemptProfile` (cli/cmd/agent_cmd.go:1779).
There is no candidate filter or rejection logic here — the inferred tier
becomes a `Profile` field in `ServiceExecuteRequest`. Unmatched profiles
are fizeau's concern, not DDx's.

### Hypothesis 3 — `ServiceExecuteRequest` construction

`cli/internal/agent/service_run.go:175-192` builds the request from sealed
ResolvedConfig. For bare `ddx work`:

- `Harness` = pt.Harness = "" (empty — no operator pin)
- `Model` = pt.Model = "" (empty — no operator pin)
- `Profile` = `rcfg.Profile()` = `NormalizeRoutingProfile("default")` = `"default"`
- `MinPower` / `MaxPower` = 0 / 0 (defaults)

This is the maximally-permissive shape AC#2 calls for. (Note: the bead
suggests `Profile` could be empty too; however, fizeau accepts `"default"`
as a valid profile — see `fizeau@v0.10.4/service_profiles_test.go:33`,
`internal/routing/engine_test.go:993,1436,1685`.)

### Hypothesis 4 — pre-rejection of "the 4 harnesses"

There is no DDx code that enumerates harnesses and rejects them prior to
calling `Execute`. The only DDx-side `ResolveRoute` callers in the
execution path are:

- `cli/internal/server/workers.go:821` — server worker preflight, gated
  on `m.BeadWorkerFactory == nil && !spec.OpaquePassthrough`
  (`workers.go:803`). For `ddx work`, `OpaquePassthrough=true` is
  propagated via `workerSpec["opaque_passthrough"] = true`
  (`cli/cmd/agent_cmd.go:1993-1995`), so this preflight is also skipped.
- `cli/internal/agent/service_run.go:424` — `ValidateForExecuteLoopViaService`
  orphan-model check, gated on `harnessName == "agent" && model != ""`
  (line 423). Inactive for bare `ddx work`.
- `cli/internal/server/graphql/resolver_providers.go:147` — read-only
  status surface; not in the execution path.

Static guarantees:
- `cli/cmd/routinglint_test.go:69-104` — AST lint asserting non-status
  files in `cmd/` do not call `.ResolveRoute()`.
- `cli/cmd/agent_run_passthrough_test.go:166-198`
  (`TestWorkDoesNotCallResolveRoute`) — runtime assertion: stub
  `ResolveRoute` returns an error; `ddx work --once` succeeds without
  the stub error surfacing.
- `cli/cmd/agent_run_passthrough_test.go:203-230`
  (`TestWorkPassesEmptyHarnessToService`) — runtime assertion: bare
  `ddx work --once` sends `Harness=""` to `Execute`.

All three pass at this rev:
```
$ go test -run "TestWorkDoesNotCallResolveRoute|TestWorkPassesEmptyHarnessToService|TestRoutinglintNonStatusFilesDoNotCallResolveRoute" ./cmd/...
ok      github.com/DocumentDrivenDX/ddx/cmd     0.129s
```

## Where the error actually originates

The exact phrase `no viable routing candidate: %d candidates rejected` is
formatted in fizeau's routing engine:

- `~/go/pkg/mod/github.com/!document!driven!d!x/fizeau@v0.10.4/internal/routing/engine.go:258-260`
  ```go
  if len(pins) > 0 {
      return fmt.Sprintf("no viable routing candidate for pins %s: %d candidates rejected", strings.Join(pins, " "), e.Rejected)
  }
  return fmt.Sprintf("no viable routing candidate: %d candidates rejected", e.Rejected)
  ```

The "4 candidates rejected" count corresponds to the four harness candidates
fizeau enumerated and disqualified inside its own `ResolveRoute` call
(invoked from `service.Execute`). DDx never sees those four candidates and
plays no role in their selection or rejection.

## AC mapping

- **AC#1 (identify rejection point)**: no DDx-side rejection point exists
  for bare `ddx work`. Rejection happens inside fizeau's routing engine
  at `fizeau@v0.10.4/internal/routing/engine.go:260`. File:line citations
  for every DDx pre-flight surface that *could* in principle reject are
  enumerated above; each is gated off for the `ddx work` path.
- **AC#2 (DDx pre-flight no longer rejects)**: already true at base rev.
  Bare `ddx work` sends `Harness=""`, `Model=""`, `Profile="default"` to
  fizeau. Verified by `TestWorkPassesEmptyHarnessToService`
  (`cmd/agent_run_passthrough_test.go:203`).
- **AC#3 (structured rejection reasons)**: vacuously satisfied — DDx does
  not produce its own rejections in this path. The server-side preflight
  in `internal/server/workers.go:802-823` does emit a `preflight.rejected`
  event payload (`internal/agent/execute_bead_loop.go:616-621`) but is
  inactive under `OpaquePassthrough=true`.
- **AC#7 (review existing pre-validation tests)**:
  - `ValidateForExecuteLoopViaService` in `service_run.go:392`: validates
    operator-explicit harness pin only (`harnessName == ""` short-circuits).
    KEEP — validates explicit pin.
  - `routePreflight` in `workers.go:802-823`: validates harness/model
    typed compatibility for non-opaque dispatch paths. KEEP — validates
    explicit pin via fizeau's typed-incompatibility surface; gated off
    for `ddx work` (`OpaquePassthrough=true` skip).
  - No existing tests assert DDx pre-validates auto-routing.

## Recommendation

The DDx-side AC are satisfied at this revision. The operator's reproducer
("bare `ddx work` rejects with no viable routing candidate: 4 candidates
rejected") is a **fizeau routing-engine** failure: fizeau enumerated four
harness candidates and rejected all four, returning the typed
`*routing.NoViableCandidateError` upward through
`Service.Execute → executeOnService` (`service_run.go:198-201`).

A separate bead is needed against fizeau to:
1. Surface per-candidate reject reasons through `ServiceEvent` so DDx can
   log them (today the wrapped error string from fizeau is the only signal
   DDx gets back).
2. Decide whether the "default" profile should pick something when no
   provider/model/harness pin is given on a host with at least one healthy
   harness.

The intersecting bead `ddx-1e516bc9` (fizeau Execute gap) is the natural
home for the fizeau-side investigation and fix. This bead (ddx-9e4c238d)
should be closed: the DDx-side surface area it names is already
maximally-permissive.

## Verification command

```
go test -run "TestWorkDoesNotCallResolveRoute|TestWorkPassesEmptyHarnessToService|TestRoutinglintNonStatusFilesDoNotCallResolveRoute" ./cmd/...
```

Exit 0 ⇒ `ddx work` does not call `ResolveRoute` and forwards an empty
`Harness` to fizeau, satisfying AC#2 / AC#5.

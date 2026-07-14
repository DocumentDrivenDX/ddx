---
ddx:
  id: TP-020
  depends_on:
    - FEAT-001
    - FEAT-006
    - FEAT-010
---
# Test Plan: Complete DDx Consumer Boundary For Fizeau

**Status:** Proposed

> **Supersedes the old DDx-side routing planner plan.** Fizeau is the complete
> harness-of-harnesses, not only a router. This plan proves that DDx consumes
> one Fizeau request/terminal contract and never becomes a partial Claude Code,
> Codex, Gemini, native-harness, session-loop, or process supervisor.

## Governing Assertion

The tested authority chain is:

```text
workflow intent
  -> DDx bead/claim/worktree/base revision/request
  -> Fizeau complete agent session
  -> typed immediate Fizeau error or public final outcome + opaque artifacts
  -> DDx repository gates/landing/preservation/bead-attempt decision
```

Fizeau owns concrete invocation, the session/tool loop, routing and provider
fallback, subprocess/process-tree control, progress/events, native session
logs, usage, cancellation, and harness-specific continuation. DDx owns beads,
queue/claims, attempt worktree and base revision, request construction,
repository gates, landing/preservation, durable DDx evidence, bead-attempt
success, and whether to launch a new bead attempt. A Fizeau `success` outcome
must never close a bead without DDx-owned repository evidence.

## Scope

Validate all DDx consumer obligations at the Fizeau boundary:

- map DDx-owned work facts into one Fizeau request;
- forward operator-supplied harness/provider/model constraints unchanged;
- keep route selection, provider fallback, any contract-defined continuation,
  and process-tree control inside Fizeau; cancel only the context DDx supplied
  to the in-flight `Execute`;
- treat non-terminal service events, session logs, and tool events as
  Fizeau-owned payloads, consuming only the public terminal contract fields;
- consume either the typed immediate error returned by `Execute` or one final
  `ServiceEvent` carrying `ServiceFinalData` before classifying the DDx attempt;
- decide bead success from DDx-owned commits, gates, review, and
  landing/preservation evidence;
- launch a later bead attempt only as a new Fizeau request, never by directly
  resuming or repairing a concrete harness session.

## Out Of Scope

- Whether Fizeau chooses the best harness/provider/model or implements fallback
  correctly. Those assertions belong in Fizeau's CONTRACT-003 conformance suite.
- Claude Code, Codex, Gemini, or native-harness protocol fixtures in DDx.
- The correctness of repository gates, review policy, or landing beyond proving
  that Fizeau terminal success cannot bypass them.

## Test Harness

Use a contract fake implementing the pinned public Fizeau service interface:
`Execute(context.Context, ServiceExecuteRequest) (<-chan ServiceEvent, error)`.
The fake records the request, can return an immediate typed error, emits
arbitrary opaque progress/log/tool payloads, emits a final event with public
`ServiceFinalData`, and records cancellation of the Execute context. Where
process ownership matters, the fake Fizeau service launches and reaps a
sentinel child; DDx may cancel only its Execute context and must neither
discover nor signal that child.

Install sentinel executables named `claude`, `codex`, and `gemini` at the front
of `PATH`. Each sentinel fails the test if invoked. This proves the DDx path
reaches only the injected Fizeau service. No test should encode a real harness's
stream format in DDx fixtures.

## Test Cases

### Request Construction And Raw Constraint Passthrough

- `TestFizeauConsumer_RequestCarriesOnlyDDxOwnedWorkFacts`
- `TestReviewRequestRaisesMinPowerWithoutSettingHarnessProviderModel`
- `TestRetryEscalationChangesPowerOnly`
- `TestEscalationPreservesOperatorMaxPower`
- `TestOperatorPinCannotInfluenceDDxPolicy`
- `TestReviewRequestOmitsImplementerConcreteRoute`
- `TestActualModelIdentityIsAuditOnly`
- `TestDDxDoesNotOriginateRoutingPins`
- `TestRunForwardsModelStringUnchanged`
- `TestRunForwardsHarnessAndProviderUnchanged`
- `TestTryForwardsModelStringUnchanged`
- `TestWorkForwardsPassthroughPinsUnchanged`
- `TestFizeauConsumer_PassthroughInvariantUnderPowerChange`

The review and retry tests begin with no operator pins and assert that DDx
changes only `MinPower`; `MaxPower`, `Harness`, `Provider`, `Model`, and policy
remain unset. Separate pinned cases prove explicit operator strings and an
operator `MaxPower` remain byte-for-byte identical across escalation. Pin
presence, pin values, and actual concrete model identity must not change retry,
review-tier, queue, success, or escalation policy.

### No DDx-Side Harness Or Routing Runtime

- `TestRunDoesNotNormalizeModelRef`
- `TestTryDoesNotNormalizeModelRef`
- `TestRunPreservesLiteralModelStringQwen36`
- `TestTryPreservesLiteralModelStringQwen36`
- `TestWorkDoesNotChooseRouteFromModel`
- `TestRunDoesNotFallbackToDifferentModel`
- `TestRunDoesNotInspectCatalog`
- `TestTryDoesNotFuzzyMatchProviderOrModel`
- `TestFizeauConsumer_DoesNotExecConcreteHarness`
- `TestFizeauConsumer_DoesNotParseHarnessOutput`
- `TestFizeauConsumer_NoConcreteHarnessDependency`
- `TestFizeauConsumer_HasNoCatalogDependency`

The dependency test is an AST/import and command-construction guard over the
DDx consumer path. It fails if production code names a concrete harness binary,
imports a harness adapter or model/route catalog, queries a model/route catalog,
constructs a harness-specific subprocess, adds a harness-output parser outside
the upstream Fizeau module, or branches on concrete route identity. There is no
catalog dependency or query in DDx execution policy for any purpose.

### Opaque Events, Logs, Tools, And Usage

- `TestFizeauConsumer_ProgressEventsRemainOpaque`
- `TestFizeauConsumer_SessionLogIsLinkedWithoutParsing`
- `TestFizeauConsumer_ToolEventsAreNotNormalizedByDDx`
- `TestFizeauConsumer_UsageComesFromTypedFizeauOutcome`
- `TestFizeauConsumer_FinalServiceDataIsDecoded`
- `TestFizeauConsumer_PhaseLabelDoesNotMutateServiceEvent`

Feed non-terminal payloads containing unknown fields and tool names that DDx
has never seen, plus unfamiliar opaque `SessionLogPath` values. DDx may retain
or forward bytes and envelope references; it must not reject, normalize,
count, or branch on their contents.

### Cancellation, Fallback, Continuation, And Process Ownership

- `TestFizeauConsumer_CancelUsesExecuteContext`
- `TestFizeauConsumer_ProviderFallbackStaysInsideFizeau`
- `TestFizeauConsumer_HasNoDDxContinuationAPI`
- `TestFizeauConsumer_ProcessTreeStaysInsideFizeau`
- `TestFizeauConsumer_DoesNotSignalFizeauChildDirectly`

The cancellation fixture proves DDx cancels the context supplied to `Execute`,
the Fizeau stream terminates according to the pinned contract, and only the
fake Fizeau service reaps its sentinel process tree. Provider fallback remains
inside one Fizeau execution. Current v0.14.50 has no continuation method. Any
future continuation path remains blocked on the upstream CONTRACT-003 bead
until a compatible public API is pinned; only then may a separate
`TestFizeauConsumer_ContractDefinedContinuationStaysInsideFizeau` be enabled.
DDx must not invent a `Continue`/`Cancel` method, launch a second concrete
harness, or parse a resume token.

### Fizeau Operation Outcome Versus Bead Outcome

- `TestTry_FizeauSuccessDoesNotBypassRepositoryGates`
- `TestTry_FizeauSuccessWithoutResultRevisionDoesNotCloseBead`
- `TestTry_FizeauFailurePreservesDDxAttemptEvidence`
- `TestTry_FizeauImmediateErrorPreservesDDxAttemptEvidence`
- `TestTry_BeadSuccessRequiresDDxLandingEvidence`
- `TestWork_NewBeadAttemptStartsOnlyAfterFizeauOperationEnds`
- `TestWork_NewAttemptUsesNewFizeauRequest`
- `TestWork_DoesNotRetryInsideFizeauSession`

Exercise at least these matrices: Fizeau success + failing gate; Fizeau success
+ no result revision; Fizeau success + clean gates + successful landing;
Fizeau failure + preserved repository change; Fizeau cancellation; and Fizeau
typed immediate route-unavailable. Assert the Fizeau outcome and DDx
bead-attempt outcome are stored as separate fields.

### Config Boundary

- `TestConfigSeparatesDDxAttemptAndFizeauSessionSettings`
- `TestConfigRejectsAgentRoutingDefaults`
- `TestConfigLeavesFizeauRoutingToUpstream`
- `TestConfigHasNoConcreteHarnessProcessControls`

## Fixtures

- operator input fixture with literal model strings such as `qwen36`,
  `Qwen3.6-35B-A3B`, and `vendor/qwen36`
- passthrough-pins fixture covering `--harness`, `--provider`, and `--model`
- execution-config fixture covering DDx-owned queue, new-attempt, and power-intent knobs
- Fizeau contract fake with opaque events/log attachments, typed usage, public
  final outcomes, cancellation observation, and a service-owned child tree
- sentinel concrete-harness binaries that record any forbidden direct invocation
- repository fixture with independent gate, result-revision, preserve, and
  landing outcomes

## Verification Commands

Targeted contract suite:

```bash
cd cli && go test ./internal/agent/... -run 'TestFizeauConsumer_|TestReviewRequest|TestRetryEscalation|TestEscalationPreserves|TestOperatorPin|TestActualModel|TestDDxDoesNotOriginate|TestTry_Fizeau|TestTry_BeadSuccess|TestWork_NewBeadAttempt|TestWork_NewAttempt|TestWork_DoesNotRetry'
```

CLI passthrough and configuration surface:

```bash
cd cli && go test ./cmd/... ./internal/config/... -run 'TestRunForwards|TestTryForwards|TestWorkForwards|TestRunDoesNot|TestTryDoesNot|TestWorkDoesNot|TestConfig'
```

Owning package gates and repository gate:

```bash
cd cli && go test ./internal/agent/... ./cmd/... ./internal/config/...
lefthook run pre-commit
```

## Exit Criteria

- DDx test coverage proves raw passthrough strings survive unchanged from CLI
  parsing to execution request construction.
- Structural and sentinel tests prove DDx has no model/route catalog dependency
  and never imports, invokes, parses, continues, falls back, or signals a
  concrete harness.
- Contract-fake tests prove non-terminal events/logs/tools stay opaque; immediate
  errors plus public final-event fields are handled; cancellation uses the
  Execute context; and Fizeau owns its child process tree.
- Outcome-matrix tests prove Fizeau session success and DDx bead-attempt success
  are separate, and a new bead attempt starts only after the prior Fizeau
  operation ends by immediate error or final event.
- The targeted commands, owning-package gates, and `lefthook run pre-commit`
  pass.
- Fizeau runtime behavior remains covered in Fizeau-owned CONTRACT-003 tests,
  not duplicated in DDx.

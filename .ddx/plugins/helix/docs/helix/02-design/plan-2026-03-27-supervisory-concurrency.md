# HELIX Plan: Supervisory Concurrency And Interactive Refinement

**Date**: 2026-03-27  
**Scope**: self-managing `helix run` with concurrent interactive spec and issue refinement

## Problem Statement and User Impact

HELIX already defines `helix-run` as supervisory autopilot, but the current
contract is still too static. The user wants to run `helix run` in one session
while continuing to refine specs and tracker issues in another, without the
automated loop either ignoring those changes or trampling them.

If HELIX does not support that mode safely, the product remains a bounded loop
plus manual orchestration rather than a durable self-managing control system.

## Requirements Analysis

### Functional Requirements

- `helix run` must tolerate concurrent local tracker mutations made by an
  operator or another agent.
- The runner must re-read tracker state before claim and before close rather
  than assuming the selected issue remained unchanged.
- Interactive spec or design changes that materially invalidate open execution
  work must cause `helix run` to re-check or stop rather than blindly
  continuing.
- HELIX must distinguish execution-ready work from refinement work so the
  runner does not pick the wrong class of issue.
- The tracker must support explicit issue replacement or supersession semantics
  for refined or split work.
- Deterministic tests must cover operator/runner concurrency scenarios, not
  only low-level file locking.

### Non-Functional Requirements

- Preserve bounded execution and least-power routing.
- Keep the built-in tracker file-backed and local-first.
- Prefer fail-closed behavior over hidden automation when issue validity is
  uncertain.
- Keep state transitions deterministic enough for shell-harness verification.

### Constraints

- The current HELIX surface is Bash-based and tracker-first.
- The solution must preserve direct interactive command use.
- The repo should not broaden into a general multi-user distributed tracker.

## Architecture Decisions

### Decision 1: Treat concurrent operator changes as a first-class control path

- Alternatives considered:
  - ignore queue drift until the next run
  - rely on claim ownership alone
  - explicitly revalidate issue state at execution boundaries
- Chosen approach:
  - explicitly revalidate issue state before claim and before close
- Why:
  - it is the smallest reliable step that turns the tracker into a concurrency
    contract rather than passive storage

### Decision 2: Separate execution eligibility from generic open work

- Alternatives considered:
  - keep all open issues equally runnable
  - infer execution eligibility from heuristics only
  - define explicit execution-safe issue classes or labels
- Chosen approach:
  - define an explicit execution-safe subset of issues for `helix run`
- Why:
  - the operator must be able to create refinement work without the runner
    trying to implement it opportunistically

### Decision 3: Add issue supersession semantics instead of silent mutation

- Alternatives considered:
  - mutate the old issue in place until it no longer resembles the original
  - close and recreate work manually without a relationship
  - add explicit replacement/supersession semantics
- Chosen approach:
  - add explicit supersession/replacement relationships
- Why:
  - interactive refinement needs a durable way to invalidate or replace stale
    execution slices while preserving traceability

## Interface Contracts

### Runner Contract

- `helix run` must:
  - select a candidate only from execution-safe ready work
  - re-read the issue immediately before claim
  - refuse to claim work whose governing metadata materially changed
  - re-read the issue immediately before close
  - refuse to close work that has been superseded, structurally changed, or
    made invalid by upstream refinement

### Tracker Contract Additions

- issue execution class or explicit execution-safe labeling
- issue supersession or replacement metadata
- structural mutation surfaces sufficient for refinement workflows
- conflict-visible update behavior for mutation paths used by concurrent
  operator and runner sessions

### Queue-Drift Contract

- material drift includes:
  - changed `spec-id`
  - changed `deps`
  - changed parent or replacement relationship
  - superseded execution issue
  - execution class change that makes the issue no longer runnable

## Data Model

### Required Metadata Capabilities

- update execution-eligibility metadata
- update structural metadata (`spec-id`, `parent`, `deps`)
- represent superseded or replaced work
- preserve ownership and freshness signals for claims

### Relationship Model

- refinement issues may create or revise execution issues
- execution issues may be superseded by refined slices
- the runner must honor those relationships on the next boundary check

## Error Handling Strategy

- If tracker state cannot be trusted, stop rather than guess.
- If the selected issue drifts before claim, skip it and re-run queue
  evaluation.
- If the claimed issue drifts before close, leave it open or mark follow-up
  work rather than falsely closing it.
- If supersession attribution is unclear, stop and require operator guidance.

## Security Considerations

- Hidden continuation after upstream changes is a control-safety failure.
- Concurrent local sessions are trusted operators, but trust does not remove
  the need for explicit validation boundaries.
- The runner must not use stale local assumptions to override newer operator
  intent recorded in tracker or spec artifacts.

## Test Strategy

- deterministic tracker tests for structural mutation and supersession flows
- deterministic loop tests for:
  - issue changes before claim
  - issue changes before close
  - superseded execution work
  - refinement work appearing while the runner is between cycles
  - execution-safe filtering for ready work

## Implementation Plan with Dependency Ordering

1. Update product/design contracts to define concurrent supervisory behavior.
2. Extend tracker contract for structural mutation and supersession semantics.
3. Teach `helix run` to revalidate before claim and before close.
4. Define execution-safe issue selection for the runner.
5. Add deterministic concurrency and queue-drift tests.

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| The runner keeps executing stale work after operator refinement | H | H | Add pre-claim and pre-close revalidation |
| Refinement issues are treated as runnable implementation work | M | H | Define explicit execution-safe eligibility |
| Supersession semantics become ad hoc and hard to test | M | M | Define tracker contract first, then implement |
| The feature expands into general distributed coordination | M | M | Keep the design local-first and bounded |

## Observability

- log when the runner skips a candidate because of material drift
- log when a claimed issue cannot be safely closed due to changed metadata
- surface supersession and queue-drift reasons in tracker notes or CLI output

## Initial Issue Slices

1. Specify supervisory concurrency and queue-drift contract in governing docs.
2. Add tracker structural mutation and supersession APIs.
3. Implement pre-claim and pre-close revalidation in `helix run`.
4. Add deterministic interactive-concurrency tests for the wrapper.

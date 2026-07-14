---
ddx:
  id: SD-023
  depends_on:
    - FEAT-019
    - FEAT-006
    - FEAT-004
    - FEAT-012
    - FEAT-014
    - SD-006
    - TD-006
    - TD-010
---
# Solution Design: Agent Evaluation and Prompt Comparison

## Purpose

FEAT-019 adds an evaluation layer above DDx's work tracker and git-aware
executor. The layer answers questions about DDx-controlled inputs and observed
work results:

- Did a prompt, rubric, or work-fact change improve the result?
- Did a higher abstract `MinPower` floor produce stronger work?
- Does a preserved bead attempt still reproduce from the same base revision?
- Did a stronger reviewer find problems that a weaker review missed?

DDx does not answer which concrete harness, provider, or model should perform
the work. Fizeau is the full agent runtime and owns that decision. Returned
route identity is execution audit evidence, never an evaluation variable or a
policy input.

This design is authoritative for TP-019. Tests derived from TP-019 must enforce
the ownership boundary and the current Fizeau public service contract described
here.

## Scope

In scope:

- Isolated DDx worktrees for side-effecting comparison arms
- Route-neutral comparison-arm identity and persistence
- Prompt, rubric, abstract `MinPower`, and work-fact experiments
- Unchanged passthrough of explicit operator constraints
- Repository diff, gate, commit, and evidence capture
- Route-blind grading through a stronger Fizeau request
- Benchmark aggregation over DDx-controlled variables
- Replay from preserved DDx execution evidence
- Current Fizeau `Execute` request, event, final, error, and cancellation
  semantics

Out of scope:

- Concrete harness, provider, endpoint, or model selection
- Route catalogs, route preflight, provider health, quota inspection, or
  DDx-managed fallback
- Grouping, ranking, grading, warning, or policy by concrete route identity
- Fizeau's session/tool loop, native logs, process tree, cancellation
  implementation, or continuation implementation
- Prompt optimization loops
- Container or VM isolation
- Cross-project evaluation

## Ownership Boundary

The boundary is architectural, not an implementation convenience.

| Concern | Owner | DDx behavior |
|---|---|---|
| Beads, work selection, dependencies, and attempt state | DDx | Select and track work |
| Base revision, worktree, repository gates, diff, merge/preserve | DDx | Create and evaluate repository evidence |
| Prompt, rubric, role, correlation, permissions, and work facts | DDx | Construct the work request |
| Retry and review strength | DDx | Raise abstract `MinPower` for stronger review intent, or on a distinct new DDx attempt after capability-sensitive evidence; never for infrastructure/route failure |
| Explicit operator pins and `MaxPower` | Operator | Pass through unchanged; never infer, relax, or rewrite |
| Harness, provider, endpoint, model, and fallback | Fizeau | Do not select, validate, rank, or emulate |
| Agent loop, tools, compaction, and native transcript | Fizeau | Treat as runtime-private |
| Native session log | Fizeau | Retain only its opaque public reference |
| Cancellation and process-tree cleanup | Fizeau | Cancel the `Execute` context; do not kill harness processes |
| Harness-specific continuation | Fizeau | Do not implement or emulate a DDx continuation API |
| Final application text, usage, cost, and actual route | Fizeau | Decode public final fields; actual route is audit-only |
| Bead-attempt and evaluation result | DDx | Decide from Fizeau outcome plus repository evidence |

A successful Fizeau session does not by itself make a DDx work item or
comparison arm successful. DDx still evaluates repository gates, required
effects, review evidence, and the governing rubric. Conversely, DDx never
reimplements the runtime in order to produce those effects.

## Current Fizeau Service Contract

This design targets the public Fizeau v0.14.50 in-process contract:

```go
Execute(
    ctx context.Context,
    req fizeau.ServiceExecuteRequest,
) (<-chan fizeau.ServiceEvent, error)
```

### Request Construction

DDx may populate request fields that describe the work, including:

- `Prompt` and `SystemPrompt`
- `WorkDir`
- `Role` and `CorrelationID`
- `Permissions`, timeouts, estimated prompt size, and tool requirement facts
- abstract `MinPower`
- explicit operator-supplied `MaxPower`, `Harness`, `Provider`, `Model`, and
  `Policy`, copied without modification

DDx must leave concrete route fields unset when the operator did not provide
them. It must not derive them from an arm label, task class, rubric, benchmark
suite, prior `RoutingActual`, cost, usage, or a Fizeau diagnostic surface.
DDx must not populate `SelectedRoute` as a route choice.

An operator-supplied constraint remains byte-for-byte the same on every new
operation unless the operator changes it. If a higher DDx `MinPower` conflicts
with an operator pin or cap, Fizeau reports the incompatibility. DDx records the
outcome and requests operator action; it does not remove the pin or choose an
alternative.

### Immediate Errors

`Execute` may return an error before it returns an event channel. DDx records
that as an immediate Fizeau operation error. It must not invent a final event,
start a route fallback, or parse error text to select another route.

The current public transient queue signal is an immediate
`*fizeau.NoViableProviderForNow`. Its `RetryAfter` is the only Fizeau retry time
DDx may use. A worker or evaluation scheduler may defer new work until that
instant after releasing any claim or attempt resources. `RetryAfter` is not
present on `ServiceFinalData` and must not be synthesized from a final error
string.

Other immediate errors may retain their public Go error identity for display
and stable classification when the contract defines one. They do not authorize
DDx to inspect routing internals or perform concrete fallback.

### Event Stream And Final Data

After `Execute` returns a channel, Fizeau owns the session until the stream
ends. DDx may forward non-final `ServiceEvent` values and their metadata as
opaque evidence, but it must not parse tool calls, reconstruct a tool loop, or
derive DDx worker state from native runtime events.

DDx decodes the public final event as `fizeau.ServiceFinalData`:

- `Status`
- `ExitCode`
- `Error`
- `FinalText`
- `DurationMS`
- `Usage`
- `Warnings`
- `CostUSD`
- `SessionLogPath`
- `RoutingActual`

`FinalText` is the application response consumed by grading and other declared
DDx output schemas. `SessionLogPath` is an opaque native-log reference. DDx
does not parse that file to recover output, tools, usage, failures, or routing.

The current generic final `Error` string has no public typed cause or stage.
DDx stores it verbatim as opaque, unclassified evidence. It must not parse the
string for retry, fallback, escalation, grouping, warning, or route policy.
`Status` and `ExitCode` contribute to the operation outcome independently of
that opaque text.

`RoutingActual` may be copied verbatim into a per-operation audit envelope. Its
concrete harness, provider, server, model, fallback-chain, and failure-class
fields must never:

- define or relabel a comparison arm;
- enter the grading prompt or grading rubric inputs;
- select a later request or change its constraints;
- drive warnings, retry, escalation, or review policy;
- become a benchmark grouping, ranking, or quality dimension.

The abstract returned power may be retained as audit evidence and may inform a
later `MinPower` floor where ADR-024 permits it. Concrete identity remains
audit-only.

A closed stream without a valid final event is a Fizeau contract failure, not
an agent-quality result. A malformed final payload is likewise a contract
failure. DDx preserves the available envelope and does not repair it by reading
native logs.

### Cancellation And Continuation

DDx cancels an in-flight operation by canceling the context passed to
`Execute`. Fizeau owns termination of the active agent, subprocess group,
descendants, tool loop, event stream, and native session record. DDx does not
send signals to a concrete harness or inspect a process tree.

Fizeau v0.14.50 has no public `Continue` API. DDx therefore exposes no
continuation implementation and does not reconstruct one from session logs or
provider-native identifiers. Replay is a new `Execute` operation built from
preserved DDx evidence. If Fizeau later publishes a continuation contract,
Fizeau still owns its semantics and DDx may only consume that public contract.

## Evaluation Architecture

```text
DDx evaluation plan
  prompt/rubric/MinPower/work-fact variants
  + unchanged explicit operator constraints
                    |
                    v
          DDx comparison dispatcher
                    |
         +----------+----------+
         |                     |
         v                     v
  DDx worktree A        DDx worktree B
         |                     |
         +--- Execute(ctx, request) ---+
                    |
                    v
          Fizeau full agent runtime
       route + session + tools + logs
                    |
                    v
      public final envelope per operation
                    |
                    v
  DDx diff/gates/evidence + ComparisonRecord
                    |
         +----------+----------+
         |                     |
         v                     v
  stronger review        benchmark summary
   via MinPower          by logical arm inputs
```

The comparison dispatcher is DDx git-aware orchestration around repeated
Fizeau operations. It is not an agent runtime and has no direct harness
adapter. Benchmark and replay compose the same dispatcher.

## Comparison Arms

### Logical Identity

Each arm has a stable route-neutral `arm_id` and display label. Its identity is
the declared experiment input, such as:

- prompt variant or prompt artifact reference;
- rubric reference;
- requested abstract `MinPower`;
- role, permissions, timeouts, and other work facts;
- no concrete route constraint; an operator passthrough envelope, when
  supplied, is comparison-wide and identical on every arm.

The resolved harness, provider, or model is not part of the arm key, display
label, deduplication key, or grade identity. Every label must be route-neutral
regardless of who authors it. Labels should
describe the experiment, for example `baseline-prompt`, `concise-prompt`,
`min-power-6`, or `min-power-10`. Labels such as `claude-arm`, `codex-arm`, or
model names are invalid because later grouping or grading would turn the label
into route-keyed comparison policy.

Explicit operator pins may not differ between arms and are not part of arm
identity or fingerprints. DDx stores one comparison-wide envelope and forwards
it unchanged to every arm. It does not validate the values, advertise them as
preferred routes, or include them in grouping, grading, or comparison policy.

### Isolation

Every side-effecting arm runs in its own DDx-owned git worktree at the same
base revision:

1. Resolve the repository root and immutable base revision.
2. Allocate a comparison ID and route-neutral arm IDs.
3. Create a detached worktree under an arm-ID-keyed path.
4. Construct one `ServiceExecuteRequest` using that worktree as `WorkDir`.
5. Call Fizeau `Execute` and consume its public operation outcome.
6. After the operation ends, capture repository diff, untracked files, result
   revision, and configured repository gates.
7. Remove the worktree unless the operator requested retention.

All arms start from the same base. Parallel execution is permitted because
worktrees are independent. Sequential mode changes scheduling only; it does
not change arm identity or record shape.

DDx locks are held only around their short git or tracker mutations. No DDx
lock may be held while waiting for Fizeau.

### Evidence Per Arm

DDx records two separate evidence classes.

DDx work evidence:

- logical arm inputs and their provenance;
- base revision and worktree result revision;
- output schema parsed from `FinalText`, when declared;
- repository diff and untracked-file manifest;
- repository gate results;
- attempt timestamps and DDx outcome;
- cleanup warnings and retained worktree path, if any.

Fizeau operation envelope:

- immediate error or decoded `ServiceFinalData`;
- public usage, cost, warnings, and duration;
- opaque `SessionLogPath`;
- exact `RoutingActual` in an audit-only subrecord.

The repository diff is the cross-arm side-effect comparison. Fizeau's native
tool and session logs are richer runtime evidence but remain outside DDx's
evaluation schema.

### Failure Semantics

Comparison is best-effort across independent arms:

- worktree creation failure marks that arm as a DDx setup failure;
- an immediate Fizeau error marks the operation failed or deferred according
  to its public type;
- a final non-success status records a Fizeau operation failure;
- an opaque final `Error` is stored but not classified from its text;
- a repository gate failure records a DDx work-result failure even if Fizeau
  reported session success;
- cleanup failure records a DDx warning and retained path;
- one arm's failure does not erase evidence from completed arms.

The comparison request itself fails only when DDx cannot construct a record,
for example invalid comparison input, no arms, unreadable prompt, or unresolved
repository root.

## Data Model

### ComparisonRecord

Required fields:

- `id`
- `started_at` and `ended_at`
- `base_rev`
- `prompt_source`
- `arms[]`

Optional fields:

- `suite` and `prompt_id` for benchmark runs;
- `correlation` containing bead, attempt, spec, or execution IDs;
- `source_execution` for replay or preserved-attempt comparison;
- `rubric_ref` and rubric provenance;
- `grades[]`;
- `storage_refs` for large prompt, output, diff, and raw grader attachments.

### ComparisonArm

Required fields:

- `arm_id`
- `label`
- `input_fingerprint`
- `requested_min_power`
- `operation_outcome`
- `work_outcome`
- `duration_ms`

Optional fields:

- prompt variant and work-fact references;
- unchanged operator passthrough envelope;
- parsed application output from `FinalText`;
- diff, untracked-file manifest, result revision, and gate results;
- usage and cost copied from the public final data;
- opaque session-log reference;
- immediate public error classification;
- opaque final error text;
- audit-only `routing_actual`;
- retained worktree path.

Concrete route identity must not be duplicated into top-level arm identity or
summary fields. It remains nested in the Fizeau audit envelope.

### ComparisonGrade

Required fields:

- `arm_id`
- `score`
- `max_score`
- `pass`
- `rationale`
- `rubric_id` and `rubric_version`
- `graded_at`

Optional fields:

- reviewer requested `MinPower`;
- unchanged operator passthrough provenance;
- raw grader-response attachment;
- reviewer operation envelope with audit-only `RoutingActual`.

There are no grader-harness, grader-provider, or grader-model policy fields.
Any concrete reviewer route returned by Fizeau stays inside the audit-only
operation envelope and is excluded from grade identity and aggregation.

## Persistence

Comparison, replay, benchmark, and grade records use the FEAT-010 run
substrate. Small DDx metadata stays in the record envelope. Large prompts,
application outputs, diffs, and raw grader responses use referenced DDx
attachments.

Fizeau's `SessionLogPath` is stored as an opaque external evidence reference.
DDx does not copy its internal schema into `ComparisonRecord`, normalize it to
DDx tool events, or use it as a recovery source for missing public final data.

Persistence is required for:

- evaluation list and detail views;
- append-only grade events;
- benchmark history;
- replay provenance;
- CI gates consuming stable comparison IDs.

Summary indexes may group by logical arm label, prompt/rubric identity,
requested `MinPower`, work facts, outcome, time, bead, or suite. They must not
group by, rank, or make claims about actual harness, provider, endpoint, model,
fallback chain, failure class, or the comparison-wide operator passthrough.
Per-operation `RoutingActual` may be shown only in an explicitly labeled audit
detail.

## Grading Pipeline

### Rubric Resolution

DDx resolves one rubric before requesting a reviewer:

1. Use an explicit rubric artifact when supplied.
2. Otherwise use the suite's rubric reference when present.
3. Otherwise use the versioned default rubric for correctness, completeness,
   and implementation quality.

DDx owns rubric storage and provenance. Skills and operators own rubric
content. A rubric may judge task outcomes and repository evidence; it must not
score or express preference for a concrete route identity.

### Route-Blind Review Prompt

The review prompt includes:

- rubric text and required JSON schema;
- original task and logical experiment labels;
- each arm's application output from `FinalText`;
- each arm's repository diff and gate results;
- declared work facts relevant to the rubric;
- explicit notice when evidence was omitted or truncated.

It excludes:

- `RoutingActual` and routing-decision events;
- concrete harness, provider, endpoint, server, or model names;
- fallback-chain and concrete route failure-class fields;
- native Fizeau session/tool logs;
- any DDx inference about which route is stronger, cheaper, or preferred.

Usage, cost, and latency may be displayed in operator reports under FEAT-014,
but a quality grade must not use them unless the governing rubric explicitly
defines a route-neutral resource constraint. Even then, the rubric evaluates
the numeric work outcome, not the concrete route that produced it.

### Stronger Reviewer Request

The grading workflow creates a new Fizeau `Execute` operation with
`Role="reviewer"` and a higher abstract `MinPower` according to ADR-024. It
does not set `Harness`, `Provider`, `Model`, or `Policy` unless those exact
values came from explicit operator passthrough. An operator `MaxPower` and
pins remain unchanged.

DDx parses the declared grade JSON from `ServiceFinalData.FinalText`. A valid
example is:

```json
{
  "arms": [
    {
      "arm_id": "min-power-10",
      "score": 8,
      "max_score": 10,
      "pass": true,
      "rationale": "The required behavior and regression tests are present."
    }
  ]
}
```

Malformed application JSON fails the grade event without mutating existing
arm evidence. A reviewer immediate error or non-success final records a review
operation failure. The generic final `Error` remains opaque and unclassified.

Grades are append-only. Consumers select the latest grade for the same rubric
version by default and may inspect history. Scores are comparable only within
the same rubric identity and `max_score`.

## Benchmark Architecture

`benchmark-suite` is a batch workflow over comparison records. A suite may
define:

- name and version;
- prompt variants;
- requested abstract `MinPower` values;
- rubric versions;
- route-neutral work facts;
- at most one operator passthrough envelope, copied identically to every arm
  and excluded from aggregate keys;
- post-operation repository gates;
- timeout and sandbox policy.

For each prompt, the workflow builds logical arms and calls the shared
comparison dispatcher. The aggregate includes completed/failed/deferred arm
counts, gate outcomes, token/cost totals, duration, and rubric-local grades.

Aggregates are keyed only by suite, prompt/rubric version, logical arm,
requested `MinPower`, and other DDx-controlled work facts. The benchmark must
not produce per-harness, per-provider, per-model, fallback, or route-quality
tables from `RoutingActual`.

## Replay

Replay asks what a new operation does from preserved DDx evidence. It is not a
Fizeau continuation and does not choose a replacement route.

### Source Precedence

DDx reconstructs replay inputs in this order:

1. `.ddx/executions/<attempt-id>/` bundle linked by attempt evidence;
2. DDx run/session envelope linked by the work item;
3. bead title, description, and acceptance criteria as degraded fallback.

The execution bundle provides the exact prompt artifact, base revision, result
revision, and original repository evidence. The DDx envelope may provide the
opaque Fizeau `SessionLogPath` for audit, but replay never reads the native log
to reconstruct a prompt or route.

If exact prompt evidence is absent, replay marks `degraded_prompt=true`. If the
base revision is absent, it uses the documented degraded fallback and marks
`degraded_base=true`.

### New Operation

Replay creates a fresh worktree and a fresh `Execute` request. The operator or
workflow may change only declared evaluation inputs: prompt, rubric,
`MinPower`, or work facts. Any operator passthrough constraints are copied
unchanged from the declared replay request and excluded from the comparison
identity; changing concrete pins is not a DDx evaluation variable. DDx does not
translate a request such as "try something stronger" into a concrete model; it
raises `MinPower` and lets Fizeau route.

The new result is compared with the original repository diff and DDx work
outcome. The original and replay `RoutingActual` values remain separate
audit-only envelopes and are excluded from the comparison grade.

### Original Diff

DDx resolves the baseline diff in this order:

1. preserved execution-bundle result diff;
2. `git diff <base_rev> <result_rev>` when both refs are known;
3. implementation close-commit diff when the close commit is known to contain
   the governed work;
4. no original diff, with explicit degradation evidence.

Tracker-only close commits must not masquerade as implementation baselines.

## Workflow Contracts

### Compare

A comparison workflow accepts an explicit list of route-neutral arms. For
example, a suite may compare the same task with `baseline-prompt` and
`concise-prompt`, or at `MinPower` 6 and 10. It must reject a DDx-generated
configuration that attempts to enumerate Fizeau routes or convert a route
catalog into arms, and must reject concrete passthrough differences between
arms.

### Grade

A grade workflow accepts a comparison ID, rubric reference, and optional
reviewer `MinPower`. It appends grade evidence only. It does not mutate arm
inputs or choose a reviewer route.

### Benchmark

A benchmark workflow expands prompts, rubrics, abstract power, and work facts
into comparisons and persists the complete records plus route-neutral
aggregates.

### Replay

A replay workflow accepts a bead or attempt ID plus allowed input changes. It
starts a new `Execute` operation. It has no `Continue` call and no native-log
reconstruction path.

### Consensus

Consensus is a workflow policy over route-blind result evidence. It may count
independent arms or review verdicts, but it must not require route diversity,
group votes by concrete route, or weight a verdict by harness/provider/model.

## Error Handling

| Condition | DDx behavior |
|---|---|
| Prompt or rubric missing | Fail before creating the affected arm |
| No logical arms | Fail before creating a comparison record |
| Worktree setup failure | Record DDx setup failure for that arm |
| Immediate `*NoViableProviderForNow` | Record deferred operation and honor only its `RetryAfter` |
| Other immediate `Execute` error | Record immediate Fizeau operation failure; no invented final |
| Final non-success | Record operation failure from public status/exit data |
| Generic final `Error` text | Store opaque and unclassified; do not parse |
| Stream closes without valid final | Record Fizeau contract failure |
| Context canceled | Record interruption after canceling the `Execute` context |
| Repository gate fails | Record DDx work-result failure; preserve Fizeau envelope |
| Grade JSON malformed | Fail grade event; leave comparison evidence unchanged |
| Replay evidence incomplete | Use documented fallback and mark degradation |
| Worktree cleanup fails | Warn with retained DDx worktree path |

No error path authorizes DDx to enumerate, choose, retry, or fall back across
concrete Fizeau routes.

## Security And Privacy

Comparison and replay records may contain proprietary code, prompts, diffs,
gate output, and application responses. DDx stores them repository-locally by
default and applies its artifact retention and redaction policy.

Rubrics are local DDx artifacts. DDx does not fetch them from the network.

Fizeau owns native transcript retention and redaction. DDx stores only the
opaque public reference and must not expose native logs through a second DDx
schema. Worktree isolation prevents accidental cross-arm repository
interference; it is not a malicious-code sandbox.

## Observability

Each persisted evaluation operation includes:

- DDx correlation, logical arm, timestamps, and requested `MinPower`;
- prompt/rubric/work-fact provenance;
- unchanged explicit operator constraints when present;
- Fizeau public operation outcome, usage, cost, and opaque session-log ref;
- repository base/result revisions, diff, and gate evidence;
- degradation and cleanup flags;
- audit-only `RoutingActual` nested under the operation envelope.

Operator views must visually separate requested DDx work facts, Fizeau
operation outcome, DDx work outcome, and route audit. They must not offer a
DDx route picker, provider-health view, route recommendation, route-quality
warning, or model-efficacy ranking.

## Validation Requirements For TP-019

TP-019 must cover:

- exact `Execute(ctx, ServiceExecuteRequest) (<-chan ServiceEvent, error)`
  consumption;
- immediate errors versus public final events;
- `RetryAfter` only on immediate `*NoViableProviderForNow`;
- generic final `Error` stored opaque and unclassified;
- final `Status`, `ExitCode`, `FinalText`, `DurationMS`, `Usage`, `Warnings`,
  `CostUSD`, `SessionLogPath`, and `RoutingActual` handling;
- context cancellation without DDx process-tree logic;
- absence of a DDx continuation API and replay as a new operation;
- route-neutral arm identity and worktree isolation;
- unchanged operator passthrough constraints;
- stronger review by `MinPower` only;
- exclusion of concrete route fields from grading prompts and aggregates;
- `RoutingActual` confined to audit details;
- repository diff, gate, replay provenance, and degraded fallback behavior.

Fizeau runtime behavior belongs in Fizeau conformance tests. DDx tests use a
public-contract fake and must not embed virtual providers, concrete harness
executors, native tool loops, provider streams, or Fizeau session-log parsers.

## Implementation Order

1. Replace DDx harness/model arm identity with route-neutral logical arms.
2. Implement the current Fizeau consumer seam and public final envelope.
3. Persist separate Fizeau operation and DDx work outcomes.
4. Add comparison worktree isolation, diff capture, and repository gates.
5. Implement route-blind grading with stronger reviewer `MinPower`.
6. Persist route-neutral benchmark aggregates.
7. Implement replay from DDx execution evidence as a new operation.
8. Add audit-only rendering for returned `RoutingActual`.
9. Enforce the TP-019 negative tests for forbidden DDx routing behavior.

## Risks

- **Concrete route identity leaks back into arm keys or grade prompts.** Keep
  `RoutingActual` nested in an audit-only envelope and add structural tests.
- **DDx mistakes Fizeau session success for work success.** Persist separate
  operation and repository outcomes and require DDx gates.
- **A final error string becomes accidental routing policy.** Store it as
  opaque evidence and prohibit text parsing.
- **Replay becomes an unofficial continuation implementation.** Rebuild only
  from DDx evidence and always start a fresh `Execute` operation.
- **Large outputs and diffs overload record rows.** Store large DDx bodies as
  attachments with stable references.
- **Benchmark statistics imply route quality.** Aggregate only by declared
  route-neutral experiment inputs and audit the output schema.
- **Operator pins are silently loosened during escalation.** Preserve the raw
  envelope unchanged and stop for operator action on incompatibility.

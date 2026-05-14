---
ddx:
  id: TD-037
  depends_on:
    - FEAT-006
    - FEAT-010
    - FEAT-014
    - ADR-024
  status: draft
---
# Technical Design: Bead-Level Execution Hints

## Purpose

DDx needs a durable way to say what level of agent effort a bead needs without
turning the queue into a collection of model pins. The mechanism must be
portable across machines and providers, auditable after the fact, and hard to
cargo-cult into concrete harness or model choices.

This design defines bead-level execution hints as abstract power intent. It
does not let beads choose providers, harnesses, or concrete models.

The governing policy is:

1. Bias toward work getting done automatically.
2. By default, use the cheapest and fastest available profile that is likely to
   get the work done.
3. Reserve the most powerful profiles for the most difficult work: bead
   breakdown, debugging, high-risk reviews, architecture-sensitive work, or
   retries with concrete lower-power failure evidence.

## Current State

`ddx try` already has a partial mechanism:

- Beads preserve unknown custom fields in `Extra`, but `ddx try` does not read
  arbitrary routing custom fields.
- `ddx try` accepts CLI execution constraints: abstract power bounds plus raw
  passthrough strings for profile, harness, provider, model, and model-ref.
- When no routing flags are supplied and no project routing config exists,
  `ddx try` calls `escalation.InferPowerClass(bead)`.
- `InferPowerClass` treats labels `power:smart`, `power:standard`, and `power:cheap` as
  explicit power overrides before falling back to priority, kind, and scope
  heuristics.
- DDx has Fizeau profile snapshot helpers that can read `ListPolicies` and
  `ListModels`, but implementation dispatch must use those helpers to select a
  profile/policy for inferred power intent instead of falling through to
  Fizeau's default policy.

That gives DDx a usable short-term path, but it is underspecified. It does not
define when `power:smart` is justified, how the choice is audited, or how to
reject durable model/harness cargo culting.

## Design

### Durable Hint Surface

DDx recognizes exactly one durable bead-level hint surface in v1:

| Surface | Values | Meaning |
|---|---|---|
| `power:cheap` label | cheap | Mechanical work where low-cost models should be enough. |
| `power:standard` label | standard | Ordinary implementation or review work. |
| `power:smart` label | smart | High-judgment, broad, ambiguous, or architecture-sensitive work. |

The label is a request for abstract execution power. It is not a model name,
provider name, harness name, or Fizeau profile name. Fizeau may expose
shortcut profile names, but those names are configurable and DDx must not
treat them as canonical. At the DDx request-construction boundary, DDx maps the
hint to a Fizeau profile/policy by inspecting Fizeau-owned profile metadata.
Fizeau still owns the concrete route inside that profile.

### Profile Selection Policy

For `ddx try` and `ddx work`, when no explicit routing flags and no project
routing configuration are present, DDx selects request-level profile intent as
follows:

1. Infer what the task needs: difficulty/power band, breadth, risk, likely
   context size, need for tools, review/debug/breakdown role, and retry history.
2. Fetch Fizeau profile and model metadata via `ListPolicies` and `ListModels`.
3. Treat Fizeau profile names as opaque. Do not hard-code names such as
   `cheap`, `standard`, `smart`, `frontier`, or provider-specific aliases.
4. Classify candidate profiles by metadata: power range, availability,
   auto-routable model coverage, hard policy requirements, cost/billing data,
   speed/perf signals, and later diversity signals when exposed.
5. Choose the profile that best satisfies the task need while optimizing for
   forward progress, then cost and speed. For default implementation work, this
   normally means the weakest no-extra-requirement profile expected to complete
   the task.
6. If the requested band has no viable profile but another free provider is
   available, try the best available free/low-cost profile instead of blocking
   the bead. Record the downgrade in routing-intent evidence.
7. If no provider/profile is available at all, return an execution error. Do
   not park the bead as a capability blocker.
8. Escalate only after DDx-owned evidence shows the lower profile did not get
   the work done.
9. Break ties inside that band by preferring profiles that are cheaper and faster
   while still available, auto-routable, and appropriate for default execution.
10. Send only the selected Fizeau profile/policy name and abstract power bounds.
   Do not send a concrete harness, provider, or model unless the operator
   supplied that passthrough explicitly.

DDx must treat policy requirements such as local-only/no-remote as hard routing
semantics, not as ordinary low-power profile variants. A requirement-bearing
profile is eligible for automatic selection only when DDx has explicit matching
intent; otherwise DDx leaves it for explicit `--profile` passthrough or a
future explicit local-only workflow. DDx must not duplicate Fizeau's provider
preference mappings such as local-first, subscription-first, or local-only
ordering. Fizeau owns provider preference and concrete route choice inside the
selected policy. When a retry raises `MinPower` above every available
no-extra-requirement profile, DDx may drop the previously selected policy and
send `MinPower` only rather than reusing a stale lower-power policy; Fizeau then
routes from live availability or returns its normal no-eligible-candidate error.

For ordinary implementation work, DDx should select the medium/default band by
metadata, not by a hard-coded Fizeau profile name. It should not drift to the
highest-power profile merely because a stronger subscription model is
available. The strongest band is appropriate when the bead carries an explicit
strong-power hint, the work is breakdown/debug/high-risk by policy, or prior
attempt evidence shows a weaker profile could not meet acceptance criteria.

If Fizeau profile metadata is unavailable, DDx may fall back to a `MinPower`
floor. That fallback must preserve the same weak-first policy: ordinary
implementation work uses a medium-band lower-bound floor, not the top available
model. If that floor is unsatisfied but a weaker free/available provider exists,
DDx should try it and record a degraded route rather than failing before
attempting the bead. Escalation raises profile/power only after DDx-owned
evidence shows the lower-power attempt did not complete the work.

DDx should not add bead-level `harness`, `provider`, `model`, or `model-ref`
fields. Those remain operator-supplied CLI passthrough constraints for one
attempt, or project/Fizeau configuration when a workspace intentionally pins
routing policy.

### Smart-Power Justification

`power:smart` requires a justification in the bead description. The justification
must name the reason the bead is likely capability-sensitive, using one or more
of these categories:

- cross-cutting behavior or multiple subsystems;
- architecture, API, or data-model decision;
- ambiguous or competing requirements that need reconciliation;
- high-risk review, security, data-loss, or migration concern;
- evaluation of non-deterministic output quality;
- decomposition or planning work where weak reasoning creates downstream waste.

Valid example:

```text
SMART JUSTIFICATION: This bead decides the durable execution-hint contract and
its precedence against ADR-024 routing policy. A weak implementation could
persist concrete model pins into queue metadata.
```

Invalid example:

```text
SMART JUSTIFICATION: Claude worked well last time.
```

The first enforcement step should live in the bead lifecycle lint hook so
authors and agents get feedback before dispatch. A later hardening pass may add
the same validation to `ddx bead create` and `ddx bead update`, but lint is the
minimum gate because all automated execution passes through it.

### Precedence

Execution intent resolves in this order:

1. Explicit CLI flags for the current invocation.
2. Project/Fizeau routing configuration.
3. Durable bead hint label.
4. DDx heuristic inference from priority, kind, issue type, and scope.
5. DDx default.

CLI flags win because they are operator actions for one attempt and are visible
in the command/evidence. Durable bead hints win over heuristics because they are
part of the reviewed work item.

If a project has explicit routing configuration, `ddx try` must not silently
override it with a bead label. The label should still be recorded as requested
intent so operators can audit divergence between requested and actual routing.

### Rejected Durable Route Pins

Bead metadata must not persist concrete route choices. The lint hook should
reject or block execution when a bead includes durable fields or labels that
look like route pins, including:

- `harness`, `agent-harness`, `execution-harness`, `try-harness`;
- `provider`, `agent-provider`, `execution-provider`, `try-provider`;
- `model`, `agent-model`, `execution-model`, `try-model`;
- `model-ref`, `agent-model-ref`, `execution-model-ref`, `try-model-ref`;
- labels such as `harness:claude`, `provider:openai`, or `model:gpt-...`.

The diagnostic should say to use a one-off CLI flag for a reproduction or
explicit operator constraint:

```bash
ddx try <id> --harness claude
```

This keeps durable queue metadata portable and prevents agents from copying
route choices that happened to work once.

### Audit Evidence

Every `ddx try` / `ddx work` attempt should record routing-intent evidence
before execution starts. The evidence should be append-only and attached to the
attempt/run record, not only printed to stdout.

Minimum fields:

| Field | Meaning |
|---|---|
| `bead_id` | Target bead. |
| `attempt_id` | Attempt/run id. |
| `routing_intent_source` | `cli`, `project_config`, `bead_hint`, `heuristic`, or `default`. |
| `requested_power_class` | `cheap`, `standard`, `smart`, or empty when not power-based. |
| `requested_profile` | Fizeau profile/policy name DDx requested, when selected. |
| `requested_min_power` | Resolved `MinPower`, when available. |
| `requested_max_power` | Resolved `MaxPower`, when available. |
| `smart_justification` | Extracted justification text when `requested_power_class=smart`. |
| `actual_harness` | Harness reported by Fizeau/agent after execution. |
| `actual_provider` | Provider reported after execution. |
| `actual_model` | Model reported after execution. |
| `actual_power` | Actual power reported after execution. |
| `routing_intent_degraded` | True when requested intent could not be met or was overridden. |
| `routing_intent_note` | Short reason for degradation or override. |

The evidence event name should be stable, for example
`execution-routing-intent`. Existing attempt result fields already capture some
actual route facts; this event ties those facts back to why DDx requested that
power.

### Metrics

The agent metrics rollup should ingest the routing-intent fields once they are
present in run evidence. At minimum, operators need to answer:

- How many attempts used `power:smart`?
- Which beads requested smart, and what was the justification?
- What was the success rate and cost of smart-hinted work compared with
  heuristic or default work?
- How often did CLI or project config override a bead hint?
- Which beads carried rejected durable route pins?
- Which authors or agents are adding smart hints repeatedly?

Initial metrics can be implemented by extending the normalized attempt
projection used by TD-032. A materialized rollup is not required.

Suggested dimensions:

| Dimension | Example |
|---|---|
| `routing_intent_source` | `bead_hint` |
| `requested_power_class` | `smart` |
| `actual_power_bucket` | `>=80` |
| `degraded` | `true` |
| `bead_author` | `erik` or agent id when available |
| `smart_reason_category` | `architecture` |

Suggested counters:

- attempts by requested power class and source;
- success rate by requested power class;
- cost and token usage by requested power class;
- smart-hint count by bead author/agent;
- rejected durable route-pin count;
- override/degradation count.

### Operator Reporting

`ddx try` output should stay concise, but it should include the source when a
bead hint affects execution:

```text
routing intent: powerClass=smart source=bead_hint
```

If `power:smart` is present without justification, the lint failure should be
plain:

```text
bead uses power:smart but has no SMART JUSTIFICATION section
```

If a durable concrete pin is found:

```text
bead metadata contains execution-model=gpt-5.5; durable model pins are not
allowed. Use ddx try <id> --model ... for one-off debugging.
```

## Implementation Plan

### Bead 1: specify and test hint parsing

Scope:

- Parse `power:*` labels into a typed execution hint.
- Extract `SMART JUSTIFICATION:` from bead descriptions.
- Detect forbidden durable route-pin fields and labels.

Acceptance:

1. Tests cover valid `power:cheap`, `power:standard`, and `power:smart`.
2. Tests cover missing smart justification.
3. Tests cover forbidden concrete route-pin fields and labels.
4. Existing heuristic inference behavior remains unchanged when no hint exists.

### Bead 2: enforce hints in bead lifecycle lint

Scope:

- Add lint findings for missing smart justification.
- Add lint failures for durable concrete route pins.
- Keep diagnostics actionable.

Acceptance:

1. A smart bead without justification fails lint.
2. A bead with `harness:claude` or `execution-model=...` fails lint.
3. A bead with valid `power:smart` and justification passes lint.

### Bead 3: record routing-intent evidence

Scope:

- Resolve execution-intent source during `ddx try` / `ddx work`.
- Attach `execution-routing-intent` evidence before execution.
- Update result/evidence tests.

Acceptance:

1. Attempts with `power:smart` record source `bead_hint`.
2. CLI routing flags record source `cli`.
3. Heuristic inference records source `heuristic`.
4. Actual route facts are linked to requested intent when the attempt finishes.

### Bead 4: expose audit metrics

Scope:

- Extend the TD-032 normalized attempt projection with routing-intent fields.
- Add rollup dimensions/counters for requested power class, source, degradation, and
  rejected pins.

Acceptance:

1. Metrics can report smart-hinted attempt count over a time window.
2. Metrics can list smart-hinted beads and justifications.
3. Metrics can compare success/cost for bead-hinted vs heuristic/default work.

### Bead 5: update bead authoring guidance

Scope:

- Update bead authoring docs and DDx skills.
- Add examples for justified smart hints and rejected cargo-cult pins.

Acceptance:

1. Bead authoring template explains when to use `power:smart`.
2. DDx skill guidance says not to persist harness/provider/model choices.
3. Skill validation passes.

## Non-Goals

- No durable bead-level concrete harness/provider/model pins.
- No DDx-side concrete route ranking or fallback.
- No change to Fizeau's routing algorithm.
- No materialized metrics store in the first implementation.
- No automatic promotion to `power:smart` solely because a previous attempt used
  a particular harness or model.
- No default use of the strongest profile for ordinary implementation work.

## Open Questions

- Should `power:smart` without justification be a hard blocker immediately, or
  start as a warning for one release?
- Should `power:*` remain labels long term, or should DDx eventually expose a
  first-class `execution-power` field while continuing to read labels for
  compatibility?
- Which additional Fizeau profile facts should participate in tie-breaking
  after power, speed, cost, and availability are stable across providers?

---
ddx:
  id: FEAT-006
  depends_on:
    - helix.prd
---
# Feature: DDx Agent Service (consumer of ddx-agent contract)

**ID:** FEAT-006
**Status:** In Progress (migrating to ddx-agent CONTRACT-003)
**Priority:** P0
**Owner:** DDx Team

## Overview

DDx invokes LLMs through the `ddx-agent` module, defined by
**CONTRACT-003-ddx-agent-service** in the `~/Projects/agent` repo
(`docs/helix/02-design/contracts/CONTRACT-003-ddx-agent-service.md`).

That contract is the entire boundary. DDx's CLI commands (`ddx agent run`,
`ddx agent execute-bead`, `ddx work`, `ddx agent providers`, `ddx agent models`,
`ddx agent route-status`, `ddx agent check`) are thin wrappers that call into
the `agentlib.DdxAgent` interface and render its results.

## DDx-side responsibilities

DDx owns the bead-driven workflow surface. It does NOT own:
- Harness orchestration (claude, codex, opencode, pi, gemini, native)
- Provider routing
- Model catalog
- Compaction
- Tool registration
- Session-log shape

Those all live inside ddx-agent per CONTRACT-003.

DDx owns:

- **Bead-driven invocation.** `ddx agent execute-bead`, `ddx work`, and the
  server's queue-drain worker translate bead state into `ExecuteRequest`
  values and surface results back into the bead tracker.
- **Execute-bead orchestration.** Worktree creation, base-revision pinning,
  result landing (merge / preserve / no-changes), gate evaluation, evidence
  bundle capture. The agent provides the LLM execution; DDx provides the
  git-aware orchestration.
- **Evidence and session capture.** DDx writes `.ddx/executions/<id>/`
  bundles with prompts, manifests, and result artifacts. The agent's session
  log path (returned in `ExecuteResponse.SessionLogPath`) is captured into
  the bundle.
- **Profile policy at the request level.** DDx selects `Profile`, `Effort`,
  `Permissions` per bead (or per CLI invocation) based on bead metadata
  and user input. The agent receives those as `ExecuteRequest` fields and
  performs the routing.

## Profile Routing

`ddx work --profile default` is the primary queue-drain invocation. DDx maps
the user-facing profile to an ordered tier ladder and resolves each tier through
the agent service with the configured provider affinity and capability gating.

| Profile | Ladder | Intent |
| --- | --- | --- |
| `default` | `[cheap, standard, smart]` | Local first, then subscription cloud, then high-quality cloud when earlier tiers fail. This is the common throughput-per-dollar path. |
| `cheap` | `[cheap]` | Local only. Never escalates; if local cannot serve the bead, the bead is unclaimed and waits. |
| `fast` | `[fast, smart]` | Cloud-fast first, skipping local warmup; escalates to smart on failure. |
| `smart` | `[smart]` | High-quality cloud from the start. No escalation. |

The `.ddx/config.yaml` field `agent.routing.profile_ladders` can override the
ordered tier list per profile. The legacy flat `agent.routing.profile_priority`
is still read as the `default` profile fallback and emits a deprecation warning;
new configs should use `profile_ladders.default`. `agent.routing.model_overrides`
can map a ladder tier such as `cheap`, `standard`, `fast`, or `smart` to a
concrete model reference before DDx asks the agent service to resolve the route.

Escalation advances to the next tier for `execution_failed`,
`land_conflict`, `post_run_check_failed`, and
`structural_validation_failed`. `no_changes` keeps the existing cooldown and
satisfaction-adjudication path rather than consuming a higher tier.

### Conflict-recovery structured outcomes (bead ddx-0097af14)

Before parking a bead on `land_conflict` (or on `execution_failed` when the
agent produced a commit before failing), the execute-loop now attempts to reuse
the preserved iteration ref rather than discarding the agent's work:

1. **3-way ort auto-merge** (`git merge --no-ff -s ort -X ours`). If clean,
   the bead closes as `success`.
2. **Focused conflict-resolve agent** (if `ConflictResolver` is configured on
   the worker). If clean, the bead closes as `success`.
3. **Park with structured outcome.** Two new statuses replace the generic
   reopen when both recovery paths fail:

| Status | Event kind | Meaning |
| --- | --- | --- |
| `land_conflict_unresolvable` | `land-conflict-unresolvable` | All auto-recovery failed; retryable after 15-min cooldown. |
| `land_conflict_needs_human` | `land-conflict-needs-human` | Focused-resolve agent returned BLOCKING; human input required. |

Both statuses use `LandConflictCooldown` (15 min) rather than the 24h cap used
for `push_failed`, because land conflicts typically unblock quickly as sibling
beads advance the base branch.

## Execute-Bead Worker Sub-task Discovery Policy

**Design position: option (b) — surface via result.**

Workers executing inside `ddx agent execute-bead` MUST NOT call `ddx bead create`
in-band during their execution run. Creating beads in-band (option c) allows a
single worker to flood the queue with unreviewed children — in one observed case
(ddx-44236615), one worker spawned 11 P0 children without operator review. This
violates the architectural principle from FEAT-013: *"DDx provides primitives,
not orchestration. Orchestration policy stays in HELIX and other workflow tools."*

Workers MAY surface discovered sub-tasks as structured data in `result.json`
under a `discovered_subtasks` array. Each entry is a lightweight object with at
minimum a `title` and optional `description`, `labels`, and `priority` fields.
The execute-loop or supervisor reads this array from the result bundle and passes
it to the workflow tool (HELIX or operator) for decomposition decisions. The
supervisor decides whether, when, and at what priority to file new beads — DDx
does not do this automatically.

### Rationale

| Option | Description | Problem |
| --- | --- | --- |
| (a) Append to parent | Worker writes sub-tasks as notes or events on the parent bead | Loses structure; sub-tasks are not queryable or executable without manual decomposition |
| (b) Surface via result (**chosen**) | Agent emits `discovered_subtasks` in result.json; supervisor decides | Gates tracker mutations at an explicit decision point; aligns with "primitives not orchestration" |
| (c) Create in-band (former behavior) | Worker calls `ddx bead create` directly | Unreviewed children flood the queue; P0 tagging without operator review; violates workflow ownership boundary |

Option (b) preserves the operator control point required by workflow tools while
providing structure that automation can act on. It is the only option consistent
with the FEAT-013 boundary.

### Result schema extension

`result.json` gains an optional `discovered_subtasks` array. The execute-bead
orchestrator writes it from the agent's structured output when present. The
supervisor MUST NOT act on this field automatically; it is surfaced as an
observation for the workflow tool.

```json
{
  "status": "success",
  "detail": "...",
  "discovered_subtasks": [
    {
      "title": "Add read-coverage for X",
      "description": "Optional detail about what is needed",
      "labels": ["area:tests"],
      "priority": "P1"
    }
  ]
}
```

If the agent does not emit `discovered_subtasks`, the field is absent. The
execute-loop treats an absent or empty array identically — no automatic bead
creation occurs in either case.

### Enforcement

The execute-bead prompt delivered to the agent (via the prompt rationalizer)
MUST include an explicit instruction that workers must not call `ddx bead create`
and must instead emit `discovered_subtasks` in their result if sub-tasks are
identified. This constraint is part of the prompt contract; it is not enforced
by the CLI at runtime.

## Malformed-Bead Remediation Decision (beads ddx-7eab13a6 era)

**Decision: close and refile.**

The layer-1 worktree-path bug fixed in ddx-7eab13a6 caused 22 beads to be
created with IDs of the form `.execute-bead-wt-<parent>-<timestamp>-<random>-<hex>`
(the worktree directory path was mistakenly used as the bead ID). These beads
are invalid: their IDs do not match the `ddx-<8hex>` format expected by all DDx
CLI commands, and the IDs appear in git commit trailers as malformed values.

Repair in-place is not safe. Bead IDs are immutable identifiers embedded in git
commit trailers (`Ddx-Attempt-Id` etc.), inter-bead dependency edges, and the
JSONL append-only store. Renaming an existing bead ID requires rewriting
history, which violates the merge policy recorded in project memory.

The operator (human or supervising workflow tool) should:
1. Inspect each malformed bead's description to determine whether the underlying
   work is still valid.
2. If valid: refile a new bead with the same title and description, using
   `ddx bead create`. The new bead will receive a correct ID.
3. Close the malformed bead with `ddx bead close <id> --reason "malformed-id
   refiled as <new-id>"`.
4. If the work is no longer needed: close the malformed bead with reason
   `malformed-id obsolete`.

No automated bulk remediation is performed. The 22 beads are left open pending
operator triage. A future `ddx bead validate` command (not yet implemented) can
flag structurally invalid bead IDs to aid discovery.

## Asking ddx-agent for changes

When DDx needs new behavior from the agent — a new method, a new field on
`ExecuteRequest`, a new event type, a new policy knob — file a PR against
CONTRACT-003 in the agent repo. Maintainers decide whether the surface grows.

Do not import agent internal packages. They live under `internal/` and the
Go compiler blocks external imports after agent v0.5.0 ships.

## Migration status

DDx is currently migrating from the legacy DDx-internal routing/harness
implementation to the CONTRACT-003 surface. Migration epic: `ddx-ac5c7fdb`.
Pre-migration, FEAT-006 documented harness orchestration responsibilities
that have since moved upstream.

## Design References

- `docs/helix/02-design/solution-designs/SD-006-agent-session-capture.md` — session log shape and evidence bundles
- `docs/helix/02-design/solution-designs/SD-013-multi-agent-coordination.md` — worktree-aware dispatch, concurrent bead safety
- `docs/helix/02-design/solution-designs/SD-014-token-awareness.md` — provider availability, quota, and signal model
- `docs/helix/02-design/solution-designs/SD-016-process-metrics.md` — bead-level cost and rework derivation
- `docs/helix/02-design/solution-designs/SD-020-multi-machine-coordinator-topology.md` — host+user coordinator scoping
- `docs/helix/02-design/solution-designs/SD-021-service-backed-multi-node-topology.md` — service-backed node topology
- `docs/helix/02-design/technical-designs/TD-006-agent-session-capture.md` — session log and artifact format
- `docs/helix/02-design/technical-designs/TD-010-executions.md` — execution attempt bundles at `.ddx/executions/`
- `docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` — current thin-consumer migration plan
- `docs/helix/03-test/test-plans/TP-006-agent-session-capture.md` — session capture test coverage
- `docs/helix/03-test/test-plans/TP-014-token-awareness.md` — token-awareness coverage
- `docs/helix/03-test/test-plans/TP-020-agent-routing-and-catalog-resolution.md` — routing and catalog resolution coverage

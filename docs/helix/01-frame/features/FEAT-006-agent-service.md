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

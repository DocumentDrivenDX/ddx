---
ddx:
  id: FEAT-006
  depends_on:
    - helix.prd
---
# Feature: DDx Consumer of the Fizeau Execution Contract

**ID:** FEAT-006
**Status:** In Progress (migrating to CONTRACT-003 Fizeau execution contract)
**Priority:** P0
**Owner:** DDx Team

## Overview

DDx invokes LLMs through the upstream Fizeau execution contract,
**CONTRACT-003**, in the `~/Projects/agent` repo
(`docs/helix/02-design/contracts/CONTRACT-003-fizeau-service.md`).

That contract is the entire boundary. DDx exposes the public `ddx run` /
`ddx try` / `ddx work` layers and sends requests to Fizeau for the actual
invocation. Upstream diagnostic/status commands may remain as Fizeau-owned
observability, but the retired workflow namespace is not a public workflow
surface and has no compatibility alias.

## DDx-side responsibilities

DDx owns the bead-driven workflow surface. It does NOT own:
- Harness orchestration (claude, codex, opencode, pi, gemini, native)
- Provider routing
- Model catalog
- Compaction
- Tool registration
- Session-log shape

Those all live inside Fizeau per CONTRACT-003.

DDx owns:

- **Bead-driven invocation.** `ddx try`, `ddx work`, and the server's
  queue-drain worker translate bead state into `ExecuteRequest` values and
  surface results back into the bead tracker.
- **Bead-attempt orchestration.** Worktree creation, base-revision pinning,
  result landing (merge / preserve / no-changes), gate evaluation, evidence
  bundle capture. Fizeau provides the LLM execution; DDx provides the
  git-aware orchestration.
- **Evidence and session capture.** DDx writes `.ddx/executions/<id>/`
  bundles with prompts, manifests, and result artifacts. The Fizeau session
  log path (returned in `ExecuteResponse.SessionLogPath`) is captured into
  the bundle.
- **Power policy at the request level.** DDx selects requested power bounds,
  effort, and permissions per bead (or per CLI invocation) based on bead
  metadata and user input. DDx sends `MinPower` and optionally `MaxPower` to
  Fizeau and leaves the concrete route to Fizeau.

## Profile Routing

`ddx work --profile default` is the primary queue-drain invocation. DDx maps
the user-facing profile to an ordered tier ladder and resolves each tier through
Fizeau with the configured provider affinity and capability gating.

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
concrete model reference before DDx asks Fizeau to resolve the route.

Escalation advances to the next tier for `execution_failed`,
`land_conflict`, `post_run_check_failed`, and
`structural_validation_failed`. `no_changes` keeps the existing cooldown and
satisfaction-adjudication path rather than consuming a higher tier.

## Asking Fizeau for changes

When DDx needs new behavior from Fizeau — a new method, a new field on
`ExecuteRequest`, a new event type, a new policy knob — file a PR against
CONTRACT-003 in the agent repo. Maintainers decide whether the surface grows.

Do not import agent internal packages. They live under `internal/` and the
Go compiler blocks external imports after Fizeau v0.5.0 ships.

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
- Historical thin-consumer migration plan — superseded by the current Fizeau consumer spec
- `docs/helix/03-test/test-plans/TP-006-agent-session-capture.md` — session capture test coverage
- `docs/helix/03-test/test-plans/TP-014-token-awareness.md` — token-awareness coverage
- `docs/helix/03-test/test-plans/TP-020-agent-routing-and-catalog-resolution.md` — routing and catalog resolution coverage

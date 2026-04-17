---
ddx:
  id: SD-023
  depends_on:
    - FEAT-006
    - SD-015
---
# Solution Design: DDx / ddx-agent Boundary and Routing Visibility

## Overview

DDx imports `github.com/DocumentDrivenDX/agent` as a Go library for in-process
agent execution. The `ddx-agent` library already owns provider configuration,
`/v1/models` discovery, health-aware routing, model catalog, and has working
CLI subcommands (`ddx-agent providers`, `models`, `check`, `route-status`).
However, DDx users currently cannot access any of that state through the DDx
CLI, and the `--model` flag silently falls back to the default provider when no
catalog match is found.

This document records the boundary between DDx and ddx-agent, describes how DDx
surfaces ddx-agent state without duplicating logic, and defines the eight beads
that implement the visibility epic.

## The Boundary

### ddx-agent owns

- **Provider configuration and discovery**: loading `agentconfig`, enumerating
  configured providers, probing `/v1/models` endpoints
- **Model catalog**: aliases, profiles, canonical targets, deprecation metadata,
  surface-specific concrete model strings
- **Health-aware routing**: selecting a provider for a given model-ref or profile,
  tracking provider health, applying cooldowns after failures, choosing fallback
  chains
- **Runtime telemetry**: OTEL spans, session event logging, cost estimation,
  provider transcripts

### DDx owns

- **Beads and worktrees**: bead state, worktree lifecycle, execution evidence
- **Orchestration**: execute-loop, execute-bead, multi-agent coordination, harness
  registry, cross-harness routing policy (which harness to use, not which backend
  within a harness)
- **UI and CLI surface**: `ddx agent` subcommands, GraphQL schema, SvelteKit
  dashboard pages
- **Normalized routing signals**: the minimal DDx-observed metrics needed to rank
  harnesses (see SD-015); DDx consumes ddx-agent outputs and derived facts, not
  raw provider transcripts

### How DDx accesses ddx-agent state

DDx calls ddx-agent's **public Go package APIs** directly — the same functions
that ddx-agent's own CLI subcommands use. This means:

- `github.com/DocumentDrivenDX/agent/config` (`agentconfig.Load`, `cfg.Providers`,
  `cfg.ProviderNames`, `cfg.DefaultName`)
- `github.com/DocumentDrivenDX/agent/modelcatalog` (catalog lookup, alias
  resolution, canonical target projection)
- `github.com/DocumentDrivenDX/agent/provider/openai` (`RankModels`, live-probe
  helpers)
- Any future exported functions from ddx-agent's routing and health packages

DDx does **not**:

- Shell out to the `ddx-agent` binary
- Copy-paste internal probing or routing logic from `ddx-agent`'s `cmd/` package
- Parse `ddx-agent` stdout/stderr to extract state
- Maintain its own provider tables or duplicate model catalog entries

### Upstream bead workflow

When a DDx feature requires porcelain that ddx-agent has not yet exported as a
public Go API, the workflow is:

1. Identify the internal function or type in ddx-agent that provides the needed
   behavior.
2. File a bead on the `github.com/DocumentDrivenDX/agent` repository requesting a
   public export (e.g. `config.ListProviderStatus(ctx, cfg) ([]ProviderStatus, error)`).
3. Block the DDx bead on the upstream bead.
4. Once the ddx-agent release lands, bump the DDx `go.mod` pin and implement the
   thin wrapper in DDx.

This keeps the boundary clean and prevents ddx-agent internals from leaking into
DDx as copy-pasted dead weight.

## The Eight Visibility Beads

These beads collectively implement the routing visibility epic
(`ddx-bd0691fe: Epic: surface ddx-agent routing visibility through DDx CLI + bead evidence`).

| Bead | Title | Scope |
|------|-------|-------|
| ddx-48cee66e | Visibility 1: SD-023 ddx/ddx-agent boundary design doc | This document |
| ddx-3d64fd16 | Visibility 2: bump github.com/DocumentDrivenDX/agent to v0.3.5+ | `cli/go.mod` version bump and compile-error fixes |
| ddx-09ea1884 | Visibility 3: ddx agent providers [--json] command | New `cli/cmd/agent_providers.go`; surfaces live provider list via Go package |
| ddx-51de0667 | Visibility 4: ddx agent models [--provider X] [--all] command | New `cli/cmd/agent_models.go`; lists models per provider via Go package |
| ddx-c4e3d91b | Visibility 5: ddx agent check [--provider X] — runtime availability | New `cli/cmd/agent_check.go`; probes liveness distinct from `ddx agent doctor` |
| ddx-a842dccd | Visibility 6: ddx agent route-status command | New `cli/cmd/agent_route_status.go`; shows routing state and recent decisions |
| ddx-35af61e8 | Visibility 7: teach execute-loop + execute-bead about --provider and --model-ref, error on orphan --model | Adds `--provider` and `--model-ref` flags; replaces silent OpenRouter fallback with an explicit error |
| ddx-bfc1aeaf | Visibility 8: capture routing decisions in bead evidence (kind:routing) | Records resolved provider/model/route-reason in bead evidence after each execute-bead run |

All beads except Visibility 1 are blocked on Visibility 2 (the version bump),
which in turn is blocked on this design document.

**Note on `--model-ref`**: Visibility 7's title references `--model-ref` as a
separate flag. Per the updated SD-015 resolution order, `--model` now handles
both catalog refs and exact model names via the unified pool — the user always
specifies `--model`. The `--model-ref` flag remains available as an explicit
catalog-only override for scripts that want to bypass fuzzy matching and live
discovery, but it is not the primary user-facing surface.

## Implementation Constraints

### No shell-out, no copy-paste

Every new `ddx agent` subcommand in this epic must import the Go package
directly. If `grep`-ing the new file for `exec.Command`, `os/exec`, or
`ReadLine` returns results, the implementation violates the boundary.

### Semantic separation: `doctor` vs `check`

`ddx agent doctor` answers "is my config healthy?" — config validation, harness
registry, missing API keys. `ddx agent check` answers "what agents can I use
right now?" — runtime liveness, which providers respond, which models are
available at this moment. Both commands coexist; their scopes must not blur.

### `--model` must discover or error, not fall back

The silent fallback that routed `--model qwen/qwen3-coder-next` to OpenRouter
when vidar was the intended provider is the root bug this epic fixes. After
Visibility 7, `--model X` follows the three-step explicit-model resolution
defined in SD-015 §"Resolution Order" mode 2:

1. **Resolve**: build a unified model pool from the catalog and live provider
   discovery, match X (exact or fuzzy prefix with shortest-suffix tiebreak).
2. **Find providers**: identify all providers that can serve the resolved model.
3. **Pick the best**: rank by standard scoring; `--provider P` is a soft
   preference, not a constraint.

If no match is found at step 1, or no provider can serve the resolved model
at step 2, DDx must error before dispatching. DDx must never silently route
an explicit `--model` request to a harness that cannot serve it.

### Evidence is structured JSON

Routing evidence captured in Visibility 8 must be parseable JSON in the bead
evidence body. Fields: `resolved_provider`, `resolved_model`, `route_reason`,
`fallback_chain`, `base_url`. This feeds the cost-tiered routing analytics epic
without requiring session-log scraping.

## Future Follow-up: Cost-Tiered Routing Epic

This visibility epic is a prerequisite for a planned **cost-tiered routing**
epic in which DDx uses routing evidence to drive escalation decisions — for
example, retrying a failed run against a more capable (and more expensive)
provider, or automatically downgrading to a local provider when cloud quota is
exhausted.

The cost-tiered routing epic is out of scope here. The eight beads above
deliberately stop at surfacing state and fixing the orphan-model bug. Cost
policy, escalation triggers, and analytics are deferred.

## Open Questions

- If ddx-agent's public API does not expose a clean `ListProviderStatus` helper
  by the time Visibility 3 is implemented, what is the minimum API surface the
  upstream bead should request?
- Should `ddx agent check` write a short-lived cache to avoid repeated probing
  within the same terminal session?
- Should `ddx agent route-status` support a `--json` flag for scripting (similar
  to `ddx agent providers --json`)?

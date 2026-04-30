# CONTRACT-003 Audit — 2026-04-29

> **Boundary update:** this audit predates the final docs-corpus cleanup for
> the top-level `ddx run` / `ddx try` / `ddx work` stack. Its upstream CLI
> mounting findings remain useful for diagnostic passthrough, but DDx must not
> revive removed agent execution leaves through help text or wrapper commands.

Bead: ddx-90234d47 — gates plan-2026-04-29-artifact-and-run-architecture.md bead #7
(FEAT-006 refactor). Source contract:
`~/Projects/agent/docs/helix/02-design/contracts/CONTRACT-003-ddx-agent-service.md`.

## Scope

Per the plan, this audit confirms whether the upstream `ddx-agent` module
already exports the surfaces DDx needs for:

1. The structural `ddx agent` passthrough (mountable Cobra root).
2. Layer-1 generate-artifact runs (`produces_artifact` / `media_type` metadata
   round-trip).
3. The three-layer architecture (`ddx run` / `ddx try` / `ddx work`).
4. Per-attempt session-log envelope ownership.

It then enumerates the CONTRACT-003 amendments that must land in
`~/Projects/agent` before bead #7 can complete.

## Findings

### 1. Mountable Cobra root — **GAP, amendment required**

The plan (line 28) commits to:

> DDx imports the upstream agent module's Cobra root and mounts it under
> `ddx agent`. Notionally `cli(ddx).agent(load_cli(agent))`.

That is not currently possible. The upstream CLI does not use Cobra at
all:

- `~/Projects/agent/cmd/agent/main.go:5–17, 60–80` builds its CLI with
  `flag.NewFlagSet` and a hand-rolled subcommand normalizer.
- A repo-wide grep for `cobra` against `~/Projects/agent` (excluding
  `worktrees/`) returns zero hits.
- The publicly exported `agent` package (`public_api.go`) re-exports
  `Tool`, `Reasoning`, and `ReasoningTokens` only — no CLI surface.

Therefore:

- DDx cannot write `agentcli.NewRootCommand()` against the current
  contract.
- The plan's "delete with prejudice" list (agent_list, agent_capabilities,
  agent_doctor, agent_check, agent_models, agent_providers,
  agent_route_status, agent_catalog, agent_reindex) presupposes that
  upstream owns these subcommands inside a mountable tree. Today they
  exist only as `flag`-driven branches in `cmd/agent/main.go` that DDx
  cannot consume programmatically.

**Required amendment (CONTRACT-003-A):** add a public CLI surface to the
`agent` Go module. Recommended shape (lowest coupling, keeps "contract is
the boundary"):

```go
// Package agent
//
// MountCLI returns the upstream Cobra command tree for the
// ddx-agent CLI. Consumers (DDx) may attach it as a subcommand of
// their own root. Calling MountCLI more than once is safe; each call
// returns a fresh, unattached *cobra.Command.
func MountCLI(opts ...MountOption) *cobra.Command
```

`MountOption` should let DDx:

- Override command naming/help text without reintroducing removed execution
  leaves under the legacy namespace.
- Inject a logger / output streams (so DDx can capture stdout/stderr
  for evidence bundles).
- Suppress upstream `os.Exit` calls (return errors instead) so a
  passthrough invocation does not abort the parent process.

Implementation work upstream:

1. Port `cmd/agent/main.go` from `flag` to `cobra` (subcommands as
   discrete files).
2. Move the resulting commands into a non-`internal/` package
   (e.g. `agent/cli`).
3. Re-export `MountCLI` from the root `agent` package.
4. Bump the agent module version (proposed `v0.9.0`); leave
   `cmd/agent/main.go` calling `MountCLI` so the standalone binary
   keeps building.
5. Update CONTRACT-003 §"Public surface" with the `MountCLI`
   signature and the stability guarantees on flag names that DDx
   help text will reference.

Plan bead #6 ("open CONTRACT-003 amendment in `~/Projects/agent` if #5
surfaces gaps") is **required, not conditional**, because of this gap.

### 2. Generate-artifact run metadata — **NO amendment needed**

The plan converges on "generating an artifact is a layer-1 agent run with
`produces_artifact` metadata" (line 13, line 40).

CONTRACT-003 already exposes a bidirectional metadata channel on
`ServiceExecuteRequest`:

```
// ~/Projects/agent/service.go:622–624
// Metadata is bidirectional: echoed back in every Event AND stamped
// onto every line of the session log so external consumers correlate.
Metadata map[string]string
```

DDx can:

- Set `Metadata["produces_artifact"] = "<id>"` and
  `Metadata["media_type"] = "<t>"` when invoking `Execute` from the
  layer-1 generate path.
- Read those keys back off the final event / session log to attach
  provenance to the sidecar.

DDx already passes `Metadata` from three call sites
(`cli/internal/agent/service_run.go:172`,
`cli/internal/agent/agent_runner_service.go:120`,
`cli/internal/agent/execute_bead.go:1035`). The new keys are additive.

**Recommended (non-blocking) doc-only amendment (CONTRACT-003-B):** add
a "Reserved metadata keys" section to CONTRACT-003 that names
`produces_artifact` and `media_type` so future contract revisions do
not stomp them. This is a courtesy clarification, not a contract
change — DDx can ship without it.

### 3. Three-layer run architecture surface — **NO amendment needed**

The contract sits at the layer-1 boundary (single agent invocation).
Layers 2 (`ddx try`) and 3 (`ddx work`) are entirely DDx-side:

- Worktree allocation, base-rev capture, merge/preserve disposition —
  DDx-only, never crosses the contract.
- Bead-queue drain, no-progress detection, stop conditions — DDx-only.

The contract does not need to grow run-type vocabulary or layer
metadata; the plan explicitly forbids a run-type catalog beyond the
three layers (line 23, line 41).

### 4. Per-attempt session-log envelope — **NO amendment needed**

Plan FEAT-006 update (line 77): "DDx owns the envelope/pointer in
evidence bundles; upstream owns inner log shape."

`ServiceExecuteRequest.SessionLogDir` (`service.go:607–609`) is exactly
that pointer: DDx passes the per-attempt evidence directory; upstream
owns what it writes inside. Already in contract — no amendment.

### 5. Cross-checked also-confirmed surfaces

These surfaces the plan implicitly relies on are already in CONTRACT-003
and need no change:

- `ServiceExecuteRequest.Provider` / `Model` / `ModelRef` / `Profile` —
  enable the FEAT-006 "non-bead Profile/Permissions selection
  (artifact-keyed path)" story without contract growth.
- `ServiceExecuteRequest.Permissions` — same.
- Stream events `final` / `routing_decision` / `tool_call` /
  `tool_result` — sufficient for layer-1 metadata capture.
- `StallPolicy` — sufficient for layer-2/3 stall handling without
  contract growth.

## Required amendments — summary

| # | ID | Area | Required for | Status |
|---|----|------|---|---|
| 1 | CONTRACT-003-A | Public `MountCLI() *cobra.Command` on the `agent` package; port `cmd/agent` from `flag` to `cobra` | Plan §"`ddx agent` subcommand fates" — entire structural-passthrough story; bead #15 (delete-with-prejudice list); bead #7 (FEAT-006 §"`ddx agent` is structural passthrough"). | **BLOCKING** — open bead #6 in `~/Projects/agent` |
| 2 | CONTRACT-003-B | Document reserved metadata keys `produces_artifact`, `media_type` | Plan §"Generate-artifact layer placement" forward-compat. | Optional — courtesy clarification |

## Recommendation

- Mark bead #6 (`~/Projects/agent` CONTRACT-003 amendment) as
  required, not conditional. Open it with the CONTRACT-003-A scope
  above as the description.
- Sequence bead #7 (FEAT-006 refactor) **after** the upstream
  amendment merges and DDx bumps its `github.com/DocumentDrivenDX/agent`
  dependency — otherwise the FEAT-006 update will commit to a
  passthrough shape the contract has not ratified.
- Land CONTRACT-003-B opportunistically with the same upstream PR;
  no separate bead needed.
- No other layer-1/2/3 contract gaps were found.

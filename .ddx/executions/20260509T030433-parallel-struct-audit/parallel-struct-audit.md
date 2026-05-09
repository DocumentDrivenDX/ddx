# Parallel Struct Audit

Run ID: `20260509T030433-parallel-struct-audit`
Bead: `ddx-fb290074`

Generated field extraction used `rg` and source slices over Cobra flag registration,
anonymous REST request structs, MCP adapter signatures, canonical spec structs, and
final consumer construction. The tables below are normalized from those extracted
field lists; field names are lower-case CLI/API names where possible.

## Summary

| Command / surface | Layer count | Drop-risk count | Recommendation |
|---|---:|---:|---|
| `ddx agent run` + `/api/agent/run` + MCP `agent_dispatch` | 5 | 12 | High risk. Follow-up `ddx-78ed38c9`: unify or retire legacy dispatch surfaces. |
| `ddx work` / `ddx agent execute-loop` | 5 | 1 | High risk because the single dropped field is a live retry budget. Follow-up `ddx-c0fc00cd`. |
| GraphQL `workerDispatchAdapter` | 4 | 0 | Clean after ExecuteLoopSpec refactor. Keep rejecting legacy args explicitly. |
| Server worker specs | 3 | 0 | Clean. Execute-loop is an alias to `executeloop.ExecuteLoopSpec`; plugin action has one small spec. |
| `ddx bead create/update` + REST + MCP | 4 | 11 | High risk. Follow-up `ddx-9e5e6fa7`: canonical mutation specs/shared apply helpers. |
| `ddx server workers list/show/log` | 2 | 0 | N/A for mutation/spec drift; read-only fetch/display commands. |
| `ddx try` | 4 | 0 | No follow-up now. There is duplicate local wiring, but no transport/spec boundary drop found. |
| `ddx run` | 3 | 0 | Clean. CLI fields collapse directly into `config.CLIOverrides` and `AgentRunRuntime`. |

Drop-risk count counts execution or mutation fields that exist at an earlier
layer but are missing from a later layer. Validation-only acknowledgements,
deprecated no-ops, project-root path selection, and output formatting are listed
but not counted unless they affect the downstream operation.

## `ddx agent run`

Source anchors:
- Cobra flags: `cli/cmd/agent_cmd.go:401-420`
- REST request: `cli/internal/server/server.go:2277-2281`
- MCP adapter: `cli/internal/server/server.go:4635-4649`
- Final consumer: `config.CLIOverrides` plus `agent.AgentRunRuntime`

| Layer | Fields |
|---|---|
| Cobra | `project`, `prompt`, `text`, `harness`, `model`, `profile`, `effort`, `timeout`, `quorum`, `harnesses`, `json`, `output`, `worktree`, `permissions`, `record`, `compare`, `sandbox`, `keep-sandbox`, `post-run`, `arm` |
| REST request | `harness`, `prompt`, `model`, `effort` |
| MCP args | `workingDir`, `harness`, `prompt`, `model`, `effort` |
| Config/runtime | `Harness`, `Model`, `Effort`, `Prompt`, `WorkDir` |
| Service | `RunWithConfigViaService(ctx, workDir, rcfg, runtime)` |

| Field | Classification | Later-layer disposition |
|---|---|---|
| `project` | control-plane | REST has scoped project/working-dir resolution; MCP has `workingDir`. Not counted. |
| `prompt`, `text` | execution | Collapsed to `prompt`; prompt-file/source distinction absent from REST/MCP. |
| `harness`, `model`, `effort` | execution | Preserved. |
| `profile`, `timeout`, `permissions` | execution | Dropped by REST/MCP. |
| `quorum`, `harnesses`, `compare`, `arm` | execution mode | Dropped by REST/MCP. |
| `worktree`, `sandbox`, `keep-sandbox`, `post-run`, `record` | execution mode | Dropped by REST/MCP. |
| `json`, `output` | output-only | CLI formatter only. Not counted. |

Drop-risk score: 12.

Recommendation: `ddx-78ed38c9` should either introduce a canonical run dispatch
spec shared by CLI, REST, and MCP, or remove the legacy REST/MCP dispatch
surfaces from advertised support so callers cannot assume parity.

## `ddx work` / Execute Loop

Source anchors:
- Cobra flags: `cli/cmd/work.go:60-91`
- Cobra-to-spec parser: `cli/cmd/agent_cmd.go:1394-1452`
- Canonical spec: `cli/internal/agent/executeloop/spec.go:65-99`
- Runtime consumer: `cli/cmd/agent_cmd.go:1653-1675`

| Layer | Fields |
|---|---|
| Cobra | `project`, `from`, `harness`, `model`, `profile`, `provider`, `model-ref`, `effort`, `once`, `poll-interval`, `json`, `local`, `no-review`, `no-review-i-know-what-im-doing`, `review-harness`, `review-model`, `max-cost`, `request-timeout`, `rate-limit-max-wait`, `min-power`, `max-power` |
| ExecuteLoopSpec | `project_root`, `harness`, `model`, `profile`, `provider`, `model_ref`, `effort`, `label_filter`, `mode`, `idle_interval`, `no_review`, `review_harness`, `review_model`, `opaque_passthrough`, `max_cost_usd`, `request_timeout`, `min_power`, `max_power`, `from_rev`, `spec_version` |
| DispatchOptions | `local`, `json` |
| Config/runtime | `CLIOverrides`, `ExecuteBeadRuntime`, `ExecuteBeadLoopRuntime` |
| Execute consumer | `ExecuteBeadWithConfig` per selected bead |

| Field | Classification | Later-layer disposition |
|---|---|---|
| `project`, `from` | execution/control | Preserved as `project_root`, `from_rev`. |
| routing fields | execution | Preserved: `harness`, `model`, `profile`, `provider`, `model-ref`, `effort`, `min-power`, `max-power`, `opaque_passthrough`. |
| loop mode fields | execution | Preserved as `mode` and `idle_interval`. |
| review fields | execution | Preserved. Ack flag is validation-only. |
| `max-cost`, `request-timeout` | execution | Preserved. |
| `json`, `local` | control-plane | Preserved in `DispatchOptions`; `local` remains deprecated no-op semantics. |
| `rate-limit-max-wait` | execution | Dropped. Registered at `cli/cmd/work.go:86`, never read by `parseExecuteLoopSpec`, and not assigned to `ExecuteBeadRuntime.RateLimitMaxWait`. |

Drop-risk score: 1.

Recommendation: `ddx-c0fc00cd` should add a canonical rate-limit retry budget
field to `ExecuteLoopSpec` and pass it into `agent.ExecuteBeadRuntime`.

## GraphQL Worker Dispatch

Source anchors:
- Adapter: `cli/internal/server/graphql_adapters.go:52-111`
- Shared prep helper: `cli/internal/server/workers.go:313-322`
- Worker spec alias: `cli/internal/server/workers.go:28`

| Layer | Fields |
|---|---|
| GraphQL raw args | JSON object decoded directly into `executeloop.ExecuteLoopSpec` |
| Adapter defaults | `workers.default_spec.profile`, `workers.default_spec.effort` applied to spec only when unset |
| Prepared worker spec | `prepareExecuteLoopWorkerSpec(projectRoot, spec, ModeWatch)` |
| Worker manager | `StartExecuteLoop(ExecuteLoopWorkerSpec)` |

| Field class | Classification | Later-layer disposition |
|---|---|---|
| Execute-loop spec fields | execution | Preserved through direct decode and shared preparation. |
| `poll_interval`, `once` | legacy | Explicitly rejected rather than silently mapped/dropped. |
| Plugin dispatch fields | separate action | Uses `PluginActionWorkerSpec`; not an execute-loop parallel struct. |

Drop-risk score: 0.

Recommendation: clean. No follow-up filed.

## Server Worker Specs

Source anchors:
- `cli/internal/server/workers.go:28-32`
- `cli/internal/server/workers.go:364`
- `cli/internal/server/workers.go:470`

| Worker kind | Spec fields | Consumer | Drop-risk |
|---|---|---|---:|
| Execute loop | Alias to `executeloop.ExecuteLoopSpec` | `StartExecuteLoop` stores and runs the same spec | 0 |
| Plugin action | `project_root`, `name`, `action`, `scope` | `StartPluginAction` and `runPluginAction` | 0 |

Recommendation: clean. No follow-up filed.

## `ddx bead create/update`

Source anchors:
- Create flags: `cli/cmd/bead.go:326-332`
- Update flags: `cli/cmd/bead.go:604-616`
- REST create/update: `cli/internal/server/server.go:1638-1646`, `cli/internal/server/server.go:1686-1693`
- MCP create/update: `cli/internal/server/server.go:4366-4414`

### Create

| Layer | Fields |
|---|---|
| Cobra | `title`, `type`, `priority`, `labels`, `acceptance`, `description`, `parent`, `set` |
| REST request | `title`, `type`, `priority`, `labels`, `description`, `acceptance`, `parent` |
| MCP args | `title`, `issueType`, `priority`, `labelsStr`, `description`, `acceptance` |
| Store consumer | `bead.Bead` plus optional `Extra` map |

Create drop-risk score: 2 (`set`; `parent` on MCP).

### Update

| Layer | Fields |
|---|---|
| Cobra | `id`, `title`, `status`, `priority`, `labels`, `acceptance`, `assignee`, `parent`, `description`, `notes`, `claim`, `unclaim`, `set`, `unset` |
| REST request | path `id`, `status`, `labels`, `description`, `acceptance`, `priority`, `notes` |
| MCP args | `id`, `status`, `labelsStr`, `description`, `acceptance` |
| Store consumer | `bead.Bead` fields plus `Extra` map and dedicated claim/unclaim store methods |

Update drop-risk score: 9 (`title`, `priority` on MCP, `assignee`,
`parent`, `notes` on MCP, `claim`, `unclaim`, `set`, `unset`).

Combined drop-risk score: 11.

Recommendation: `ddx-9e5e6fa7` should introduce canonical bead create/update
mutation specs and shared apply helpers for Cobra, REST, and MCP.

## `ddx server workers list/show/log`

Source anchors:
- `cli/cmd/server_workers.go:31-61`
- `cli/cmd/server_workers.go:99-120`
- `cli/cmd/server_workers.go:175-209`

| Command | Layers | Fields | Drop-risk |
|---|---:|---|---:|
| `ddx server workers list` | 2 | Fetches `GET /api/agent/workers`, formats returned records | 0 |
| `ddx server workers show` | 2 | Fetches `GET /api/agent/workers/{id}`, formats returned record | 0 |
| `ddx server workers log` | 2 | Fetches `GET /api/agent/workers/{id}/log`, streams text | 0 |

Recommendation: N/A for this smell. These are read-only display commands with
no local mutation/spec that can silently drop execution fields.

## `ddx try`

Source anchors:
- Cobra flags: `cli/cmd/try.go:82-96`
- Flag reads: `cli/cmd/try.go:105-129`
- Attempt config/runtime: `cli/cmd/try.go:238-257`
- Loop runtime: `cli/cmd/try.go:310-350`

| Layer | Fields |
|---|---|
| Cobra | `project`, `from`, `harness`, `model`, `profile`, `provider`, `model-ref`, `effort`, `no-review`, `no-review-i-know-what-im-doing`, `review-harness`, `review-model`, `request-timeout`, `min-power`, `max-power` |
| Attempt config | `harness`, `model`, `provider`, `model-ref`, `profile`, `effort`, `min-power`, `max-power`, `opaque_passthrough`, `provider_request_timeout` |
| ExecuteBeadRuntime | `from_rev`, `output`, `bead_events`, `resource_checker` |
| ExecuteBeadLoopRuntime | `once`, `no_review`, hooks, project/session metadata |

| Field | Classification | Later-layer disposition |
|---|---|---|
| routing fields | execution | Preserved in attempt config. |
| `request-timeout` | execution | Preserved as provider request timeout. |
| `from` | execution | Preserved in `ExecuteBeadRuntime`. |
| review fields | execution | Preserved in reviewer construction and loop runtime. Ack flag is validation-only. |
| `project` | control-plane | Used to resolve store/root. Not counted. |

Drop-risk score: 0.

Recommendation: no follow-up now. `ddx try` still has duplicate local wiring,
but the audit found no silent field drop at a transport/spec boundary.

## `ddx run`

Source anchors:
- Cobra flags: `cli/cmd/run.go:49-64`
- Config/runtime construction: `cli/cmd/run.go:139-160`
- Service call: `cli/cmd/run.go:162`

| Layer | Fields |
|---|---|
| Cobra | `project`, `prompt`, `text`, `harness`, `model`, `provider`, `model-ref`, `min-power`, `max-power`, `persona`, `profile`, `effort`, `permissions`, `timeout`, `json`, `output` |
| Config/runtime | `Harness`, `Model`, `Provider`, `ModelRef`, `Profile`, `Effort`, `Permissions`, `MinPower`, `MaxPower`, `OpaquePassthrough`, `Timeout`, `Prompt`, `PromptFile`, `PromptSource`, `WorkDir` |
| Service | `RunWithConfigViaService` |

| Field | Classification | Later-layer disposition |
|---|---|---|
| routing fields | execution | Preserved in `config.CLIOverrides`. |
| prompt fields | execution | Preserved in `AgentRunRuntime`; persona is applied before runtime construction. |
| `permissions`, `timeout` | execution | Preserved in `config.CLIOverrides`. |
| `project` | control-plane | Used to resolve workdir. |
| `json`, `output` | output-only | CLI formatter only. |

Drop-risk score: 0.

Recommendation: clean. No follow-up filed.


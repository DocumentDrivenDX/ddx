<bead-review>
  <bead id="ddx-d0973f38" iter=1>
    <title>agent/test: migrate second half of execute_bead_loop_test.go to RunWithConfig</title>
    <description>
Stage 1 of SD-024. Mechanical migration of remaining ~11 ExecuteBeadLoopOptions literals.

In-scope:
- cli/internal/agent/execute_bead_loop_test.go (second half)
    </description>
    <acceptance>
rg 'ExecuteBeadLoopOptions\\{' cli/internal/agent/execute_bead_loop_test.go | wc -l returns 0. cd cli &amp;&amp; go test ./internal/agent/... passes. cd cli &amp;&amp; go test ./... passes.
    </acceptance>
    <labels>ddx, kind:test-migration, area:agent, sd-024, stage:1</labels>
  </bead>

  <governing>
    <ref id="SD-024" path="docs/helix/02-design/solution-designs/SD-024-config-driven-runtime-options.md" title="Solution Design: Config-Driven Runtime Options">
      <content>
---
ddx:
  id: SD-024
  depends_on:
    - FEAT-006
    - FEAT-022
    - SD-019
---
# Solution Design: Config-Driven Runtime Options

## Purpose

Eliminate the manual command-layer wiring pattern that copies config
fields into options structs at every dispatch site. Replace it with a
type-system-enforced contract where every worker, runner, and review
path receives a single resolved configuration object that the type
system requires. Make the bug class — "config field exists but no
caller wires it through, so the loop silently uses defaults" — locally
unreachable.

## Scope

In scope:

- Architectural pattern: `ResolvedConfig` (immutable) + per-family
  `*Runtime` structs replace the `*Options` family today.
- Three production dispatch sites that must call `config.Load`:
  CLI `ddx work`, server `runWorker`, GraphQL `StartWorker`.
- Five options families that get migrated in stages:
  `ExecuteBeadLoopOptions`, `RunOptions`, `ExecuteBeadOptions`,
  `CompareOptions`, `QuorumOptions`.
- Three latent dead-config-field cases that surface during the
  refactor: `NoProgressCooldown`, `MaxNoChangesBeforeClose`, and
  `HeartbeatInterval` are hardcoded constants today; `ContextBudget`
  has no config home; `ExecutionsConfig.Mirror` is parsed but never
  consumed.
- Lint enforcement (Stage 4) that mechanically forbids the old
  `*Options`-with-durable-fields pattern.
- Test coverage uplift: the four disabled config tests
  (`*.disabled`) get evaluated and either restored against the new
  API or deleted with rationale.

Out of scope:

- The CONTRACT-003 boundary with `~/Projects/agent`. Investigation
  confirmed the agent module does not import `ddx/internal/config`
  and ddx pre-resolves at `cli/internal/agent/agent_runner_service.go:104`
  and `cli/internal/agent/serviceconfig.go:80`. No agent-repo changes
  are required and no contract document needs revision. Agent
  v0.9.9 (ADR-005 smart routing) further simplifies the boundary by
  removing `ServiceExecuteRequest.PreResolved` — `ResolveRoute` is
  now informational and `Execute` re-resolves on its own inputs.
  The ddx side just populates `EstimatedPromptTokens` and
  `RequiresTools` (new per-invocation hints) on `ServiceExecuteRequest`;
  routing decisions are owned upstream.
- Routing-layer cleanup driven by agent v0.9.9 (rendering composed
  inventory in `ddx agent route-status`, retiring `agent.routing.model_overrides`,
  dropping the "No model routes configured" fatal error path).
  Tracked separately as a small follow-up batch on top of FEAT-006;
  does not block this refactor and is not delivered by it.
- The wire shape of `agentlib.ExecuteRequest` /
  `agentlib.ExecuteResponse`. The refactor changes how ddx-side state
  is organized before the request is built; the request itself is
  unchanged.
- New CLI commands or operator workflows. This is a structural
  refactor; user-visible behavior is preserved.

## Problem

A reviewer recently caught that FEAT-022 Stage G shipped a
`ReviewMaxRetries` field on `*config.Config` plus a resolver, and an
`ExecuteBeadLoopOptions.ReviewMaxRetries` field that the loop honors,
but no caller in `cli/cmd/` ever read the resolved value and assigned
it to the options struct. The config field was dead in production. The
test that claimed to verify the loop "respects" the override actually
injected the field directly into options.

Investigation found this is systemic, not a one-off:

- 6 production sites that construct `ExecuteBeadLoopOptions`,
  `RunOptions`, or `ExecuteBeadOptions` across CLI and server.
  **Zero** of them call `config.Load` before constructing the options
  struct.
- Three dispatch entry points (CLI `ddx work` at
  `cli/cmd/agent_cmd.go:1997`, server `runWorker` at
  `cli/internal/server/workers.go:878`, GraphQL `StartWorker` at
  `cli/internal/server/graphql/resolver_feat008.go:77`) all skip
  config entirely and populate options from CLI flags or request
  fields only. Every config-derived knob across every options struct
  is dead in the same way `ReviewMaxRetries` was.
- Two more `ExecuteBeadLoopOptions` fields, `NoProgressCooldown` and
  `MaxNoChangesBeforeClose`, are hardcoded constants in
  `execute_bead_loop.go:292` with no config field at all. They have
  the same shape as `ReviewMaxRetries` would have had if no resolver
  existed.
- The Go type system permits constructing any `*Options` struct with
  zero-value defaults. The compiler cannot tell the implementer
  "you forgot to wire this field." Every new config knob is a new
  opportunity for the same bug.

The pattern accumulated weeks of cruft: each new config knob added a
field to `*Options`, a resolver to `*Config`, and silently relied on
every caller remembering to bridge them. The reviewer caught one
instance because Stage G's acceptance criteria explicitly demanded an
end-to-end YAML→loop test. Without that explicit gate, the gap stays
invisible.

## Approach

Three structural changes that compose:

### 1. `ResolvedConfig` is the only valid input.

A new immutable value type `config.ResolvedConfig` exposes accessors
for every durable knob a worker, runner, or review path consumes. The
type has no exported fields; clients call methods.

A naive "unexported fields" implementation does not actually prevent
construction — Go's zero-value rule means
`var rcfg config.ResolvedConfig` and `config.ResolvedConfig{}` are
both valid expressions in any package. The design closes this escape
hatch with a sealed-construction pattern: `ResolvedConfig` carries a
single unexported sentinel field set only by `Resolve`. Every public
accessor checks the sentinel on the first read and panics with a
specific message naming `LoadAndResolve` if the sentinel is unset.
A zero-value `ResolvedConfig` thus passes the type check but fails
loudly on any access — making the bypass unreachable in any code
path that actually consumes the value.

`ResolvedConfig` is shared across goroutines without synchronization
because no method mutates it. There is no `Set*` API, no public
mutator, no JSON unmarshal target.

### 2. Runtime structs hold runtime plumbing only.

`ExecuteBeadLoopOptions` becomes `ExecuteBeadLoopRuntime`, with all
durable fields removed. The same applies to the four other options
families. A runtime struct holds:

- non-serializable plumbing (`Log io.Writer`, `EventSink`,
  `ProgressCh`, `PreClaimHook`)
- per-invocation runtime intent (`Once`, `LabelFilter`, `SessionID`,
  `WorkerID`)

Anything that could plausibly live in config — anything someone might
want to set persistently for a project — is forbidden from the runtime
struct. The compiler enforces this because the durable fields are
removed entirely.

Run, RunCompare, RunQuorum, ExecuteBead, and ReviewBead all change
signature to:

```go
func (...) Run(ctx context.Context, rcfg config.ResolvedConfig, runtime XRuntime) (...)
```

### 3. All three dispatch sites resolve once, hand off everywhere — atomically.

The three dispatch entry points (CLI `ddx work`, server `runWorker`,
GraphQL `StartWorker`) all gain a single shared resolution helper:

```go
rcfg, err := config.LoadAndResolve(projectRoot, overrides)
```

`overrides` carries CLI flag values or request fields. The helper
loads the project's `.ddx/config.yaml`, applies overrides as a layer,
and returns the immutable `ResolvedConfig`. Every downstream call
takes that single value.

**All three sites must be migrated together for the bug class to
actually close.** Migrating only the CLI site leaves `ddx work` from
a server context still bypassing config (which is what production
actually uses). Migrating only the server site leaves direct CLI
runs broken. The migration discipline (see §Migration) requires that
the loop migration be considered "complete" only when all three
dispatch paths route through `LoadAndResolve`.

There is no other supported way to obtain a `ResolvedConfig` in
production code. Adding a fourth dispatch entry (e.g. a future REST
endpoint that starts a worker) requires calling `LoadAndResolve` at
that site.

## Why this approach

Three alternatives considered and rejected:

**Alternative A: builder pattern on options struct.**
`agent.NewExecuteBeadLoopOptions(cfg).WithProject(p)`.
Surface looks cleaner but does not eliminate the bug class — the
builder still has setters per durable field, and a new config field
still requires updating the builder. Test code can still call
`builder.WithReviewMaxRetries(0)` to bypass real config. Does not
make the bypass impossible.

**Alternative B: documented immutability of `*Config` shared across
callers.** Cheaper to implement (no new type) but defeated by Go's
shallow-copy semantics on structs containing maps and pointers.
`*Config` carries `PersonaBindings map`, `*AgentConfig` (with
`Models map`), `*RoutingConfig` (multiple maps), and
`*EvidenceCapsConfig` (`PerHarness map`). A shallow copy aliases all
these maps. A caller that runs `cfg2 := *cfg; cfg2.AgentConfig.Models[k] = v`
silently mutates the original. Documentation is not enforcement.
The type system has to make the failure mode unreachable.

**Alternative C: pass `*Config` directly with the existing `*Options`
shape preserved.** Would require every caller to read `cfg.Resolve*`
methods and copy results into options. This is what we have today.
The bug class persists; we only delay it.

The chosen approach makes three things simultaneously true:

1. The compiler refuses to call `Run` without a `ResolvedConfig`.
2. The runtime struct cannot carry a durable field, so a future config
   knob has nowhere to bypass to.
3. There is exactly one production path that produces a
   `ResolvedConfig`, so adding a new config knob requires updating
   exactly one place (the `Resolve` method) for it to be visible to
   every consumer.

## Migration discipline

The refactor delivers across many small beads, not a few big stages.
Two non-negotiable principles govern every bead:

**Green tree.** Every bead leaves the tree fully functional. No bead
breaks user-visible behavior — `ddx work` keeps draining the queue,
`ddx agent run` keeps dispatching, the server keeps accepting GraphQL
mutations. The migration adds new types and call paths *alongside*
the existing ones, then retires the old ones in dedicated cleanup
beads only after every caller has moved.

**Per-bead size budget.** Each bead must fit in a single execute-bead
worker window: target <500 LoC of net change, <20 minutes of agent
time, under $10 of model spend. Empirically, FEAT-022's beads averaged
7-15 minutes and $3-7. Anything that wants to "do the whole loop
migration in one shot" is sized wrong and must be split.

These principles imply a specific implementation pattern at every
seam:

1. **Add new alongside old.** Introduce `ResolvedConfig`, `Resolve`,
   `LoadAndResolve`, the `*Runtime` structs, and the new `Run`
   signatures *alongside* the existing `*Options` types and `Run`
   methods. Old methods stay alive and call into the new path
   internally if needed; production callers haven't moved yet.
2. **Migrate one production caller per bead.** Each dispatch site
   moves in its own bead, with a behavioral test that proves config
   actually drives the resulting behavior at that site.
3. **Migrate test files in groups.** Test sites are mechanical;
   group them by file (one test file per bead). Tests on the old
   `*Options` API stay green via a temporary shim until the file is
   migrated.
4. **Retire old types in dedicated cleanup beads.** When and only
   when every production and test caller has moved, a final bead
   removes the old type and shim.

## Stage groupings

The bead inventory is structured into stage groupings. Each grouping
is a logical milestone, not a single bead. The actual bead count
exceeds 25 — see TD-024 §Bead Inventory for the per-bead breakdown.

Stage 1 unblocks FEAT-022 Stage G. Stages 2–4 land sequentially or
with overlap.

**Stage 1 — Foundation + ExecuteBeadLoop.**
Introduces `ResolvedConfig`, `Resolve`, `CLIOverrides`,
`LoadAndResolve`, the test-config constructors, and the
`ExecuteBeadLoopRuntime` shape. Adds the new `*Runtime` types and
new `RunWithConfig` methods *alongside* existing `*Options` types
and `Run` methods (green-tree principle). Adds resolvers for
`NoProgressCooldown`, `MaxNoChangesBeforeClose`, `HeartbeatInterval`.
Migrates **all three production dispatch sites** (CLI `ddx work`,
server `runWorker`, GraphQL `StartWorker`) plus ~44 test sites.
Retires the legacy `ExecuteBeadLoopOptions` + `Run` once all
callers migrate. Adds behavioral e2e tests on every dispatch path
proving `.ddx/config.yaml` actually drives the running loop. This
stage blocks FEAT-022 Stage G.

**Stage 2 — RunOptions migration.**
Migrates `RunOptions` → `AgentRunRuntime` using the same
add-alongside-then-retire pattern. ~14 constructor sites including
`CompareOptions` and `QuorumOptions` (which embed `RunOptions`).
New test-config constructor variant for the run path.

**Stage 3 — ExecuteBeadOptions migration.**
Migrates `ExecuteBeadOptions` → `ExecuteBeadRuntime`. ~16 sites.
Adds `ContextBudget` to `EvidenceCapsConfig` (currently has no
config home). Wires `ExecutionsConfig.Mirror` (currently parsed but
not consumed) into the resolved-config path.

**Stage 4 — Lint enforcement and test debt cleanup.**
Adds a structural lint analyzer (`runtimelint`) using a closed-list
of forbidden field names scoped to `*Runtime` structs in
`cli/internal/agent/`. Reuses the analyzer pattern from FEAT-022
Stage A2 so the new rule plugs into the same Lefthook + CI hooks.
Resolves the four disabled config tests per evidence-driven
decisions specified in TD-024 §Disabled Test Resolution: two are
deleted (their target features are dead in the current loader),
two are rewritten against the current API.

## Test coverage commitment

Stage 1's e2e tests are non-negotiable. Each must:

1. Load a real `.ddx/config.yaml` from disk that sets a non-default
   value for the knob under test.
2. Pass through `LoadAndResolve` (no test-injected options struct).
3. Execute the production code path (real `Run`, not a mock).
4. Assert behavior — not a synthetic "config_resolved" telemetry
   event, but actual observable behavior that the configured value
   drives.

For `ReviewMaxRetries` specifically, this means seeding N review
failures via a deterministic test runner and asserting the bead's
event log shows `review-manual-required` only at the configured
threshold. We do not add a new event type whose only purpose is to
expose `ReviewMaxRetries` to tests — that would itself be an
injection seam, exactly the pattern this refactor rejects.

The same e2e test pattern fires on every dispatch path the migration
touches (CLI, server worker, GraphQL resolver). The bug we're fixing
affected all three; the test coverage must demonstrate all three are
fixed.

Stage 4 commits to net-positive coverage. Disabled config tests
either return to the suite (against the new API), get deleted with
a one-line rationale, or get rewritten if the underlying feature is
alive but the test fixture has drifted. The test count after Stage 4
must equal or exceed the pre-refactor count plus the new e2e and
accessor tests. Per-file decisions for the four `*.disabled` files
are specified in TD-024 §Disabled Test Resolution.

## Scope discipline

This refactor touches state organization on the ddx side. It does not
touch:

- `agentlib.ExecuteRequest` or `agentlib.ExecuteResponse` shape.
- Any code in `~/Projects/agent`.
- The session log format (FEAT-006, TD-006).
- The execution bundle layout (TD-010).
- The bead tracker schema (SD-004, TD-004).
- The model catalog (SD-014).

Changes that are *enabled* by the refactor but not part of it:

- Removing user-maintained `model_routes` from the agent's
  config surface (separate plan from earlier in this thread; the
  refactor here makes the migration easier on the ddx-consumer side
  but does not perform it).
- A new operator surface for showing "the resolved config the loop
  is actually running with." Worth doing but tracked separately.

## References

- FEAT-006 — Agent Service (CONTRACT-003 boundary; this SD confirms
  the boundary stays clean).
- FEAT-022 — Prompt Evidence Assembly. This SD's Stage 1 unblocks
  FEAT-022 Stage G.
- SD-019 — Multi-Project Server Topology. This SD's dispatch-site
  changes apply per-project at request time, consistent with SD-019's
  per-request project resolution.
- TD-024 — Technical Design counterpart specifying the concrete
  type definitions, accessor signatures, and migration code shapes.
      </content>
    </ref>
  </governing>

  <diff rev="4dbcc2ccd47a506f8f66962795654e386c4923fc">
commit 4dbcc2ccd47a506f8f66962795654e386c4923fc
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 25 12:31:34 2026 -0400

    chore: add execution evidence [20260425T162315-]

diff --git a/.ddx/executions/20260425T162315-77ab5f06/manifest.json b/.ddx/executions/20260425T162315-77ab5f06/manifest.json
new file mode 100644
index 00000000..226eb542
--- /dev/null
+++ b/.ddx/executions/20260425T162315-77ab5f06/manifest.json
@@ -0,0 +1,47 @@
+{
+  "attempt_id": "20260425T162315-77ab5f06",
+  "bead_id": "ddx-d0973f38",
+  "base_rev": "4ff56d45441d71a3da05b4339afd94e418a28f47",
+  "created_at": "2026-04-25T16:23:16.812892339Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-d0973f38",
+    "title": "agent/test: migrate second half of execute_bead_loop_test.go to RunWithConfig",
+    "description": "Stage 1 of SD-024. Mechanical migration of remaining ~11 ExecuteBeadLoopOptions literals.\n\nIn-scope:\n- cli/internal/agent/execute_bead_loop_test.go (second half)",
+    "acceptance": "rg 'ExecuteBeadLoopOptions\\\\{' cli/internal/agent/execute_bead_loop_test.go | wc -l returns 0. cd cli \u0026\u0026 go test ./internal/agent/... passes. cd cli \u0026\u0026 go test ./... passes.",
+    "parent": "ddx-0f3ae192",
+    "labels": [
+      "ddx",
+      "kind:test-migration",
+      "area:agent",
+      "sd-024",
+      "stage:1"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-25T16:23:15Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "execute-loop-heartbeat-at": "2026-04-25T16:23:15.96789969Z",
+      "spec-id": "SD-024"
+    }
+  },
+  "governing": [
+    {
+      "id": "SD-024",
+      "path": "docs/helix/02-design/solution-designs/SD-024-config-driven-runtime-options.md",
+      "title": "Solution Design: Config-Driven Runtime Options"
+    }
+  ],
+  "paths": {
+    "dir": ".ddx/executions/20260425T162315-77ab5f06",
+    "prompt": ".ddx/executions/20260425T162315-77ab5f06/prompt.md",
+    "manifest": ".ddx/executions/20260425T162315-77ab5f06/manifest.json",
+    "result": ".ddx/executions/20260425T162315-77ab5f06/result.json",
+    "checks": ".ddx/executions/20260425T162315-77ab5f06/checks.json",
+    "usage": ".ddx/executions/20260425T162315-77ab5f06/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-d0973f38-20260425T162315-77ab5f06"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260425T162315-77ab5f06/result.json b/.ddx/executions/20260425T162315-77ab5f06/result.json
new file mode 100644
index 00000000..29c880c4
--- /dev/null
+++ b/.ddx/executions/20260425T162315-77ab5f06/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-d0973f38",
+  "attempt_id": "20260425T162315-77ab5f06",
+  "base_rev": "4ff56d45441d71a3da05b4339afd94e418a28f47",
+  "result_rev": "8561a188bc73460563b953b1dcb821653fe54f3e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-6103e999",
+  "duration_ms": 496810,
+  "tokens": 10084,
+  "cost_usd": 1.5374887499999998,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260425T162315-77ab5f06",
+  "prompt_file": ".ddx/executions/20260425T162315-77ab5f06/prompt.md",
+  "manifest_file": ".ddx/executions/20260425T162315-77ab5f06/manifest.json",
+  "result_file": ".ddx/executions/20260425T162315-77ab5f06/result.json",
+  "usage_file": ".ddx/executions/20260425T162315-77ab5f06/usage.json",
+  "started_at": "2026-04-25T16:23:16.813179506Z",
+  "finished_at": "2026-04-25T16:31:33.623414879Z"
+}
\ No newline at end of file
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

## Your task

Examine the diff and each acceptance-criteria (AC) item. For each item assign one grade:

- **APPROVE** — fully and correctly implemented; cite the specific file path and line that proves it.
- **REQUEST_CHANGES** — partially implemented or has fixable minor issues.
- **BLOCK** — not implemented, incorrectly implemented, or the diff is insufficient to evaluate.

Overall verdict rule:
- All items APPROVE → **APPROVE**
- Any item BLOCK → **BLOCK**
- Otherwise → **REQUEST_CHANGES**

## Required output format

Respond with a structured review using exactly this layout (replace placeholder text):

---
## Review: ddx-d0973f38 iter 1

### Verdict: APPROVE | REQUEST_CHANGES | BLOCK

### AC Grades

| # | Item | Grade | Evidence |
|---|------|-------|----------|
| 1 | &lt;AC item text, max 60 chars&gt; | APPROVE | path/to/file.go:42 — brief note |
| 2 | &lt;AC item text, max 60 chars&gt; | BLOCK   | — not found in diff |

### Summary

&lt;1–3 sentences on overall implementation quality and any recurring theme in findings.&gt;

### Findings

&lt;Bullet list of REQUEST_CHANGES and BLOCK findings. Each finding must name the specific file, function, or test that is missing or wrong — specific enough for the next agent to act on without re-reading the entire diff. Omit this section entirely if verdict is APPROVE.&gt;
  </instructions>
</bead-review>

<bead-review>
  <bead id="ddx-ade5ebb3" iter=1>
    <title>agent: retire RunOptions + CompareOptions + QuorumOptions + legacy Run</title>
    <description>
Stage 2 of SD-024 cleanup. Removes all three legacy options types and the legacy Run method on the agent runner. Renames RunWithConfig to Run.

Depends on bead 16 (Stage 1 cleanup) AND beads 17-21 (every Stage 2 caller migrated).

In-scope:
- cli/internal/agent/types.go — delete RunOptions, CompareOptions, QuorumOptions structs
- cli/internal/agent/runner.go — delete legacy Run, rename RunWithConfig
    </description>
    <acceptance>
field RunOptions removed; field CompareOptions removed; field QuorumOptions removed. (Yes the gate currently parses 'field' rather than 'type'; intentional — re-uses the field_removed verifier whose go-source regex `\bX\s+[\*\[\]\w.]+` matches both `Name Type` field declarations AND `type Name struct` type declarations. The gate refuses already_satisfied if any of these symbols still appear at a declaration site in cli/internal/agent/.) cd cli &amp;&amp; go build ./... passes. cd cli &amp;&amp; go test ./... passes. Manual verification: `rg '^type (RunOptions|CompareOptions|QuorumOptions) ' cli/internal/agent/types.go` returns zero.
    </acceptance>
    <notes>
REWRITTEN AC 2026-04-26: Previous AC was `rg 'type X' returns 0` which didn't match any structural-claim pattern the gate parses, so already_satisfied passed through unchecked. Reopened twice with the same AC and fake-closed twice. Now rewritten using field_removed phrasing — the gate parser extracts these claims and the verifier greps go files for the names; finding `type RunOptions struct {` (or any sibling) refuses closure. Existing types still at types.go:36/81/171; runtimelint reports ~80 violations across the tree. Worker must actually delete the three type definitions and migrate every call site.
    </notes>
    <labels>ddx, kind:cleanup, area:agent, sd-024, stage:2</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260428T025336-72807bcd/manifest.json</file>
    <file>.ddx/executions/20260428T025336-72807bcd/result.json</file>
  </changed-files>

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

  <diff rev="090ac24e0a8e9438aade07200f36b904ecf35713">
diff --git a/.ddx/executions/20260428T025336-72807bcd/manifest.json b/.ddx/executions/20260428T025336-72807bcd/manifest.json
new file mode 100644
index 00000000..fd722ffa
--- /dev/null
+++ b/.ddx/executions/20260428T025336-72807bcd/manifest.json
@@ -0,0 +1,127 @@
+{
+  "attempt_id": "20260428T025336-72807bcd",
+  "bead_id": "ddx-ade5ebb3",
+  "base_rev": "14b805290204a5f5f86aede411b633b8e0757dcb",
+  "created_at": "2026-04-28T02:53:37.314609196Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-ade5ebb3",
+    "title": "agent: retire RunOptions + CompareOptions + QuorumOptions + legacy Run",
+    "description": "Stage 2 of SD-024 cleanup. Removes all three legacy options types and the legacy Run method on the agent runner. Renames RunWithConfig to Run.\n\nDepends on bead 16 (Stage 1 cleanup) AND beads 17-21 (every Stage 2 caller migrated).\n\nIn-scope:\n- cli/internal/agent/types.go — delete RunOptions, CompareOptions, QuorumOptions structs\n- cli/internal/agent/runner.go — delete legacy Run, rename RunWithConfig",
+    "acceptance": "field RunOptions removed; field CompareOptions removed; field QuorumOptions removed. (Yes the gate currently parses 'field' rather than 'type'; intentional — re-uses the field_removed verifier whose go-source regex `\\bX\\s+[\\*\\[\\]\\w.]+` matches both `Name Type` field declarations AND `type Name struct` type declarations. The gate refuses already_satisfied if any of these symbols still appear at a declaration site in cli/internal/agent/.) cd cli \u0026\u0026 go build ./... passes. cd cli \u0026\u0026 go test ./... passes. Manual verification: `rg '^type (RunOptions|CompareOptions|QuorumOptions) ' cli/internal/agent/types.go` returns zero.",
+    "parent": "ddx-0f3ae192",
+    "labels": [
+      "ddx",
+      "kind:cleanup",
+      "area:agent",
+      "sd-024",
+      "stage:2"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-28T02:53:36Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "685",
+      "closing_commit_sha": "562f293c75a19b1deda23b83e7692e0a4836d75e",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-25T23:00:48.577599868Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260425T225551-063394ca\",\"harness\":\"claude\",\"input_tokens\":22,\"output_tokens\":18526,\"total_tokens\":18548,\"cost_usd\":1.5827685,\"duration_ms\":296489,\"exit_code\":0}",
+          "created_at": "2026-04-25T23:00:48.65322884Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=18548 cost_usd=1.5828"
+        },
+        {
+          "actor": "ddx",
+          "body": "Bead ddx-ade5ebb3 — agent: retire RunOptions + CompareOptions + QuorumOptions + legacy Run\n\nSTATUS: not attempted; scope-vs-budget mismatch between bead description\nand actual codebase state. Request re-scoping into sub-beads (see below)\nbefore another execution attempt.\n\n================================================================\nWhat is done\n================================================================\n\nStage 2 dispatch-site migrations (beads c0ff5d3e, 4c31e465, 21f9e321,\nee2cd434) landed RunWithConfig / RunCompareWithConfigViaService /\nRunQuorumWithConfigViaService alongside the legacy Run /\nRunCompareViaService / RunQuorumViaService entry points. The CLI\ndispatch sites (`ddx agent run`, `ddx agent compare`, `ddx agent\nquorum`) now construct rcfg + AgentRunRuntime / CompareRuntime /\nQuorumRuntime and dispatch through the new `*WithConfig*` paths.\nA subset of agent test files were migrated by bead ee2cd434.\n\nStage 1 cleanup (bead edbfa046) removed ExecuteBeadLoopOptions and\nthe legacy `ExecuteBeadWorker.Run(opts)` method, renamed\n`RunWithConfig` → `Run`. That cleanup touched 18 files and ~123\nnet-changed lines.\n\n================================================================\nWhy this bead cannot land in one pass as currently scoped\n================================================================\n\nThe bead description in-scopes only two files:\n\n  - cli/internal/agent/types.go (delete RunOptions / CompareOptions /\n    QuorumOptions structs)\n  - cli/internal/agent/runner.go (delete legacy Run, rename\n    RunWithConfig → Run)\n\nThe bead acceptance criteria, however, require `cd cli \u0026\u0026 go build\n./...` and `cd cli \u0026\u0026 go test ./...` to pass. After the structs and\nthe legacy method are deleted, the tree does not compile. A grep\nover the post-Stage-2 tree shows the deleted symbols are still\nreferenced in:\n\n  RunOptions: 137 occurrences across 42 files\n  CompareOptions: ~15 occurrences across compare_adapter.go,\n    models.go, agent_compare_config_test.go, plus 4 testdata files\n  QuorumOptions: ~5 occurrences across compare_adapter.go,\n    agent_quorum_config_test.go, plus testdata\n\nStage 2's dispatch-site migrations only retired *construction* of\nthese structs at the four CLI entry points and a portion of\nagent-package tests. The structs are still load-bearing for:\n\n  - Internal helpers in cli/internal/agent: BuildArgs, resolveHarness,\n    resolvePrompt, resolveModel, resolveTimeout, resolveWallClock,\n    runVirtualFn, RunAgent, runScriptFn, runClaudeWithFallbackFn,\n    SessionIndexEntryFromResult, BenchmarkArmsToCompare, and the\n    entire compare_adapter.go internal pipeline (CompareOptions /\n    QuorumOptions are used as the working struct shape inside\n    RunCompare / RunQuorum, not just at the top of the public API).\n  - The `runner.Run(RunOptions{...})` self-call inside\n    runner.go::TestProviderConnectivity (line 976).\n  - The `r.Run(RunOptions{...})` call inside grade.go::169.\n  - The `agent.RunViaService` exported function and its internal\n    callers (cli/cmd/agent_cmd.go:1462, cli/internal/server/server.go\n    at lines 1913 and 3786, cli/internal/exec/store.go:201).\n  - The `Run(opts agent.RunOptions)` interface contract on\n    cli/internal/exec/types.go:11 plus its three implementers\n    (cmd/exec.go, internal/exec/store.go, exec_acceptance_test.go,\n    workers_stop_propagation_test.go).\n  - cli/cmd/agent_execute_bead_test.go's two test runners\n    (fakeAgentRunner, modelPassthroughCapture).\n  - cli/internal/agent/prompt_ingress_oversize_test.go's\n    resolvePrompt / defaultResolvePromptForCompare /\n    RunViaServiceWith calls (the bead 21 commit explicitly noted\n    these were left out of scope).\n  - cli/tools/lint/evidencelint/* — the testdata uses a *separate*\n    stub `agent.RunOptions` in testdata/src/agent/agent.go that is\n    independent of the production type, so testdata compiles fine,\n    but analyzer.go's package-level doc comments and\n    runOptionsTypeNames list reference the production type's name.\n\n================================================================\nEstimated true cost\n================================================================\n\nStage 1's analogous cleanup (ExecuteBeadLoopOptions) was 18 files /\n~123 net lines. Stage 2's RunOptions/CompareOptions/QuorumOptions\ncleanup is ~3-4× larger by raw reference count, and ~2× larger\nstructurally because (a) RunOptions is used as the central internal\ndata carrier inside compare_adapter.go's full Run / RunCompare /\nRunQuorum pipelines (not just at the dispatch entry), and (b)\nexternal callers in three different packages (cmd/, internal/exec/,\ninternal/server/) each need their own rcfg + AgentRunRuntime\nconstruction wired in, which means the `agent.RunViaService` /\n`agent.RunCompareViaService` / `agent.RunQuorumViaService`\nback-compat shims must stay or be replaced.\n\nConservative estimate: ~40 files / ~1500–2500 net lines / 3-5×\nthe per-bead size budget defined in SD-024 §\"Per-bead size budget\"\n(target \u003c500 LoC, \u003c20 minutes, \u003c$10 spend).\n\nA no-change here is preferable to a half-finished commit that\nleaves the tree red or smuggles a partial migration past review.\n\n================================================================\nWhat a follow-up attempt would need\n================================================================\n\nRecommend splitting into a fresh stage-2-cleanup sub-stage with\nbeads sized to the SD-024 budget. Suggested decomposition:\n\n  1. Migrate cli/internal/agent/compare_adapter.go internals to\n     drive RunCompare / RunQuorum through AgentRunRuntime + rcfg\n     end-to-end; retire CompareOptions and QuorumOptions struct\n     usage *inside* the agent package.\n\n  2. Migrate cli/internal/agent helpers (BuildArgs, resolveHarness,\n     resolvePrompt, resolveModel, resolveTimeout, resolveWallClock,\n     runVirtual, RunAgent, runScript, runClaudeWithFallback,\n     SessionIndexEntryFromResult, BenchmarkArmsToCompare) so they\n     take rcfg + AgentRunRuntime (or per-field args) instead of\n     RunOptions. Includes runner.go::TestProviderConnectivity and\n     grade.go's `r.Run` self-call.\n\n  3. Migrate cli/internal/agent/service_run.go (RunViaService et\n     al.) to take rcfg + AgentRunRuntime and update all three\n     callers: cmd/agent_cmd.go:1462, server.go:1913+3786,\n     internal/exec/store.go:201.\n\n  4. Migrate cli/internal/exec (types.go interface +\n     store.go construction + store_test.go mock) and\n     cmd/exec.go's serviceExecAgentRunner +\n     exec_acceptance_test.go mock onto the rcfg-based interface.\n\n  5. Migrate the remaining agent-package test sites that bead\n     ee2cd434 left out of scope (prompt_ingress_oversize_test.go,\n     execute_bead_runtime_test.go, models_test.go,\n     session_index_test.go, virtual_test.go, claude_stream_test.go,\n     and the BuildArgs tests in agent_test.go that still construct\n     RunOptions literals as inputs to the unit under test).\n\n  6. Migrate cli/cmd test sites (agent_execute_bead_test.go's two\n     fake runners, agent_run_config_test.go remnants).\n\n  7. Migrate cli/internal/server (server.go RunOptions\n     construction + workers_stop_propagation_test.go mock).\n\n  8. THEN run this cleanup bead as the no-op final-deletion\n     bead it was designed to be: delete RunOptions /\n     CompareOptions / QuorumOptions structs from types.go,\n     delete the legacy Run method on Runner, rename\n     RunWithConfig → Run. At that point the AC's `go build`\n     and `go test` will pass on the existing tree.\n\nAfter step 7, the tree should grep-clean for the three struct\nnames except in cli/tools/lint/evidencelint/testdata/src/agent/\n(self-contained stub package, leave alone) and in analyzer.go's\npackage doc comment + runOptionsTypeNames list (which can be\nupdated cosmetically in step 8 alongside the deletion).\n\n================================================================\nVerification\n================================================================\n\n  cd cli \u0026\u0026 go build ./...   # green at base rev (b199cf3c)\n  rg 'type (RunOptions|CompareOptions|QuorumOptions) ' cli/\n    # 3 matches in cli/internal/agent/types.go, all current\n\nNo code changes were made in this worktree.\nrationale: Bead ddx-ade5ebb3 — agent: retire RunOptions + CompareOptions + QuorumOptions + legacy Run\n\nSTATUS: not attempted; scope-vs-budget mismatch between bead description\nand actual codebase state. Request re-scoping into sub-beads (see below)\nbefore another execution attempt.\n\n================================================================\nWhat is done\n================================================================\n\nStage 2 dispatch-site migrations (beads c0ff5d3e, 4c31e465, 21f9e321,\nee2cd434) landed RunWithConfig / RunCompareWithConfigViaService /\nRunQuorumWithConfigViaService alongside the legacy Run /\nRunCompareViaService / RunQuorumViaService entry points. The CLI\ndispatch sites (`ddx agent run`, `ddx agent compare`, `ddx agent\nquorum`) now construct rcfg + AgentRunRuntime / CompareRuntime /\nQuorumRuntime and dispatch through the new `*WithConfig*` paths.\nA subset of agent test files were migrated by bead ee2cd434.\n\nStage 1 cleanup (bead edbfa046) removed ExecuteBeadLoopOptions and\nthe legacy `ExecuteBeadWorker.Run(opts)` method, renamed\n`RunWithConfig` → `Run`. That cleanup touched 18 files and ~123\nnet-changed lines.\n\n================================================================\nWhy this bead cannot land in one pass as currently scoped\n================================================================\n\nThe bead description in-scopes only two files:\n\n  - cli/internal/agent/types.go (delete RunOptions / CompareOptions /\n    QuorumOptions structs)\n  - cli/internal/agent/runner.go (delete legacy Run, rename\n    RunWithConfig → Run)\n\nThe bead acceptance criteria, however, require `cd cli \u0026\u0026 go build\n./...` and `cd cli \u0026\u0026 go test ./...` to pass. After the structs and\nthe legacy method are deleted, the tree does not compile. A grep\nover the post-Stage-2 tree shows the deleted symbols are still\nreferenced in:\n\n  RunOptions: 137 occurrences across 42 files\n  CompareOptions: ~15 occurrences across compare_adapter.go,\n    models.go, agent_compare_config_test.go, plus 4 testdata files\n  QuorumOptions: ~5 occurrences across compare_adapter.go,\n    agent_quorum_config_test.go, plus testdata\n\nStage 2's dispatch-site migrations only retired *construction* of\nthese structs at the four CLI entry points and a portion of\nagent-package tests. The structs are still load-bearing for:\n\n  - Internal helpers in cli/internal/agent: BuildArgs, resolveHarness,\n    resolvePrompt, resolveModel, resolveTimeout, resolveWallClock,\n    runVirtualFn, RunAgent, runScriptFn, runClaudeWithFallbackFn,\n    SessionIndexEntryFromResult, BenchmarkArmsToCompare, and the\n    entire compare_adapter.go internal pipeline (CompareOptions /\n    QuorumOptions are used as the working struct shape inside\n    RunCompare / RunQuorum, not just at the top of the public API).\n  - The `runner.Run(RunOptions{...})` self-call inside\n    runner.go::TestProviderConnectivity (line 976).\n  - The `r.Run(RunOptions{...})` call inside grade.go::169.\n  - The `agent.RunViaService` exported function and its internal\n    callers (cli/cmd/agent_cmd.go:1462, cli/internal/server/server.go\n    at lines 1913 and 3786, cli/internal/exec/store.go:201).\n  - The `Run(opts agent.RunOptions)` interface contract on\n    cli/internal/exec/types.go:11 plus its three implementers\n    (cmd/exec.go, internal/exec/store.go, exec_acceptance_test.go,\n    workers_stop_propagation_test.go).\n  - cli/cmd/agent_execute_bead_test.go's two test runners\n    (fakeAgentRunner, modelPassthroughCapture).\n  - cli/internal/agent/prompt_ingress_oversize_test.go's\n    resolvePrompt / defaultResolvePromptForCompare /\n    RunViaServiceWith calls (the bead 21 commit explicitly noted\n    these were left out of scope).\n  - cli/tools/lint/evidencelint/* — the testdata uses a *separate*\n    stub `agent.RunOptions` in testdata/src/agent/agent.go that is\n    independent of the production type, so testdata compiles fine,\n    but analyzer.go's package-level doc comments and\n    runOptionsTypeNames list reference the production type's name.\n\n================================================================\nEstimated true cost\n================================================================\n\nStage 1's analogous cleanup (ExecuteBeadLoopOptions) was 18 files /\n~123 net lines. Stage 2's RunOptions/CompareOptions/QuorumOptions\ncleanup is ~3-4× larger by raw reference count, and ~2× larger\nstructurally because (a) RunOptions is used as the central internal\ndata carrier inside compare_adapter.go's full Run / RunCompare /\nRunQuorum pipelines (not just at the dispatch entry), and (b)\nexternal callers in three different packages (cmd/, internal/exec/,\ninternal/server/) each need their own rcfg + AgentRunRuntime\nconstruction wired in, which means the `agent.RunViaService` /\n`agent.RunCompareViaService` / `agent.RunQuorumViaService`\nback-compat shims must stay or be replaced.\n\nConservative estimate: ~40 files / ~1500–2500 net lines / 3-5×\nthe per-bead size budget defined in SD-024 §\"Per-bead size budget\"\n(target \u003c500 LoC, \u003c20 minutes, \u003c$10 spend).\n\nA no-change here is preferable to a half-finished commit that\nleaves the tree red or smuggles a partial migration past review.\n\n================================================================\nWhat a follow-up attempt would need\n================================================================\n\nRecommend splitting into a fresh stage-2-cleanup sub-stage with\nbeads sized to the SD-024 budget. Suggested decomposition:\n\n  1. Migrate cli/internal/agent/compare_adapter.go internals to\n     drive RunCompare / RunQuorum through AgentRunRuntime + rcfg\n     end-to-end; retire CompareOptions and QuorumOptions struct\n     usage *inside* the agent package.\n\n  2. Migrate cli/internal/agent helpers (BuildArgs, resolveHarness,\n     resolvePrompt, resolveModel, resolveTimeout, resolveWallClock,\n     runVirtual, RunAgent, runScript, runClaudeWithFallback,\n     SessionIndexEntryFromResult, BenchmarkArmsToCompare) so they\n     take rcfg + AgentRunRuntime (or per-field args) instead of\n     RunOptions. Includes runner.go::TestProviderConnectivity and\n     grade.go's `r.Run` self-call.\n\n  3. Migrate cli/internal/agent/service_run.go (RunViaService et\n     al.) to take rcfg + AgentRunRuntime and update all three\n     callers: cmd/agent_cmd.go:1462, server.go:1913+3786,\n     internal/exec/store.go:201.\n\n  4. Migrate cli/internal/exec (types.go interface +\n     store.go construction + store_test.go mock) and\n     cmd/exec.go's serviceExecAgentRunner +\n     exec_acceptance_test.go mock onto the rcfg-based interface.\n\n  5. Migrate the remaining agent-package test sites that bead\n     ee2cd434 left out of scope (prompt_ingress_oversize_test.go,\n     execute_bead_runtime_test.go, models_test.go,\n     session_index_test.go, virtual_test.go, claude_stream_test.go,\n     and the BuildArgs tests in agent_test.go that still construct\n     RunOptions literals as inputs to the unit under test).\n\n  6. Migrate cli/cmd test sites (agent_execute_bead_test.go's two\n     fake runners, agent_run_config_test.go remnants).\n\n  7. Migrate cli/internal/server (server.go RunOptions\n     construction + workers_stop_propagation_test.go mock).\n\n  8. THEN run this cleanup bead as the no-op final-deletion\n     bead it was designed to be: delete RunOptions /\n     CompareOptions / QuorumOptions structs from types.go,\n     delete the legacy Run method on Runner, rename\n     RunWithConfig → Run. At that point the AC's `go build`\n     and `go test` will pass on the existing tree.\n\nAfter step 7, the tree should grep-clean for the three struct\nnames except in cli/tools/lint/evidencelint/testdata/src/agent/\n(self-contained stub package, leave alone) and in analyzer.go's\npackage doc comment + runOptionsTypeNames list (which can be\nupdated cosmetically in step 8 alongside the deletion).\n\n================================================================\nVerification\n================================================================\n\n  cd cli \u0026\u0026 go build ./...   # green at base rev (b199cf3c)\n  rg 'type (RunOptions|CompareOptions|QuorumOptions) ' cli/\n    # 3 matches in cli/internal/agent/types.go, all current\n\nNo code changes were made in this worktree.\nresult_rev=b199cf3cb11f50cfe842c5ad7faa9ac9db183ef7\nbase_rev=b199cf3cb11f50cfe842c5ad7faa9ac9db183ef7",
+          "created_at": "2026-04-25T23:00:49.314257321Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "already_satisfied"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-26T18:20:15.132295251Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260426T181711-4cdff442\",\"harness\":\"claude\",\"input_tokens\":30,\"output_tokens\":9369,\"total_tokens\":9399,\"cost_usd\":1.738788,\"duration_ms\":182464,\"exit_code\":0}",
+          "created_at": "2026-04-26T18:20:15.320825955Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=9399 cost_usd=1.7388"
+        },
+        {
+          "actor": "ddx",
+          "body": "Bead: ddx-ade5ebb3 — agent: retire RunOptions + CompareOptions + QuorumOptions + legacy Run\n\nNO CHANGES — bead preconditions not satisfied; scope exceeds single-window budget.\n\n## What is done\n\n- Read the bead, SD-024, TD-024, runtimelint analyzer, and current state of\n  cli/internal/agent/types.go and runner.go.\n- Surveyed actual caller surface in the worktree (HEAD = 562f293c).\n\n## Why no commit was made\n\nThe bead description states explicit dependencies:\n\n  \"Depends on bead 16 (Stage 1 cleanup) AND beads 17-21 (every Stage 2\n   caller migrated).\"\n\nThe bead's own reopen note (2026-04-26) confirms those dependencies are\nNOT satisfied:\n\n  \"type RunOptions still exists at cli/internal/agent/types.go:36;\n   QuorumOptions at :81; CompareOptions at :171. runtimelint reports\n   ~80 violations across the tree using these types. Closure was fake\n   (already_satisfied or premature). The retirement was NOT actually\n   performed — only ExecuteBeadLoopOptions (B16) and ExecuteBeadOptions\n   (B28) shipped real type-deletion commits.\"\n\nSo B17-B21 (the per-caller Stage 2 migration beads) were never done.\nToday's measured surface, from this worktree:\n\n  rg \"RunOptions|CompareOptions|QuorumOptions\" cli/  →  188 references\n                                                        across 48 files\n  rg \"RunOptions\\{|CompareOptions\\{|QuorumOptions\\{\" cli/\n                                                     →  ~50 composite literals\n                                                        (~13 production,\n                                                         ~31 in tests,\n                                                         ~5 in lint testdata,\n                                                         rest in agent\n                                                         package internals)\n\nProduction constructor sites:\n  cli/cmd/agent_cmd.go:1462\n  cli/internal/agent/compare_adapter.go:61, 261, 414, 685\n  cli/internal/agent/execute_bead.go:647\n  cli/internal/agent/execute_bead_review.go:531\n  cli/internal/agent/grade.go:169\n  cli/internal/agent/service_run.go:82\n  cli/internal/exec/store.go:201\n  cli/internal/server/server.go:1913, 3786\n\nInternal-implementation surface (beyond the two files the bead names as\n\"in-scope\"): every dispatch helper in the agent package threads\nRunOptions through its signature —\n\n  agent_runner.go, agent_runner_service.go, claude_stream.go,\n  compare_adapter.go, execute_bead.go, routing_metrics.go, runner.go,\n  script.go, service_run.go, session_index.go, virtual.go\n\nplus BuildArgs, RunAgent, runVirtualFn, runScriptFn,\nrunClaudeWithFallbackFn, finalizeClaudeResult, processResult, and\nTestProviderConnectivity all take RunOptions or fields from it.\nCompareOptions and QuorumOptions both EMBED RunOptions, so deleting\nRunOptions also forces a redesign of compare_adapter.go's option flow.\n\n## What is blocking\n\nThe bead is framed as a one-shot type-deletion (\"delete the structs,\nrename RunWithConfig → Run\"), but the AC requires `go build` and\n`go test` to pass tree-wide. To pass those gates the bead has to\nalso:\n\n1. Replace every internal RunOptions parameter (agent package\n   helpers) with an AgentRunRuntime + ResolvedConfig pair, OR keep\n   them on a non-RunOptions internal type.\n2. Migrate all ~13 production caller sites to RunWithConfig\n   (renamed Run), including building a config.ResolvedConfig at each\n   site (CLI, server, exec store, grade, compare_adapter,\n   execute_bead, execute_bead_review, service_run).\n3. Migrate or delete the CompareOptions/QuorumOptions surface\n   (compare_adapter.go is heavily built around it; this is essentially\n   a rewrite of the comparison/quorum dispatcher, not a rename).\n4. Migrate ~31 test composite literals across agent_test.go,\n   claude_stream_test.go, prompt_ingress_oversize_test.go,\n   session_index_test.go, models_test.go, virtual_test.go,\n   plus runtime tests.\n5. Update or remove evidencelint analyzer entries for RunOptions\n   and the runtimelint testdata (which deliberately uses the legacy\n   types as fixtures).\n\nThis is the SD-024 §\"Migrate one production caller per bead\" / §\"Migrate\ntest files in groups\" work — the ~5 beads (B17–B21) the bead description\npoints at as completed dependencies. Doing all of that here in a single\nexecute-bead window violates the SD's own per-bead size budget\n(\"\u003c500 LoC of net change, \u003c20 minutes of agent time, under $10 of model\nspend\") by an order of magnitude. The realistic LoC delta is in the\n1500–2500 range across 30+ files.\n\n## What a follow-up attempt would need\n\nReopen B17–B21 (or their successor beads) with concrete per-file scopes\nbefore re-queuing this bead. Suggested split:\n\n- B17: migrate cli/cmd/agent_cmd.go RunViaService caller +\n       cli/internal/server/server.go (2 sites). Add a\n       config.ResolvedConfig handoff at each site.\n- B18: migrate cli/internal/agent/execute_bead.go +\n       execute_bead_review.go + service_run.go (3 sites; agent-package\n       internal callers).\n- B19: migrate cli/internal/agent/compare_adapter.go and\n       cli/internal/agent/grade.go — biggest surface; redesigns\n       CompareOptions/QuorumOptions on top of AgentRunRuntime\n       (likely promotes Harnesses/Strategy/Threshold/ArmModels/\n       ArmLabels/Sandbox/KeepSandbox/PostRun onto a new\n       CompareRuntime/QuorumRuntime).\n- B20: migrate cli/internal/exec/store.go + types.go.\n- B21: migrate the ~31 test composite literals; update\n       evidencelint analyzer + testdata.\n\nOnce those land green, this bead (B22) becomes the trivially-correct\nfinal cleanup — delete the three types, rename RunWithConfig → Run,\nupdate the runtimelint testdata that intentionally uses the legacy\nnames, and confirm runtimelint reports zero violations on the live\ntree.\n\nRecommendation: do NOT escalate this bead to a stronger model yet.\nThe blocker is the missing prior beads, not model capability. A\nstronger model attempting the full ~2000-LoC migration in one\nworker window would produce churn that is hard to review and likely\nbreaks one of the three production dispatch sites in a non-obvious\nway (the historical bug class SD-024 was created to prevent).\nrationale: Bead: ddx-ade5ebb3 — agent: retire RunOptions + CompareOptions + QuorumOptions + legacy Run\n\nNO CHANGES — bead preconditions not satisfied; scope exceeds single-window budget.\n\n## What is done\n\n- Read the bead, SD-024, TD-024, runtimelint analyzer, and current state of\n  cli/internal/agent/types.go and runner.go.\n- Surveyed actual caller surface in the worktree (HEAD = 562f293c).\n\n## Why no commit was made\n\nThe bead description states explicit dependencies:\n\n  \"Depends on bead 16 (Stage 1 cleanup) AND beads 17-21 (every Stage 2\n   caller migrated).\"\n\nThe bead's own reopen note (2026-04-26) confirms those dependencies are\nNOT satisfied:\n\n  \"type RunOptions still exists at cli/internal/agent/types.go:36;\n   QuorumOptions at :81; CompareOptions at :171. runtimelint reports\n   ~80 violations across the tree using these types. Closure was fake\n   (already_satisfied or premature). The retirement was NOT actually\n   performed — only ExecuteBeadLoopOptions (B16) and ExecuteBeadOptions\n   (B28) shipped real type-deletion commits.\"\n\nSo B17-B21 (the per-caller Stage 2 migration beads) were never done.\nToday's measured surface, from this worktree:\n\n  rg \"RunOptions|CompareOptions|QuorumOptions\" cli/  →  188 references\n                                                        across 48 files\n  rg \"RunOptions\\{|CompareOptions\\{|QuorumOptions\\{\" cli/\n                                                     →  ~50 composite literals\n                                                        (~13 production,\n                                                         ~31 in tests,\n                                                         ~5 in lint testdata,\n                                                         rest in agent\n                                                         package internals)\n\nProduction constructor sites:\n  cli/cmd/agent_cmd.go:1462\n  cli/internal/agent/compare_adapter.go:61, 261, 414, 685\n  cli/internal/agent/execute_bead.go:647\n  cli/internal/agent/execute_bead_review.go:531\n  cli/internal/agent/grade.go:169\n  cli/internal/agent/service_run.go:82\n  cli/internal/exec/store.go:201\n  cli/internal/server/server.go:1913, 3786\n\nInternal-implementation surface (beyond the two files the bead names as\n\"in-scope\"): every dispatch helper in the agent package threads\nRunOptions through its signature —\n\n  agent_runner.go, agent_runner_service.go, claude_stream.go,\n  compare_adapter.go, execute_bead.go, routing_metrics.go, runner.go,\n  script.go, service_run.go, session_index.go, virtual.go\n\nplus BuildArgs, RunAgent, runVirtualFn, runScriptFn,\nrunClaudeWithFallbackFn, finalizeClaudeResult, processResult, and\nTestProviderConnectivity all take RunOptions or fields from it.\nCompareOptions and QuorumOptions both EMBED RunOptions, so deleting\nRunOptions also forces a redesign of compare_adapter.go's option flow.\n\n## What is blocking\n\nThe bead is framed as a one-shot type-deletion (\"delete the structs,\nrename RunWithConfig → Run\"), but the AC requires `go build` and\n`go test` to pass tree-wide. To pass those gates the bead has to\nalso:\n\n1. Replace every internal RunOptions parameter (agent package\n   helpers) with an AgentRunRuntime + ResolvedConfig pair, OR keep\n   them on a non-RunOptions internal type.\n2. Migrate all ~13 production caller sites to RunWithConfig\n   (renamed Run), including building a config.ResolvedConfig at each\n   site (CLI, server, exec store, grade, compare_adapter,\n   execute_bead, execute_bead_review, service_run).\n3. Migrate or delete the CompareOptions/QuorumOptions surface\n   (compare_adapter.go is heavily built around it; this is essentially\n   a rewrite of the comparison/quorum dispatcher, not a rename).\n4. Migrate ~31 test composite literals across agent_test.go,\n   claude_stream_test.go, prompt_ingress_oversize_test.go,\n   session_index_test.go, models_test.go, virtual_test.go,\n   plus runtime tests.\n5. Update or remove evidencelint analyzer entries for RunOptions\n   and the runtimelint testdata (which deliberately uses the legacy\n   types as fixtures).\n\nThis is the SD-024 §\"Migrate one production caller per bead\" / §\"Migrate\ntest files in groups\" work — the ~5 beads (B17–B21) the bead description\npoints at as completed dependencies. Doing all of that here in a single\nexecute-bead window violates the SD's own per-bead size budget\n(\"\u003c500 LoC of net change, \u003c20 minutes of agent time, under $10 of model\nspend\") by an order of magnitude. The realistic LoC delta is in the\n1500–2500 range across 30+ files.\n\n## What a follow-up attempt would need\n\nReopen B17–B21 (or their successor beads) with concrete per-file scopes\nbefore re-queuing this bead. Suggested split:\n\n- B17: migrate cli/cmd/agent_cmd.go RunViaService caller +\n       cli/internal/server/server.go (2 sites). Add a\n       config.ResolvedConfig handoff at each site.\n- B18: migrate cli/internal/agent/execute_bead.go +\n       execute_bead_review.go + service_run.go (3 sites; agent-package\n       internal callers).\n- B19: migrate cli/internal/agent/compare_adapter.go and\n       cli/internal/agent/grade.go — biggest surface; redesigns\n       CompareOptions/QuorumOptions on top of AgentRunRuntime\n       (likely promotes Harnesses/Strategy/Threshold/ArmModels/\n       ArmLabels/Sandbox/KeepSandbox/PostRun onto a new\n       CompareRuntime/QuorumRuntime).\n- B20: migrate cli/internal/exec/store.go + types.go.\n- B21: migrate the ~31 test composite literals; update\n       evidencelint analyzer + testdata.\n\nOnce those land green, this bead (B22) becomes the trivially-correct\nfinal cleanup — delete the three types, rename RunWithConfig → Run,\nupdate the runtimelint testdata that intentionally uses the legacy\nnames, and confirm runtimelint reports zero violations on the live\ntree.\n\nRecommendation: do NOT escalate this bead to a stronger model yet.\nThe blocker is the missing prior beads, not model capability. A\nstronger model attempting the full ~2000-LoC migration in one\nworker window would produce churn that is hard to review and likely\nbreaks one of the three production dispatch sites in a non-obvious\nway (the historical bug class SD-024 was created to prevent).\nresult_rev=562f293c75a19b1deda23b83e7692e0a4836d75e\nbase_rev=562f293c75a19b1deda23b83e7692e0a4836d75e",
+          "created_at": "2026-04-26T18:20:15.843654204Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "already_satisfied"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-26T18:56:10.959558626Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260426T184511-edb52c8c\",\"harness\":\"claude\",\"input_tokens\":30,\"output_tokens\":42695,\"total_tokens\":42725,\"cost_usd\":2.8876382499999997,\"duration_ms\":658618,\"exit_code\":0}",
+          "created_at": "2026-04-26T18:56:11.059244075Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=42725 cost_usd=2.8876"
+        },
+        {
+          "actor": "ddx",
+          "body": "no_changes\nrationale: no_changes rationale — ddx-ade5ebb3 (Stage 2 retire RunOptions/CompareOptions/QuorumOptions + legacy Run)\n\nWHAT IS DONE\n- Surveyed full call graph for the three doomed types and the legacy Run method.\n- Confirmed bead notes: dependencies (beads 17–21) ARE all closed, but they migrated\n  only the public CLI command paths (`ddx agent run`, `ddx agent compare`,\n  `ddx agent quorum`) — not the internal agent-package plumbing nor the\n  external server/exec callers.\n\nWHAT IS BLOCKING\nThe bead's stated in-scope is two files (cli/internal/agent/types.go,\ncli/internal/agent/runner.go), but the AC requires `cd cli \u0026\u0026 go build ./...`\nand `cd cli \u0026\u0026 go test ./...` to pass after the three exported types and the\nlegacy Run method are removed. Reality of references that must move\ntogether (ripgrep counts):\n\nA. agent.RunOptions / CompareOptions / QuorumOptions — 88 references in\n   non-testdata Go files outside the lint testdata trees. Of these:\n\n   Internal to cli/internal/agent/ (cannot be deleted without parallel rewrite):\n   - virtual.go (runVirtualFn signature + builder)\n   - script.go (runScriptFn signature)\n   - claude_stream.go (4 helpers: resolveClaudeProgressLogDir,\n     runClaudeStreamingFn, runClaudeWithFallbackFn, finalizeClaudeResult)\n   - agent_runner.go (RunAgent), agent_runner_service.go (runAgentViaService)\n   - models.go (BenchmarkArmsToCompare → CompareOptions)\n   - grade.go (r.Run(RunOptions{...}))\n   - service_run.go (RunViaService, RunViaServiceWith, runFixtureHarnessViaRunner — public exported entry points)\n   - session_index.go (SessionIndexEntryFromResult — exported)\n   - routing_metrics.go (recordRoutingOutcome)\n   - compare_adapter.go (~25 references — RunFunc, RunCompareWith, RunCompareViaService, RunCompareWithAgent, defaultResolvePromptForCompare, runCompareArmWith, RunQuorumWith, RunQuorumViaService, RunQuorumWithAgent, RunBenchmarkWith, plus the existing RunCompareWithConfigViaService/RunQuorumWithConfig adapters that still build CompareOptions/QuorumOptions internally)\n   - execute_bead.go (AgentRunner interface signature + dispatchAgentRun + the runOpts builder around line 647)\n   - execute_bead_review.go (DefaultBeadReviewer.dispatchReviewRun + the AgentRunner field at line 425)\n   - runner.go itself (resolveHarness, resolvePrompt, resolveModel, resolveTimeout, resolveWallClock, BuildArgs, recordRoutingOutcome, TestProviderConnectivity, plus the legacy Run body and the Run alias for AgentRunner satisfaction)\n\n   External callers (cli/internal/server, cli/internal/exec, cli/cmd):\n   - cli/internal/server/server.go:1913 + :3786 (handleAgentRun + workers handler — both build agent.RunOptions{} for RunViaService)\n   - cli/internal/exec/types.go:11 (AgentRunner interface signature uses agent.RunOptions)\n   - cli/internal/exec/store.go:201 (builds agent.RunOptions for s.AgentRunner.Run)\n   - cli/cmd/exec.go:98 (serviceExecAgentRunner.Run wraps RunViaService)\n   - cli/cmd/agent_cmd.go:1462 (RunViaService call building RunOptions)\n\n   Test mocks that implement AgentRunner (must update their Run signature\n   in lockstep with the interface change):\n   - cli/internal/agent/execute_bead_runtime_test.go (capturingAgentRunner)\n   - cli/internal/agent/execute_bead_review_test.go (reviewRunnerStub)\n   - cli/internal/agent/execute_bead_review_evidence_test.go (countingRunner)\n   - cli/internal/agent/execute_bead_env_isolation_test.go (envIsoRunner)\n   - cli/internal/agent/execute_bead_e2e_test.go (gateTestAgentRunner, rationaleTestRunner)\n   - cli/internal/agent/execute_bead_artifacts_test.go (artifactTestAgentRunner)\n   - cli/internal/agent/executions_mirror_test.go (artifactTestAgentRunner usage)\n   - cli/internal/exec/store_test.go (mockAgentRunner)\n   - cli/cmd/exec_acceptance_test.go (mockAgentRunnerCmd)\n   - cli/cmd/agent_execute_bead_test.go (fakeAgentRunner, modelPassthroughCapture — also captures `last agent.RunOptions`)\n   - cli/internal/agent/agent_test.go (run-closure for RunCompareWith on line 584)\n   - Plus the BuildArgs(h, RunOptions{...}, \"\") test fixtures across agent_test.go (~16 sites) — these use the struct literal form, not interface satisfaction, so they migrate trivially via field-by-field replacement, but they still count.\n\nB. Legacy Runner.Run satisfies agent.AgentRunner interface today. Several\n   tests pass *Runner directly into ExecuteBeadRuntime{AgentRunner: runner}\n   (integration_helper_test.go:184, execute_bead_runtime_test.go:56,\n   execute_bead_routing_evidence_test.go:35/85/116,\n   execute_bead_e2e_test.go:358/403). Removing the legacy Run AND renaming\n   RunWithConfig → Run breaks this satisfaction unless either:\n     (a) The agent.AgentRunner interface itself is migrated to take\n         (ctx, ResolvedConfig, AgentRunRuntime) — cascades to every mock\n         listed in (A) above, AND to cli/internal/exec/types.go's\n         AgentRunner because cmd/agent_execute_bead.go:152 assigns one to\n         the other (Go's structural interface satisfaction).\n     (b) An adapter type is introduced (e.g., agent.RunnerAsAgentRunner)\n         and every test wiring rewritten — ~10 test sites.\n\nWHY THIS IS A NO_CHANGES, NOT A BAD COMMIT\nTwo prior attempts on this bead were \"fake-closed\" (per the bead notes) —\nlikely because they tried to satisfy the AC verifier via the loophole that\nthe field_removed regex `\\bX\\s+[\\*\\[\\]\\w.]+` does not match\n`type X = Y` aliases or `(opts X)` parameter lists. A type-alias\nrelocation (defining the structs in cli/internal/agentopts/ and aliasing\nthem back into the agent package) WOULD satisfy the regex check and the\nmanual rg over types.go alone, while keeping every external caller\nworking. But that is the same loophole that the bead author explicitly\ncalled out and reopened twice for. The bead notes say: \"Worker must\nactually delete the three type definitions and migrate every call site.\"\nDoing that requires the multi-file refactor described above.\n\nThat refactor is well-defined but is at least 30 files, ~150 hunks of\nedits, and depends on a coordinated public-API redesign of:\n  - agent.RunViaService / RunViaServiceWith (signature change)\n  - agent.AgentRunner interface (signature change, cascades to exec.AgentRunner)\n  - cli/internal/exec.AgentRunner interface (signature change)\n  - The Runner.Run / Runner.RunWithConfig swap with a new private\n    runInternal(opts) for internal callers (grade.go, service_run.go,\n    runner.go itself, execute_bead_review.go) so they don't build a fake\n    ResolvedConfig just to dispatch.\n\nThat is genuinely epic-sized — bigger than the four Stage-2 dependency\nbeads put together — and committing it half-done in this single worktree\nrisks leaving the build red on main.\n\nWHAT A FOLLOW-UP ATTEMPT WOULD NEED\nEither:\n\n(1) Re-decompose this bead into ≥5 smaller beads that can each land green:\n    - 24a \"agent: introduce private runInternal + new agent.AgentRunner signature with RunArgs adapter type\"\n    - 24b \"exec: migrate AgentRunner interface to (ctx, rcfg, runtime) signature; update store + cmd/exec wrapper\"\n    - 24c \"server: migrate handleAgentRun + workers handler to agent.RunViaServiceWithConfig\"\n    - 24d \"agent: migrate internal helpers (virtual, script, claude_stream, agent_runner, models, grade, service_run, session_index, routing_metrics, compare_adapter) off RunOptions/CompareOptions/QuorumOptions struct names\"\n    - 24e (this bead) \"agent: delete the three exported type definitions + delete legacy Runner.Run + rename RunWithConfig → Run\"\n    Each prerequisite bead has a clear gate-verifiable AC and the final\n    bead becomes a 2-file mechanical removal as originally scoped.\n\n(2) Or: explicitly authorize this bead to expand its scope to the full\n    30-file refactor and accept that it will land as one large PR. The\n    bead description's \"In-scope\" lines and the AgentRunner interface\n    discussion both need updating before a worker tries again.\n\nThe structural blocker is that the bead is written as a 2-file cleanup\nbut the AC + dependencies (which were not actually completed) require an\nepic-scale rewrite. A stronger model will not solve that mismatch — it\nneeds either re-decomposition or an explicit scope-expansion ack.\nresult_rev=396c6b49d942f74f0209e23b728480eba3d477ca\nbase_rev=396c6b49d942f74f0209e23b728480eba3d477ca\nretry_after=2026-04-27T00:56:14Z",
+          "created_at": "2026-04-26T18:56:15.079808163Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-28T02:53:36.816778953Z",
+      "execute-loop-last-detail": "no_changes",
+      "execute-loop-last-status": "no_changes",
+      "execute-loop-no-changes-count": 3,
+      "execute-loop-retry-after": "2026-04-27T00:56:14Z",
+      "session_id": "eb-bb03fed8",
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
+    "dir": ".ddx/executions/20260428T025336-72807bcd",
+    "prompt": ".ddx/executions/20260428T025336-72807bcd/prompt.md",
+    "manifest": ".ddx/executions/20260428T025336-72807bcd/manifest.json",
+    "result": ".ddx/executions/20260428T025336-72807bcd/result.json",
+    "checks": ".ddx/executions/20260428T025336-72807bcd/checks.json",
+    "usage": ".ddx/executions/20260428T025336-72807bcd/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-ade5ebb3-20260428T025336-72807bcd"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260428T025336-72807bcd/result.json b/.ddx/executions/20260428T025336-72807bcd/result.json
new file mode 100644
index 00000000..adc8c5ac
--- /dev/null
+++ b/.ddx/executions/20260428T025336-72807bcd/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-ade5ebb3",
+  "attempt_id": "20260428T025336-72807bcd",
+  "base_rev": "14b805290204a5f5f86aede411b633b8e0757dcb",
+  "result_rev": "2f6e13a445a992d51a24084f138097fc873d8e80",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-2dcf05f1",
+  "duration_ms": 1049886,
+  "tokens": 35413,
+  "cost_usd": 7.24405725,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260428T025336-72807bcd",
+  "prompt_file": ".ddx/executions/20260428T025336-72807bcd/prompt.md",
+  "manifest_file": ".ddx/executions/20260428T025336-72807bcd/manifest.json",
+  "result_file": ".ddx/executions/20260428T025336-72807bcd/result.json",
+  "usage_file": ".ddx/executions/20260428T025336-72807bcd/usage.json",
+  "started_at": "2026-04-28T02:53:37.315253071Z",
+  "finished_at": "2026-04-28T03:11:07.201730229Z"
+}
\ No newline at end of file
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

For each acceptance-criteria (AC) item, decide whether it is implemented correctly, then assign one overall verdict:

- APPROVE — every AC item is fully and correctly implemented.
- REQUEST_CHANGES — some AC items are partial or have fixable minor issues.
- BLOCK — at least one AC item is not implemented or incorrectly implemented; or the diff is insufficient to evaluate.

## Required output format (schema_version: 1)

Respond with EXACTLY one JSON object as your final response, fenced as a single ```json … ``` code block. Do not include any prose outside the fenced block. The JSON must match this schema:

```json
{
  "schema_version": 1,
  "verdict": "APPROVE",
  "summary": "≤300 char human-readable verdict justification",
  "findings": [
    { "severity": "info", "summary": "what is wrong or notable", "location": "path/to/file.go:42" }
  ]
}
```

Rules:
- "verdict" must be exactly one of "APPROVE", "REQUEST_CHANGES", "BLOCK".
- "severity" must be exactly one of "info", "warn", "block".
- Output the JSON object inside ONE fenced ```json … ``` block. No additional prose, no extra fences, no markdown headings.
- Do not echo this template back. Do not write the words APPROVE, REQUEST_CHANGES, or BLOCK anywhere except as the JSON value of the verdict field.
  </instructions>
</bead-review>

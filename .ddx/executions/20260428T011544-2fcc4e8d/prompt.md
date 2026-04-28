<bead-review>
  <bead id="ddx-ce7a796e" iter=1>
    <title>B22d-g: RunFunc + runCompareArmWith + public Run*With signatures off RunOptions</title>
    <description>
Step g/h of B22d decomposition.

Change RunFunc's signature, runCompareArmWith, defaultResolvePromptForCompare, the public RunCompareWith/RunQuorumWith/RunBenchmarkWith signatures, and BenchmarkArmsToCompare's return shape. Update prompt_ingress_oversize_test.go, models_test.go, claude_stream_test.go in the same commit.

After this step, cli/internal/agent/compare_adapter.go has no RunOptions/CompareOptions/QuorumOptions remaining (B22d AC items 2, 3 satisfied for compare_adapter).

In-scope:
- cli/internal/agent/compare_adapter.go (RunFunc, runCompareArmWith, defaultResolvePromptForCompare, RunCompareWith/RunQuorumWith/RunBenchmarkWith, BenchmarkArmsToCompare)
- cli/internal/agent/prompt_ingress_oversize_test.go
- cli/internal/agent/models_test.go
- cli/internal/agent/claude_stream_test.go

Out-of-scope:
- service_run.go public API deletions (step h)
    </description>
    <acceptance>
field RunOptions removed from cli/internal/agent/compare_adapter.go. field CompareOptions removed from cli/internal/agent/compare_adapter.go. field QuorumOptions removed from cli/internal/agent/compare_adapter.go. cd cli &amp;&amp; go build ./... passes. cd cli &amp;&amp; go test ./internal/agent/... passes.
    </acceptance>
    <labels>ddx, kind:implementation, area:agent, sd-024, stage:2</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260428T010018-c0d5a52b/manifest.json</file>
    <file>.ddx/executions/20260428T010018-c0d5a52b/result.json</file>
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

  <diff rev="1e32f08d09ccda1e739b93c509fd1dfe5057f852">
diff --git a/.ddx/executions/20260428T010018-c0d5a52b/manifest.json b/.ddx/executions/20260428T010018-c0d5a52b/manifest.json
new file mode 100644
index 00000000..21087722
--- /dev/null
+++ b/.ddx/executions/20260428T010018-c0d5a52b/manifest.json
@@ -0,0 +1,117 @@
+{
+  "attempt_id": "20260428T010018-c0d5a52b",
+  "bead_id": "ddx-ce7a796e",
+  "base_rev": "3ecf0701bdafae8ea7bbf43d683ad31930a8051d",
+  "created_at": "2026-04-28T01:00:18.59760715Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-ce7a796e",
+    "title": "B22d-g: RunFunc + runCompareArmWith + public Run*With signatures off RunOptions",
+    "description": "Step g/h of B22d decomposition.\n\nChange RunFunc's signature, runCompareArmWith, defaultResolvePromptForCompare, the public RunCompareWith/RunQuorumWith/RunBenchmarkWith signatures, and BenchmarkArmsToCompare's return shape. Update prompt_ingress_oversize_test.go, models_test.go, claude_stream_test.go in the same commit.\n\nAfter this step, cli/internal/agent/compare_adapter.go has no RunOptions/CompareOptions/QuorumOptions remaining (B22d AC items 2, 3 satisfied for compare_adapter).\n\nIn-scope:\n- cli/internal/agent/compare_adapter.go (RunFunc, runCompareArmWith, defaultResolvePromptForCompare, RunCompareWith/RunQuorumWith/RunBenchmarkWith, BenchmarkArmsToCompare)\n- cli/internal/agent/prompt_ingress_oversize_test.go\n- cli/internal/agent/models_test.go\n- cli/internal/agent/claude_stream_test.go\n\nOut-of-scope:\n- service_run.go public API deletions (step h)",
+    "acceptance": "field RunOptions removed from cli/internal/agent/compare_adapter.go. field CompareOptions removed from cli/internal/agent/compare_adapter.go. field QuorumOptions removed from cli/internal/agent/compare_adapter.go. cd cli \u0026\u0026 go build ./... passes. cd cli \u0026\u0026 go test ./internal/agent/... passes.",
+    "parent": "ddx-0f3ae192",
+    "labels": [
+      "ddx",
+      "kind:implementation",
+      "area:agent",
+      "sd-024",
+      "stage:2"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-28T01:00:17Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "685",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-27T00:16:43.222604616Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260427T000819-3d343cfa\",\"harness\":\"claude\",\"input_tokens\":48,\"output_tokens\":31356,\"total_tokens\":31404,\"cost_usd\":3.4293735,\"duration_ms\":502589,\"exit_code\":0}",
+          "created_at": "2026-04-27T00:16:43.307701906Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=31404 cost_usd=3.4294"
+        },
+        {
+          "actor": "ddx",
+          "body": "no_changes\nrationale: Bead ddx-ce7a796e (B22d-g) cannot be landed in isolation against base\n41d464b03c4e5d12b7a9244c3c59ee312e64e255. Same fundamental blocker as\nB22d-f (ddx-9f4b1a8c, no_changes 2026-04-27): lefthook `runtime-lint`\nhook (lefthook.yml) filters analyzer diagnostics to the set of staged\nfiles, and the in-scope test files contain pre-existing RunOptions /\nCompareOptions composite literals whose underlying APIs are explicitly\nnamed out-of-scope by this bead's parent decomposition (step h owns\nservice_run.go; runner.go and claude_stream.go aren't in this step's\nin-scope at all).\n\nWhat is done (verified locally, then reverted)\n- compare_adapter.go is straightforward to drain of RunOptions /\n  CompareOptions / QuorumOptions:\n    * RunFunc redeclared as `func(rcfg config.ResolvedConfig,\n      runtime AgentRunRuntime) (*Result, error)`.\n    * defaultResolvePromptForCompare(runtime AgentRunRuntime).\n    * runCompareArmWith(run RunFunc, rcfg, runtime CompareRuntime,\n      armIdx, harnessName, baseDir, prompt, worktreePath).\n    * RunCompareWith(run, rcfg, runtime CompareRuntime, resolvePrompt,\n      cleanupSandbox).\n    * RunQuorumWith(run, rcfg, runtime QuorumRuntime).\n    * RunBenchmarkWith(runCompare func(config.ResolvedConfig,\n      CompareRuntime) (*ComparisonRecord, error), suite).\n    * RunCompareViaService / RunCompareWithAgent / RunQuorumViaService /\n      RunQuorumWithAgent / RunBenchmarkViaService / RunBenchmarkWithAgent\n      rewritten to take (rcfg, runtime) instead of CompareOptions /\n      QuorumOptions and to delegate through the new RunFunc.\n    * RunCompareWithConfigViaService / RunQuorumWithConfigViaService\n      collapsed into a single forwarding hop (they no longer need to\n      assemble a CompareOptions/QuorumOptions intermediary).\n    * RunQuorumWithConfig retained, now calling RunQuorumWith with the\n      new shape.\n- models.go BenchmarkArmsToCompare changes return shape from\n  CompareOptions to CompareRuntime (and accepts an AgentRunRuntime\n  baseline rather than a RunOptions). compare_adapter.go's\n  RunBenchmarkWith uses that shape directly.\n- models_test.go updates the BenchmarkArmsToCompare call to pass\n  `AgentRunRuntime{Prompt: \"test\"}` and assert\n  CompareRuntime.{Harnesses, ArmModels, ArmLabels}. This is the only\n  RunOptions-related drift in models_test.go — the rest of that file\n  doesn't reference RunOptions.\n- After the in-package edits, `cd cli \u0026\u0026 go build ./...` passes and\n  `cd cli \u0026\u0026 go test ./internal/agent/...` is clean once the test\n  files in the next bullet are also updated.\n- agent_test.go (out of scope) needs one closure-shape update at\n  line 584 because RunFunc's signature changed; with that one-line\n  diff applied locally, TestQuorumRunsAllHarnesses passes.\n\nWhat is blocking (lint policy vs. bead scope)\n\n1. claude_stream_test.go (in-scope) contains RunOptions composite\n   literals at lines 206, 212, 218, 224, 291, and 324. These are all\n   inside test cases that call helpers declared in claude_stream.go —\n   resolveClaudeProgressLogDir(opts RunOptions, cfg Config) at\n   claude_stream.go:284 and runClaudeStreamingFn(r, ctx, harness,\n   \"claude\", \"\", opts RunOptions, \"hi\", \"\", 10*time.Second) at\n   claude_stream.go:304/477/507. claude_stream.go is NOT in this\n   bead's in-scope; B22d's per-step decomposition reserves it for a\n   separate step. With claude_stream.go's RunOptions surface\n   unchanged, the test cannot construct anything other than\n   RunOptions{...} to drive resolveClaudeProgressLogDir, and the\n   runtime-lint hook flags every one of those literals when the\n   file is staged.\n\n2. prompt_ingress_oversize_test.go (in-scope) has three RunOptions\n   composite literals:\n     * line 78  — r.resolvePrompt(RunOptions{PromptFile: fixture})\n                  drives Runner.resolvePrompt in runner.go:976.\n                  runner.go is out of scope.\n     * line 85  — defaultResolvePromptForCompare(RunOptions{...})\n                  drives the in-scope helper. CAN be updated.\n     * line 112 — RunViaServiceWith(ctx, svc, dir, RunOptions{...})\n                  drives service_run.go:104. service_run.go's public\n                  API is explicitly named out-of-scope (\"step h\").\n   Even after migrating line 85 and updating line 98's RunBenchmarkWith\n   closure to the new (rcfg, runtime CompareRuntime) shape, the file\n   still carries lines 78 and 112 as pre-existing RunOptions\n   composite literals. runtime-lint scopes diagnostics by file (not\n   by hunk), so the staged file fails the gate on lines that this\n   bead is not authorized to touch.\n\n3. agent_test.go (out of scope, but transitively required) — once\n   RunFunc's signature changes, agent_test.go:584's\n   `func(opts RunOptions) (*Result, error)` no longer matches\n   RunQuorumWithConfig's RunFunc parameter, so the package fails to\n   build. The minimal fix is one line, but agent_test.go has 18\n   other RunOptions composite literals (BuildArgs call sites at\n   lines 118, 130, 148, 157, 165, 188, 196, 209, 214, 677, 684,\n   718, 727, 856, 864, 971, plus 678, 684) that are pre-existing,\n   unrelated to this bead, and depend on BuildArgs (runner.go) being\n   migrated separately. Staging agent_test.go would surface every\n   one of those as a lint violation.\n\nThe runtime-lint hook is mandatory (per project policy: --no-verify is\nforbidden). Diagnostic filtering is whole-file, not hunk-aware, so\n\"only edit my own lines\" doesn't help when the file already contained\nviolations that this bead's named scope can't reach.\n\nWhat a follow-up attempt would need (any of the following)\n\nA. Re-decompose B22d-g to also include claude_stream.go (so\n   resolveClaudeProgressLogDir / runClaudeStreamingFn migrate off\n   RunOptions in the same commit, draining claude_stream_test.go),\n   AND service_run.go's RunViaServiceWith + runner.go's\n   resolvePrompt (so prompt_ingress_oversize_test.go drains), AND\n   agent_test.go's BuildArgs call sites (so the RunFunc signature\n   change compiles cleanly across the package). This is essentially\n   the previously-attempted atomic B22d scope (worker-3f7a's\n   recommendation, applied 2026-04-26 in commit 97561a6e); the\n   subsequent re-decomposition into steps a-h reintroduced this\n   exact tension.\n\nB. Land step h FIRST (and in fact most of the rest of the\n   per-file decomposition) so that claude_stream.go, runner.go,\n   service_run.go, and agent_test.go no longer contain pre-existing\n   RunOptions usages, then run step g — at that point the in-scope\n   test files have no out-of-scope violations and lint passes on the\n   staged set. The current bead-queue ordering puts step g before\n   step h, so this requires a tracker reordering.\n\nC. Change runtime-lint's filter from whole-file to hunk-scoped (so\n   pre-existing violations on lines this bead doesn't touch don't\n   block the commit). This is a lint-policy change in lefthook.yml;\n   not in this bead's scope and not authorized by the bead\n   description.\n\nD. Authorize the worker (model-side) to extend in-scope to cover\n   the transitively-required files (claude_stream.go,\n   runner.go.resolvePrompt + BuildArgs, service_run.go.RunViaServiceWith,\n   and agent_test.go), the same way B22d itself was authorized in\n   2026-04-26 (commit 97561a6e). The diff for that expansion is\n   already prepared and verified locally; only the bead-scope\n   constraint stands in the way.\n\nThe implementation is mechanical and verified locally; the blocker is\nproject-level lint policy versus per-step bead scope, not\nimplementation difficulty. No partial commit is justified, since\nlanding only the in-scope-clean half (compare_adapter.go +\nmodels.go + models_test.go) breaks the package build by leaving\nagent_test.go with an incompatible RunFunc closure, and updating\nthat closure trips the lint on agent_test.go's pre-existing 18+\nRunOptions literals.\nresult_rev=41d464b03c4e5d12b7a9244c3c59ee312e64e255\nbase_rev=41d464b03c4e5d12b7a9244c3c59ee312e64e255\nretry_after=2026-04-27T06:16:44Z",
+          "created_at": "2026-04-27T00:16:44.862270268Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        },
+        {
+          "actor": "ddx",
+          "body": "staging tracker: fatal: Unable to create '/Users/erik/Projects/ddx/.git/index.lock': File exists.\n\nAnother git process seems to be running in this repository, or the lock file may be stale: exit status 128",
+          "created_at": "2026-04-27T12:12:54.952990183Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-28T00:58:49.765026278Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-28T00:58:49.860943466Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-28T00:58:49.934256928Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-04-28T00:58:50.081667517Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-28T01:00:17.972404663Z",
+      "execute-loop-last-detail": "no_changes",
+      "execute-loop-last-status": "no_changes",
+      "execute-loop-no-changes-count": 1,
+      "execute-loop-retry-after": "2026-04-27T06:16:44Z",
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
+    "dir": ".ddx/executions/20260428T010018-c0d5a52b",
+    "prompt": ".ddx/executions/20260428T010018-c0d5a52b/prompt.md",
+    "manifest": ".ddx/executions/20260428T010018-c0d5a52b/manifest.json",
+    "result": ".ddx/executions/20260428T010018-c0d5a52b/result.json",
+    "checks": ".ddx/executions/20260428T010018-c0d5a52b/checks.json",
+    "usage": ".ddx/executions/20260428T010018-c0d5a52b/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-ce7a796e-20260428T010018-c0d5a52b"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260428T010018-c0d5a52b/result.json b/.ddx/executions/20260428T010018-c0d5a52b/result.json
new file mode 100644
index 00000000..7247383f
--- /dev/null
+++ b/.ddx/executions/20260428T010018-c0d5a52b/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-ce7a796e",
+  "attempt_id": "20260428T010018-c0d5a52b",
+  "base_rev": "3ecf0701bdafae8ea7bbf43d683ad31930a8051d",
+  "result_rev": "6845659548d0e81c6784d78b334c3501aac5c51e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-eccd939f",
+  "duration_ms": 917580,
+  "tokens": 55460,
+  "cost_usd": 7.987444750000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260428T010018-c0d5a52b",
+  "prompt_file": ".ddx/executions/20260428T010018-c0d5a52b/prompt.md",
+  "manifest_file": ".ddx/executions/20260428T010018-c0d5a52b/manifest.json",
+  "result_file": ".ddx/executions/20260428T010018-c0d5a52b/result.json",
+  "usage_file": ".ddx/executions/20260428T010018-c0d5a52b/usage.json",
+  "started_at": "2026-04-28T01:00:18.598056232Z",
+  "finished_at": "2026-04-28T01:15:36.17805879Z"
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

<bead-review>
  <bead id="ddx-b8926c2d" iter=1>
    <title>agent: final sweep — runtimelint passes against the tree</title>
    <description>
Stage 4 final sweep of SD-024. Runs runtimelint against the post-cleanup tree and asserts zero violations. Catches any drift introduced during the long migration.
    </description>
    <acceptance>
TestRuntimelintCleanTree passes. (Add this test in cli/tools/lint/runtimelint/integration_test.go: it runs `go run ./cmd/runtimelint ./...` against the project root and asserts exit code 0 with empty stdout. The gate's test_name verifier requires the worker's no_changes rationale to cite this test by name.) cd cli &amp;&amp; go test ./tools/lint/runtimelint/... passes including TestRuntimelintCleanTree.
    </acceptance>
    <notes>
REWRITTEN AC 2026-04-26: Previous AC was `go run ./tools/lint/runtimelint/cmd/runtimelint ./... exits 0` which is a shell invocation the gate parser can't extract as a claim. Reopened once and fake-closed twice via already_satisfied. Now rewritten as a test_name claim that requires adding TestRuntimelintCleanTree and citing it. Will only land after B22 (ddx-ade5ebb3) actually retires the legacy types.
    </notes>
    <labels>ddx, kind:cleanup, area:agent, sd-024, stage:4</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260428T031154-4880951a/manifest.json</file>
    <file>.ddx/executions/20260428T031154-4880951a/result.json</file>
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

  <diff rev="5c0c73f17adbde0131bf40e1d81e79adb36b2243">
diff --git a/.ddx/executions/20260428T031154-4880951a/manifest.json b/.ddx/executions/20260428T031154-4880951a/manifest.json
new file mode 100644
index 00000000..7c6bccdb
--- /dev/null
+++ b/.ddx/executions/20260428T031154-4880951a/manifest.json
@@ -0,0 +1,124 @@
+{
+  "attempt_id": "20260428T031154-4880951a",
+  "bead_id": "ddx-b8926c2d",
+  "base_rev": "5500b64c4b38a27591ec1fc2188a737df9f2f141",
+  "created_at": "2026-04-28T03:11:54.80296331Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b8926c2d",
+    "title": "agent: final sweep — runtimelint passes against the tree",
+    "description": "Stage 4 final sweep of SD-024. Runs runtimelint against the post-cleanup tree and asserts zero violations. Catches any drift introduced during the long migration.",
+    "acceptance": "TestRuntimelintCleanTree passes. (Add this test in cli/tools/lint/runtimelint/integration_test.go: it runs `go run ./cmd/runtimelint ./...` against the project root and asserts exit code 0 with empty stdout. The gate's test_name verifier requires the worker's no_changes rationale to cite this test by name.) cd cli \u0026\u0026 go test ./tools/lint/runtimelint/... passes including TestRuntimelintCleanTree.",
+    "parent": "ddx-0f3ae192",
+    "labels": [
+      "ddx",
+      "kind:cleanup",
+      "area:agent",
+      "sd-024",
+      "stage:4"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-28T03:11:54Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "685",
+      "closing_commit_sha": "8b04b12bba503fe1db29ada01dcbfdb1c9d2af4c",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-26T11:15:46.52866042Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260426T111359-73a03a29\",\"harness\":\"claude\",\"input_tokens\":26,\"output_tokens\":6097,\"total_tokens\":6123,\"cost_usd\":0.8238755000000001,\"duration_ms\":106346,\"exit_code\":0}",
+          "created_at": "2026-04-26T11:15:46.599267208Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=6123 cost_usd=0.8239"
+        },
+        {
+          "actor": "ddx",
+          "body": "Bead: ddx-b8926c2d — agent: final sweep — runtimelint passes against the tree\nStage: SD-024 Stage 4\nResult: NO CHANGES (prerequisite work not actually done)\n\n(a) What is done\n- Read SD-024 §\"Stage 4 — Lint enforcement\" and TD-024 §\"Lint rule (Stage 4)\"\n  to confirm the bead's expectation: run\n  `cd cli \u0026\u0026 go run ./tools/lint/runtimelint/cmd/runtimelint ./...`\n  and assert exit 0 against a post-cleanup tree.\n- Verified the analyzer is missing:\n    `find . -iname '*runtimelint*'` → no matches.\n    No file in cli/tools/lint/ named runtimelint; only evidencelint\n    exists at cli/tools/lint/evidencelint/.\n    `go run ./tools/lint/runtimelint/cmd/runtimelint ./...` →\n    \"stat .../tools/lint/runtimelint/cmd/runtimelint: directory not\n    found\" (exit 1).\n- Inspected the dependency bead ddx-880b7d8b (\"tools/lint: runtimelint\n  analyzer + Lefthook + CI wiring\"). It is marked status=closed in the\n  tracker (commit e9f2d53f), but its event body in the tracker shows\n  the previous attempt itself returned a NO CHANGES rationale citing\n  blocked prerequisite work. The analyzer was never actually written\n  in the tree; the bead was closed without its acceptance criteria\n  being satisfied.\n- Verified the upstream legacy-type retirements (Stages 1–3) are also\n  incomplete:\n    `grep -rn \"type RunOptions\\|type ExecuteBeadOptions\\|type\n     CompareOptions\\|type QuorumOptions\" cli/internal/agent/` →\n    cli/internal/agent/types.go:36:type RunOptions struct {\n    cli/internal/agent/types.go:81:type QuorumOptions struct {\n    cli/internal/agent/types.go:171:type CompareOptions struct {\n  TD-024's Stage 4 lint rule pattern (2)/(3) flags any composite\n  literal or function parameter of these names. They are still\n  declared and used throughout the tree, so even if runtimelint\n  existed it would emit a large number of legitimate violations.\n\n(b) What is blocking\n1. The runtimelint analyzer source does not exist. The acceptance\n   criterion (\"go run ./tools/lint/runtimelint/cmd/runtimelint ./...\n   exits 0\") cannot be evaluated against a non-existent program.\n2. The Stage 1/2/3 retirements (beads 16, 22, 28 per TD-024) have not\n   landed: ExecuteBeadLoopOptions, RunOptions, ExecuteBeadOptions,\n   CompareOptions, QuorumOptions are all still declared in\n   cli/internal/agent/types.go and constructed from many call sites.\n   The lint by design fails on those.\n\n(c) What a follow-up attempt would need\n- The dependency bead ddx-880b7d8b must actually produce the analyzer\n  at cli/tools/lint/runtimelint/ (analyzer.go + cmd/runtimelint/main.go\n  + analyzer_test.go + testdata) following the FEAT-022 evidencelint\n  pattern, with the closed forbidden-name list and exemptions\n  (NoReview, PollInterval, SessionLogDirOverride) per TD-024 §\"Lint\n  rule (Stage 4)\".\n- Stages 1, 2, 3 of SD-024 must actually retire ExecuteBeadLoopOptions,\n  RunOptions, ExecuteBeadOptions, CompareOptions, QuorumOptions and\n  migrate every constructor + signature site to *Runtime. The TD-024\n  §\"Stage ordering\" section enumerates the inventory.\n- Only then is this final-sweep bead meaningful. Until then, the\n  acceptance test would either crash on the missing program or report\n  many real violations from the un-retired legacy types — neither\n  outcome is \"exit 0\".\n\nThis bead intentionally adds nothing. The right next step is to\nre-open ddx-880b7d8b (and its own deps 16/22/28) and complete them\nbefore retrying ddx-b8926c2d.\nrationale: Bead: ddx-b8926c2d — agent: final sweep — runtimelint passes against the tree\nStage: SD-024 Stage 4\nResult: NO CHANGES (prerequisite work not actually done)\n\n(a) What is done\n- Read SD-024 §\"Stage 4 — Lint enforcement\" and TD-024 §\"Lint rule (Stage 4)\"\n  to confirm the bead's expectation: run\n  `cd cli \u0026\u0026 go run ./tools/lint/runtimelint/cmd/runtimelint ./...`\n  and assert exit 0 against a post-cleanup tree.\n- Verified the analyzer is missing:\n    `find . -iname '*runtimelint*'` → no matches.\n    No file in cli/tools/lint/ named runtimelint; only evidencelint\n    exists at cli/tools/lint/evidencelint/.\n    `go run ./tools/lint/runtimelint/cmd/runtimelint ./...` →\n    \"stat .../tools/lint/runtimelint/cmd/runtimelint: directory not\n    found\" (exit 1).\n- Inspected the dependency bead ddx-880b7d8b (\"tools/lint: runtimelint\n  analyzer + Lefthook + CI wiring\"). It is marked status=closed in the\n  tracker (commit e9f2d53f), but its event body in the tracker shows\n  the previous attempt itself returned a NO CHANGES rationale citing\n  blocked prerequisite work. The analyzer was never actually written\n  in the tree; the bead was closed without its acceptance criteria\n  being satisfied.\n- Verified the upstream legacy-type retirements (Stages 1–3) are also\n  incomplete:\n    `grep -rn \"type RunOptions\\|type ExecuteBeadOptions\\|type\n     CompareOptions\\|type QuorumOptions\" cli/internal/agent/` →\n    cli/internal/agent/types.go:36:type RunOptions struct {\n    cli/internal/agent/types.go:81:type QuorumOptions struct {\n    cli/internal/agent/types.go:171:type CompareOptions struct {\n  TD-024's Stage 4 lint rule pattern (2)/(3) flags any composite\n  literal or function parameter of these names. They are still\n  declared and used throughout the tree, so even if runtimelint\n  existed it would emit a large number of legitimate violations.\n\n(b) What is blocking\n1. The runtimelint analyzer source does not exist. The acceptance\n   criterion (\"go run ./tools/lint/runtimelint/cmd/runtimelint ./...\n   exits 0\") cannot be evaluated against a non-existent program.\n2. The Stage 1/2/3 retirements (beads 16, 22, 28 per TD-024) have not\n   landed: ExecuteBeadLoopOptions, RunOptions, ExecuteBeadOptions,\n   CompareOptions, QuorumOptions are all still declared in\n   cli/internal/agent/types.go and constructed from many call sites.\n   The lint by design fails on those.\n\n(c) What a follow-up attempt would need\n- The dependency bead ddx-880b7d8b must actually produce the analyzer\n  at cli/tools/lint/runtimelint/ (analyzer.go + cmd/runtimelint/main.go\n  + analyzer_test.go + testdata) following the FEAT-022 evidencelint\n  pattern, with the closed forbidden-name list and exemptions\n  (NoReview, PollInterval, SessionLogDirOverride) per TD-024 §\"Lint\n  rule (Stage 4)\".\n- Stages 1, 2, 3 of SD-024 must actually retire ExecuteBeadLoopOptions,\n  RunOptions, ExecuteBeadOptions, CompareOptions, QuorumOptions and\n  migrate every constructor + signature site to *Runtime. The TD-024\n  §\"Stage ordering\" section enumerates the inventory.\n- Only then is this final-sweep bead meaningful. Until then, the\n  acceptance test would either crash on the missing program or report\n  many real violations from the un-retired legacy types — neither\n  outcome is \"exit 0\".\n\nThis bead intentionally adds nothing. The right next step is to\nre-open ddx-880b7d8b (and its own deps 16/22/28) and complete them\nbefore retrying ddx-b8926c2d.\nresult_rev=127f60c4688ac6c53c76aa8efe0289a188c39378\nbase_rev=127f60c4688ac6c53c76aa8efe0289a188c39378",
+          "created_at": "2026-04-26T11:15:47.010333994Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "already_satisfied"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-26T18:14:29.3572421Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260426T181227-fd8d0bc2\",\"harness\":\"claude\",\"input_tokens\":24,\"output_tokens\":5908,\"total_tokens\":5932,\"cost_usd\":0.9332865000000001,\"duration_ms\":120837,\"exit_code\":0}",
+          "created_at": "2026-04-26T18:14:29.447814047Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=5932 cost_usd=0.9333"
+        },
+        {
+          "actor": "ddx",
+          "body": "Bead: ddx-b8926c2d — agent: final sweep — runtimelint passes against the tree\nStage: SD-024 Stage 4 (final sweep)\nResult: NO CHANGES (prerequisite Stage 1–3 retirements not actually landed)\n\n(a) What is done\n- Confirmed the runtimelint analyzer is now present and runnable\n  (B29 / ddx-880b7d8b shipped in commit 40ac58bb):\n    cli/tools/lint/runtimelint/analyzer.go\n    cli/tools/lint/runtimelint/cmd/runtimelint/\n    cli/tools/lint/runtimelint/analyzer_test.go\n    cli/tools/lint/runtimelint/testdata/\n- Ran the acceptance command:\n    cd cli \u0026\u0026 go run ./tools/lint/runtimelint/cmd/runtimelint ./...\n  → exit 3, 88 violations across 29 files.\n- Cross-checked the analyzer's closed forbidden list (legacyOptionsTypes\n  in tools/lint/runtimelint/analyzer.go) against current source:\n    grep '^type \\(RunOptions\\|QuorumOptions\\|CompareOptions\\) ' \\\n         cli/internal/agent/types.go\n  → still declared at types.go:36, types.go:81, types.go:171.\n  ExecuteBeadOptions and ExecuteBeadLoopOptions appear retired\n  (no declarations found), but their replacement Runtime structs\n  are not adopted at every call site — many *_test.go files and\n  several production files still construct or accept the legacy\n  options types.\n\n(b) What is blocking\n1. Stage 2 retirement bead ddx-ade5ebb3 (\"agent: retire RunOptions +\n   CompareOptions + QuorumOptions + legacy Run\") is marked closed in\n   the tracker, but the types are still present at\n   cli/internal/agent/types.go lines 36, 81, 171, and they are\n   constructed in 88 sites across 29 files including production code:\n     cli/cmd/agent_cmd.go\n     cli/internal/agent/runner.go            (8 sites)\n     cli/internal/agent/compare_adapter.go   (10 sites)\n     cli/internal/agent/claude_stream.go     (4 sites)\n     cli/internal/agent/service_run.go       (4 sites)\n     cli/internal/agent/execute_bead.go, execute_bead_review.go\n     cli/internal/agent/virtual.go, models.go, grade.go, script.go,\n       agent_runner.go, agent_runner_service.go, routing_metrics.go,\n       session_index.go\n     cli/internal/exec/store.go\n     cli/internal/server/server.go           (2 sites)\n   And many *_test.go files in cli/internal/agent/ also still use\n   the legacy composites/parameters.\n2. The acceptance criterion (\"exits 0\") is impossible to satisfy\n   without retiring those types and migrating every call site to the\n   *Runtime + ResolvedConfig pair — work that is the substance of\n   Stage 2 (ddx-ade5ebb3) and Stage 3 (ddx-a8eaef0a) retirement\n   beads, plus their many test-migration siblings. That work is\n   far outside the single-bead scope of this final sweep.\n\n(c) What a follow-up attempt would need\n- Re-open ddx-ade5ebb3 and the Stage 3 retirement/migration beads and\n  actually perform the migrations: delete the type declarations in\n  cli/internal/agent/types.go, port the 28 production call sites and\n  the test sites enumerated by `runtimelint ./...` to the\n  *Runtime + ResolvedConfig API. The analyzer's own output is the\n  authoritative punch list — all 88 violations must be addressed.\n- Once `runtimelint ./...` exits 0 against the tree, retry this\n  final-sweep bead. At that point the bead becomes a trivial\n  green-light commit asserting the post-cleanup invariant.\n\nThis bead intentionally adds nothing because the Stages 1–3 closure\nin the tracker does not match the state of the tree. A NO CHANGES\nreturn is the correct signal: the right next step is to reopen and\ngenuinely complete the upstream retirement beads, not to bulk-edit\n89 violations under the cover of a \"final sweep\".\nrationale: Bead: ddx-b8926c2d — agent: final sweep — runtimelint passes against the tree\nStage: SD-024 Stage 4 (final sweep)\nResult: NO CHANGES (prerequisite Stage 1–3 retirements not actually landed)\n\n(a) What is done\n- Confirmed the runtimelint analyzer is now present and runnable\n  (B29 / ddx-880b7d8b shipped in commit 40ac58bb):\n    cli/tools/lint/runtimelint/analyzer.go\n    cli/tools/lint/runtimelint/cmd/runtimelint/\n    cli/tools/lint/runtimelint/analyzer_test.go\n    cli/tools/lint/runtimelint/testdata/\n- Ran the acceptance command:\n    cd cli \u0026\u0026 go run ./tools/lint/runtimelint/cmd/runtimelint ./...\n  → exit 3, 88 violations across 29 files.\n- Cross-checked the analyzer's closed forbidden list (legacyOptionsTypes\n  in tools/lint/runtimelint/analyzer.go) against current source:\n    grep '^type \\(RunOptions\\|QuorumOptions\\|CompareOptions\\) ' \\\n         cli/internal/agent/types.go\n  → still declared at types.go:36, types.go:81, types.go:171.\n  ExecuteBeadOptions and ExecuteBeadLoopOptions appear retired\n  (no declarations found), but their replacement Runtime structs\n  are not adopted at every call site — many *_test.go files and\n  several production files still construct or accept the legacy\n  options types.\n\n(b) What is blocking\n1. Stage 2 retirement bead ddx-ade5ebb3 (\"agent: retire RunOptions +\n   CompareOptions + QuorumOptions + legacy Run\") is marked closed in\n   the tracker, but the types are still present at\n   cli/internal/agent/types.go lines 36, 81, 171, and they are\n   constructed in 88 sites across 29 files including production code:\n     cli/cmd/agent_cmd.go\n     cli/internal/agent/runner.go            (8 sites)\n     cli/internal/agent/compare_adapter.go   (10 sites)\n     cli/internal/agent/claude_stream.go     (4 sites)\n     cli/internal/agent/service_run.go       (4 sites)\n     cli/internal/agent/execute_bead.go, execute_bead_review.go\n     cli/internal/agent/virtual.go, models.go, grade.go, script.go,\n       agent_runner.go, agent_runner_service.go, routing_metrics.go,\n       session_index.go\n     cli/internal/exec/store.go\n     cli/internal/server/server.go           (2 sites)\n   And many *_test.go files in cli/internal/agent/ also still use\n   the legacy composites/parameters.\n2. The acceptance criterion (\"exits 0\") is impossible to satisfy\n   without retiring those types and migrating every call site to the\n   *Runtime + ResolvedConfig pair — work that is the substance of\n   Stage 2 (ddx-ade5ebb3) and Stage 3 (ddx-a8eaef0a) retirement\n   beads, plus their many test-migration siblings. That work is\n   far outside the single-bead scope of this final sweep.\n\n(c) What a follow-up attempt would need\n- Re-open ddx-ade5ebb3 and the Stage 3 retirement/migration beads and\n  actually perform the migrations: delete the type declarations in\n  cli/internal/agent/types.go, port the 28 production call sites and\n  the test sites enumerated by `runtimelint ./...` to the\n  *Runtime + ResolvedConfig API. The analyzer's own output is the\n  authoritative punch list — all 88 violations must be addressed.\n- Once `runtimelint ./...` exits 0 against the tree, retry this\n  final-sweep bead. At that point the bead becomes a trivial\n  green-light commit asserting the post-cleanup invariant.\n\nThis bead intentionally adds nothing because the Stages 1–3 closure\nin the tracker does not match the state of the tree. A NO CHANGES\nreturn is the correct signal: the right next step is to reopen and\ngenuinely complete the upstream retirement beads, not to bulk-edit\n89 violations under the cover of a \"final sweep\".\nresult_rev=e081ef6ed03a4abab39ebd8ce14f429e99c77b8f\nbase_rev=e081ef6ed03a4abab39ebd8ce14f429e99c77b8f",
+          "created_at": "2026-04-26T18:14:32.276251916Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "already_satisfied"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-26T18:24:14.949036662Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260426T182313-e23c9642\",\"harness\":\"claude\",\"input_tokens\":8,\"output_tokens\":2386,\"total_tokens\":2394,\"cost_usd\":0.31652899999999995,\"duration_ms\":60268,\"exit_code\":0}",
+          "created_at": "2026-04-26T18:24:15.041923849Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=2394 cost_usd=0.3165"
+        },
+        {
+          "actor": "ddx",
+          "body": "Bead: ddx-b8926c2d — agent: final sweep — runtimelint passes against the tree\nDecision: no_changes (blocked on prerequisite work)\n\n## What is done\n- Ran `go run ./tools/lint/runtimelint/cmd/runtimelint ./...` against the post-cleanup tree.\n- Confirmed the bead's AC (`exits 0`) is NOT met: the lint reports 88 violations and exits with status 3.\n\n## What is blocking\nThe bead's notes explicitly warn that B22 closed as already_satisfied without actually retiring the legacy options types. The current tree still uses `agent.RunOptions` extensively:\n\n- Composite-literal usages (RunOptions{...}) across production code:\n  cli/internal/agent/{compare_adapter,execute_bead,execute_bead_review,grade,runner,service_run}.go\n  cli/internal/exec/store.go\n  cli/internal/server/server.go\n  cli/cmd/agent_cmd.go\n- Function parameters typed as the legacy options across ~15 production files\n  (agent_runner.go, agent_runner_service.go, claude_stream.go, compare_adapter.go,\n   execute_bead.go, execute_bead_review.go, models.go, routing_metrics.go,\n   runner.go, script.go, service_run.go, session_index.go, virtual.go, ...)\n- Same pattern repeated across ~20 test files (agent_test.go, claude_stream_test.go,\n  models_test.go, prompt_ingress_oversize_test.go, session_index_test.go,\n  virtual_test.go, execute_bead_*_test.go, etc.)\n\nTotal: 88 violations. This is exactly the state the bead's \"REOPENED 2026-04-26\" note\npredicts: \"runtimelint actually run produces ~80 violations against the tree because\nB22 didn't actually retire RunOptions/CompareOptions/QuorumOptions.\"\n\n## Why this is not a final-sweep task\nThe \"final sweep\" bead is intentionally narrow: rerun the lint and assert green to\ncatch drift introduced after the migration is done. It is not the bead that performs\nthe migration. Resolving 88 violations across ~25 production files plus ~20 test files\nrequires:\n  - Threading *Runtime + ResolvedConfig through every harness, service, runner,\n    compare adapter, execute_bead, claude_stream, virtual, session_index, models,\n    routing_metrics, script, exec/store, server, and cmd/agent_cmd call site.\n  - Updating every test fixture that constructs a RunOptions{...}.\n  - Removing the legacy types themselves once all references are gone.\nThat is the body of B22, not a sweep. Doing it inside this bead would silently absorb\nthe entirety of a separate migration bead and mask the fact that B22 is incomplete —\nwhich is precisely the problem the reopened note flags.\n\n## What a follow-up attempt would need\n1. B22 (or a successor) must actually land: migrate all RunOptions/CompareOptions/\n   QuorumOptions call sites to *Runtime + config.ResolvedConfig and delete the\n   legacy types. The runtimelint output above is an exhaustive checklist of the\n   remaining call sites.\n2. Once B22 is genuinely green (lint exits 0 on the tree), this final-sweep bead\n   becomes a one-liner: re-run the same command and confirm no new drift appeared.\n\nRe-queueing this bead now would just produce another no_changes (or, worse, an\nattempt that absorbs B22's scope under the wrong bead id). Recommend keeping it\nblocked on B22 until B22 ships for real.\nrationale: Bead: ddx-b8926c2d — agent: final sweep — runtimelint passes against the tree\nDecision: no_changes (blocked on prerequisite work)\n\n## What is done\n- Ran `go run ./tools/lint/runtimelint/cmd/runtimelint ./...` against the post-cleanup tree.\n- Confirmed the bead's AC (`exits 0`) is NOT met: the lint reports 88 violations and exits with status 3.\n\n## What is blocking\nThe bead's notes explicitly warn that B22 closed as already_satisfied without actually retiring the legacy options types. The current tree still uses `agent.RunOptions` extensively:\n\n- Composite-literal usages (RunOptions{...}) across production code:\n  cli/internal/agent/{compare_adapter,execute_bead,execute_bead_review,grade,runner,service_run}.go\n  cli/internal/exec/store.go\n  cli/internal/server/server.go\n  cli/cmd/agent_cmd.go\n- Function parameters typed as the legacy options across ~15 production files\n  (agent_runner.go, agent_runner_service.go, claude_stream.go, compare_adapter.go,\n   execute_bead.go, execute_bead_review.go, models.go, routing_metrics.go,\n   runner.go, script.go, service_run.go, session_index.go, virtual.go, ...)\n- Same pattern repeated across ~20 test files (agent_test.go, claude_stream_test.go,\n  models_test.go, prompt_ingress_oversize_test.go, session_index_test.go,\n  virtual_test.go, execute_bead_*_test.go, etc.)\n\nTotal: 88 violations. This is exactly the state the bead's \"REOPENED 2026-04-26\" note\npredicts: \"runtimelint actually run produces ~80 violations against the tree because\nB22 didn't actually retire RunOptions/CompareOptions/QuorumOptions.\"\n\n## Why this is not a final-sweep task\nThe \"final sweep\" bead is intentionally narrow: rerun the lint and assert green to\ncatch drift introduced after the migration is done. It is not the bead that performs\nthe migration. Resolving 88 violations across ~25 production files plus ~20 test files\nrequires:\n  - Threading *Runtime + ResolvedConfig through every harness, service, runner,\n    compare adapter, execute_bead, claude_stream, virtual, session_index, models,\n    routing_metrics, script, exec/store, server, and cmd/agent_cmd call site.\n  - Updating every test fixture that constructs a RunOptions{...}.\n  - Removing the legacy types themselves once all references are gone.\nThat is the body of B22, not a sweep. Doing it inside this bead would silently absorb\nthe entirety of a separate migration bead and mask the fact that B22 is incomplete —\nwhich is precisely the problem the reopened note flags.\n\n## What a follow-up attempt would need\n1. B22 (or a successor) must actually land: migrate all RunOptions/CompareOptions/\n   QuorumOptions call sites to *Runtime + config.ResolvedConfig and delete the\n   legacy types. The runtimelint output above is an exhaustive checklist of the\n   remaining call sites.\n2. Once B22 is genuinely green (lint exits 0 on the tree), this final-sweep bead\n   becomes a one-liner: re-run the same command and confirm no new drift appeared.\n\nRe-queueing this bead now would just produce another no_changes (or, worse, an\nattempt that absorbs B22's scope under the wrong bead id). Recommend keeping it\nblocked on B22 until B22 ships for real.\nresult_rev=8b04b12bba503fe1db29ada01dcbfdb1c9d2af4c\nbase_rev=8b04b12bba503fe1db29ada01dcbfdb1c9d2af4c",
+          "created_at": "2026-04-26T18:24:17.671341459Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "already_satisfied"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-28T03:11:54.14798696Z",
+      "execute-loop-no-changes-count": 3,
+      "session_id": "eb-4933801d",
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
+    "dir": ".ddx/executions/20260428T031154-4880951a",
+    "prompt": ".ddx/executions/20260428T031154-4880951a/prompt.md",
+    "manifest": ".ddx/executions/20260428T031154-4880951a/manifest.json",
+    "result": ".ddx/executions/20260428T031154-4880951a/result.json",
+    "checks": ".ddx/executions/20260428T031154-4880951a/checks.json",
+    "usage": ".ddx/executions/20260428T031154-4880951a/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-b8926c2d-20260428T031154-4880951a"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260428T031154-4880951a/result.json b/.ddx/executions/20260428T031154-4880951a/result.json
new file mode 100644
index 00000000..b1c7cc29
--- /dev/null
+++ b/.ddx/executions/20260428T031154-4880951a/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-b8926c2d",
+  "attempt_id": "20260428T031154-4880951a",
+  "base_rev": "5500b64c4b38a27591ec1fc2188a737df9f2f141",
+  "result_rev": "e7896284ea0a49d9385f3ace57c3c3e74727160d",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-6ce44940",
+  "duration_ms": 99828,
+  "tokens": 4772,
+  "cost_usd": 0.594723,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260428T031154-4880951a",
+  "prompt_file": ".ddx/executions/20260428T031154-4880951a/prompt.md",
+  "manifest_file": ".ddx/executions/20260428T031154-4880951a/manifest.json",
+  "result_file": ".ddx/executions/20260428T031154-4880951a/result.json",
+  "usage_file": ".ddx/executions/20260428T031154-4880951a/usage.json",
+  "started_at": "2026-04-28T03:11:54.803398143Z",
+  "finished_at": "2026-04-28T03:13:34.632223949Z"
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

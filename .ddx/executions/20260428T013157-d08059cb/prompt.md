<bead-review>
  <bead id="ddx-b602ccce" iter=1>
    <title>B22d-h: delete RunViaService + RunViaServiceWith + runFixtureHarnessViaRunner</title>
    <description>
Step h/h of B22d decomposition (final).

Delete RunViaService, RunViaServiceWith, runFixtureHarnessViaRunner from cli/internal/agent/service_run.go and inline their bodies into the new RunWithConfigViaService internal helper. After steps b, c, d all migrated their callers, this deletion is mechanical.

In-scope:
- cli/internal/agent/service_run.go (delete legacy entry points)

Out-of-scope:
- Test file updates (covered by earlier steps)

Depends on B22d-b, B22d-c, B22d-d having migrated their callers.
    </description>
    <acceptance>
field RunOptions removed from cli/internal/agent/service_run.go. cd cli &amp;&amp; go build ./... passes. cd cli &amp;&amp; go test ./... passes.
    </acceptance>
    <labels>ddx, kind:implementation, area:agent, sd-024, stage:2</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260428T011647-59cb7b13/manifest.json</file>
    <file>.ddx/executions/20260428T011647-59cb7b13/result.json</file>
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

  <diff rev="749104a54030ff43a1a678b981b8bbc7cbd0f035">
diff --git a/.ddx/executions/20260428T011647-59cb7b13/manifest.json b/.ddx/executions/20260428T011647-59cb7b13/manifest.json
new file mode 100644
index 00000000..1657c53e
--- /dev/null
+++ b/.ddx/executions/20260428T011647-59cb7b13/manifest.json
@@ -0,0 +1,133 @@
+{
+  "attempt_id": "20260428T011647-59cb7b13",
+  "bead_id": "ddx-b602ccce",
+  "base_rev": "0d5c0b206b12e8dea69e144bf500964e5502af24",
+  "created_at": "2026-04-28T01:16:47.691407337Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b602ccce",
+    "title": "B22d-h: delete RunViaService + RunViaServiceWith + runFixtureHarnessViaRunner",
+    "description": "Step h/h of B22d decomposition (final).\n\nDelete RunViaService, RunViaServiceWith, runFixtureHarnessViaRunner from cli/internal/agent/service_run.go and inline their bodies into the new RunWithConfigViaService internal helper. After steps b, c, d all migrated their callers, this deletion is mechanical.\n\nIn-scope:\n- cli/internal/agent/service_run.go (delete legacy entry points)\n\nOut-of-scope:\n- Test file updates (covered by earlier steps)\n\nDepends on B22d-b, B22d-c, B22d-d having migrated their callers.",
+    "acceptance": "field RunOptions removed from cli/internal/agent/service_run.go. cd cli \u0026\u0026 go build ./... passes. cd cli \u0026\u0026 go test ./... passes.",
+    "parent": "ddx-0f3ae192",
+    "labels": [
+      "ddx",
+      "kind:implementation",
+      "area:agent",
+      "sd-024",
+      "stage:2"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-28T01:16:47Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "685",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-27T00:23:48.948849831Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260427T002150-d08557af\",\"harness\":\"claude\",\"input_tokens\":15,\"output_tokens\":6790,\"total_tokens\":6805,\"cost_usd\":0.7659057499999999,\"duration_ms\":117597,\"exit_code\":0}",
+          "created_at": "2026-04-27T00:23:49.048863398Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=6805 cost_usd=0.7659"
+        },
+        {
+          "actor": "ddx",
+          "body": "no_changes\nrationale: Bead: B22d-h — delete RunViaService + RunViaServiceWith + runFixtureHarnessViaRunner\n\nSTATUS: Cannot be completed in this pass. The bead's premise is false: it claims\n\"After steps b, c, d all migrated their callers, this deletion is mechanical.\"\nThat is incorrect — steps a, e, f, and g (which migrate the remaining callers)\nare NOT yet closed.\n\n(a) WHAT IS DONE\nNothing. No code changes were made. Verification only.\n\n(b) WHAT IS BLOCKING\n\n`ddx bead list | grep B22d` shows the current decomposition state:\n\n  ddx-636ca6d4  closed        B22d-b\n  ddx-06267d44  closed        B22d-c\n  ddx-c168f77f  closed        B22d-d\n  ddx-a4c8ed23  in_progress   B22d-a (grade.go r.Run -\u003e r.runInternal)\n  ddx-7c6664be  in_progress   B22d-e (BuildArgs signature off RunOptions)\n  ddx-9f4b1a8c  open          B22d-f (SessionIndexEntryFromResult off RunOptions)\n  ddx-ce7a796e  open          B22d-g (RunFunc + runCompareArmWith + public\n                                       Run*With signatures off RunOptions)\n  ddx-b602ccce  in_progress   B22d-h (this bead)\n\n`grep -rn \"RunViaService\\|RunViaServiceWith\\|runFixtureHarnessViaRunner\"\ncli/internal/agent cli/cmd` shows live, non-test callers that still depend on\nthe three functions this bead asks me to delete:\n\n  - cli/internal/agent/compare_adapter.go:214  RunCompareViaService -\u003e RunViaService\n  - cli/internal/agent/compare_adapter.go:223  RunCompareWithAgent  -\u003e RunViaServiceWith\n  - cli/internal/agent/compare_adapter.go:435  RunQuorumWithConfigViaService -\u003e RunViaService\n  - cli/internal/agent/compare_adapter.go:481  RunQuorumViaService  -\u003e RunViaService\n  - cli/internal/agent/compare_adapter.go:489  RunQuorumWithAgent   -\u003e RunViaServiceWith\n  - cli/internal/agent/dispatch.go:79          dispatchViaResolvedConfig -\u003e RunViaServiceWith\n  - cli/internal/agent/prompt_ingress_oversize_test.go:112  RunViaServiceWith\n  - cli/internal/agent/session_index_test.go:283            RunViaServiceWith\n\nThe compare_adapter.go callers are owned by step B22d-g (\"RunFunc +\nrunCompareArmWith + public Run*With signatures off RunOptions\"). The two test\nfiles are part of the broader RunOptions retirement (steps e/f/g). Step c's\nown commit message (96b243d1) confirms this scope split: \"step g/h covers\nRunOptions retirement\", which means g must run before h is mechanical.\n\nThe dispatch.go call site at line 79 (introduced in B22d-d) was deliberately\nleft calling RunViaServiceWith because RunWithConfigViaService delegates back\nthrough RunViaService — there is no in-package internal helper today that\ntakes (svc, projectRoot, opts). Inlining the three functions into\nRunWithConfigViaService alone (a public ResolvedConfig+runtime function) does\nnot give dispatch.go a substitute either, because dispatch.go already has\noverride-resolved opts and would have to build a synthetic ResolvedConfig.\n\nIf I tried to satisfy this bead in isolation today I would have to either:\n  1. Delete the three functions and watch `go build ./...` fail on at least\n     8 call sites — violating \"do not commit red code\".\n  2. Migrate compare_adapter.go (step g), dispatch.go, and the two test files\n     myself — silently absorbing scope from B22d-a, -e, -f, -g, which the\n     decomposition deliberately separated.\n  3. Convert the three deletions into shims (re-export under new names) —\n     which contradicts the AC (\"removed from cli/internal/agent/service_run.go\").\n\n(c) WHAT A FOLLOW-UP ATTEMPT NEEDS\n\nClose (or merge into this bead) B22d-a, B22d-e, B22d-f, and especially\nB22d-g first. Once B22d-g lands, compare_adapter.go's Run*ViaService and\nRun*WithAgent helpers will no longer call RunViaService/RunViaServiceWith,\nand the two test files will have been converted off raw RunOptions. At that\npoint the only remaining caller will be dispatch.go:79, and the deletion\ntruly is mechanical: rename RunViaServiceWith to a lowercase\nrunViaServiceWith (or fold it into dispatchViaResolvedConfig), then delete\nRunViaService and runFixtureHarnessViaRunner.\n\nRe-queue this bead after B22d-a/e/f/g close. A stronger model is not the\nissue — sequencing is.\nresult_rev=b64d4fc99fa6e950826d48eceaedb490b6734dff\nbase_rev=b64d4fc99fa6e950826d48eceaedb490b6734dff\nretry_after=2026-04-27T06:23:50Z",
+          "created_at": "2026-04-27T00:23:51.048683858Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-27T12:14:31.725424051Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260427T121255-6194020c\",\"harness\":\"claude\",\"input_tokens\":18,\"output_tokens\":5150,\"total_tokens\":5168,\"cost_usd\":0.7328777500000001,\"duration_ms\":95695,\"exit_code\":0}",
+          "created_at": "2026-04-27T12:14:31.80225501Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=5168 cost_usd=0.7329"
+        },
+        {
+          "actor": "ddx",
+          "body": "no_changes\nrationale: Bead: B22d-h — delete RunViaService + RunViaServiceWith + runFixtureHarnessViaRunner\n\nSTATUS: Cannot be completed in this pass. Same blocker as the previous attempt:\nthe bead's premise — \"After steps b, c, d all migrated their callers, this\ndeletion is mechanical\" — is still false. Steps B22d-f and B22d-g, which own\nthe remaining live callers and the RunOptions retirement, are NOT yet closed.\n\n(a) WHAT IS DONE\nNothing. No code changes. Verification only.\n\n(b) WHAT IS BLOCKING\n\nCurrent B22d decomposition state (`ddx bead list | grep B22d`):\n\n  ddx-a4c8ed23  closed         B22d-a\n  ddx-636ca6d4  closed         B22d-b\n  ddx-06267d44  closed         B22d-c\n  ddx-c168f77f  closed         B22d-d\n  ddx-7c6664be  closed         B22d-e\n  ddx-9f4b1a8c  in_progress    B22d-f (SessionIndexEntryFromResult off RunOptions)\n  ddx-ce7a796e  open           B22d-g (RunFunc + runCompareArmWith + public\n                                       Run*With signatures off RunOptions)\n  ddx-b602ccce  in_progress    B22d-h (this bead)\n\n`grep -rn \"RunViaService\\|RunViaServiceWith\\|runFixtureHarnessViaRunner\"\ncli/internal/agent cli/cmd` shows live, non-test callers that still depend on\nthe three functions this bead asks to delete:\n\n  - cli/internal/agent/compare_adapter.go:214  RunCompareViaService -\u003e RunViaService\n  - cli/internal/agent/compare_adapter.go:223  RunCompareWithAgent  -\u003e RunViaServiceWith\n  - cli/internal/agent/compare_adapter.go:435  RunQuorumWithConfigViaService -\u003e RunViaService\n  - cli/internal/agent/compare_adapter.go:481  RunQuorumViaService  -\u003e RunViaService\n  - cli/internal/agent/compare_adapter.go:489  RunQuorumWithAgent   -\u003e RunViaServiceWith\n  - cli/internal/agent/dispatch.go:79          dispatchViaResolvedConfig -\u003e RunViaServiceWith\n  - cli/internal/agent/prompt_ingress_oversize_test.go:112  RunViaServiceWith\n  - cli/internal/agent/session_index_test.go:283            RunViaServiceWith\n\nThe compare_adapter.go callers are owned by step B22d-g. The dispatch.go call\nsite is internal plumbing that has no in-package replacement until B22d-g\nintroduces a Run*With signature that does not take RunOptions, or until a new\ninternal helper is introduced as part of g/h. The two test files are part of\nthe broader RunOptions retirement.\n\nInlining the three functions into RunWithConfigViaService alone (a public\nResolvedConfig+runtime function) does not give compare_adapter.go or\ndispatch.go a substitute, because those callers do not have a ResolvedConfig\non hand — they hold override-resolved RunOptions assembled from per-arm\noverrides (compare/quorum) or from runtime+overrides (dispatch).\n\nThe AC also requires `cd cli \u0026\u0026 go test ./...` to pass, which is impossible\nwhile session_index_test.go and prompt_ingress_oversize_test.go still call\nRunViaServiceWith. Those test sites are owned by step B22d-f/g (test\nmigration is explicitly out of scope here).\n\n(c) WHAT A FOLLOW-UP ATTEMPT WOULD NEED\n\nEither:\n\n  1. Close B22d-f and B22d-g first (the proper sequencing the bead description\n     itself implies). Once compare_adapter.go and the two oversize/session\n     tests are migrated off RunOptions, this bead becomes a mechanical delete\n     of three functions plus a single inlining into RunWithConfigViaService.\n\n  2. Or, expand this bead's scope to include the work currently scoped to\n     B22d-f and B22d-g — i.e., introduce a service-level internal helper that\n     dispatch.go and compare_adapter.go can call without RunOptions, migrate\n     all five compare/quorum sites and dispatchViaResolvedConfig onto it,\n     migrate the two test sites, and only then delete the three functions.\n     This is the \"expand scope\" path called out in the SD-024 tracker note\n     (97561a6e: \"atomic cross-caller migration\"); it is well beyond the\n     \"mechanical deletion\" scope described in this bead's own description.\n\nThe previous attempt at this bead reached the same conclusion. Re-queueing\nwithout first closing B22d-f and B22d-g will reproduce the same blocker.\nresult_rev=05e6f4385c4304cd57b02c4f0d7b4e4ffc74fa82\nbase_rev=05e6f4385c4304cd57b02c4f0d7b4e4ffc74fa82\nretry_after=2026-04-27T18:14:35Z",
+          "created_at": "2026-04-27T12:14:35.298727933Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-28T00:58:52.380318393Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-28T00:58:52.491561814Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-28T00:58:52.564735526Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-04-28T00:58:52.709163744Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-28T01:16:47.048722869Z",
+      "execute-loop-last-detail": "no_changes",
+      "execute-loop-last-status": "no_changes",
+      "execute-loop-no-changes-count": 2,
+      "execute-loop-retry-after": "2026-04-27T18:14:35Z",
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
+    "dir": ".ddx/executions/20260428T011647-59cb7b13",
+    "prompt": ".ddx/executions/20260428T011647-59cb7b13/prompt.md",
+    "manifest": ".ddx/executions/20260428T011647-59cb7b13/manifest.json",
+    "result": ".ddx/executions/20260428T011647-59cb7b13/result.json",
+    "checks": ".ddx/executions/20260428T011647-59cb7b13/checks.json",
+    "usage": ".ddx/executions/20260428T011647-59cb7b13/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-b602ccce-20260428T011647-59cb7b13"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260428T011647-59cb7b13/result.json b/.ddx/executions/20260428T011647-59cb7b13/result.json
new file mode 100644
index 00000000..f3001bdc
--- /dev/null
+++ b/.ddx/executions/20260428T011647-59cb7b13/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-b602ccce",
+  "attempt_id": "20260428T011647-59cb7b13",
+  "base_rev": "0d5c0b206b12e8dea69e144bf500964e5502af24",
+  "result_rev": "35eeebdd484f3ab19c85fca4e4bb77119f476bd1",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-c21a028f",
+  "duration_ms": 907075,
+  "tokens": 42138,
+  "cost_usd": 3.6913365,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260428T011647-59cb7b13",
+  "prompt_file": ".ddx/executions/20260428T011647-59cb7b13/prompt.md",
+  "manifest_file": ".ddx/executions/20260428T011647-59cb7b13/manifest.json",
+  "result_file": ".ddx/executions/20260428T011647-59cb7b13/result.json",
+  "usage_file": ".ddx/executions/20260428T011647-59cb7b13/usage.json",
+  "started_at": "2026-04-28T01:16:47.691836045Z",
+  "finished_at": "2026-04-28T01:31:54.767439355Z"
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

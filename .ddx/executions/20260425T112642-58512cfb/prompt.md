<bead-review>
  <bead id="ddx-d33377a5" iter=1>
    <title>review+grading: metrics surface exposes prompt-size quantiles + failure-class breakdown (Stage F2)</title>
    <description>
Stage F2 of FEAT-022. Extend the review-outcomes metrics surface to
expose prompt-size quantiles and the four-class review-failure
breakdown over a configurable time window.

Implements FEAT-022 requirement §17. Satisfies the metrics-surface
criterion of US-221.

Depends on Stage F1 (ddx-76e83e7f) — needs telemetry fields present on
artifacts and events. Also depends on Stage G (ddx-70c1d2e2) for the
failure-class labels; if Stage G has not shipped, this bead can still
compute the quantiles and surface existing single-class counts, then
be extended when G lands.

In-scope files:
- cli/cmd/agent_metrics.go (extend the review-outcomes subcommand's
  JSON output and human-readable output with the new fields).
- cli/internal/agent/metrics_review.go (or equivalent package that
  aggregates the review-outcomes data from bead events and attempt
  bundles): compute prompt_size_p50, prompt_size_p95, prompt_size_p99
  over input_bytes values collected from review events in the window;
  compute failure_classes counts keyed on context_overflow,
  provider_empty, unparseable, transport.

Out-of-scope:
- A new CLI command (FEAT-022 §17 explicitly says "no new command").
- Any web UI consumer of the new fields.
- GraphQL exposure of the aggregated metrics — separate work if
  needed.

Rollback: revert the extensions; existing review-outcomes fields
remain unchanged.
    </description>
    <acceptance>
cd cli &amp;&amp; go test -run TestReviewOutcomesMetrics ./... passes: fixture bead history with varied prompt sizes and failure classes; assert ddx agent metrics review-outcomes --json produces output containing keys prompt_size_p50, prompt_size_p95, prompt_size_p99, failure_classes (object keyed on context_overflow, provider_empty, unparseable, transport with integer counts). ddx agent metrics review-outcomes --json | jq -e '.prompt_size_p95 | type == "number"' exits 0. ddx agent metrics review-outcomes --json | jq -e '.failure_classes | has("context_overflow") and has("provider_empty") and has("unparseable") and has("transport")' exits 0. Human-readable output (without --json) renders a table that includes the new columns. cd cli &amp;&amp; go test ./cmd/... passes; existing metrics tests unchanged.
    </acceptance>
    <labels>ddx, kind:implementation, area:agent, area:metrics, stage:F2</labels>
  </bead>

  <governing>
    <ref id="FEAT-022" path="docs/helix/01-frame/features/FEAT-022-prompt-evidence-assembly.md" title="Feature: Prompt Evidence Assembly and Size Invariants">
      <content>
---
ddx:
  id: FEAT-022
  depends_on:
    - helix.prd
    - FEAT-002
    - FEAT-005
    - FEAT-006
    - FEAT-014
---
# Feature: Prompt Evidence Assembly and Size Invariants

**ID:** FEAT-022
**Status:** Proposed
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx constructs prompts for LLM agents from multiple evidence sources: bead
metadata, governing documents, git diffs, comparison-arm outputs, user-supplied
prompt files, and inline request bodies. Today these assembly paths inline
content without size bounds, produce oversize prompts, and in the reviewer's
case trigger silent provider-side failures that cascade into redone primary
work and wasted tokens.

This feature establishes a single repo-wide invariant: **no prompt DDx
constructs or accepts may have unbounded size.** It specifies the shared
primitives every prompt-building call site must use, the two supported
assembly strategies, the failure semantics when inputs exceed caps, and the
telemetry needed to observe the system.

## Problem Statement

**Current situation:**

- The post-merge bead reviewer (`BuildReviewPrompt`) inlines full governing
  documents and full `git show` diffs without a cap. Large diffs push the
  prompt past the reviewer model's context window; some providers return a
  hard error, some return zero bytes. DDx surfaces these as `review-error`,
  fails to close the bead, and the next worker redoes the primary task —
  burning the same large token budget 3–4 times per bead before an operator
  intervenes.
- The comparison grader (`buildGradingPrompt`) has the same failure shape:
  per-arm outputs and diffs are inlined without a cap.
- A second review-prompt assembler lives in `cmd/bead_review.go` as
  duplicated code, with divergent escape semantics from the agent-side
  assembler.
- Five call sites (`runner.go`, `compare_adapter.go:181`,
  `compare_adapter.go:614`, `service_run.go:83`, `execute_bead.go:1291`)
  read `--prompt` files with `os.ReadFile` and no cap. A 100 MB prompt file
  reaches the provider unchecked.
- Three inline prompt-ingress paths (`/api/agent/run` body `Prompt`,
  `DDX_AGENT_PROMPT` env in exec definitions, docgraph prompt metadata)
  accept prompt bodies with no cap.
- Server text egress (MCP, REST, GraphQL) returns full document, diff,
  session, and persona payloads without caps. Any of these responses may
  later flow back into a prompt.
- No DDx call site has section-level telemetry on prompt assembly. When
  reviews fail, diagnosis today requires per-execution bundle archaeology.

**Desired outcome:** Every LLM prompt DDx constructs, accepts, or emits
through text-returning server endpoints is bounded by explicit caps.
Prompt-assembly call sites use shared primitives that enforce caps with
clear truncation markers. Oversize inputs fail fast with actionable errors
rather than silently corrupting provider calls. Operators see per-section
byte accounting on every review and grading attempt.

## Requirements

### Functional

**Shared evidence primitives**

1. **Single evidence-assembly package.** DDx provides one package exporting
   the primitives every prompt-building call site must use. The package
   covers, at minimum, these capabilities:
   - Byte-size caps for inlined files, diffs, governing documents, and
     total prompts, each with per-harness override resolution.
   - Clamped file read (returns content plus amount truncated).
   - Clamped text output (bounded length with a canonical truncation
     marker).
   - Section fitting under a total byte budget with priority-ordered
     inclusion. Default behavior is line-based and type-agnostic;
     markdown-aware heading extraction is an optional secondary mode.
   - Diff clamping and diff decomposition. The decomposer returns file
     inventory, per-file stat lines, and hunk headers so large diffs
     can degrade to stat + hunk-headers only.
   - Two strategy helpers, one for inline-with-cap assembly and one for
     ref-only metadata assembly (see §2).
   - A canonical truncation-marker string so downstream parsers
     recognize truncation uniformly across call sites.

   Specific Go identifiers, type shapes, and constant values are defined
   in the solution design document for this feature, not in this spec.

1a. **Cap defaults and configurability.** Byte-size caps have conservative
    defaults shipped in the binary. Project-level overrides are expressed
    in `.ddx/config.yaml` under a dedicated evidence-caps section;
    per-harness overrides resolve from the same configuration file.
    Defaults and override precedence are defined in the solution design
    for this feature. The invariant is that every cap is configurable at
    the project level without a rebuild; specific numeric defaults are
    not frozen by this spec.

2. **Two supported assembly strategies.** Each call site must adopt one:
   - **Ref-only metadata** — the primary bead prompt's pattern. Paths and
     titles only; agent reads file content via its own tools. Appropriate
     when the agent has filesystem access and the prompt does not need to
     guarantee the agent saw specific content.
   - **Inline-with-cap** — reviewer and grader's pattern. Content is
     inlined with bounded size, priority-ordered sections, and truncation
     markers. Appropriate when the call must guarantee the agent saw
     specific evidence.

3. **Repo-wide invariant (no unbounded prompts).** No code path may produce
   a value that ends up as an agent-invocation prompt, as a text response
   from a server handler that carries library, execution, diff, session,
   or persona content, or as an inline prompt body accepted through any
   ingress surface, without going through an evidence-package primitive.
   Exceptions require an explicit `evidence:allow-unbounded` source
   annotation with a justifying comment and are enforced by lint (see
   Non-Functional §Lint). Static/literal prompt fragments compiled into
   the binary are exempt; the invariant governs dynamic assembly from
   runtime inputs, not constant strings.

**Bounded prompt construction**

4. **Single review assembler.** Review prompts are assembled by exactly one
   function (`agent.BuildReviewPrompt`). Any duplicate review-prompt
   assembler in the CLI command layer is collapsed into this function.
   Divergent escape, instruction-template, or section-ordering logic is
   reconciled with byte-equivalence tests before the duplicate is removed.

5. **Review prompt contents.** The review prompt includes, in priority
   order:
   1. Bead metadata + acceptance criteria + bead notes (minimum evidence
      floor — always present regardless of budget).
   2. Changed-file inventory (names + stat lines — minimum evidence floor).
   3. Governing document content, bounded per §1. Governing docs that
      exceed their cap degrade to heading outline + first N lines.
   4. Diff, ranked by relevance (files named in AC text > files matching
      governing ref paths > others); per-file cap applies; large files
      degrade to stat + hunk headers.

6. **Grading prompt contents.** The grading prompt follows the same
   bounded-assembly pattern and additionally includes, bounded via
   `ClampOutput`, each comparison arm's `PostRunOut`, `PostRunOK`, and
   `ToolCalls`. These are first-class evidence fields on the comparison
   arm type and must not be silently dropped.

7. **Pre-dispatch short-circuit.** When a bounded assembly still exceeds
   `MaxPromptBytes` after all trimming, the call site must not dispatch
   the provider. Reviewer paths emit a distinct
   `review-error: context_overflow` outcome (see §13).

**Bounded prompt ingress**

8. **Hard-fail on oversize `--prompt` files.** Every call site that reads a
   user-supplied prompt file uses `evidence.ReadFileClamped(MaxPromptBytes)`
   and hard-fails on oversize with an actionable error message that
   includes observed size and cap. Silent truncation is forbidden — a
   user-supplied prompt truncated without notice produces incorrect
   results the user cannot diagnose.

9. **Hard-fail on oversize inline prompt bodies.** The same hard-fail
   applies to inline prompt bodies in `/api/agent/run` request bodies,
   `DDX_AGENT_PROMPT` environment values, and any other ingress path that
   accepts a raw prompt string. Size is checked against `MaxPromptBytes`
   before the prompt is handed to the agent.

**Bounded server text egress**

10. **Universal bounded text responses.** Every server text-returning
    handler — MCP tools, REST endpoints, GraphQL resolvers — that emits
    library document content, execution bundle content, diff content,
    session content, or persona content applies `ClampOutput` and marks
    the response as truncated in the response structure when trimming
    occurred. The MCP tool response structure, the REST JSON response,
    and the GraphQL resolver return type each carry an explicit
    `truncated: bool` and `original_bytes` field where applicable.

11. **No prompt-adjacent surface is unbounded.** The invariant applies
    regardless of whether a specific response is, today, actually consumed
    back into a prompt. A bounded-by-default policy prevents new
    consumers from reintroducing the overflow class.

**Error classes and review policy**

12. **Review error taxonomy.** The reviewer outcome event distinguishes
    four classes:
    - `review-error: context_overflow` — bounded assembly exceeded
      `MaxPromptBytes` post-trim; provider was not called.
    - `review-error: provider_empty` — provider returned zero bytes.
    - `review-error: unparseable` — provider returned text but no
      recognizable verdict line.
    - `review-error: transport` — network or provider error.

13. **Reviewer failure preserves the gate.** Reviewer failure must not
    close the bead. This invariant (enforced today by
    `execute_bead_review_failure_modes_test.go`) is preserved: bounded
    prompts change how the reviewer is called, not whether a failed
    review can auto-approve.

14. **Bounded review retry.** Reviewer invocations are capped at a
    configurable N attempts per `result_rev`. On the Nth failure, DDx
    emits a terminal `review-manual-required` event and stops re-
    executing primary. A new `result_rev` (fresh primary attempt after
    manual operator action) resets the counter. Iterations that do not
    produce a `result_rev` (e.g. `--no-merge` runs, execution-failed
    outcomes) do not consume the review-retry budget; the counter is
    scoped strictly to reviewer attempts against a committed result.
    This bounds the cost-bleed pattern observed in the filed bug (same
    primary task retried 3–4 times) without weakening the gate.

**Telemetry and observability**

15. **Section-level assembly telemetry.** Review and grading paths
    extend the execute-bead attempt bundle (FEAT-014 §19/§20, FEAT-005
    §Execute-Bead Evidence Bundle) with an evidence-assembly block
    containing:
    - per-section bytes (bead / governing / diff / arm-output)
    - list of selected vs omitted governing refs
    - list of selected vs omitted diff files
    - truncation reasons per section
    - total `input_bytes` and `output_bytes`
    - `elapsed_ms`, `harness`, `model`

    Runtime metrics fields already defined by FEAT-014 (`harness`,
    `model`, `session_id`, `elapsed_ms`, `input_tokens`,
    `output_tokens`, `total_tokens`, `cost_usd`, `base_rev`,
    `result_rev`) are not duplicated by FEAT-022; evidence-assembly
    telemetry is additive to that record, not a replacement.

16. **Compact event summary.** `review`, `review-error`, and
    `compare-result` event bodies carry a compact summary
    (`input_bytes`, `output_bytes`, `elapsed_ms`, `harness`, `model`).
    Full section detail stays in artifacts; event bodies respect the
    existing size cap on bead event bodies.

17. **Metrics surface.** DDx's existing review-outcomes metrics surface
    exposes prompt-size quantiles and the four-class failure breakdown
    defined in §12. The specific CLI and API entry points that carry
    this data are inherited from FEAT-006 and FEAT-014; FEAT-022 adds
    fields, not new commands.

### Non-Functional

- **Lint (structural sink coverage).** A vet/lint rule, scoped
  structurally not by dataflow, enforces the invariant in §3. Rule keys:
  - assignment to `RunOptions.Prompt`
  - construction of `mcpContent{Type:"text", ...}` literals
  - server handlers that write text response bodies from library,
    execution, diff, session, or persona data
  - readers of `opts.PromptFile` or inline prompt-body request fields
  Any flagged path must route through an `internal/evidence` primitive
  or carry an explicit `evidence:allow-unbounded` annotation with a
  justifying comment.

- **Byte-based enforcement and calibration.** All caps are enforced in
  bytes. Token-based caps are deferred until DDx has a per-harness
  tokenizer binding. Default byte caps are calibrated conservatively
  relative to the smallest common-reviewer model's context window using
  an assumed worst-case bytes-per-token ratio; the solution design
  document carries the concrete calibration table and the per-harness
  override mechanism (see §1a). If runtime telemetry (§15) shows that
  the default is either rejecting valid prompts or failing to prevent
  provider-side overflow, the cap is retuned via configuration, not
  via a rebuild.

- **Failure mode clarity.** Oversize inputs produce errors that name the
  input source, the observed size, and the cap. Empty provider responses
  are classified distinctly from unparseable responses (see §12).

- **Backward compatibility.** Bead 0 primitives ship with permissive
  default caps and cause no behavioral change on land. Tightening caps
  and consuming the primitives happens in subsequent beads; each
  consumer change is independently reviewable.

- **Performance.** Assembly cost per call is bounded by total prompt
  size (`O(MaxPromptBytes)`), not by input size on disk. Clamping
  occurs at read time; oversize files are not fully loaded into memory.

## User Stories

### US-220: Reviewer succeeds on large changes without silent overflow
**As** an operator running `ddx work` against beads with large diffs
**I want** the post-merge reviewer to produce a verdict on every
acceptable diff, and to fail with an actionable overflow error otherwise
**So that** primary work is not silently redone after empty reviewer
responses

**Acceptance Criteria:**
- Given a bead whose diff plus governing docs would exceed
  `MaxPromptBytes`, when the reviewer runs, then the assembler trims
  per §5 and the reviewer sees a bounded prompt
- Given trimming cannot reduce the prompt below `MaxPromptBytes`, when
  the reviewer runs, then the provider is not called and the bead
  receives a `review-error: context_overflow` event with observed and
  cap sizes
- Given a reviewer invocation emits a recognized verdict, when the
  outcome is recorded, then the event body carries `input_bytes`,
  `output_bytes`, `elapsed_ms`, `harness`, and `model`

### US-221: Operator distinguishes reviewer-failure classes
**As** an operator diagnosing a wave of reviewer failures
**I want** to distinguish context overflow, empty provider output,
unparseable output, and transport errors
**So that** I know whether to tune prompt caps, retry a transient
failure, or escalate a provider bug

**Acceptance Criteria:**
- Given a reviewer produces zero bytes of output, then the event
  summary classifies the outcome as `review-error: provider_empty`
- Given a reviewer produces text that lacks a verdict line, then the
  event summary classifies the outcome as `review-error: unparseable`
- Given `ddx agent metrics review-outcomes` is queried, then results
  include counts per error class and prompt-size quantiles

### US-222: Reviewer failure has a bounded retry ceiling
**As** an operator running `ddx work` overnight
**I want** reviewer failures on the same `result_rev` to stop
re-executing primary after a small number of attempts
**So that** a single broken bead cannot consume arbitrary tokens

**Acceptance Criteria:**
- Given a bead's most recent `result_rev` has accumulated N reviewer
  failures, when the next worker evaluates it, then the bead does not
  re-execute primary and emits a terminal `review-manual-required`
  event
- Given a `review-manual-required` event exists, when an operator
  lists blocked beads, then the bead appears there with the triggering
  failure class

### US-223: Oversize `--prompt` files fail fast with a clear error
**As** a developer running `ddx agent run --prompt ./huge.md`
**I want** DDx to reject the oversize file with an actionable error
**So that** I don't silently send a multi-megabyte prompt to a provider

**Acceptance Criteria:**
- Given `--prompt path` points at a file exceeding `MaxPromptBytes`,
  when the command runs, then it exits non-zero with an error naming
  the file, observed size, and cap
- Given the same file is supplied to `ddx agent run` server-side via
  `/api/agent/run`, then the API returns a 4xx error with the same
  information in the response body
- Given `DDX_AGENT_PROMPT` in an exec definition exceeds
  `MaxPromptBytes`, then the exec fails at startup with the same error

### US-224: Server text responses never exceed configured bounds
**As** a client consuming MCP, REST, or GraphQL responses that may
later be reused as prompt context
**I want** every response that inlines library, execution, diff,
session, or persona content to be bounded
**So that** response consumers cannot accidentally reintroduce the
overflow class

**Acceptance Criteria:**
- Given an MCP `read_document` call targets a file exceeding
  `MaxInlinedFileBytes`, then the response carries a truncated
  payload plus `truncated: true` and `original_bytes`
- Given a GraphQL `DocumentByPath` query returns content, then the
  same truncation contract applies
- Given a REST handler returns execution detail, session content,
  worker prompt, or diff content, then the same contract applies

### US-225: Review and grading prompts carry full minimum evidence
**As** an implementer reviewing a failed bead
**I want** the reviewer to always see bead acceptance, notes, and the
full list of changed files, even under heavy trimming
**So that** small surface-level issues are never missed due to
truncation

**Acceptance Criteria:**
- Given any bounded review assembly, then the assembled prompt
  contains the bead's `Title`, `Description`, `Acceptance`, `Notes`,
  and the full list of changed file names with stat lines
- Given a bounded grading assembly, then the assembled prompt
  contains each arm's `Output`, `PostRunOut`, `PostRunOK`, and
  `ToolCalls` after output clamping but never as omitted-entirely

### US-226: Unannotated unbounded prompt sinks fail CI
**As** a maintainer of DDx
**I want** any new code path that writes into a prompt sink without
going through the shared evidence primitives to be rejected by CI
**So that** the no-unbounded-prompts invariant (§3) cannot decay as
the codebase grows

**Acceptance Criteria:**
- Given a change introduces a direct file read whose output flows into
  an agent prompt without going through a shared evidence primitive,
  when CI runs the repository lint, then the lint exits non-zero and
  names the offending file and line
- Given a change introduces a server text-response handler that
  inlines library, execution, diff, session, or persona content
  without bounding the payload, when CI runs the repository lint,
  then the lint exits non-zero with the same reporting
- Given an intentional exception is required, when the code path
  carries an `evidence:allow-unbounded` annotation with a non-empty
  justifying comment, then the lint accepts the path

### US-227: Duplicate review assemblers are collapsed without drift
**As** a maintainer responsible for keeping review behavior consistent
between CLI and automated invocations
**I want** the two historical review-prompt assemblers collapsed into
one, with a gating test that proves byte-equivalence before the
duplicate is deleted
**So that** behavior does not silently drift between the CLI and the
execute-loop review path

**Acceptance Criteria:**
- Given a fixture bead plus a fixture result revision plus fixture
  governing refs, when both the pre-collapse and post-collapse code
  paths assemble a review prompt for the same inputs, then the
  resulting prompt bytes are identical
- Given the collapse is complete, when the repository is grepped for
  the duplicate assembler's function name and its helper symbols,
  then zero matches remain

## Dependencies

- **FEAT-002** (DDx Server) — `/api/agent/run`, `/api/providers`,
  `/api/executions`, and the MCP/GraphQL surfaces are the server-side
  text sinks governed by the §10 bounded-egress requirement. FEAT-002
  owns the response-shape definitions; FEAT-022 adds the `truncated`
  and `original_bytes` fields to those shapes and expects FEAT-002 to
  reciprocate the field-definition reference.
- **FEAT-005** (Artifacts) — the execute-bead attempt bundle structure
  that FEAT-022 §15 extends with an evidence-assembly block.
- **FEAT-006** (Agent Service) — review, grading, execute-bead, and
  session capture are owned here. FEAT-022 adds requirements on how
  the evidence these paths carry is assembled without re-owning the
  paths themselves.
- **FEAT-014** (Token Awareness) — owns the `result.json` runtime
  metrics schema (§19/§20 of FEAT-014). FEAT-022 §15 is additive to
  that schema; FEAT-022 does not redefine the runtime metrics fields
  FEAT-014 already governs.

## Delivery Sequencing

FEAT-022 is delivered in incremental stages. The specific tracker
beads that implement each stage are maintained in the beads tracker,
not in this spec; the stages below describe the *sequencing
constraints*, not the bead breakdown.

### Stage A — Shared primitives and structural sink lint
Lands requirement §1, §1a, and the lint rule under Non-Functional
§Lint. Ships with permissive default caps so no existing behavior
changes on land. Blocks all subsequent stages.

### Stage B — Collapse duplicate review assembler
Lands requirement §4. Byte-equivalence test (US-227) gates the
deletion of the duplicate. Blocks Stage C for the review path.

### Stage C — Bounded review and grading assembly
Lands requirements §5, §6, §7. The review and grading paths are
independently shippable after Stage B completes for review; the
grading path has no dependency on Stage B.

### Stage D — Hard-fail on oversize prompt ingress
Lands requirements §8 and §9 across every `--prompt` file site and
every inline-ingress surface. Depends only on Stage A.

### Stage E — Universal bounded server text egress
Lands requirements §10 and §11 across MCP, REST, and GraphQL
handlers. Depends only on Stage A.

### Stage F — Section-level telemetry
Lands requirements §15, §16, §17. Depends on Stage C for the
evidence-assembly block shape.

### Stage G — Error-class distinction and bounded review retry
Lands requirements §12, §13, §14. Depends on Stage C for the
context-overflow outcome class.

## Out of Scope

- A repo-wide `EvidenceAssembler` struct that every caller instantiates
  and drives. Primitives are shared; the top-level assembly order
  differs per caller (review, grading, primary) and forcing unification
  would couple unrelated call sites.
- Token-based caps. Deferred until a per-harness tokenizer binding
  exists; byte caps are the enforcement unit.
- Map-reduce per-file review (small per-file reviews + an aggregator).
  Noted as a potential follow-on if bounded assembly plus diff caps
  proves insufficient; not in scope here.
- `FEAT-*` section anchoring for governing documents. Requires new
  `spec-id` syntax; separate effort.
- JSON reviewer output contract. Orthogonal to size management; the
  existing markdown-with-strict-parse contract is adequate for the
  current failure modes.
- Mirroring review-style telemetry onto primary execute-bead events.
  Primary already has `result.json` / `usage.json` artifacts and the
  symmetry would add noise.
- Extending the invariant to non-prompt-adjacent server responses
  (e.g. metadata-only endpoints). The invariant is scoped to text
  content that could plausibly become prompt input.
      </content>
    </ref>
  </governing>

  <diff rev="a03b6824a63a4f2711d6d373bfa56273d9070cdd">
commit a03b6824a63a4f2711d6d373bfa56273d9070cdd
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 25 07:26:39 2026 -0400

    chore: add execution evidence [20260425T111752-]

diff --git a/.ddx/executions/20260425T111752-c108bda9/manifest.json b/.ddx/executions/20260425T111752-c108bda9/manifest.json
new file mode 100644
index 00000000..b6097258
--- /dev/null
+++ b/.ddx/executions/20260425T111752-c108bda9/manifest.json
@@ -0,0 +1,47 @@
+{
+  "attempt_id": "20260425T111752-c108bda9",
+  "bead_id": "ddx-d33377a5",
+  "base_rev": "bbdc8626bb3c9c1b9753844e19621203cee59cfd",
+  "created_at": "2026-04-25T11:17:53.054830413Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-d33377a5",
+    "title": "review+grading: metrics surface exposes prompt-size quantiles + failure-class breakdown (Stage F2)",
+    "description": "Stage F2 of FEAT-022. Extend the review-outcomes metrics surface to\nexpose prompt-size quantiles and the four-class review-failure\nbreakdown over a configurable time window.\n\nImplements FEAT-022 requirement §17. Satisfies the metrics-surface\ncriterion of US-221.\n\nDepends on Stage F1 (ddx-76e83e7f) — needs telemetry fields present on\nartifacts and events. Also depends on Stage G (ddx-70c1d2e2) for the\nfailure-class labels; if Stage G has not shipped, this bead can still\ncompute the quantiles and surface existing single-class counts, then\nbe extended when G lands.\n\nIn-scope files:\n- cli/cmd/agent_metrics.go (extend the review-outcomes subcommand's\n  JSON output and human-readable output with the new fields).\n- cli/internal/agent/metrics_review.go (or equivalent package that\n  aggregates the review-outcomes data from bead events and attempt\n  bundles): compute prompt_size_p50, prompt_size_p95, prompt_size_p99\n  over input_bytes values collected from review events in the window;\n  compute failure_classes counts keyed on context_overflow,\n  provider_empty, unparseable, transport.\n\nOut-of-scope:\n- A new CLI command (FEAT-022 §17 explicitly says \"no new command\").\n- Any web UI consumer of the new fields.\n- GraphQL exposure of the aggregated metrics — separate work if\n  needed.\n\nRollback: revert the extensions; existing review-outcomes fields\nremain unchanged.",
+    "acceptance": "cd cli \u0026\u0026 go test -run TestReviewOutcomesMetrics ./... passes: fixture bead history with varied prompt sizes and failure classes; assert ddx agent metrics review-outcomes --json produces output containing keys prompt_size_p50, prompt_size_p95, prompt_size_p99, failure_classes (object keyed on context_overflow, provider_empty, unparseable, transport with integer counts). ddx agent metrics review-outcomes --json | jq -e '.prompt_size_p95 | type == \"number\"' exits 0. ddx agent metrics review-outcomes --json | jq -e '.failure_classes | has(\"context_overflow\") and has(\"provider_empty\") and has(\"unparseable\") and has(\"transport\")' exits 0. Human-readable output (without --json) renders a table that includes the new columns. cd cli \u0026\u0026 go test ./cmd/... passes; existing metrics tests unchanged.",
+    "parent": "ddx-0c35470e",
+    "labels": [
+      "ddx",
+      "kind:implementation",
+      "area:agent",
+      "area:metrics",
+      "stage:F2"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-25T11:17:52Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "execute-loop-heartbeat-at": "2026-04-25T11:17:52.567408924Z",
+      "spec-id": "FEAT-022"
+    }
+  },
+  "governing": [
+    {
+      "id": "FEAT-022",
+      "path": "docs/helix/01-frame/features/FEAT-022-prompt-evidence-assembly.md",
+      "title": "Feature: Prompt Evidence Assembly and Size Invariants"
+    }
+  ],
+  "paths": {
+    "dir": ".ddx/executions/20260425T111752-c108bda9",
+    "prompt": ".ddx/executions/20260425T111752-c108bda9/prompt.md",
+    "manifest": ".ddx/executions/20260425T111752-c108bda9/manifest.json",
+    "result": ".ddx/executions/20260425T111752-c108bda9/result.json",
+    "checks": ".ddx/executions/20260425T111752-c108bda9/checks.json",
+    "usage": ".ddx/executions/20260425T111752-c108bda9/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-d33377a5-20260425T111752-c108bda9"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260425T111752-c108bda9/result.json b/.ddx/executions/20260425T111752-c108bda9/result.json
new file mode 100644
index 00000000..6a011488
--- /dev/null
+++ b/.ddx/executions/20260425T111752-c108bda9/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-d33377a5",
+  "attempt_id": "20260425T111752-c108bda9",
+  "base_rev": "bbdc8626bb3c9c1b9753844e19621203cee59cfd",
+  "result_rev": "61ee7e8a5a749357399ce9cd4cb5916bd35ad72c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-2afc9c1d",
+  "duration_ms": 525103,
+  "tokens": 28518,
+  "cost_usd": 4.116753,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260425T111752-c108bda9",
+  "prompt_file": ".ddx/executions/20260425T111752-c108bda9/prompt.md",
+  "manifest_file": ".ddx/executions/20260425T111752-c108bda9/manifest.json",
+  "result_file": ".ddx/executions/20260425T111752-c108bda9/result.json",
+  "usage_file": ".ddx/executions/20260425T111752-c108bda9/usage.json",
+  "started_at": "2026-04-25T11:17:53.055136496Z",
+  "finished_at": "2026-04-25T11:26:38.158995578Z"
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
## Review: ddx-d33377a5 iter 1

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

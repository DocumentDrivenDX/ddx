---
ddx:
  id: plan-2026-04-29-artifact-and-run-architecture
---
# Artifact Generalization & Three-Layer Run Architecture

Date: 2026-04-29
Status: Approved (refined through 3 review rounds with codex + fresh-eyes)

## Summary

Two threads converge in this plan:

1. **Generalize artifacts beyond markdown documents.** Diagrams, wireframes, hero images, prompts — text and binary — become first-class artifacts via media-type plurality and sidecar `.ddx.yaml` files. Generated artifacts are checked into git; regeneration is source-hash-driven.
2. **Three-layer run architecture.** DDx owns three explicit layered primitives: `ddx run` (single agent invocation), `ddx try <bead>` (bead attempt in isolated worktree), `ddx work` (queue drain). The current two-class storage split (`.ddx/exec-runs/` vs `.ddx/executions/<attempt-id>/`) collapses into one substrate.

These threads converge: **generating an artifact is a layer-1 agent run** with `produces_artifact` metadata.

## Decisions (resolved through review)

- **Artifact authority:** identity present → artifact. No "managed asset" second-class concept.
- **FEAT-019 fate:** child of FEAT-010 (not peer). Contracts to evaluation UX; workflow shapes (comparison, replay, benchmark) move to skills library.
- **Server posture:** 100% read coverage on HTTP/MCP for all DDx state, sequenced by impact. Writes case-by-case as workflows demand. Only `artifactRegenerate` write surface added in this plan.
- **Generated artifacts:** committed to git unconditionally. Size guard (50MB warn / 100MB hard error) with actionable git-lfs guidance; no auto-init / auto-`.gitattributes`.
- **CONTRACT-003:** amendments allowed if FEAT-006 work surfaces gaps. The mountable Cobra root export (below) is a load-bearing audit item.
- **Run substrate:** single on-disk shape; layer-2 and layer-3 add metadata. `.ddx/exec-runs/` and `.ddx/executions/<attempt-id>/` collapse.
- **No run-type catalog beyond the three layers.** Comparison, replay, benchmark, etc. are skill compositions, never enshrined in Go core or specs.
- **Loop ownership sharpened:** DDx owns mechanical queue drain (`ddx work`); content-aware supervisory decisions (e.g., "comparison failed → enqueue reconciliation beads") remain plugin/HELIX territory.
- **CLI verbs:** `ddx run` / `ddx try` / `ddx work` promoted to top-level. Hard-deprecate `ddx agent run`, `ddx agent execute-bead`, `ddx agent execute-loop` (exit non-zero with bare redirect message — no aliases, no grace period).
- **Generate-artifact layer placement:** pure-return generators (agent returns bytes/text, DDx writes file) are layer-1. Repo-editing generators are layer-2 with bead-less worktree.
- **Quorum dispatch:** moves to skill composition. `--quorum` flag goes away; replaced by `compare-prompts` skill.
- **`ddx agent` passthrough is structural:** DDx imports the upstream agent module's Cobra root and mounts it under `ddx agent`. Notionally `cli(ddx).agent(load_cli(agent))`. DDx does not enumerate per-subcommand wrappers; the entire upstream tree is the source of truth.

## Three-layer architecture

| Layer | CLI | Inputs | Outputs / side effects | Owns |
|---|---|---|---|---|
| 1 | `ddx run` | prompt + agent config | structured output, side effects, run metadata (tokens, model, duration, exit) | invocation atom; consumes upstream `ddx-agent` per CONTRACT-003 |
| 2 | `ddx try <bead>` | bead id, base revision | new worktree state + agent-run evidence; merge or preserve | worktree start/end capture; bead → prompt resolution; side-effect bundling |
| 3 | `ddx work` | bead queue, stop conditions | sequence of `ddx try` records + loop-level record (drained / blocked / deferred) | mechanical queue drain; no-progress detection |

`ddx try` wraps `ddx run`. `ddx work` iterates `ddx try`. One on-disk substrate; layer metadata distinguishes records.

`ddx artifact regenerate <id>` is sugar over layer 1 (or layer 2 if the generator edits the repo) with `produces_artifact: <id>` metadata.

## Frame artifact updates

### docs/helix/00-discover/product-vision.md
- Thesis: "documents AI agents consume" → "artifacts agents produce and consume."
- Artifact-management bullet: non-document media + generators.
- One sentence on producer/consumer space (no separate matrix).
- New Design-Philosophy subsection: **Three-layer run architecture** — `run` / `try` / `work` named as DDx's loop ownership.
- KVPs: "Multi-media artifact graph"; "Three-layer run architecture (run/try/work)"; note invocation is upstream.
- Principle #1: "Artifacts are the product (documents primary)."

### docs/helix/01-frame/prd.md
- Summary: artifacts (multi-media); three-layer run architecture; one substrate.
- Problem: add "no provenance for generated artifacts."
- Goals: reword #1; add three-layer-architecture goal; add generated-artifact-regeneration goal; add "100% read coverage on HTTP/MCP."
- Non-goals: sharpen loop wording (mechanical drain in scope; supervisory content-aware decisions out). Add: "DDx does not catalog run types beyond the three layers."
- FEAT-001/005/006/007/008/010/019/021 blurbs updated.

### FEAT-001-cli.md
- Promote `run` / `try` / `work` to top-level.
- `ddx agent` mounts upstream Cobra root structurally; no DDx-defined leaf subcommands beneath it.
- Hard-deprecation handlers for `ddx agent {run, execute-bead, execute-loop}`.
- New namespaces: `ddx runs` (cross-layer evidence introspection), `ddx tries` (layer-2 specifically), `ddx work workers` (layer-3 worker management).
- Remove FEAT-001:92 backward-compatibility clause for `ddx agent execute-loop`.

### FEAT-005-artifacts.md
- Identity broadens to non-markdown via sidecar `.ddx.yaml`.
- Add `media_type` and `generated_by` fields.
- Authority rule documented: identity present → artifact.
- Terminology cleanup ("identity," not "ddx: identity").

### FEAT-006-agent-service.md
- Tighten to layer-1 consumer-side wrapper that powers `ddx run`.
- `ddx agent` is structural passthrough (mount upstream Cobra root).
- Non-bead Profile/Permissions selection (artifact-keyed path).
- Migration table: what stays in FEAT-006 (CONTRACT-003 boundary, profile/permissions), what moves to FEAT-010 (worktree, merge/preserve, evidence bundles, queue drain), what moves to FEAT-001 (CLI surface).
- Session-log boundary clarified: DDx owns the envelope/pointer in evidence bundles; upstream owns inner log shape.

### FEAT-010-executions.md (substantial refactor)
- Three-layer architecture explicit and load-bearing.
- Substrate unification: one record shape; layer metadata.
- Migrate `.ddx/exec-runs/` and `.ddx/executions/<attempt-id>/` to one layout.
- Specify `ddx work` no-progress / stop conditions.
- Narrow read-only amendment for `artifactRegenerate` only.
- No run-type catalog beyond the three layers.

### FEAT-007-doc-graph.md
- Sidecar scanner refactor; `media_type`; `generated_by` edge with separate staleness rule (provenance, not authority — does not cascade like `depends_on`); version bump on Complete-status feature.
- 100% read endpoints for graph state, generated-artifact metadata, sidecar content.

### FEAT-008-web-ui.md
- Media-type-aware rendering (markdown, mermaid SVG, excalidraw embed, image inline, PDF embed).
- Regenerate action wired to `artifactRegenerate` mutation; reconcile with existing FEAT-008:391 weak Re-run requirement.
- Layer-aware run views (`work` → `try` → `run` drill-down).

### FEAT-019-agent-evaluation.md
- Contract to evaluation UX (comparison views, grading rubric storage and display, benchmark result aggregation).
- Workflow shapes → skills library.
- Remove peer-to-FEAT-010 assertions (FEAT-019:40, :404).

### FEAT-021-dashboard-ui.md
- Layer-aware run-history routes against unified substrate.

### FEAT-014, FEAT-013
- One-sentence touches: cost attribution and concurrency for generate-artifact.

## `ddx agent` subcommand fates

`ddx agent` becomes a structural mount of the upstream agent CLI Cobra root. Per-file fate in DDx:

**Delete with prejudice (upstream owns these via mounted subtree):**
- `agent_list.go`
- `agent_capabilities.go`
- `agent_doctor.go`
- `agent_check.go`
- `agent_models.go`
- `agent_providers.go`
- `agent_route_status.go`
- `agent_catalog.go`
- `agent_reindex.go`
- `agent_benchmark.go` (replaced by `benchmark-suite` skill)
- `agent_replay.go` (replaced by `replay-bead` skill)

**Move to other DDx namespaces (DDx-only concerns):**
- `agent_executions.go` → `tries_*.go`
- `agent_log.go` → `runs_log.go`
- `agent_metrics.go` → `runs_metrics.go`
- `agent_usage.go` → folded into `runs_metrics.go`
- `agent_workers.go` → `work_workers.go`

**DDx-side overrides on the mounted subtree:**
- Deprecation handlers for `agent run` / `agent execute-bead` / `agent execute-loop`.
- `agent condense` (DDx-specific output filter — keep as override or move upstream during impl pass).

**`agent_cmd.go` itself thins to:** mount upstream Cobra root + register DDx overrides.

## Skill library (DDx-shipped workflows)

Per FEAT-011's path model: source at `skills/ddx/`, embedded at `cli/internal/skills/ddx/`, init-time copies to `.claude/skills/ddx/`, `.agents/skills/ddx/`, `.ddx/skills/ddx/`.

- `compare-prompts` — N-arm dispatch + aggregation (replaces `--quorum` flag and `agent benchmark`)
- `replay-bead` — re-run with altered conditions, baseline diff (replaces `agent replay`)
- `benchmark-suite` — compare across prompt matrix
- `adversarial-review`
- `effort-estimate`
- `bead-breakdown`

## Bead breakdown

**Frame updates (sequential where dependencies require):**

1. Update product-vision.md (+ three-layer subsection)
2. Update prd.md (+ three-layer goal, sharpened loop non-goal, no-catalog non-goal)
3. Update FEAT-001 (CLI surface refactor; remove backcompat clause; new namespaces) — **hard predecessor for #6, #7**
4. Update FEAT-005 (multi-media identity refactor)
5. CONTRACT-003 audit — confirm `ddx-agent` exports a mountable Cobra root and other gaps
6. *(Conditional)* Open CONTRACT-003 amendment in `~/Projects/agent` if #5 surfaces gaps
7. Update FEAT-006 (layer-1 consumer; structural passthrough; non-bead Profile selection; migration table; session-log envelope boundary)
8. Refactor FEAT-010 (three-layer architecture; substrate unification; on-disk migration; no-progress stop conditions; narrow `artifactRegenerate` amendment)
9. Update FEAT-007 (sidecar refactor; media_type; generated_by; separate staleness; version bump)
10. Contract FEAT-019 to evaluation UX
11. Update FEAT-008 (media rendering, regenerate action, layer-aware run views)
12. Update FEAT-021 (layer-aware run-history routes)
13. Update FEAT-014 (cost attribution for generate-artifact)
14. Update FEAT-013 (concurrency note for regenerate)
15. **Repo-wide audit & verb migration** — atomic with #3:
    - Delete DDx reimplementations of upstream subcommands (list above).
    - Migrate DDx-only run/evidence subcommands to `runs`/`tries`/`work workers`.
    - Register DDx overrides on the mounted subtree (deprecations + condense).
    - Delete `agent_reindex.go` outright.
    - Update scripts/hooks/library skills/personas/docs/README/demos to new verbs.

**Design artifacts (concurrent with frame):**

16. Architecture: artifact identity schema (concurrent with #4)
17. Architecture: graph schema with media_type + generated_by + separate staleness (concurrent with #9)
18. Architecture: three-layer run substrate — record shape, layer metadata, on-disk layout, migration, `ddx work` stop conditions (concurrent with #8)
19. Sidecar `.ddx.yaml` schema design

**Read-coverage thread:**

20. Read-coverage audit — enumerate every CLI-visible read; map to HTTP/MCP coverage; identify gaps
21. Per-gap beads — sequenced by user-visibility impact, land case-by-case (this is a pointer to ongoing work, not a single bead)

**Skill library:**

22. `compare-prompts` skill
23. `replay-bead` skill
24. `benchmark-suite` skill
25. `adversarial-review` skill
26. `effort-estimate` skill
27. `bead-breakdown` skill

**Implementation (P2, after frame & design):**

- CLI: `ddx run` / `ddx try` top-level commands; mount upstream agent Cobra root; deprecation handlers
- CLI: `ddx runs` / `ddx tries` / `ddx work workers` namespaces (subsuming migrated subcommands)
- CLI: `ddx artifact regenerate <id>`, `ddx artifact list --media-type <t>`
- Storage: sidecar reader/writer; size-guard threshold/warning/tests
- Graph: `generated_by` edges + provenance staleness rule
- Run framework: refactor to one substrate per #18; generator metadata; `ddx work` no-progress detection
- Server: `artifactRegenerate` write endpoint
- Server UI: media-type renderers; regenerate action; layer-aware run history
- Migration: passive `media_type` only (read-time inference from extension); on-disk run-record migration per #18

## Constraints

- Beads #3, #7, #8, #15 must merge atomically — old verbs are referenced in scripts/library/personas/init help text/CI; an isolated #3 break the in-flight `ddx-refine-plan` skill that produced this plan.
- Bead #5 (CONTRACT-003 audit) gates #7 (FEAT-006). #6 conditional.
- Architecture sketches (#16–#19) run concurrent with their corresponding frame beads, not after — keeps frame/design from drifting.
- Read-coverage epic (#20–#21) is independent and not gated on this plan.

## Refinement provenance

Refined through 3 rounds of adversarial review (codex via `ddx agent run --harness codex` + fresh-eyes general-purpose subagent in parallel) plus user-resolved questions Q1–Q12. Key user decisions: artifact authority (Q1), FEAT-019 child-of-010 (Q2), reads-matter-writes-case-by-case (Q3), check-in unconditionally (Q4), CONTRACT-003 amendments allowed (Q5), substrate unification (Q6), no run-type catalog (Q7), no v1-gate framing (Q8), FEAT-019 contracts (Q9), generate-artifact split layer-1/layer-2 (Q10), quorum-as-skill (Q11), structural passthrough not per-command (Q12 follow-up).

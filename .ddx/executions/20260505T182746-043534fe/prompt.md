<bead-review>
  <bead id="ddx-c75d2c01" iter=1>
    <title>Define FEAT-027 Prose Quality Support</title>
    <description>
CONTEXT
This bead creates the governing feature spec for DDx Prose Quality Support. The desired user outcome is not AI-detection evasion; it is clear, specific, voice-preserving prose in DDx governing artifacts and user-facing docs. The feature must define how DDx targets common AI-writing tropes with deterministic, explainable findings.

IN-SCOPE FILES
- docs/helix/01-frame/features/FEAT-027-prose-quality-support.md
- docs/helix/01-frame/prd.md only if needed to add a short feature reference

REQUIRED SPEC CONTENT
- Problem: vague/generic prose weakens document-driven development.
- Primary users: DDx maintainers, project maintainers, and agents writing/reviewing docs.
- Content modes: technical, planning, and public prose.
- Product surfaces: default skill, deterministic checker/rules, ddx doc prose --changed, later review integration.
- Non-goals: no AI detector, no detector bypass, no blocking by default, no rewriting authorial voice away.
- Measurable acceptance criteria using structural findings: file, line, rule id, severity, rationale, suggested edit; project vocabulary; advisory default.

OUT-OF-SCOPE
- Implementing the checker.
- Choosing the final tool boundary beyond naming deterministic prose checks.
- Adding CLI flags or plugin assets.
    </description>
    <acceptance>
1. `test -f docs/helix/01-frame/features/FEAT-027-prose-quality-support.md` passes.
2. `rg -n "Prose Quality Support|not an AI detector|detector bypass|advisory|technical|planning|public|ddx doc prose --changed" docs/helix/01-frame/features/FEAT-027-prose-quality-support.md` returns matches for each required concept.
3. The FEAT acceptance criteria include structural, command-verifiable outcomes and avoid subjective-only phrases such as "sounds human" as acceptance.
4. If docs/helix/01-frame/prd.md is touched, it only adds a concise reference and does not re-specify the feature.
5. `lefthook run pre-commit` passes.
    </acceptance>
    <labels>phase:2, area:docs, kind:feature, prose-quality</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T182628-a2d4b7a5/manifest.json</file>
    <file>.ddx/executions/20260505T182628-a2d4b7a5/result.json</file>
  </changed-files>

  <governing>
    <ref id="helix.prd" path="docs/helix/01-frame/prd.md" title="Product Requirements Document: DDx">
      <content>
<untrusted-data>
---
ddx:
  id: helix.prd
  depends_on:
    - product-vision
---
# Product Requirements Document: DDx

**Version:** 4.2.0
**Date:** 2026-04-29
**Status:** Active

## Summary

DDx is a monorepo producing three artifacts that together form the shared
local-first infrastructure for document-driven development:

1. **`ddx` CLI** — multi-media artifact library management, artifact graph
   operations, bead tracking, three-layer run architecture (`ddx run` /
   `ddx try` / `ddx work`) on a unified on-disk substrate, agent dispatch,
   persona composition, template application, and git sync
2. **`ddx-server`** — web server + MCP endpoints for browsing documents,
   artifacts, beads, agent session logs, and execution history over the network
3. **`ddx.github.io`** — promotional website explaining DDx to developers and
   linking to docs

DDx is the foundation layer. Workflow tools (HELIX, others) build on top. DDx
provides reusable local services; it does not impose workflow phases or
methodology.

Concrete command, API, and storage contracts belong in the DDx feature
specifications. The PRD stays at the user- and capability-level:

- FEAT-001 defines the CLI surface and operator experience: top-level `run`,
  `try`, and `work` commands; `ddx agent` as a structural mount of the
  upstream agent Cobra root (no DDx-defined leaf subcommands beneath it);
  hard-deprecation handlers for `ddx agent {run, execute-bead, execute-loop}`;
  and `ddx runs`, `ddx tries`, `ddx work workers` namespaces for cross-layer
  evidence introspection
- FEAT-002 defines the server, HTTP, and MCP surfaces
- FEAT-003 defines the promotional website and documentation
- FEAT-004 defines shared work-item storage
- FEAT-005 defines the artifact convention and sidecar schema: identity
  broadens to non-markdown via sidecar `.ddx.yaml`; `media_type` and
  `generated_by` fields added; any file with a sidecar is a first-class
  artifact; authority rule: identity present → artifact
- FEAT-006 defines the layer-1 agent-dispatch boundary: the `ddx run`
  consumer-side wrapper that powers single agent invocation per CONTRACT-003;
  `ddx agent` mounts the upstream agent Cobra root structurally; non-bead
  profile and permissions selection; session-log envelope owned by DDx, inner
  log shape by upstream
- FEAT-007 defines the artifact graph and staleness model: sidecar-aware
  scanner; `media_type` field; `generated_by` edge with a separate provenance
  staleness rule (does not cascade like `depends_on`); 100% read endpoints for
  graph state, generated-artifact metadata, and sidecar content
- FEAT-008 defines the embedded web UI: media-type-aware rendering (markdown,
  mermaid SVG, excalidraw embed, image inline, PDF embed); regenerate action
  wired to `artifactRegenerate`; layer-aware run views (`work` → `try` → `run`
  drill-down)
- FEAT-009 defines the online library and plugin registry
- FEAT-010 defines the three-layer run architecture and unified substrate:
  `ddx run` / `ddx try` / `ddx work` as explicit primitives; one on-disk
  record shape with layer metadata; `.ddx/exec-runs/` and
  `.ddx/executions/<attempt-id>/` collapse into one layout; `ddx work`
  no-progress detection and stop conditions; `artifactRegenerate` as the only
  write surface added in this plan
- FEAT-011 defines agent-facing skills for DDx CLI operations
- FEAT-012 defines git awareness: auto-commit for documents and bead tracker,
  document history, write-then-commit for MCP/UI clients, and agent guidance
  generation on init
- FEAT-013 defines multi-agent coordination: concurrent bead safety,
  MCP supervisor surface, worktree-aware dispatch
- FEAT-014 defines agent token awareness: usage tracking, budget enforcement,
  and model selection guidance across harnesses
- FEAT-015 defines the installation architecture: clean separation of
  install.sh (binary), ddx install --global (skills), ddx init (project),
  and ddx install <plugin> (plugin lifecycle)
- FEAT-016 defines process metrics: bead lifecycle cost, rework rates, and
  derived measures computed from existing stores (beads, agent sessions).
  Distinct from FEAT-010 which covers operational metrics you *run*.
- ~~FEAT-017~~: adversarial review is a form of multi-agent dispatch covered by
  FEAT-006 quorum infrastructure. The "review against governing artifacts →
  structured findings → beads" workflow needs a design cycle to find the right
  abstraction, not a standalone feature.
- FEAT-018 defines plugin API documentation and stability: document existing
  extension surfaces (package.yaml, plugin directory layout, SKILL.md, hooks,
  bead conventions), add schema versioning, commit to backward compatibility
- FEAT-019 defines evaluation UX as a child of FEAT-010: comparison views,
  grading rubric storage and display, and benchmark result aggregation.
  Workflow shapes (comparison dispatch, replay, benchmark) move to the skills
  library — FEAT-019 does not own run orchestration
- FEAT-020 defines server node state and project registry: the server acquires
  a stable node identity (hostname or DDX_NODE_NAME), persists a project
  registry in an XDG-standard JSON file, writes a discovery addr file, and
  CLI commands auto-register their project with the server via a fire-and-
  forget background call
- FEAT-021 defines the multi-node dashboard UI: extends the FEAT-008 web UI
  with node/project-aware routing so every view is bookmarkable
  (`/nodes/:nodeId/projects/:projectId/...`), combined cross-project views for
  beads and agent sessions, project-scoped views for documents, dependency
  graph, and commit log, and layer-aware run-history routes against the unified
  substrate

## Problem

AI-assisted development needs more than prompt files. Teams need a shared way
to manage declarative artifacts, reusable runtime evidence, and local
automation infrastructure without hardcoding workflow semantics into each tool.

The six recurring pain points teams hit at the new productivity ceiling are
named in `product-vision.md` ("The Productivity Shift"). The PRD groups them
into named problem clusters by the physics principle they violate (see
`product-vision.md`). Every pain point listed below maps to a DDx capability;
problems outside that mapping belong in workflow tools, not the platform.

**Abstraction** (Principle 1 — abstraction is the lever)
- **No structure**: Artifacts, prompts, personas, and patterns accumulate as
  ad hoc files with weak identity and no explicit relationships
- **No composition**: Assembling the right combination of persona + pattern +
  spec into agent context is manual and error-prone
- **No document integrity guarantees**: When an upstream document changes,
  dependent documents silently drift — no automatic staleness detection or
  reconciliation tasking
- **No transferability**: Framework knowledge is trapped in its author;
  onboarding new team members requires manual explanation

**Iteration over tracked work** (Principle 2 — software is iteration over tracked work)
- **No reusable work-item store**: Workflow tools reimplement issue storage,
  dependency tracking, and coordination instead of sharing a local substrate
- **No reusable execution evidence**: Metrics, checks, and similar operations
  fall back to bespoke scripts and logs with no shared history model

**Methodology plurality** (Principle 3 — methodology is plural)
- **No reusable agent dispatch**: Each tool grows its own harness registry,
  logging, and output-capture behavior — every workflow tool reinvents the same
  invocation plumbing
- **No reuse**: Every project reinvents its agent instructions and supporting
  mechanics from scratch; proven patterns stay trapped in individual repos

**LLM physics** (Principle 4 — LLMs are stochastic, unreliable, and costly)
- **No cost-tier enforcement**: Token cost is a first-order constraint, not an
  optimization. Without capability-keyed routing and model-selection guidance,
  teams overspend on routine work and have no signal on the cheapest model that
  reliably closes beads

**Evidence and provenance** (Principle 5 — evidence provides memory)
- **No provenance for generated artifacts**: Generated files carry no record of
  which agent run, model, or prompt produced them — regeneration is ad hoc and
  lineage is lost
- **No measurement**: No standard way to track bead lifecycle metrics, token
  costs, or plugin-defined measures across projects
- **No feedback capture**: Lessons learned from agent interactions, project
  completions, and bead lifecycle stay informal — no structured way to capture,
  query, or act on what worked and what didn't

**Human-AI collaboration** (Principle 6 — human-AI collaboration is the fulcrum)
- **No composition for handoffs**: Assembling the right artifact context for a
  human-to-agent or agent-to-human handoff is manual and ad hoc — no DDx
  primitive covers that assembly
- **No discoverability**: Developers can't easily browse what documents,
  artifacts, or local runtime evidence are available
- **No network access**: Agents and tools can only read state from the local
  filesystem unless projects build their own HTTP or MCP layer

## Goals

### Primary
1. **Manage multi-media artifact libraries** — provide structure, conventions,
   and CLI tooling so declarative project knowledge — documents, diagrams,
   wireframes, images, prompts, and other media — stays organized and
   agent-discoverable
2. **Provide reusable local runtime services** — expose beads, agent dispatch,
   and execution history as workflow-agnostic DDx primitives
3. **Enable document composition** — combine personas, patterns, specs, and
   templates into coherent agent context
4. **Serve project state to agents and tools** — expose documents, artifacts,
   beads, and execution evidence via MCP endpoints and HTTP
5. **Support cross-project reuse** — share document libraries and workflow
   plugins through an online registry (`ddx install`)
6. **Provide agent-facing skills for DDx operations** — ship interactive
   skills (slash commands) that guide agents through complex DDx CLI
   operations like bead triage, agent dispatch, and package installation
7. **Integrate with revision control** — auto-commit document changes to
   protect work, expose document history to agents and tools, enable
   write-then-commit workflows for MCP and UI clients
8. **Support multi-agent coordination** — make bead operations, document
   writes, and agent dispatch safe under concurrent multi-agent use, with
   MCP as the remote observation and control surface
9. **Embed essential utilities** — bundle common developer tools (jq, etc.)
   so workflow tools have a consistent, cross-platform base without external
   runtime dependencies
10. **Maintain artifact graph integrity** — track relationships between
    documents, detect staleness when upstream artifacts change, and generate
    reconciliation tasks automatically (FEAT-007)
11. **Track process metrics** — derive bead lifecycle cost, rework rates, and
    other process measures from existing stores (beads, agent sessions) so
    teams can understand the economics and efficiency of their workflow
    (FEAT-016)
12. **Stabilize the plugin API** — document existing extension surfaces, add
    schema versioning, and commit to backward compatibility so plugin authors
    can build with confidence (FEAT-018)
13. **Provide a three-layer run architecture** — ship `ddx run` (single agent
    invocation), `ddx try` (bead attempt in isolated worktree), and `ddx work`
    (mechanical queue drain) as DDx-owned primitives on one unified on-disk
    substrate; layer metadata distinguishes records (FEAT-010)
14. **Enable source-hash-driven regeneration of generated artifacts** — track
    which agent run, model, and prompt produced each generated artifact;
    support on-demand regeneration when the source changes (FEAT-005, FEAT-007)
15. **Expose 100% of DDx state via HTTP and MCP read endpoints** — every
    piece of CLI-visible DDx state (artifacts, beads, run history, graph,
    sidecar metadata) is readable over the network; write surfaces are added
    case-by-case as workflows demand, starting with `artifactRegenerate`
    (FEAT-002, FEAT-010)

### Secondary
1. **Promote the practice** — website explains document-driven development and
   drives adoption
2. **Keep artifacts honest** — detect drift between governing documents and
   lower-level artifacts or runtime evidence
3. **Enable team transferability** — self-documenting project structures,
   getting-started guides, and pairing workflows so DDx is productive without
   requiring its author in the room

### Success Metrics

| Metric | Target |
|--------|--------|
| Time to assemble agent context | <30 seconds |
| Document reuse rate across projects | >40% |
| MCP endpoint response time | <200ms |
| Website explains DDx clearly to new visitor | <2 minutes to understand |

### Non-Goals

- A workflow methodology (that's HELIX and others, not DDx)
- Workflow-specific artifact ladders or stage progression (for example,
  `FEAT -> SD -> TD -> TP`) beyond storing IDs and relationships
- Workflow-specific bead validation (phase labels, spec-id enforcement — that's
  the workflow layer via hooks)
- Supervisory loop orchestration — DDx owns mechanical queue drain (`ddx
  work`); content-aware supervisory decisions (deciding what to do next based
  on agent or execution results — for example, "comparison failed → enqueue
  reconciliation beads") remain plugin/HELIX territory
- Cataloging run types beyond the three layers — comparison, replay,
  benchmark, and similar workflow shapes are skill compositions; DDx does not
  enshrine them in Go core or specs
- An AI agent or agent framework
- A standalone desktop GUI for editing documents (the embedded web UI editor
  in `ddx-server` is in-scope per FEAT-008; a separate desktop application is not)
- A cloud/SaaS service
- Real-time collaboration
- A storage system — artifacts are versioned in Git; future backends are
  possible but not DDx's concern
- Defining artifact types or templates — those come from plugins. DDx provides
  the infrastructure for storing and relating them.
- Operational metric definitions — plugins define what to measure; DDx
  provides the execution and collection infrastructure (FEAT-010)
- Optimization loop logic — DDx provides primitives (run, measure, compare);
  plugins define what to try next and when to converge
- Feedback collection features — beads already capture structured feedback;
  no separate feedback subsystem needed

## Users

### Primary: Developer Using AI Agents

**Role:** Professional developer directing AI agents and local automation
**Goals:** Keep project artifacts organized, compose context quickly, reuse
patterns across projects, inspect local execution evidence
**Pain:** Documents and evidence scattered everywhere, manual context assembly,
reinventing instructions and runtime tooling per project

### Secondary: Workflow Tool Author

**Role:** Developer building a methodology tool (like HELIX) on DDx primitives
**Goals:** Leverage DDx's document management, bead storage, agent dispatch,
execution history, persona system, and sync without reimplementing them
**Pain:** No standard infrastructure to build on; every workflow tool reinvents
local state, execution, and document management

### Tertiary: Agent (Machine Consumer)

**Role:** AI agent consuming documents via MCP or filesystem
**Goals:** Discover available documents and artifacts, read their contents,
understand their relationships, and inspect reusable runtime evidence
**Pain:** No programmatic way to browse document libraries or local execution
history; relies on humans to copy-paste context

## Requirements

### Must Have (P0)

**CLI experience**
The exact CLI contract lives in FEAT-001. At the PRD level, DDx must provide a
local operator surface that lets users:

- initialize and maintain a repo-local DDx workspace
- discover, inspect, and manage document-library content and declarative
  artifacts
- understand artifact relationships, dependency structure, and document
  freshness
- manage shared work items and their dependencies for higher-level tools
- dispatch supported AI agents through one reusable interface and inspect the
  resulting evidence
- validate installation and configuration health
- reuse and update shared DDx library content across projects
- invoke DDx operations through agent-facing skills (slash commands) that
  provide guided, validated workflows for complex CLI commands
- query process metrics (bead lifecycle cost, rework rates) derived from
  existing bead and agent session data

**Plugin API**
The exact API contract lives in FEAT-018. At the PRD level, DDx must provide:

- documentation of existing extension surfaces (package.yaml, plugin directory
  layout, SKILL.md format, hook scripts, bead conventions)
- schema versioning so plugin authors know what they can depend on
- backward compatibility commitment for documented surfaces

**Server experience**
The exact server, HTTP, and MCP contracts live in FEAT-002. At the PRD level,
DDx must provide a local network surface that lets tools and agents:

- browse and read document-library content remotely
- query artifact metadata, relationships, and staleness
- inspect shared work-item state
- inspect recorded agent session evidence
- rely on a stateless, filesystem-backed implementation rather than a hosted
  service

**Website experience**
- Clear explanation of what DDx is and why it exists
- Quick start guide
- Link to CLI installation
- Link to documentation
- Embedded terminal recordings (asciinema) demonstrating core workflows
- README with animated demos that sell the tool at a glance

**Release infrastructure**
- CI pipeline that runs the full test suite (via lefthook) on every push and PR
- E2E smoke tests validating the install-to-use journey
- Automated demo recording regeneration when CLI behavior changes
- GitHub Pages deployment gated on CI passing
- Multi-platform release builds with changelog generation

### Should Have (P1)

**CLI experience**
The CLI feature spec should also define requirements for:

- assembling context from multiple DDx resources
- stronger document quality checks and health diagnostics
- generic execution definitions and immutable run history for evidence-producing
  operations
- higher-level projections over execution history for domains such as metrics
  and acceptance evidence
- process metrics derived from existing stores: bead lifecycle cost (time,
  tokens, rework), reopen rates, revision counts (FEAT-016)

**Team enablement**
- getting-started guides that teach the platform alongside whichever plugin
  the user installs
- self-documenting project structures — after `ddx init` + plugin install,
  the project explains itself to new team members (human or agent)
- support for pairing workflows: structured onboarding where an experienced
  user guides a new team member through their first project
- internal project templates that teams can own end-to-end

**Server experience**
The server feature spec should also define requirements for:

- richer search across document-library contents
- persona resolution for remote consumers
- read-oriented access to generic execution definitions and run history

**Website experience**
- Ecosystem page showing workflow tools built on DDx
- Document library browser (interactive)
- "See It In Action" section with recordings of end-to-end workflows
  (init, plugin install, project creation, feature evolution)

### Nice to Have (P2)

**CLI**
- Document testing (validate documents produce expected agent behavior)
- Document analytics (most reused, most effective)
- IDE integration for document browsing

**Server**
- WebSocket notifications when documents change
- Multi-library aggregation (serve documents from multiple projects)

## Constraints

- **Technical:** Git-native. File-based. No external services required. Go for CLI and server.
- **Scope:** DDx manages documents, not agents. No workflow enforcement.
- **Platform:** macOS, Linux, Windows for CLI. Server runs anywhere Go runs.
- **License:** MIT, open source.
- **Agent safety:** DDx defaults to safe agent permissions. Permissive modes
  (`unrestricted`) require explicit opt-in via config or CLI flag. Normal
  users should never accidentally run agents with bypassed safety rails.
  See FEAT-006 Agent Permission Model.

## Risks

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|-----------|
| Documents go stale and get ignored | High | High | Reconciliation beads auto-generated on upstream changes; adversarial review agents check consistency; `ddx doctor` checks freshness |
| DDx/plugin boundary is fuzzy | Medium | High | Resolved for feedback (beads), metrics (FEAT-010 operational / FEAT-016 process), optimization (primitives in DDx, loop in plugins). Document remaining boundaries in FEAT-018. |
| Framework requires its author to explain it | High | High | Self-documenting project structures; getting-started guides bundled with plugins; team enablement as a P1 requirement |
| Agent testing and validation is unsolved industry-wide | Medium | High | DDx gives agents better context for first-pass correctness; adversarial review catches more issues; but fundamental problem remains open |
| MCP spec changes break server | Medium | Medium | Keep MCP integration thin; abstract behind internal API |
| Too much structure discourages adoption | Medium | Medium | Minimal defaults; let teams grow into structure |
| Rate of change in agentic ecosystem | High | Medium | Flexible plugin API; minimal DDx core; adapt without breaking plugin contracts |
| Git subtree complexity confuses users | Medium | Low | Wrap in simple commands; clear error messages |

## Success Criteria

- [ ] Users can set up DDx in a repository and manage project knowledge without
      relying on ad hoc file conventions
- [ ] Workflow tools can rely on DDx for shared work-item state instead of
      reimplementing local tracker storage
- [ ] Workflow tools can rely on DDx for agent dispatch and reusable invocation
      evidence
- [ ] Agents and tools can inspect repository documents and project state over
      local MCP or HTTP surfaces
- [ ] Website: live at ddx.github.io with clear messaging and embedded demos
- [ ] At least one workflow tool (HELIX) successfully building on DDx beads and
      agent dispatch
- [ ] `ddx install helix` bootstraps HELIX from the registry
- [ ] Document library syncing between 2+ projects
- [ ] CI pipeline green on every merge to main; Pages deploy gated on CI
- [ ] README includes animated terminal recordings of core workflows
- [ ] Upstream document changes auto-generate reconciliation beads for stale
      dependents
- [ ] Process metrics (bead cost, rework rate) queryable from existing data
- [ ] Multi-agent review workflow produces structured findings from quorum
      dispatch
- [ ] Plugin API is documented and stable enough for external plugin authors
- [ ] A new team member can orient in a DDx-initialized project without
      external explanation
</untrusted-data>
      </content>
    </ref>
  </governing>

  <diff rev="2b2e19f43e5572fc530e5935d446f05dc06f58d3">
<untrusted-data>
diff --git a/.ddx/executions/20260505T182628-a2d4b7a5/manifest.json b/.ddx/executions/20260505T182628-a2d4b7a5/manifest.json
new file mode 100644
index 00000000..8aa1d46e
--- /dev/null
+++ b/.ddx/executions/20260505T182628-a2d4b7a5/manifest.json
@@ -0,0 +1,46 @@
+{
+  "attempt_id": "20260505T182628-a2d4b7a5",
+  "bead_id": "ddx-c75d2c01",
+  "base_rev": "c16acd5fbf979f5537cdd3943e89cbf44525d1fe",
+  "created_at": "2026-05-05T18:26:31.03035533Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-c75d2c01",
+    "title": "Define FEAT-027 Prose Quality Support",
+    "description": "CONTEXT\nThis bead creates the governing feature spec for DDx Prose Quality Support. The desired user outcome is not AI-detection evasion; it is clear, specific, voice-preserving prose in DDx governing artifacts and user-facing docs. The feature must define how DDx targets common AI-writing tropes with deterministic, explainable findings.\n\nIN-SCOPE FILES\n- docs/helix/01-frame/features/FEAT-027-prose-quality-support.md\n- docs/helix/01-frame/prd.md only if needed to add a short feature reference\n\nREQUIRED SPEC CONTENT\n- Problem: vague/generic prose weakens document-driven development.\n- Primary users: DDx maintainers, project maintainers, and agents writing/reviewing docs.\n- Content modes: technical, planning, and public prose.\n- Product surfaces: default skill, deterministic checker/rules, ddx doc prose --changed, later review integration.\n- Non-goals: no AI detector, no detector bypass, no blocking by default, no rewriting authorial voice away.\n- Measurable acceptance criteria using structural findings: file, line, rule id, severity, rationale, suggested edit; project vocabulary; advisory default.\n\nOUT-OF-SCOPE\n- Implementing the checker.\n- Choosing the final tool boundary beyond naming deterministic prose checks.\n- Adding CLI flags or plugin assets.",
+    "acceptance": "1. `test -f docs/helix/01-frame/features/FEAT-027-prose-quality-support.md` passes.\n2. `rg -n \"Prose Quality Support|not an AI detector|detector bypass|advisory|technical|planning|public|ddx doc prose --changed\" docs/helix/01-frame/features/FEAT-027-prose-quality-support.md` returns matches for each required concept.\n3. The FEAT acceptance criteria include structural, command-verifiable outcomes and avoid subjective-only phrases such as \"sounds human\" as acceptance.\n4. If docs/helix/01-frame/prd.md is touched, it only adds a concise reference and does not re-specify the feature.\n5. `lefthook run pre-commit` passes.",
+    "parent": "ddx-ccda7a32",
+    "labels": [
+      "phase:2",
+      "area:docs",
+      "kind:feature",
+      "prose-quality"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T18:26:28Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3918937",
+      "execute-loop-heartbeat-at": "2026-05-05T18:26:28.175932099Z",
+      "spec-id": "helix.prd"
+    }
+  },
+  "governing": [
+    {
+      "id": "helix.prd",
+      "path": "docs/helix/01-frame/prd.md",
+      "title": "Product Requirements Document: DDx"
+    }
+  ],
+  "paths": {
+    "dir": ".ddx/executions/20260505T182628-a2d4b7a5",
+    "prompt": ".ddx/executions/20260505T182628-a2d4b7a5/prompt.md",
+    "manifest": ".ddx/executions/20260505T182628-a2d4b7a5/manifest.json",
+    "result": ".ddx/executions/20260505T182628-a2d4b7a5/result.json",
+    "checks": ".ddx/executions/20260505T182628-a2d4b7a5/checks.json",
+    "usage": ".ddx/executions/20260505T182628-a2d4b7a5/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-c75d2c01-20260505T182628-a2d4b7a5"
+  },
+  "prompt_sha": "b72493ec20a57a109531fdfdb61e372aff80c203a6941194d499edbba48f039e"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T182628-a2d4b7a5/result.json b/.ddx/executions/20260505T182628-a2d4b7a5/result.json
new file mode 100644
index 00000000..ce2c81c0
--- /dev/null
+++ b/.ddx/executions/20260505T182628-a2d4b7a5/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-c75d2c01",
+  "attempt_id": "20260505T182628-a2d4b7a5",
+  "base_rev": "c16acd5fbf979f5537cdd3943e89cbf44525d1fe",
+  "result_rev": "5d22622037ed5223c58dbd501f2bf95d9ceb3dd9",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-5a2acc47",
+  "duration_ms": 65228,
+  "tokens": 387156,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T182628-a2d4b7a5",
+  "prompt_file": ".ddx/executions/20260505T182628-a2d4b7a5/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T182628-a2d4b7a5/manifest.json",
+  "result_file": ".ddx/executions/20260505T182628-a2d4b7a5/result.json",
+  "usage_file": ".ddx/executions/20260505T182628-a2d4b7a5/usage.json",
+  "started_at": "2026-05-05T18:26:31.03078008Z",
+  "finished_at": "2026-05-05T18:27:36.258905852Z"
+}
\ No newline at end of file
</untrusted-data>
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

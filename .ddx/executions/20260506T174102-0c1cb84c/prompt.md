<bead-review>
  <bead id="ddx-424fdac4" iter=1>
    <title>Ratify DDx specs for Fizeau-owned routing and no-agent cleanup</title>
    <description>
PROBLEM
Implementation agents need durable specs before removing `ddx agent`, renaming DDx internals, or consuming Fizeau contract changes. Today the specs still mix the old `agent` vocabulary with the desired DDx/Fizeau boundary, so agents can reasonably follow stale instructions.

ROOT CAUSE WITH FILE:LINE
FEAT-001 still describes an `Agent Passthrough` structural mount for `ddx agent` at docs/helix/01-frame/features/FEAT-001-cli.md:70-84. FEAT-006 still describes the upstream invocation boundary as `agentlib.DdxAgent` and `DDx Agent Service` at docs/helix/01-frame/features/FEAT-006-agent-service.md:1-23. FEAT-010 still says layer 1 consumes upstream `ddx-agent` at docs/helix/01-frame/features/FEAT-010-task-execution.md:16-19. TP-020 remains a historical DDx-side routing test plan even though Fizeau owns routing. docs/migrations/routing-config.md:1-13 removes some old routing keys but does not yet document the full `agent.*` removal / Fizeau-vs-execution config split.

PROPOSED FIX
Update the specs before implementation. Required docs: FEAT-001, FEAT-006 or its renamed replacement, FEAT-010, ADR-024, TP-020, docs/migrations/routing-config.md, docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md, and DDx skill references. The specs must state: DDx owns task execution (`run`, `try`, `work`); Fizeau owns routing, provider/model discovery, native transcript/session rendering, and fuzzy model constraint resolution; `--harness`, `--provider`, and `--model` are explicit operator passthrough only; DDx passes raw strings unchanged; `ddx agent` is removed without deprecation; and `agent` is allowed only for actual external AI agent concepts.

NON-SCOPE
No production code removal in this bead. No Fizeau implementation work. No compatibility alias design. No generated CLI docs rebuild unless the docs generator is part of the spec-doc workflow.

UPSTREAM REFERENCES
- fizeau-61668fd3 for native harness `agent` -&gt; `fiz` contract identity
- fizeau-400b57fe for Fizeau-owned model alias/fuzzy matching

PARENT AND DEPENDENCIES
Parent: ddx-f088a3fd. Dependencies: none; this bead comes first.
    </description>
    <acceptance>
1. FEAT-001 removes the `ddx agent` structural mount and defines only `ddx run`, `ddx try`, `ddx work`, and any Fizeau diagnostics namespace that remains, with no aliases.
2. FEAT-006 is renamed or rewritten as the DDx consumer spec for Fizeau, and no longer presents `agentlib.DdxAgent` / `ddx-agent` as the desired contract name.
3. FEAT-010 names DDx-owned packages/roles using task execution vocabulary (`run`, `try`, `work`, `taskexec`, `fizeauadapter`) rather than `internal/agent`.
4. ADR-024 says escalation changes only MinPower/MaxPower/request facts and Fizeau chooses the concrete route.
5. TP-020 is replaced or superseded with a DDx boundary test plan proving DDx does not normalize, fuzzy-match, or route models.
6. docs/migrations/routing-config.md documents removal of `agent.*` defaults and the split between Fizeau routing config and DDx execution config.
7. DDx skill references explain explicit passthrough and Fizeau-owned model matching using examples such as `--model qwen36`, while stating DDx passes the raw string unchanged.
8. `rg -n "ddx agent|ddx-agent|agentlib.DdxAgent|Agent Service|cli/internal/agent" docs .agents/skills .claude/skills skills` returns only audited historical or literal-external-agent hits documented in the change.
9. `ddx doc validate` passes or any unrelated pre-existing failures are listed in the bead notes.
10. `lefthook run pre-commit` passes.
    </acceptance>
    <labels>phase:2, area:docs, area:spec, area:fizeau, kind:design, breaking</labels>
  </bead>

  <governing>
    <ref id="FEAT-001" path="docs/helix/01-frame/features/FEAT-001-cli.md" title="Feature: DDx CLI">
      <content>
<untrusted-data>
---
ddx:
  id: FEAT-001
  depends_on:
    - helix.prd
---
# Feature: DDx CLI

**ID:** FEAT-001
**Status:** In Progress
**Priority:** P0
**Owner:** DDx Team

## Overview

The `ddx` CLI is a single Go binary providing all DDx platform services locally: document library management, bead tracker, task execution dispatch, execution definitions and runs, document dependency graph, persona composition, template application, git sync, and meta-prompt injection.

## Requirements

### Functional

**Document Library (implemented)**
1. `ddx init` — create `.ddx/library/` structure, generate config, optionally sync from upstream
2. `ddx list [type]` — browse documents by category with filtering and JSON output
3. `ddx prompts list/show` — browse and inspect AI prompts
4. `ddx persona --list/--show/--bind` — manage persona definitions and role
   bindings (flag-based interface: `--list`, `--show <name>`, `--bind <name>`,
   `--role <name>`, `--tag <name>`)
5. ~~`ddx mcp list/install`~~ — **deprecated**. Removed to avoid confusion
   with DDx's own MCP server (FEAT-002).
5b. ~~`ddx auth`~~ — **deprecated**. Authentication for git subtree removed
   alongside subtree itself.
6. `ddx doctor` — validate library structure, config, git setup, dependencies
7. ~~`ddx update`~~ — **deprecated**. Library sync via git subtree removed.
   Content comes from `ddx install`. Run `ddx install <name>` to add resources.
8. ~~`ddx contribute`~~ — **deprecated**. Git subtree push removed. Contribute
   improvements via PRs to ddx-library or workflow repos directly.
9. `ddx upgrade` — self-upgrade binary
10. `ddx status` / `ddx log` — show sync state and change history
11. Meta-prompt injection into CLAUDE.md during init
11b. Project version tracking via `.ddx/versions.yaml` — stamps `ddx_version` on init, gates old binaries from running against newer projects, hints when skills are stale (FEAT-015)

**Bead Tracker (implemented — FEAT-004)**
12. `ddx bead create/show/update/close` — work item CRUD with `--set key=value` for custom fields
13. `ddx bead list/ready/blocked/status` — query and filter beads
14. `ddx bead dep add/remove/tree` — dependency DAG management
15. `ddx bead import/export` — JSONL interchange with bd/br
15b. `ddx bead evidence` — attach execution evidence to a bead
16. Claim/unclaim semantics for agent coordination
17. bd-compatible schema (issue_type, owner, created_at, dependencies)
18. Configurable ID prefix (auto-detected from repo name)
19. Backend abstraction (jsonl/bd/br)

**Task Execution (top-level — FEAT-006/FEAT-010)**
20. `ddx run --min-power <n> [--max-power <n>] --prompt <file>` — layer-1: invoke an AI agent with requested abstract power bounds; Fizeau owns model/provider routing
20b. `ddx run --top-power --prompt <file>` — request a `MinPower` threshold derived from Fizeau's available model/power catalog
20c. `ddx run --effort <level>` — pass non-routing reasoning/effort intent to Fizeau
20c-1. `ddx run --harness <name> --provider <name> --model <name>` — optional passthrough constraints sent unchanged to Fizeau; DDx does not validate, resolve, fallback, branch on, or widen them
20d. `ddx try <bead> [--from <rev>] [--no-merge]` — layer-2: bead attempt in isolated worktree with merge-or-preserve semantics
20e. `ddx work` — layer-3: drain the bead execution queue (mechanical drain; supervisory decisions stay in plugins/HELIX)
21. ~~`--quorum=majority --harnesses=a,b`~~ — **removed**. Multi-agent consensus moves to the `compare-prompts` skill (FEAT-011); no `--quorum` flag in core.

**Run Evidence & Layer Namespaces**
21a. `ddx runs <subcommand>` — cross-layer evidence introspection (log, metrics, history) over the unified run substrate
21b. `ddx tries <subcommand>` — layer-2 specifically (worktree start/end records, merge or preserve outcomes)
21c. `ddx work workers <subcommand>` — layer-3 worker management

**Task Execution Boundary (FEAT-006/FEAT-010)**

DDx owns the public workflow verbs `ddx run`, `ddx try`, and `ddx work`. Fizeau
owns routing, provider/model discovery, and concrete route selection. DDx does
not mount `ddx agent`, does not provide aliases for it, and does not keep any
legacy workflow verbs under that namespace.

22. Fizeau diagnostics, if exposed, remain read-only and separate from the
    workflow verbs; DDx may surface them only as Fizeau-owned observability, not
    as workflow aliases.
23. Run-evidence surfaces are no longer DDx-defined under the old agent
    namespace — they live under `ddx runs` (see 21a). Token/cost/usage surfaces
    likewise consolidate under `ddx runs metrics` (FEAT-014).

**Execution Service (in progress — FEAT-010)**
26. `ddx exec list [--artifact ID]` — list execution definitions
27. `ddx exec show <id>` — show one execution definition
28. `ddx exec validate <id>` — validate an execution definition and its linked artifacts
29. `ddx exec run <id>` — execute a definition and persist an immutable run record
30. `ddx exec log <run-id>` — show captured raw logs for one run
31. `ddx exec result <run-id>` — show structured result data for one run
32. `ddx exec history [--artifact ID] [--definition ID]` — inspect historical runs
32b. `ddx exec define <name> --artifact <id> --command <cmd>` — create an execution definition
32c. `ddx metric validate/run/history/compare/trend` — metric projections over execution substrate

**Document Graph (implemented — FEAT-007)**
33. `ddx doc graph` — show document dependency graph
34. `ddx doc stale` — list stale documents
35. `ddx doc stamp` — update review hashes
36. `ddx doc show/deps/dependents` — query artifact relationships
37. `ddx doc validate` — check frontmatter, deps, orphans
37b. `ddx doc history/diff/changed` — git-aware document history (FEAT-012)
37c. `ddx doc migrate` — convert dun: frontmatter to ddx:
37d. `ddx checkpoint <name> [--list] [--restore]` — lightweight git tag checkpoints (FEAT-012)

**Package Registry (in progress — FEAT-009)**
38. `ddx install <name>` — install packages from registries
39. `ddx search <query>` — search available resources
40. `ddx installed` — list installed packages
41. `ddx verify` — check integrity of installed packages

**Embedded Utilities**
47. `ddx jq <filter> [file...]` — embedded jq processor (powered by gojq), eliminating external jq dependency for HELIX and other workflow tools. Supports standard jq flags: `-r`, `-c`, `-s`, `-n`, `-R`, `-e`, `-j`, `-S`, `--tab`, `--indent`, `--arg`, `--argjson`, `--slurpfile`. Reads from stdin or file arguments. Pure Go, no CGo.

**DDx Skills (not started — FEAT-011)**
42. DDx ships agent-facing skills (Claude Code slash commands) for its own CLI operations
43. Skills are installed project-locally to `<projectRoot>/.agents/skills/ddx-*` and discoverable via `/ddx-<name>` (FEAT-015: home-directory targets retired)
44. Core skills: `ddx-bead` (guided bead create/triage), `ddx` (guided task execution with `run` / `try` / `work` and Fizeau passthrough constraints), `ddx-install` (guided package installation)
45. Skills validate inputs, suggest flags, and provide contextual guidance that the raw CLI doesn't
46. `ddx init` registers DDx skills alongside library content

### Non-Functional

- **Repo-root awareness:** On startup, `ddx` resolves the git repository root
  via `git rev-parse --show-toplevel` and uses it as the project working
  directory. All `.ddx/` operations (config, beads, agent logs, plugins) are
  anchored to the repo root regardless of which subdirectory the user invokes
  `ddx` from. If the current directory is not inside a git repository, `ddx`
  falls back to the current working directory. This prevents silent workspace
  duplication when `ddx init` or `ddx bead` are run from subdirectories.
- **Performance:** All local operations <1 second, with a startup ratchet on the hot paths. Current benchmarks: CLI startup 7.5ms, `ddx bead create` 10ms with async update checks, config load 0.46ms. Update checks run asynchronously and failures back off for 5 minutes instead of re-firing on every startup. Version gate and staleness hints are sync but zero-cost (local YAML read + string compare, no network). Ratchet coverage lives in `cli/bench_test.go` and `cli/internal/config/benchmark_test.go`; keep `ddx bead create` below 20ms and config load below 1ms on the local perf harness.
- **Portability:** Single binary, no runtime dependencies. macOS, Linux, Windows.
- **Installability:** `curl | bash` or `go install`
- **E2E smoke tests:** The install-to-use journey must be validated end-to-end in CI: `ddx init` → `ddx list` → `ddx doctor` → `ddx persona list` → `ddx persona bind` → `ddx bead create` → `ddx bead list`. Tests run against the real binary in a temp directory, not mocked. Lefthook orchestrates test execution; CI adds the E2E stage.

## Out of Scope

- Workflow enforcement (HELIX)
- Document editing UI (ddx-server web UI)
- Loop orchestration / supervisory dispatch (HELIX)

## Design References

- `docs/helix/02-design/solution-designs/SD-007-release-readiness.md` — release gating and version semantics
- `docs/helix/02-design/solution-designs/SD-011-agent-skills.md` — skill packaging and dispatch
- `docs/helix/02-design/solution-designs/SD-012-git-awareness.md` — auto-commit and history surfaces
- `docs/helix/02-design/solution-designs/SD-018-plugin-api-stability.md` — plugin manifest and extension surfaces
- `docs/helix/03-test/test-plans/TP-007-e2e-smoke-tests.md` — end-to-end CLI journey
- `docs/helix/03-test/test-plans/TP-015-onboarding-journey.md` — first-run onboarding
- `docs/helix/03-test/test-plans/TP-020-fizeau-boundary-and-pass-through.md` — DDx boundary behavior for raw routing constraint passthrough
</untrusted-data>
      </content>
    </ref>
  </governing>

  <diff rev="1287eb3056727ad078d15cf636040ba469bd5db3">
<untrusted-data>
commit 1287eb3056727ad078d15cf636040ba469bd5db3
Merge: 69cefcd70 b56ba7ae6
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed May 6 13:40:57 2026 -0400

    Merge bead ddx-424fdac4 attempt 20260506T173123- into main
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

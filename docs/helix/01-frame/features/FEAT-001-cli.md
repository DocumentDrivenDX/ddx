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

The `ddx` CLI is a single Go binary providing all DDx platform services locally: document library management, bead tracker, agent harness dispatch, execution definitions and runs, document dependency graph, persona composition, template application, git sync, and meta-prompt injection.

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

**Three-Layer Run Architecture (top-level — FEAT-006/FEAT-010)**
20. `ddx run --min-power <n> [--max-power <n>] --prompt <file>` — layer-1: invoke an AI agent with requested abstract power bounds; agent owns model/provider routing
20b. `ddx run --top-power --prompt <file>` — request a `MinPower` threshold derived from the agent's available model/power catalog
20c. `ddx run --effort <level>` — pass non-routing reasoning/effort intent to the agent
20c-1. `ddx run --harness <name> --provider <name> --model <name>` — optional passthrough constraints sent unchanged to the agent; DDx does not validate, resolve, fallback, branch on, or widen them
20d. `ddx try <bead> [--from <rev>] [--no-merge]` — layer-2: bead attempt in isolated worktree with merge-or-preserve semantics
20e. `ddx work` — layer-3: drain the bead execution queue (mechanical drain; supervisory decisions stay in plugins/HELIX)
21. ~~`--quorum=majority --harnesses=a,b`~~ — **removed**. Multi-agent consensus moves to the `compare-prompts` skill (FEAT-011); no `--quorum` flag in core.

**Run Evidence & Layer Namespaces**
21a. `ddx runs <subcommand>` — cross-layer evidence introspection (log, metrics, history) over the unified run substrate
21b. `ddx tries <subcommand>` — layer-2 specifically (worktree start/end records, merge or preserve outcomes)
21c. `ddx work workers <subcommand>` — layer-3 worker management

**Agent Passthrough — Structural Mount (FEAT-006)**

`ddx agent` mounts the upstream `ddx-agent` Cobra root structurally per CONTRACT-003. DDx imports the upstream agent module's Cobra root and grafts it under `ddx agent`; notionally `cli(ddx).agent(load_cli(agent))`. The entire upstream subcommand tree is the source of truth — DDx does not enumerate per-subcommand wrappers (no DDx reimplementations of `list`, `doctor`, `capabilities`, `models`, `providers`, etc.).

22. `ddx agent <upstream-subcommand>` — every subcommand defined by the upstream agent CLI is reachable here without DDx-side enumeration
23. DDx-side overrides on the mounted subtree:
    - **Hard-deprecation handlers** for `ddx agent run`, `ddx agent execute-bead`, `ddx agent execute-loop` — exit non-zero with a bare redirect message pointing to `ddx run` / `ddx try` / `ddx work` respectively. No aliases, no grace period, no shims.
    - `ddx agent condense` — DDx-specific output filter (override or upstream during impl pass)
24. `ddx agent log` and related run-evidence surfaces are no longer DDx-defined under `agent` — they live under `ddx runs` (see 21a). Token/cost/usage surfaces likewise consolidate under `ddx runs metrics` (FEAT-014).

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
44. Core skills: `ddx-bead` (guided bead create/triage), `ddx-agent` (guided agent dispatch with model/effort selection), `ddx-install` (guided package installation)
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
- `docs/helix/03-test/test-plans/TP-020-agent-routing-and-catalog-resolution.md` — agent routing behaviour consumed by the CLI

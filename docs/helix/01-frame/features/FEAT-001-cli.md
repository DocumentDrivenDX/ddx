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
4. `ddx persona list/show/bind` — manage persona definitions and role bindings
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
11b. Project version tracking via `.ddx/versions.yaml` — stamps `ddx_version` on init, gates old binaries from running against newer projects, hints when skills are stale (FEAT-XXX)

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

**Agent Service (implemented — FEAT-006)**
20. `ddx agent run --harness=<name> --prompt <file>` — invoke an AI agent
21. `ddx agent run --quorum=majority --harnesses=a,b` — multi-agent consensus
22. `ddx agent list` — show available harnesses with availability status
23. `ddx agent doctor` — harness health check (binary paths, availability)
24. `ddx agent log [session-id]` — session history with token tracking
25. `ddx agent capabilities [harness]` — show reasoning levels and model defaults for a harness
25b. `ddx agent usage [--since] [--harness] [--format]` — token and cost consumption summary (FEAT-014)

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
43. Skills are installed to `~/.agents/skills/ddx-*` and discoverable via `/ddx-<name>`
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

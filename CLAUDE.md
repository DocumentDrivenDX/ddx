# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

DDx (Document-Driven Development eXperience) is the shared infrastructure platform for document-driven development. It provides the tools that developers and workflow systems (like HELIX) use to manage the documents AI agents consume to build software.

## Agent Routing Reference

DDx agent routing behavior is documented in ddx-agent and should be linked, not duplicated:

- Profile catalog: https://github.com/DocumentDrivenDX/agent/blob/master/docs/routing/profiles.md
- Override precedence: https://github.com/DocumentDrivenDX/agent/blob/master/docs/routing/override-precedence.md
- Best-provider contract: https://github.com/DocumentDrivenDX/agent/blob/master/docs/routing/best-provider.md

DDx is one layer in a three-project stack:
- **DDx** (this repo) — platform services: document library, bead tracker, agent service, personas, templates, git sync
 - **DDx** (this repo) — platform services: document library, bead tracker, execution primitives, personas, templates, git sync
- **HELIX** (`~/Projects/helix`) — workflow methodology: phases, gates, supervisory dispatch, bounded actions
- **Dun** (`~/Projects/dun`) — quality check runner: check discovery, execution, agent-friendly output

## Architecture

This monorepo produces three artifacts:
- **`ddx` CLI** (`/cli/`): Go binary — document library mgmt, bead tracker, `run`/`try`/`work`, personas, templates, git sync
- **`ddx-server`** (planned): Web server + MCP endpoints for document and bead access
- **`ddx.github.io`** (`/website/`): Hugo promotional site

### Key Components

- `cli/` - Go CLI application source code
  - `cmd/` - Cobra command implementations
  - `internal/` - Internal packages (config, bead, persona, git, mcp, metaprompt, etc.)
  - `main.go` - Application entry point
- `website/` - Hugo site with Hextra theme
- `library/` - DDx default plugin source (installed to `.ddx/plugins/ddx/` by `ddx init`).
  `ddx install <name>` supports two install modes:
  - **Project-local** (default): writes to `<projectRoot>/.ddx/plugins/<name>/` (in-tree) or
    `${XDG_DATA_HOME}/ddx/projects/<identity>/plugins/<name>/` (convention mode); skill links
    land under `<project>/.agents/skills/` and `<project>/.claude/skills/`.
  - **Global** (`--global`): writes to `${XDG_DATA_HOME}/ddx/global/plugins/<name>/`; skill
    links land under `~/.agents/skills/` and `~/.claude/skills/` so the skill is available
    across every project on the machine without per-project setup.
  - `templates/` - Project templates
  - `patterns/` - Reusable code patterns
  - `prompts/` - AI prompts and instructions
  - `personas/` - AI persona definitions
  - `mcp-servers/` - MCP server registry
- `docs/helix/` - HELIX frame artifacts (vision, PRD, feature specs)
- `scripts/` - Build and automation scripts

## Development Commands

### CLI Development (run from `/cli/` directory)

```bash
# Build and test
make build          # Build for current platform
make test           # Run Go tests
make lint           # Run golangci-lint (or go vet if not available)
make fmt            # Format Go code

# Development workflow
make all            # Clean, deps, test, build
make dev            # Development mode with file watching (requires air)
make run ARGS="..."  # Run CLI with arguments
make install        # Install locally to ~/.local/bin/ddx

# Dependencies
make deps           # Install and tidy Go modules
make update-deps    # Update all dependencies

# Multi-platform builds
make build-all      # Build for all platforms
make release        # Create release archives
```

### Frontend Development (SvelteKit)

The web UI is a SvelteKit application built with Bun. Run from the repository root:

```bash
# Install frontend dependencies and start dev server
cd cli/internal/server/frontend && bun install && bun run dev

# Generate GraphQL types from schema
bun run houdini:generate

# Run unit tests
bun run test

# Run e2e tests with Playwright
bun run test:e2e
```

Frontend build output is embedded into the Go binary via `//go:embed`.

### Project Structure Navigation

The `.ddx.yml` configuration file defines:
- Repository URL and branch
- Included resources (prompts, scripts, templates, patterns)
- Template variables and overrides

### Key Patterns

1. **Command Structure**: Each CLI command is implemented as a separate file in `cli/cmd/`
2. **Configuration Management**: Uses Viper for config file handling with YAML format
3. **Template Processing**: Variable substitution system for customizing templates
4. **Library Registry**: Resources are fetched on demand via the library-registry fetch model (FEAT-009)
5. **Cross-Platform Support**: Makefile supports building for multiple platforms (macOS, Linux, Windows)

## Architectural Principles

**CRITICAL**: The DDx CLI follows the principle of "Extensibility Through Composition" - keep the CLI core minimal and add features through library resources.

1. **Platform Services in CLI, Opinions in Workflows**:
   - CLI owns: document library, bead tracker, agent dispatch, personas, templates, git sync
   - Workflow tools own: phase enforcement, supervisory loops, methodology-specific validation
   - DDx provides primitives; HELIX and others provide opinions

2. **Feature Addition Pattern**:
   - Document-type resources go in the library (personas, patterns, templates)
   - Platform capabilities go in the CLI (beads, agent service, MCP)
   - Workflow-specific logic stays in workflow tools (HELIX, etc.)

## Testing and Quality

**CRITICAL**: All tests are release-critical and must pass before committing:

```bash
# Run full test suite - all tests must pass
cd cli && go test -v ./cmd
```

Every test validates critical functionality and must pass before any release.

- Go tests are in `*_test.go` files alongside source code
- Linting uses golangci-lint (fallback to go vet)
- Code formatting with `go fmt`
- Cross-platform compatibility is maintained

### Pre-commit Checks

The project uses Lefthook for git hooks. To run pre-commit checks manually:

```bash
# Run all pre-commit checks
lefthook run pre-commit

# Or stage files and run checks
git add <files>
lefthook run pre-commit
```

### Test Timing Artifacts

CI uploads test timing as a JSON artifact. The `test-timing` job in `.github/workflows/ci.yml` runs `go test -json -race -cover ./...` and uploads two artifacts (retained 30 days) to the GitHub Actions run page:

- `test-timing-<run-id>`: raw `go test -json` output for post-processing
- `test-timing-summary.md`: Markdown table of the top 20 slowest tests (generated by `cli/tools/test-timing/`)

Use these to investigate slow-test regressions — download from the Actions run page after a push or PR.

### Test fixtures

Tests and demos that need a clean ddx-initialized git repo (without polluting this project) should use the shared fixture helper rather than rolling their own `t.TempDir()` setup. Call `testutils.NewFixtureRepo(t, profile)` (under `cli/internal/testutils/`) which wraps `scripts/build-fixture-repo.sh` and auto-cleans via `t.Cleanup`. Profiles: `minimal`, `standard`, `multi-project`, `federated`. See `scripts/build-fixture-repo.md` for layout details.

Pre-commit checks include:
- Secrets detection
- Binary file prevention
- Debug statement detection
- Merge conflict detection
- DDx configuration validation
- Go linting, formatting, building, and testing

### Pre-push / CI / Local Verification Split

Pre-push hooks run cheap safety checks only (build, lint, secrets, git-config-health, merge-policy, no-debug) and return fast operator feedback. The full test suite (`go test -race -cover ./...`) is **not** run on pre-push — CI on origin runs the full suite for every push to the remote, making a local duplicate redundant. Operators who want to verify before pushing can run `make test` (verbose, no race detector) or `make test-full` (mirrors CI exactly: race detector + coverage) from the `cli/` directory.

## CLI Command Overview

The CLI follows a noun-verb command structure for clarity and consistency:

**Core Commands:**
- `ddx init` - Initialize DDx in a project
- `ddx doctor` - Check installation health and diagnose issues
- `ddx upgrade` - Upgrade DDx binary to latest release version
- `ddx update` - Update toolkit resources from master repository
- `ddx status` - Show version and sync status

**Bead Tracker:**
- `ddx bead create/show/update/close` - Work item CRUD
- `ddx bead list/ready/blocked/status` - Query and filter beads
- `ddx bead dep add/remove/tree` - Dependency DAG management
- `ddx bead import/export` - JSONL interchange with bd/br
- `ddx bead migrate` - Externalize closed-bead events to `.ddx/attachments/` and move eligible closed beads into `.ddx/beads-archive.jsonl`. Idempotent; safe to re-run.

**Queue Work:**
  - `ddx work` - Drain the bead execution queue

**Execution:**
- `ddx run --harness=<name> --prompt <file>` - Invoke the service once
- `ddx try <id> [--from <rev>] [--no-merge]` - Run one bead in an isolated worktree
- `ddx work` - Drain the bead queue
- `ddx log` - Session history

**Resource Commands:**
- `ddx prompts list/show` - AI prompts
- `ddx templates list/apply` - Project templates
- `ddx persona list/show/bind` - AI personas
- `ddx mcp list/install` - MCP servers

**Embedded Utilities:**
- `ddx jq <filter> [file...]` - Embedded jq processor (via gojq), no external jq required

## Persona System

DDX includes a persona system that provides consistent AI personalities for different roles:

- **Personas**: Reusable AI personality templates (e.g., `code-reviewer`, `test-engineer`, `architect`)
- **Roles**: Abstract functions that personas fulfill (e.g., `code-reviewer`, `test-engineer`)
- **Bindings**: Project-specific mappings between roles and personas in `.ddx.yml`

Personas enable consistent, high-quality AI interactions across team members and projects. Projects bind specific personas to roles. See `library/personas/` for available personas and `library/personas/README.md` for detailed documentation.

Plugin and persona lookup follows a three-layer precedence: project-local (`<project>/.ddx/plugins/<name>/`) → global (`${XDG_DATA_HOME}/ddx/global/plugins/<name>/`) → baked-in binary default (only for the `ddx` plugin). The project layer always wins when present. `ddx doctor` reports both the global install layer and the project install layer, including when the project copy is absent and falls through to the global layer (`lazy-resolves-to-global`). Use `ddx install <name> --global` to install machine-wide (skills land in `~/.claude/skills/` and `~/.agents/skills/`); omit `--global` for per-project installs. See `docs/helix/02-design/adr/ADR-027-skill-install-topology.md` for the full decision record.

## When filing beads

Bead descriptions + AC must be sufficient context for a sub-agent to execute the work without operator hand-curation (principle P7, `docs/helix/06-iterate/reliability-principles.md`). Before invoking `ddx bead create`, walk the 8-criterion rubric in `docs/helix/06-iterate/bead-authoring-template.md`: title clarity, root cause with file:line, numbered AC with specific test names, wired-in assertion, `cd cli && go test` + lefthook gate, labels with cross-refs, parent + deps, standalone description (no `/tmp/*`, no chat references). Beads scoring < 7/8 will fail in autonomous drain.

See docs/helix/06-iterate/reliability-principles.md for the 7 reliability principles applied to ddx try / ddx work execution.

## Execution Evidence Convention

`.ddx/executions/` is **per-machine working state** — it is listed in `.gitignore` and is NOT committed to git. Files written there by agents (prompt.md, result.json, manifest.json, etc.) exist only on the local machine. The durable audit trail is the events stream in `.ddx/attachments/`, not the execution dirs.

When a bead's acceptance criteria asks for an investigation report, findings document, or any other freestanding non-source artifact, the agent must write that artifact under the per-attempt evidence directory `.ddx/executions/<run-id>/` (the `{{.AttemptDir}}` slot in execute-bead prompts). **Never write reports to `/tmp` or any path outside the repository working directory** — such paths are invisible to the post-execution reviewer and do not survive between machines. If the bead names a specific tracked in-repo path for the report, use that path; otherwise default to `.ddx/executions/<run-id>/<short-name>.md` (which will be on-disk but untracked).

## When filing beads

A bead's body is the entire prompt the executing sub-agent will see — there is no chat history, no operator hand-curation, no out-of-band context. Every bead must therefore satisfy the 8-criterion rubric documented in `docs/helix/06-iterate/bead-authoring-template.md` before it is filed or dispatched.

In short:

- **Title**: imperative, names subsystem + change.
- **Description**: PROBLEM + ROOT CAUSE WITH `path/file.go:LINE` + PROPOSED FIX + NON-SCOPE.
- **AC**: numbered, each verifiable; at least one names a `Test*` function or `go test -run` filter; final two lines are `cd cli && go test ./<paths>/... green` and `lefthook run pre-commit passes`.
- **Labels**: `phase:N`, `area:*`, `kind:*`, plus cross-refs (adr/spec/prevention) when applicable.
- **Parent + Deps**: explicit. State "no deps" if there are none.
- **Self-test**: re-read the bead cold. If a competent agent given only the bead body cannot pick a file to edit and run tests without asking, retrofit before dispatch.

Do not cite `/tmp/...` plan files as load-bearing context — they do not survive between machines or sessions. Inline the relevant excerpt into the description instead.

<!-- DDX-META-PROMPT:START -->
<!-- Source: claude/system-prompts/focused.md -->
# System Instructions

**Execute ONLY what is requested:**

- **YAGNI** (You Aren't Gonna Need It): Implement only specified features. No "useful additions" or "while we're here" features.
- **KISS** (Keep It Simple, Stupid): Choose the simplest solution that meets requirements. Avoid clever code or premature optimization.
- **DOWITYTD** (Do Only What I Told You To Do): Stop when the task is complete. No extra refactoring, documentation, or improvements unless explicitly requested.

**Response Style:**
- Be concise and direct
- Skip preamble and postamble
- Provide complete information without unnecessary elaboration
- Stop immediately when the task is done

**When coding:**
- Write only code needed to pass tests
- No gold-plating or speculative features
- Follow existing patterns and conventions
- Add only requested functionality
<!-- DDX-META-PROMPT:END -->
# important-instruction-reminders
Do what has been asked; nothing more, nothing less.
NEVER create files unless they're absolutely necessary for achieving your goal.
ALWAYS prefer editing an existing file to creating a new one.
NEVER proactively create documentation files (*.md) or README files unless the task explicitly requires creating or editing documentation.

# DDx — Document-Driven Development eXperience

[![CI](https://github.com/DocumentDrivenDX/ddx/actions/workflows/ci.yml/badge.svg)](https://github.com/DocumentDrivenDX/ddx/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/DocumentDrivenDX/ddx?filename=cli/go.mod)](https://github.com/DocumentDrivenDX/ddx)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> Documents drive the agents. DDx drives the documents.

<p align="center">
  <img src="website/static/demos/07-quickstart.gif" alt="DDx quickstart — init, install helix, create beads" width="700">
</p>

**[Full Documentation →](https://DocumentDrivenDX.github.io/ddx/)**

## What You Just Saw

DDx + HELIX takes a project from zero to working software:

1. `ddx init` — create a document library
2. `ddx plugin install helix` — pin HELIX, cache it like an `npx` dependency, and generate local adapters
3. Agent frames the project — creates PRD, feature specs, and tracker beads
4. Agent builds it — TDD, one commit per bead, all tests passing
5. Agent evolves it — adds a feature, updates specs, extends code
6. `ddx bead list` — every step tracked, every bead closed

## Quick Start

```bash
# Install DDx
curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash

# Initialize your project
cd your-project
ddx init

# Install HELIX workflow plugin
ddx plugin install helix

# Explore
ddx doctor
ddx persona list
ddx bead list
```

Marketplace plugins are project dependencies, not copied source assets. DDx
records plugin intent in `.ddx/plugins.lock.yaml`, stores payloads in the shared
XDG cache, and writes only generated adapter links under `.agents/skills/` and
`.claude/skills/`. Those adapters are local rebuild output; `ddx plugin sync`
recreates them from the lock and cache.

## Pre-claim Intake and Silent-Idle Diagnosis

Before a bead is claimed, DDx runs the pre-claim intake hook and evaluates the
shared `readiness_checks` payload. The intake contract accepts these canonical
verdict forms:

- JSON bool `true` -> `pass`
- JSON bool `false` -> `fail`
- JSON strings are passed through after trimming
- `null` or absent -> empty

See `ClassifyReadinessWithMode`
(`cli/internal/agent/readiness_classification.go:56-115`) for the mapping from
readiness classifications to worker behavior. The worker treats the shared
schema as a read-only intake contract and classifies the verdict into these
operator-visible failure classes:

| Intake classification | Worker behavior | Operator signal |
| --- | --- | --- |
| `ready` | claim proceeds | normal claim / attempt events |
| `needs_refine` in warn-only mode | warn, then proceed | `pre_claim_intake.warn`; no park |
| `needs_refine` in block/factory mode | park for operator attention | `pre_claim_intake.blocked` |
| `operator_required` | park the bead | `pre_claim_intake.blocked` |
| `needs_split` | decompose or park for decomposition | `pre_claim_intake.decomposed` or `pre_claim_intake.blocked` |
| `system_unready`, `intake_error`, timeout, or schema drift | hard errors from intake; fail-open, skip claim | `pre_claim_intake.warn` / `pre_claim_intake.error`, followed by `loop.idle` if every ready bead is skipped |

Two escalation guards make silent-idle visible:

- `preClaimIdleEscalationThreshold` is `5` consecutive `loop.idle` cycles with
  the same pre-claim blocker detail (`ddx-df77e668`). At that point the loop
  emits a non-terminal `loop.operator_attention` event instead of silently
  polling forever.
- `--preclaim-warn-threshold` defaults to `5` consecutive identical pre-claim
  warn fingerprints across distinct bead IDs. This escalates repeated warn-only
  intake failures to operator attention.

If the worker idles on a full queue, diagnose it this way:

1. Confirm the scope with `ddx work status --json` from the project root. Use
   `--project <path>` when checking another checkout.
2. Inspect the newest `.ddx/agent-logs/agent-loop-*.jsonl` file and filter for
   `loop.idle`, `pre_claim_intake.warn`, `pre_claim_intake.error`, and
   `loop.operator_attention`.
3. Compare the `fingerprint` or blocker detail across repeated events. Rows
   with the same fingerprint mean the same readiness blocker is recurring; a
   changing fingerprint means the worker is reaching different blockers.
4. Distinguish `preclaim_systemic` from `preclaim_tracker_contention`.
   `preclaim_tracker_contention` is usually transient concurrent tracker churn;
   `preclaim_systemic` means a real repo/config/intake problem is blocking
   every candidate.
5. Check `ddx bead operator-attention --json` for durable operator-attention
   rows when the loop escalates or releases a wedged lease.

For a queue that is technically moving but still claims very slowly, use the
claim-success-rate warning knobs. `--claim-rate-window` defaults to `10` recent
claim attempts and `--claim-rate-threshold` defaults to `0.0`, which warns once
the full window has a 0% claim success rate. `ddx work --watch` emits the
warning to the operator stream once the rolling success rate over a full window
falls at or below the threshold, so a low-claim loop is visible even when the
queue is not fully idle. Use the latest agent loop log plus `ddx work status`
to distinguish slow progress from a silent intake failure.

**Related reliability context:** AR-2026-05-17 follow-up; lock handling
(ddx-57c40485); route-resolution wedge handling (ddx-8f2e0ebf).

## Development

### Local Install

The canonical local binary is `${HOME}/.local/bin/ddx`. Use the installer for
all local deployments:

```bash
make build
./install.sh --from-build
```

`make install` runs the same installer path after building. Avoid copying DDx
to other PATH directories by hand; add an installer mode when a different
deployment source is needed. Use `./install.sh --from-build --no-shell --prefix
"$TMPDIR/ddx-install-test"` for throwaway installer checks.

### Build and Test CLI

```bash
cd cli
make dev      # Start Go development server
make test     # Run all tests
```

### Frontend Development (SvelteKit)

The web UI is built with SvelteKit and Bun:

```bash
# Install dependencies and start dev server
cd cli/internal/server/frontend && bun install && bun run dev

# Generate GraphQL types from schema
bun run houdini:generate

# Run unit tests
bun run test

# Run e2e tests with Playwright
bun run test:e2e
```

## Build Something

```bash
# Frame: create specs and work items
ddx run --harness claude --prompt frame-prompt.md

# Build: agent implements per specs, TDD, closes beads
ddx try ddx-abc12345

# Evolve: add a feature
ddx work

# Inspect
ddx bead list          # all beads tracked
ddx log                 # history
ddx doc history PRD-001  # spec evolution
ddx metric list        # metric artifacts
ddx metric show MET-001  # metric definition + recent history
```

`ddx metric` inspection commands are read-only projections over the exec
substrate. `ddx metric run <MET-id>` is the convenience wrapper for
`ddx exec run <definition-id>` that resolves the latest active definition
bound to the metric artifact.

## Key Commands

| Command | What it does |
|---------|-------------|
| `ddx init` | Initialize document library |
| `ddx plugin install <name>` | Pin a workflow plugin, cache its payload, and generate adapters |
| `ddx doctor` | Validate installation health |
| [Prose quality support](docs/prose-quality.md) | Guidance for `ddx doc prose --changed` and prose config |
| `ddx bead create/list/ready` | Track work items |
| `ddx metric list/show/validate/run/history/compare/trend` | Inspect metric artifacts and run history |
| `ddx run` | Invoke an AI agent once |
| `ddx try` | Execute a specific bead |
| `ddx work` | Drain the bead queue |
| `ddx log` | View history |
| `ddx persona bind` | Assign personas to roles |

## Ecosystem

```
Workflow tools (HELIX, etc.)  →  opinionated practices
DDx (this project)            →  document infrastructure
AI agents (Claude, etc.)      →  consume docs, produce code
```

## License

MIT. See [LICENSE](LICENSE).

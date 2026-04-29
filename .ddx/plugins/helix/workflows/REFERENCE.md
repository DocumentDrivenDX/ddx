---
dun:
  id: helix.workflow.reference
  depends_on:
    - helix.workflow
---
# HELIX Quick Reference Card

## Canonical Docs

- [README.md](README.md): high-level model and authority order
- [EXECUTION.md](EXECUTION.md): queue control and operator loop
- Tracker conventions: `ddx bead --help` (DDx FEAT-004)
- [implementation.md](actions/implementation.md): one bounded execution pass
- [check.md](actions/check.md): queue-drain decision
- [reconcile-alignment.md](actions/reconcile-alignment.md): top-down review
- [backfill-helix-docs.md](actions/backfill-helix-docs.md): conservative reconstruction
- [plan.md](actions/plan.md): iterative design document creation
- [polish.md](actions/polish.md): issue refinement before implementation
- [fresh-eyes-review.md](actions/fresh-eyes-review.md): post-implementation review
- [experiment.md](actions/experiment.md): metric-driven optimization iteration
- [metric-definition.yaml](templates/metric-definition.yaml): shared metric definitions

## Phase Summary

| Phase | Primary Output | Main Location |
|---|---|---|
| Optional `00-discover` | vision and opportunity framing | `docs/helix/00-discover/` |
| `01-frame` | requirements and stories | `docs/helix/01-frame/` |
| `02-design` | architecture and design contracts | `docs/helix/02-design/` |
| `03-test` | test plans and failing tests | `docs/helix/03-test/`, `tests/` |
| `04-build` | project build guidance + execution issues | `docs/helix/04-build/`, `.helix/` |
| `05-deploy` | rollout docs + deploy issues | `docs/helix/05-deploy/`, `.helix/` |
| `06-iterate` | backlog, reports, follow-up planning | `docs/helix/06-iterate/` |

## Authority Order

1. Product Vision
2. Product Requirements
3. Feature Specs / User Stories
4. Architecture / ADRs
5. Solution / Technical Designs
6. Test Plans / Tests
7. Implementation Plans
8. Source Code / Build Artifacts

## Core Commands

### Bootstrap

```bash
ddx bead init
ddx install helix
helix doctor --fix
```

Installed agent skills mirror CLI commands exactly: `helix-<command>` maps to
`helix <command>`.

Canonical project package path: `./.agents/skills`
Canonical user install path: `~/.agents/skills`
Claude compatibility path: `~/.claude/skills`

HELIX-specific execution behavior lives in the workflow contract and wrapper
commands, not in the portable skill packaging layer.

### Execution Commands

```bash
helix run
helix build
helix build hx-abc123
helix check repo
helix align repo
helix backfill repo
helix status
helix evolve "requirement description"
helix triage "Issue title" --type task
```

### Planning and Quality Commands

```bash
helix design [scope]                  # create design document
helix design --rounds 8 auth         # more refinement rounds
helix polish [scope]                  # refine issues before implementation
helix polish --rounds 10              # more polish rounds
helix next                            # recommended next issue
helix review [scope]                  # fresh-eyes review of recent work
helix experiment [issue-id|goal]      # one experiment iteration
helix experiment --close              # squash-merge and close session
```

### Tracker

```bash
ddx bead ready --json
ddx bead update <id> --claim
ddx bead show <id>
ddx bead dep tree <id>
ddx bead blocked --json
ddx bead close <id>
ddx bead status
ddx bead import --from jsonl --file .ddx/beads.jsonl
ddx bead export
```

See `ddx bead --help` for full tracker conventions and setup guidance.

## Tracker Labeling

Labels are organizational conventions for triage and traceability. They are
not required by the execution loop queue guard.

Recommended labels:
- `helix` -- identifies HELIX-managed issues (recommended, not required by the queue guard).
- Phase labels: `phase:frame`, `phase:design`, `phase:test`, `phase:build`, `phase:deploy`, `phase:iterate`, `phase:review`.
- Kind labels: `kind:build`, `kind:deploy`, `kind:backlog`, `kind:review`.
- Traceability labels: `story:US-XXX`, `feature:FEAT-XXX`, `area:<name>`, `source:metrics`.

## Decision Guide

- Starting new work or a large scope:
  run `helix design`, then `helix polish`, then `helix run`.
- Ready execution issues exist:
  run `helix build` or `helix run`.
- Work lacks design authority for safe execution:
  run `helix design`, or let `helix run` dispatch it from `check`.
- Specs changed and open work needs issue refinement before implementation:
  run `helix polish`, or let `helix run` dispatch it from `check`.
- No ready execution issue, but the planning stack exists and next work is
  unclear:
  run alignment.
- Canonical docs are missing or too incomplete to execute safely:
  run backfill.
- Work exists but is blocked or already in progress:
  stop and wait.
- The queue drains:
  run `check`, not a blind loop and not `ddx bead list --ready`.
- After implementing an issue:
  run `helix review` for fresh-eyes quality check.

## Artifact Inputs

Use the prompts and templates under:

- `.ddx/plugins/helix/workflows/phases/00-discover/artifacts/`
- `.ddx/plugins/helix/workflows/phases/01-frame/artifacts/`
- `.ddx/plugins/helix/workflows/phases/02-design/artifacts/`
- `.ddx/plugins/helix/workflows/phases/03-test/artifacts/`
- `.ddx/plugins/helix/workflows/phases/04-build/artifacts/`
- `.ddx/plugins/helix/workflows/phases/05-deploy/artifacts/`
- `.ddx/plugins/helix/workflows/phases/06-iterate/artifacts/`

Those artifact directories support the canonical docs under `docs/helix/`; they
do not replace the bounded execution contract.

## Output Reports

- Alignment reviews:
  `docs/helix/06-iterate/alignment-reviews/AR-YYYY-MM-DD[-scope].md`
- Backfill reports:
  `docs/helix/06-iterate/backfill-reports/BF-YYYY-MM-DD[-scope].md`

## Validation

When changing HELIX wrapper behavior, skill packaging docs, or the execution
contract:

```bash
bash tests/helix-cli.sh
bash tests/validate-skills.sh
git diff --check
```

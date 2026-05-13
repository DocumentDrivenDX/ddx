---
ddx:
  id: helix.workflow.quickstart
  depends_on:
    - helix.workflow
---
# HELIX Workflow Quick Start Guide

Use this guide to start a repo on the current HELIX contract without learning
the whole workflow tree first.

## Start Here

Read these files in order when you need the canonical contract:

1. [README.md](README.md)
2. [EXECUTION.md](EXECUTION.md)
3. `ddx bead --help` (tracker conventions; DDx FEAT-004)

Use the bounded action prompts only when you are doing the corresponding work:

- [implementation.md](actions/implementation.md)
- [check.md](actions/check.md)
- [reconcile-alignment.md](actions/reconcile-alignment.md)
- [backfill-helix-docs.md](actions/backfill-helix-docs.md)

Keep the public layers separate:

- portable skills: `.agents/skills` in the repo and `~/.agents/skills` for the
  installed user package
- HELIX workflow contract: the docs in `.ddx/plugins/helix/workflows/` plus the built-in tracker
  and `helix` CLI

## Bootstrap A Repo

```bash
ddx bead init
ddx install helix
helix doctor --fix
```

Notes:

- `ddx bead init` creates the tracker workspace.
- `ddx install helix` creates `~/.ddx/plugins/helix` and installs skills into
  `~/.agents/skills` and `~/.claude/skills`.
- `helix doctor --fix` verifies and repairs the installation — creates missing
  `.ddx/plugins/helix` symlinks and skill links in the target repo.
- The repo exposes agent skills named `helix-<command>` at `.agents/skills` and
  `.claude/skills` (symlinks to `skills/`).
- For Claude Code: `claude --plugin-dir /path/to/helix` discovers skills
  automatically without manual install.

## Build The Canonical Planning Stack

Prompts and templates live under `.ddx/plugins/helix/workflows/phases/<phase>/artifacts/`.
Use them to draft or refine the canonical docs under `docs/helix/`.

Typical order:

1. Optional discovery in `docs/helix/00-discover/`
   Capture product vision, business case, and opportunity context when needed.
2. Frame in `docs/helix/01-frame/`
   Write the PRD, feature specs, user stories, and supporting requirement docs.
3. Design in `docs/helix/02-design/`
   Define architecture, ADRs, contracts, feature-level solution designs, and
   story-level technical designs.
4. Test in `docs/helix/03-test/` and `tests/`
   Write the test plan and failing tests before implementation.
5. Build in `docs/helix/04-build/` plus the tracker
   Keep project build guidance in docs and story-level execution work in the tracker.
6. Deploy in `docs/helix/05-deploy/` plus the tracker
   Keep rollout docs canonical and rollout tasks in the tracker.
7. Iterate in `docs/helix/06-iterate/`
   Capture backlog, lessons, reviews, and the explicit next-cycle selection.

## Create Execution Work

HELIX execution runs through the built-in tracker, not HELIX-specific task files.

Build, deploy, and iterate execution work should:

- use native tracker issues and dependencies
- carry `helix` plus one phase label
- cite the governing docs with `spec-id` and/or description
- stay small enough to close independently

See `ddx bead --help` for the mapping and examples.

## Run The Queue

Preferred HELIX wrapper commands:

```bash
helix run
helix build
helix check repo
helix align repo
helix backfill repo
```

Execution rules:

- Use `implementation` for one ready execution issue at a time.
- When the ready queue drains, run `check`.
- Run alignment only when the plan exists but the next work set is unclear.
- Run backfill only when the canonical stack is missing or too weak.
- Do not drive the queue with `ddx bead list --ready`.

## Minimal Operator Loop

If you are not using `helix run`, use the bounded manual loop from
[EXECUTION.md](EXECUTION.md):

```bash
while [ "$(ddx bead ready --json | awk 'found || /^[{[]/ { found=1; print }' | ddx jq 'length')" -gt 0 ]; do
  helix build
done

helix check
```

## Common Next Steps

- Need artifact structure or naming rules:
  Read [conventions.md](conventions.md) and the relevant phase README.
- Need queue behavior:
  Read [EXECUTION.md](EXECUTION.md).
- Need tracker labels or issue examples:
  Run `ddx bead --help`.
- Need a top-down audit:
  Run alignment with [reconcile-alignment.md](actions/reconcile-alignment.md).
- Need missing docs reconstructed:
  Run backfill with [backfill-helix-docs.md](actions/backfill-helix-docs.md).

## Validation

When you change HELIX wrapper behavior, skill packaging docs, or the workflow
contract, run:

```bash
bash tests/helix-cli.sh
bash tests/validate-skills.sh
git diff --check
```

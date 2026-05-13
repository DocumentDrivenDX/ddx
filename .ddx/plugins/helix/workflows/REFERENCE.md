---
ddx:
  id: helix.workflow.reference
  depends_on:
    - helix.workflow
---
# HELIX Quick Reference Card

This reference summarizes the runtime-neutral HELIX methodology first. Runtime
commands and DDx-specific queue guidance are collected in the DDx
reference-runtime appendix.

## Canonical Methodology Docs

- [README.md](README.md): high-level model, authority order, runtime boundary,
  and alignment methodology
- `phases/*/artifacts/`: canonical artifact-type catalog, prompts, templates,
  metadata, and examples
- [reconcile-alignment.md](actions/reconcile-alignment.md): top-down review
- [backfill-helix-docs.md](actions/backfill-helix-docs.md): conservative
  reconstruction
- [plan.md](actions/plan.md): iterative design document creation
- [polish.md](actions/polish.md): issue or work-item refinement before
  implementation
- [fresh-eyes-review.md](actions/fresh-eyes-review.md): post-implementation
  review
- [experiment.md](actions/experiment.md): metric-driven optimization iteration
- [metric-definition.yaml](templates/metric-definition.yaml): shared metric
  definitions

## Phase Summary

| Phase | Primary Output | Main Location |
|---|---|---|
| Optional `00-discover` | vision and opportunity framing | `docs/helix/00-discover/` |
| `01-frame` | requirements and stories | `docs/helix/01-frame/` |
| `02-design` | architecture and design contracts | `docs/helix/02-design/` |
| `03-test` | test plans and failing tests | `docs/helix/03-test/`, `tests/` |
| `04-build` | implementation guidance and evidence | `docs/helix/04-build/` |
| `05-deploy` | rollout, monitoring, and recovery evidence | `docs/helix/05-deploy/` |
| `06-iterate` | backlog, reports, metrics, and follow-up planning | `docs/helix/06-iterate/` |

## Authority Order

1. Product Vision
2. Product Requirements
3. Feature Specs / User Stories
4. Architecture / ADRs
5. Solution / Technical Designs
6. Test Plans / Tests
7. Implementation Plans
8. Source Code / Build Artifacts

## Methodology Actions

Use these as capability names regardless of runtime:

- **Intake**: turn sparse user intent into governed artifact updates or bounded
  work items.
- **Frame**: establish product direction, requirements, principles, and stories.
- **Design**: create architecture, decisions, solution designs, and technical
  designs.
- **Test**: define executable acceptance before implementation is considered
  safe.
- **Build**: perform one bounded implementation slice against governing tests and
  designs.
- **Deploy**: release with rollout, monitoring, and recovery evidence.
- **Review**: inspect completed work for correctness, regressions, and missing
  evidence.
- **Align**: reconcile artifacts top-down when direction or evidence diverges.
- **Backfill**: conservatively reconstruct missing documentation from current
  evidence.
- **Iterate**: record measurements, learning, and follow-up planning.

## Decision Guide

- Starting new work or a large scope: frame the intent, design the governing
  artifacts, refine bounded work items, then execute one slice at a time.
- Starting from sparse user intent: use intake to identify affected artifacts and
  create enough context for safe planning or execution.
- Ready execution work exists: execute the next bounded item in the runtime and
  record evidence against its acceptance criteria.
- Work lacks design authority: return to Frame or Design before implementation.
- Specs changed and open work may be stale: refine the affected work items before
  implementation resumes.
- The next safe work item is unclear: run alignment and record a durable
  alignment review.
- Canonical docs are missing or too incomplete to execute safely: run backfill
  and clearly label reconstructed authority.
- Work is blocked or already in progress: stop, report the blocker, and avoid
  creating duplicate execution tracks.
- After implementing an issue: run fresh-eyes review before continuing broad
  execution.

## Artifact Inputs

Use the prompts and templates under:

- `.ddx/plugins/helix/workflows/phases/00-discover/artifacts/`
- `.ddx/plugins/helix/workflows/phases/01-frame/artifacts/`
- `.ddx/plugins/helix/workflows/phases/02-design/artifacts/`
- `.ddx/plugins/helix/workflows/phases/03-test/artifacts/`
- `.ddx/plugins/helix/workflows/phases/04-build/artifacts/`
- `.ddx/plugins/helix/workflows/phases/05-deploy/artifacts/`
- `.ddx/plugins/helix/workflows/phases/06-iterate/artifacts/`

These artifact directories support canonical project docs under `docs/helix/`.
They are the portable HELIX shape; runtime queue mechanics are integration
specific.

## Skill Package Guidance

Portable HELIX skills should use stable capability names and runtime-neutral
arguments where practical. They do not need to mirror any CLI command unless a
specific runtime integration requires that compatibility surface.

Canonical project package path: `./.agents/skills`
Canonical user install path: `~/.agents/skills`
Claude compatibility path: `~/.claude/skills`

Published `SKILL.md` files must include `name` and `description`; include
`argument-hint` when the skill accepts a trailing scope, selector, issue ID, or
goal. A package must include the shared workflow resources its skills reference.

## Output Reports

- Alignment reviews:
  `docs/helix/06-iterate/alignment-reviews/AR-YYYY-MM-DD[-scope].md`
- Backfill reports:
  `docs/helix/06-iterate/backfill-reports/BF-YYYY-MM-DD[-scope].md`

## Validation

When changing methodology docs, validate the affected artifact and projection
paths required by the repository's contribution rules. Runtime-specific tests
belong to the integration that owns the changed behavior.

## DDx Reference-Runtime Appendix

DDx is the current reference runtime for HELIX execution. The commands below are
preserved for DDx-managed repositories, demos, and transitional HELIX wrapper
compatibility. They are not required to understand or adopt the HELIX
methodology.

### Bootstrap

```bash
ddx bead init
ddx install helix
helix doctor --fix
```

Existing DDx installations may publish agent skills that mirror CLI commands:
`helix-<command>` maps to `helix <command>`. This is a legacy compatibility
mapping, not a core rule for portable HELIX skills.

### DDx execution commands

```bash
helix input "natural language request"
ddx agent execute-loop
ddx agent execute-loop --once
ddx agent execute-bead hx-abc123
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

Preferred DDx operator path:

1. Use HELIX intake or issue shaping for sparse intent.
2. Use DDx queue execution for execution-ready work.
3. Use HELIX check, review, align, design, or polish behavior when HELIX must
   interpret or route the next action.

`ddx agent execute-loop` is the primary DDx queue-drain command for
execution-ready beads. `helix run` and `helix build` remain compatibility
surfaces where HELIX still adds supervisory policy or operator convenience.

Execution-ready beads must carry deterministic acceptance and
success-measurement criteria: exact commands, named checks, or observable repo
state that DDx-managed execution can use to decide success without hidden human
interpretation.

### Planning and quality commands

```bash
helix input "natural language request"
helix input "natural language request" --autonomy high
helix design [scope]
helix design --rounds 8 auth
helix polish [scope]
helix polish --rounds 10
helix next
helix review [scope]
helix experiment [issue-id|goal]
helix experiment --close
```

`helix input` is the sparse-intent entrypoint for the autonomy-slider workflow.
`--autonomy` selects the HELIX-owned behavior contract (`low`, `medium`,
`high`); the expected default is `medium` when no override is supplied.

### DDx tracker commands

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

### DDx tracker labeling

Labels are organizational conventions for triage and traceability. They are not
required by every runtime and are not part of the portable HELIX methodology.

Recommended DDx labels:

- `helix` identifies HELIX-managed issues in a DDx tracker.
- Phase labels: `phase:frame`, `phase:design`, `phase:test`, `phase:build`,
  `phase:deploy`, `phase:iterate`, `phase:review`.
- Kind labels: `kind:build`, `kind:deploy`, `kind:backlog`, `kind:review`.
- Traceability labels: `story:US-XXX`, `feature:FEAT-XXX`, `area:<name>`,
  `source:metrics`.

### DDx-specific decision guide

- Starting new work or a large scope: run the HELIX design path, then polish,
  then DDx queue execution.
- Starting from sparse user intent instead of a pre-shaped issue: run HELIX
  intake and set autonomy when needed.
- Ready execution issues exist: use DDx queue execution; use a HELIX wrapper
  when the compatibility routing behavior is still required.
- Work lacks design authority for safe execution: run the HELIX design path, or
  let the compatibility supervisor dispatch it from check.
- Specs changed and open work needs issue refinement before implementation: run
  the HELIX polish path, or let the compatibility supervisor dispatch it from
  check.
- No ready execution issue, but the planning stack exists and next work is
  unclear: run alignment and record the review output.
- Canonical docs are missing or too incomplete to execute safely: run backfill.
- Work exists but is blocked or already in progress: stop and wait.
- The queue drains: run check, not a blind loop and not an ad hoc ready-list
  loop.
- After implementing an issue: run fresh-eyes review.

### DDx validation commands

When changing HELIX wrapper behavior, skill packaging docs, or the DDx execution
contract, the deterministic DDx/HELIX harnesses are:

```bash
bash tests/helix-cli.sh
bash tests/validate-skills.sh
git diff --check
```

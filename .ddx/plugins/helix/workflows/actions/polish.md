# HELIX Action: Polish Issues

You are performing plan decomposition and iterative issue refinement before
implementation begins.

"Decompose the plan, check your issues N times, implement once."

Your goal is to decompose design plans into implementable tracker beads and then
improve issue quality through multiple refinement passes: deduplication,
coverage verification against the plan, acceptance criteria sharpening,
dependency correction, and convergence detection. This front-loaded investment
prevents agents from running off to implement work that hasn't been properly
broken down.

**Polish is the bridge between design and build.** A design plan is not
executable — it must be decomposed into individually implementable beads before
`helix build` can safely execute. If `helix check` routes here, your first
priority is decomposition; refinement follows.

## Action Input

You may receive:

- no argument (default: all open issues)
- a scope such as `auth`, `FEAT-003`, `phase:build`
- `--rounds N` controlling maximum refinement passes (default: 6)

Examples:

- `helix polish`
- `helix polish auth`
- `helix polish --rounds 10 FEAT-003`

## Tracker Rules

Use the built-in tracker only. Follow:

- See `ddx bead --help` for tracker conventions

Issues are stored in `.ddx/beads.jsonl`.

## PHASE 0 - Load Current State

0. **Context Recovery**: Re-read AGENTS.md so project instructions are fresh
   in your working memory. After long sessions, context compaction may have
   dropped critical project rules. This step is cheap insurance against drift.
0a. **Load active design principles** following `.ddx/plugins/helix/workflows/references/principles-resolution.md`.
   Use them as refinement guidance — flag issues whose scope or criteria
   conflict with the active principles.
0b. **Load active concerns and practices** following `.ddx/plugins/helix/workflows/references/concern-resolution.md`.
   Verify issue descriptions and acceptance criteria reference the correct
   concern tools and conventions.
0c. **Refresh context digests**: For each bead in scope that has an existing
   `<context-digest>`, re-assemble per `.ddx/plugins/helix/workflows/references/context-digest.md`
   and update if material changes exist. For beads without a digest, assemble
   one and prepend it. If the repo provides a digest refresh helper, use it so
   area-label inference and digest assembly stay deterministic.
1. Verify the built-in tracker is available.
   - If `ddx bead status` fails, stop immediately.
2. Load all open issues for the scope.
   - `ddx bead list --status open --json`
   - `ddx bead list --status in_progress --json` (if relevant to scope)
3. Load the governing plan document if one exists.
   - Check `docs/helix/02-design/plan-*.md` for the scope
   - Check other planning artifacts (PRD, feature specs, architecture docs)
4. Record initial issue count and state as the baseline.

## PHASE 0.5 - Bead Acquisition

Before modifying any beads, acquire a governing bead for this polish pass.
See `.ddx/plugins/helix/workflows/references/bead-first.md` for the full pattern.

1. Search for an existing open bead governing this work:
   - `ddx bead list --status open --label kind:planning,action:polish --json`
   - Filter by scope if the action was dispatched with a scope.
2. If found, verify it is still relevant and claim it:
   - `ddx bead update <id> --claim`
3. If not found, create one:
   ```bash
   ddx bead create "polish: <scope description>" \
     --type task \
     --labels helix,phase:design,kind:planning,action:polish \
     --set spec-id=<governing-plan-if-known> \
     --description "<context-digest>...</context-digest>
   Decompose plans and refine beads for <scope>.
   Plans to decompose: <list plan docs found in Phase 0>" \
     --acceptance "All plans in scope decomposed into beads; convergence reached (< 3 changes for 2 consecutive rounds); context digests refreshed; concern-appropriate acceptance criteria on all beads"
   ```
4. Record the bead ID. All subsequent bead modifications are governed by this
   bead.

## PHASE 1 - Plan Decomposition

**This phase runs first and is mandatory when a plan exists.** Plans must be
decomposed into tracker beads before refinement or implementation can proceed.

1. Locate the governing plan documents for the scope:
   - `docs/helix/02-design/plan-*.md`
   - `docs/helix/02-design/solution-designs/SD-*.md`
   - Other design artifacts referenced by the scope
2. For each plan, check whether tracker beads already exist that reference it
   (via `spec-id`, description, or parent epic).
3. If the plan has **not been decomposed** (no or very few corresponding beads):
   a. Read the plan's "Implementation Plan with Dependency Ordering" section
      (or equivalent work breakdown).
   b. Create one bead per implementable slice. Each bead must:
      - be individually completable in one `helix build` cycle
      - have `--labels helix,phase:build` (plus area labels)
      - set `spec-id` with `--set spec-id=<governing-plan-or-design-artifact>`
      - have deterministic acceptance criteria derived from the plan
      - have a `<context-digest>` assembled per
        `.ddx/plugins/helix/workflows/references/context-digest.md`
   c. Group related beads under an epic if the plan implies multiple
      implementation tracks.
   d. Wire dependencies with `ddx bead dep add` based on the plan's
      dependency graph.
4. If the plan has been partially decomposed, create beads only for uncovered
   sections — do not duplicate existing beads.

Only after decomposition is complete (or confirmed already done) should
refinement passes begin.

## PHASE 2 through N - Refinement Passes

Each pass performs ALL of the following checks. Track changes made per pass.

### Deduplication

- Find issues with overlapping scope, description, or acceptance criteria.
- Merge duplicates into a single canonical issue that preserves the strongest
  elements of each.
- When merging, preserve all dependency relationships from both issues.
- Close the redundant issue with a note pointing to the canonical one.

### Plan Coverage Verification

- If a plan document exists, verify every plan section has at least one issue
  (decomposition should have handled this, but coverage verification catches
  gaps).
- If a section has no issue, create one with proper labels, spec-id, and
  acceptance criteria derived from the plan.
- If an issue exists but doesn't map to any plan section, flag it for review.

### Acceptance Criteria Sharpening

- Replace vague criteria with testable statements.
  - Bad: "auth should work correctly"
  - Good: "login with valid credentials returns 200 and a JWT; login with
    invalid credentials returns 401 with error code AUTH_INVALID"
- Ensure every issue has at least one concrete acceptance criterion.
- Add verification method: what command or test proves this criterion is met?
- For execution-ready beads (`phase:build`, `phase:deploy`, `phase:iterate`
  implementation work), require acceptance text to name at least one explicit:
  - command to run
  - named check or execution doc
  - observable repository state, file, field, or tracker condition
- Treat "works", "correct", "complete", "aligned", or similar adjectives
  without a named check as non-measurable acceptance text.
- If the governing artifacts let you sharpen the bead, rewrite the acceptance
  criteria immediately.
- If the governing artifacts do **not** let you sharpen the bead into explicit
  measurement criteria, flag the bead as **not execution-ready** and route it
  back through planning/polish refinement instead of leaving hidden wrapper
  knowledge to decide success.
- A bead is not execute-loop-ready until DDx-managed execution could determine
  success from the bead contract itself without a human inferring what "done"
  means from HELIX wrapper behavior.

### Dependency Verification

- Verify dependency chains are correct: if issue A depends on issue B, does B
  actually produce what A needs?
- Look for missing dependencies: does this issue assume work that hasn't been
  captured as a dependency?
- Look for circular dependencies and break them.
- Verify `spec-id` points to the correct governing artifact.

### Sizing

- Flag issues that are too large for a single bounded implementation pass.
  An issue is too large if it touches more than 3 files in different subsystems
  or requires more than one architectural decision.
- Split oversized issues into smaller, independently verifiable slices.
- Preserve the dependency ordering when splitting.

### Label Hygiene

- Ensure every issue has the `helix` label.
- Ensure every issue has exactly one phase label (`phase:build`, `phase:deploy`,
  or `phase:iterate`).
- Ensure area labels are present where applicable.
- Ensure `kind:*` labels match the issue's actual type.

### Area Label Enforcement for Concern Matching

Area labels are required for concern filtering to work. For each bead in scope:

1. If the bead has no `area:*` labels, infer the correct area from:
   - The `spec-id` and its governing artifact's scope
   - The files or subsystems referenced in the description
   - The parent epic's area labels (if the parent has them)
2. Assign the inferred `area:*` label(s) using `ddx bead update <id> --labels`.
3. If the area is genuinely ambiguous, prefer the more inclusive label or
   assign multiple labels. A bead touching both API and CLI should have both
   `area:api` and `area:cli`.
4. Beads that touch all areas (e.g., CI config, cross-cutting refactors) may
   omit area labels — they will match only `areas: all` concerns, which is
   correct.
5. Area labels are routing metadata, not digest content. Refresh the
   `<concerns>` element from matched concern names after relabeling; do not
   leave stale area names in the digest.

### Concern-Aware Acceptance Criteria

For each bead in scope, verify acceptance criteria reference the correct
concern tools:

- A bead in a `typescript-bun` project should reference `bun:test`, not
  `vitest` or `jest`, in its test-related acceptance criteria.
- A bead in a `rust-cargo` project should reference `cargo clippy` and
  `cargo deny`, not ad-hoc lint approaches.
- If acceptance criteria reference tools inconsistent with declared concerns,
  update them.

### Concern Propagation Verification

For each active concern, verify end-to-end threading across all beads in scope:

1. **Digest coverage**: Every bead with a matching area label must have a
   `<context-digest>` that includes the concern. If missing, assemble one.
2. **Acceptance criteria coverage**: Every bead touching a concern's area must
   have at least one acceptance criterion that references the concern's quality
   gate or practice. For example:
   - A `typescript-bun` bead must reference `bun:test` or `biome check`
   - A `security-owasp` bead must reference input validation or dependency audit
   - A `rust-cargo` bead must reference `cargo clippy` or `cargo deny`
3. **Tool consistency**: Flag any bead whose acceptance criteria reference tools
   inconsistent with declared concerns (e.g., `vitest` in a `bun:test` project).
4. **New concern detection**: If concerns changed since beads were created (check
   git log on `.ddx/plugins/helix/workflows/concerns/` and `docs/helix/01-frame/concerns.md`),
   propagate the change to all affected beads — update both digests and
   acceptance criteria.

## Convergence Detection

Track a change count per round: number of issues modified, created, or merged.
Decomposition (Phase 1) does not count toward convergence — it is a one-time
setup step, not an iterative pass.

When change count drops below 3 for two consecutive refinement rounds, declare
convergence and stop refinement.

If max rounds is reached without convergence, report the current state and
recommend additional rounds or user guidance.

## PHASE N+1 - Measure

Verify the polish pass against the governing bead's acceptance criteria.
See `.ddx/plugins/helix/workflows/references/measure.md` for the full pattern.

1. **Decomposition completeness**: All plans in scope have corresponding beads.
2. **Convergence**: Change velocity dropped below threshold.
3. **Concern threading**: All beads in scope have concern-appropriate context
   digests and acceptance criteria.
4. **Dependency integrity**: No circular dependencies; all `spec-id` references
   resolve to existing artifacts.
5. **Record results** on the governing bead:
   `ddx bead update <id> --notes "<measure-results>...</measure-results>"`

## PHASE N+2 - Report

Close the polish cycle and feed back into the planning helix.
See `.ddx/plugins/helix/workflows/references/report.md` for the full pattern.

1. If measurement passed, close the governing bead with evidence summary.
2. If measurement identified gaps, create follow-on beads for:
   - Beads that still lack concern coverage
   - Plans that could not be fully decomposed (need guidance)
   - Dependency issues that need resolution
3. The polished beads are now ready for `helix build` to claim and execute.

## Output

Report a summary of all modifications made across rounds, then these trailer
lines:

```
POLISH_STATUS: CONVERGED|IN_PROGRESS
DECOMPOSITION: YES|NO|PARTIAL
POLISH_ROUNDS: N
ISSUES_DECOMPOSED: count (from plan decomposition)
ISSUES_MODIFIED: count
ISSUES_CREATED: count (from refinement, not decomposition)
ISSUES_MERGED: count
MEASURE_STATUS: PASS|FAIL|PARTIAL
BEAD_ID: <governing-bead-id>
FOLLOW_ON_CREATED: N
```

- `CONVERGED`: change velocity dropped below threshold
- `IN_PROGRESS`: max rounds reached but velocity still above threshold
- `DECOMPOSITION: YES`: plan was decomposed into beads in this run
- `DECOMPOSITION: NO`: no plan found or plan was already decomposed
- `DECOMPOSITION: PARTIAL`: plan partially decomposed, some sections could not
  be broken down without guidance

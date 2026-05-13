# HELIX Action: Reconcile Alignment

You are performing an iterative top-down reconciliation review of a HELIX
project.

Your goal is to re-align the implementation with the authoritative planning
stack, identify explicit divergence, determine whether additional execution
work remains for the reviewed scope, and produce deterministic next steps using
the built-in tracker (`ddx bead`).

This action is read-only with respect to product code unless explicitly told to
make fixes. It may create or update:

- review issues in the tracker
- execution issues in the tracker
- one durable alignment report in `docs/helix/06-iterate/alignment-reviews/`

## Action Input

You will receive a review scope as an argument, for example:

- `helix align repo`
- `helix align auth`
- `helix align FEAT-003`
- `helix align US-042`

If no scope is given, default to the repository.

## Authority Order

When artifacts disagree, use this authority order:

1. Product Vision
2. Product Requirements
3. Feature Specs / User Stories
4. Architecture / ADRs
5. Solution Designs / Technical Designs
6. Test Plans / Tests
7. Implementation Plans
8. Source Code / Build Artifacts

Rules:

- Higher layers govern lower layers.
- Tests govern build execution but do not override requirements or design.
- Source code reflects current state but does not redefine the plan.
- If a higher layer is missing or contradictory, do not infer intent from lower layers.
- Prefer aligning code to plan. Propose plan changes only when strongly justified.

## Tracker Rules

Use the built-in tracker only. Follow:

- See `ddx bead --help` for tracker conventions

Issues are stored in `.ddx/beads.jsonl`.

### Review Structure

Use two issue categories:

1. Review epic
   - `type: epic`
   - labels: `helix`, `phase:review`, `kind:review`
   - title pattern: `HELIX alignment review: <scope>`

2. Review issues
   - `type: task`
   - parented to the review epic
   - labels: `helix`, `phase:review`, `kind:review`, plus area labels

Only after consolidation, create execution issues for approved follow-up work.
Execution issues must use tracker IDs, `parent`, `deps`, `spec-id`, and
HELIX labels appropriate to the work phase.

## PHASE 0 - Review Bootstrap

0. **Context Recovery**: Re-read AGENTS.md so project instructions are fresh
   in your working memory. After long sessions, context compaction may have
   dropped critical project rules. This step is cheap insurance against drift.
0a. **Load active design principles** following `.ddx/plugins/helix/workflows/references/principles-resolution.md`.
   Use them as alignment criteria — flag artifacts whose design choices deviate
   from the active principles.
0b. **Load active concerns and practices** following `.ddx/plugins/helix/workflows/references/concern-resolution.md`.
   Concern drift is an alignment finding at every layer, not just implementation:
   - Flag **implementation** that uses tools or conventions inconsistent with the
     declared concerns (e.g., using `vitest` when `bun:test` is declared).
   - Flag **planning artifacts** (PRD technical context, design docs, test plans)
     that reference tools, frameworks, or conventions contradicted by the active
     concerns (e.g., a design doc specifying ESLint when the concern declares Biome,
     or a test plan referencing Jest when the concern declares `bun:test`).
   - Flag **ADRs** that contradict current concern selections.
   - Flag **missing concern coverage**: if the project uses a technology that has
     a library concern but that concern is not declared in `concerns.md`, note it.
   - For each concern, load its `practices.md` and check that the project's
     actual tooling matches each practice category (linter, formatter, test
     framework, build tool, dependency audit). Report specific mismatches.
0c. **Verify context digest freshness**: For beads in scope, check whether
   `<context-digest>` blocks reflect current upstream state. Stale digests are
   an alignment finding.
1. Verify the built-in tracker is available.
   - If `ddx bead status` fails, stop immediately.
2. Determine the review scope.
3. Break the scope into functional areas.
4. Reconcile any existing review epic and review issues for the same scope.
   - Reuse and update existing review work where possible.
   - Mark stale review work as closed, superseded, or split as appropriate.
5. Create or update:
   - one review epic for the run
   - one review issue per functional area
6. Record the epic ID and review issue IDs in the alignment report.
7. **Note deduplication rule**: Before appending notes to any review issue
   (existing or new), read the issue's current `notes` field. If the new
   note is substantively identical to an existing note (same proof lane,
   same scope, same verification outcome), **do not append** — the existing
   note already records the evidence. Only append when the verification
   state has materially changed (new test results, different artifact state,
   changed finding). This prevents repeated alignment reruns from bloating
   tracker records with duplicate evidence paragraphs.

## PHASE 0.5 - Bead Acquisition

Before modifying any tracker issues or writing reports, acquire a governing
bead for this alignment pass. See `.ddx/plugins/helix/workflows/references/bead-first.md` for
the full pattern.

1. Search for an existing open bead governing this work:
   - `ddx bead list --status open --label kind:planning,action:align --json`
   - Filter by scope if the action was dispatched with a scope.
2. If found, verify it is still relevant and claim it:
   - `ddx bead update <id> --claim`
3. If not found, create one:
   ```bash
   ddx bead create "align: <scope description>" \
     --type task \
     --labels helix,phase:review,kind:planning,action:align \
     --set spec-id=<governing-artifact-if-known> \
     --description "<context-digest>...</context-digest>
   Top-down reconciliation review for <scope>.
   Scope areas: <list functional areas>" \
     --acceptance "Alignment review complete; all gaps classified; execution issues created for real gaps; traceability matrix produced"
   ```
4. Record the bead ID. All subsequent tracker modifications and report writes
   are governed by this bead.
5. If this action was reached through a skill or CLI entrypoint, treat that
   entrypoint as a thin launcher only. The governing bead and stored prompt are
   the durable contract.
6. Whenever this action creates a new bead or materially updates an existing
   bead description, assemble or refresh its `<context-digest>` per
   `.ddx/plugins/helix/workflows/references/context-digest.md`. If the repo ships
   `scripts/refresh_context_digests.py`, use it after the mutation so digests
   and `area:*` labels stay deterministic.

## PHASE 1 - Reconstruct Intent

Using planning artifacts only, summarize:

- product vision
- product requirements
- feature specs
- user stories
- architecture decisions / ADRs
- solution designs
- technical designs
- test plans
- implementation plans

Do not use source code to fill planning gaps in this phase.

## PHASE 2 - Planning Stack Consistency

Validate traceability as a dependency graph, not a forced linear chain.

Check for:

- Vision -> Requirements
- Requirements -> Feature Specs / User Stories
- Requirements / Feature Specs -> Architecture
- Architecture / User Stories / Feature Specs -> Solution or Technical Designs
- Technical Designs / Test Plans -> Implementation Plans
- Specs / Stories / Designs -> Tests

Identify:

- contradictions
- missing links
- underspecified areas
- stale artifacts
- same-layer conflicts
- downstream artifacts that no longer match upstream authority
- concern-artifact inconsistencies: planning artifacts that specify tools,
  frameworks, quality gates, or conventions that conflict with the active
  concerns and their merged practices

## PHASE 3 - Implementation Review

Inspect implementation and map it to the planning stack.

Identify:

- workspace/package/module topology
- runtime entry points
- public interfaces
- tests
- feature flags and config switches
- build and deploy surfaces
- major unplanned code paths
- dead or orphaned implementations

### Concern Drift Detection

For each active concern, verify the implementation matches its declared
practices:

1. **Tech-stack concerns** (rust-cargo, typescript-bun, python-uv, go-std,
   scala-sbt): Check that the actual linter, formatter, test runner, build
   tool, and dependency manager match what the concern and project overrides
   declare. Check for drift signals listed in the concern's `## Drift Signals`
   section (if present). Report each mismatch with the specific file evidence.
2. **Infrastructure concerns** (k8s-kind): Check that deployment manifests,
   local dev setup, and service discovery patterns match the declared approach.
3. **Quality concerns** (security-owasp, o11y-otel, a11y-wcag-aa): Check that
   the concern's quality gates are present in CI or pre-commit configuration.
   Verify dependency audit commands are wired. Check that practices like
   parameterized queries, input validation, and secret management are followed.
4. **Concern-specific quality gates**: For each concern that declares quality
   gate commands (e.g., `cargo deny check advisories`, `govulncheck`,
   `bun audit`), verify those commands are present in CI configuration or
   pre-commit hooks. Missing gates are alignment findings.

### Acceptance Criteria Validation

For each user story and feature spec in the reviewed scope:

1. Extract acceptance criteria from the governing artifact.
2. For each criterion, determine whether:
   - a test exists that exercises the criterion
   - the test passes
   - the implementation satisfies the criterion based on code inspection
3. Classify each criterion as:
   - **SATISFIED** — test exists, passes, and implementation matches
   - **TESTED_NOT_PASSING** — test exists but fails
   - **UNTESTED** — no test covers this criterion
   - **UNIMPLEMENTED** — no implementation addresses this criterion
4. Record results in the Gap Register with the governing artifact as planning
   evidence and the test or code file as implementation evidence.
5. If the project has adopted an acceptance criteria ratchet
   (see `.ddx/plugins/helix/workflows/ratchets.md`), compare the current satisfaction
   count against the committed floor. Flag any regression — a decrease in
   SATISFIED criteria that was not accompanied by a floor override.

## PHASE 4 - Gap Classification

For each relevant area, assign exactly one classification:

- ALIGNED
- INCOMPLETE
- DIVERGENT
- UNDERSPECIFIED
- STALE_PLAN
- BLOCKED

Each classification must include:

- planning evidence
- implementation evidence
- explanation
- default resolution direction: `code-to-plan`, `plan-to-code`, or `decision-needed`
- owning review issue ID

### Quality Evaluation

For each area classified as ALIGNED or INCOMPLETE, evaluate:

- **Robustness** — does the implementation handle edge cases, errors, and
  degraded inputs as specified? Are failure modes defined in the design and
  tested?
- **Maintainability** — is the implementation structured for change? Are
  boundaries clean, dependencies explicit, and coupling proportional to
  cohesion?
- **Performance** — are performance constraints from requirements or design
  met or testable? Are there obvious scalability risks unaddressed by the
  planning stack?

Quality concerns do not change the gap classification. Instead, record them as
supplementary findings in the Gap Register with resolution direction
`quality-improvement` and create backlog-type execution issues in Phase 7 when
warranted.

## PHASE 5 - Traceability Matrix

Produce a matrix with:

- Vision item
- Requirement
- Feature Spec / User Story
- Architecture / ADR reference
- Solution / Technical Design reference
- Test reference
- Implementation Plan reference
- Code status
- Classification

## PHASE 6 - Consolidated Report

Create or update the durable report at:

- `docs/helix/06-iterate/alignment-reviews/AR-YYYY-MM-DD[-scope].md`

Use the template at:

- `.ddx/plugins/helix/workflows/templates/alignment-review.md`

The report must consolidate all review issues into one coherent repo artifact.
It is the durable output of the review run.

## PHASE 7 - Execution Issues

After consolidation, create or update deterministic execution issues only for
real gaps that require follow-up work.

Execution issue rules:

- one coherent gap per issue
- use native upstream types such as `task`, `chore`, or `decision`
- assign HELIX phase/kind labels that match the actual work
- set `spec-id` to the nearest governing canonical artifact
- link to the source review issue using description, parenting, or `discovered-from` dependencies
- when several execution issues belong to one larger fix, create or reuse an
  epic parent instead of leaving them as flat siblings
- when one issue must land before another can run safely, encode that with
  `ddx bead dep add` rather than prose in the description
- if a gap first requires design/doc/policy work and only then code changes,
  create the upstream planning bead first and block the downstream
  implementation bead on it
- do not close the governing alignment bead while actionable findings still
  exist only as prose in the report
- add explicit blockers with `ddx bead dep add`
- if canonical docs must change before implementation, create the doc/design issue before the code issue
- do not create duplicate issues for the same gap
- after creating or materially updating an execution issue, assemble or refresh
  its `<context-digest>`; if the repo ships `scripts/refresh_context_digests.py`,
  use it instead of hand-editing digest XML

### Issue Coverage Verification

After creating execution issues, verify completeness:

1. For every gap in the Gap Register that is not ALIGNED, confirm at least one
   execution issue exists that addresses it.
2. For every acceptance criterion classified as UNTESTED or UNIMPLEMENTED,
   confirm at least one execution issue exists that would resolve it.
3. For every quality concern recorded, confirm either an execution issue exists
   or the concern is explicitly deferred with rationale.

If coverage gaps remain, create the missing execution issues before proceeding.
The issue set must fully represent the work required to move from current state
to the end state defined by the planning stack.

If a ratchet regression was detected in Phase 3, create a regression issue that
references the specific criteria or metrics that dropped below the floor. The
issue must include the previous floor value, the current measured value, and the
governing artifact where the regression is visible.

## PHASE 8 - Execution Order

Output:

- dependency chain
- critical path
- parallelizable execution issues
- blockers
- first recommended execution set
- queue health and exhaustion assessment for the reviewed scope

## Evidence Requirements

Every non-trivial claim must cite:

- planning evidence with file path and line reference where practical
- implementation evidence with file path and line reference where practical

Be explicit about inference when a conclusion is not directly stated by the
artifacts.

## PHASE 9 - Measure

Verify the alignment review against the governing bead's acceptance criteria.
See `.ddx/plugins/helix/workflows/references/measure.md` for the full pattern.

1. **Completeness**: All functional areas in scope have a gap classification.
2. **Traceability**: The traceability matrix covers all in-scope artifacts.
3. **Issue coverage**: Every non-ALIGNED gap has at least one execution issue.
4. **Concern drift**: All concern drift findings are recorded and have
   corresponding execution issues.
5. **Record results** on the governing bead:
   `ddx bead update <id> --notes "<measure-results>...</measure-results>"`

## PHASE 10 - Report

Close the alignment cycle and feed back into the planning helix.
See `.ddx/plugins/helix/workflows/references/report.md` for the full pattern.

1. If measurement passed, close the governing bead with evidence summary.
2. If measurement identified gaps in the review itself (incomplete areas,
   missing traceability), create follow-on beads.
3. The execution issues created in Phase 7 are the primary output — they
   re-enter the planning helix for polish and then build.

## Output Format

Produce these sections in order:

1. Review Metadata
2. Scope and Governing Artifacts
3. Intent Summary
4. Planning Stack Findings
5. Implementation Map
6. Acceptance Criteria Status
7. Gap Register (with Quality Findings)
8. Traceability Matrix
9. Review Issue Summary
10. Execution Issues Generated
11. Issue Coverage Verification
12. Execution Order
13. Open Decisions
14. Queue Health and Exhaustion Assessment
15. Measurement Results
16. Follow-On Beads Created

Then emit the machine-readable trailer:

```
ALIGN_STATUS: COMPLETE|INCOMPLETE|BLOCKED
GAPS_FOUND: N
EXECUTION_ISSUES_CREATED: N
MEASURE_STATUS: PASS|FAIL|PARTIAL
BEAD_ID: <governing-bead-id>
FOLLOW_ON_CREATED: N
```

Be precise, deterministic, and evidence-driven.

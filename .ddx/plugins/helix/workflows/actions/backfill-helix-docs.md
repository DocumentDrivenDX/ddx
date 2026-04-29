# HELIX Action: Backfill HELIX Documentation

You are performing a research-first backfill of HELIX documentation for an
existing project.

Your goal is to reconstruct missing or incomplete HELIX artifacts from current
evidence, ask the user for guidance when ambiguity is material, and create
canonical HELIX docs without canonizing guesses as fact.

This action may create or update:

- canonical HELIX artifacts under `docs/helix/`
- research and review issues in the tracker
- follow-up execution issues in the tracker
- one durable backfill report in `docs/helix/06-iterate/backfill-reports/`

## Action Input

You will receive a backfill scope as an argument, for example:

- `helix backfill repo`
- `helix backfill payments`
- `helix backfill FEAT-003`
- `helix backfill auth`

If no scope is given, default to the repository.

## Core Distinction

This action is not the same as alignment review.

- `reconcile-alignment` audits an existing planning stack against implementation
- `backfill-helix-docs` reconstructs missing HELIX artifacts from incomplete evidence

Backfill must escalate ambiguity earlier than alignment review. Where confidence
is low, ask the user before finalizing canonical artifacts.

## Execution Assumptions

When this action is launched by `helix backfill`, assume you are running inside
an active writable session rooted at the target repository.

- use live `ddx bead` commands for tracker state
- write directly to `docs/helix/` when evidence supports canonical updates
- do not claim that you need a different session, different permissions, or a
  separate environment unless a concrete command actually fails

If a command fails, report the exact command and the observed error. Do not
invent capability limits.

## Authority and Evidence Rules

When authority exists, use this order:

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
- Existing code and tests are evidence of current behavior, not automatic authority.
- Recent commits, changelogs, CI, and runbooks are supporting evidence, not substitutes for requirements.
- Never write low-confidence canonical requirements or design statements as settled fact.
- If a deliberate canonical artifact already exists, refine it carefully instead of replacing it from code evidence alone.

## Confidence Model

Every inferred claim must carry one confidence level:

- `HIGH`: directly supported by multiple authoritative or near-authoritative sources
- `MEDIUM`: strong inference from several corroborating sources
- `LOW`: plausible reconstruction that requires user confirmation

Low-confidence claims must either:

- be confirmed by the user before canonization, or
- remain explicitly marked as unresolved in the backfill report and draft artifacts

## Tracker Rules

Use the built-in tracker only. Follow:

- See `ddx bead --help` for tracker conventions

Issues are stored in `.ddx/beads.jsonl`.

Use live tracker commands such as `ddx bead ready`, `ddx bead show`,
`ddx bead list` as needed for queue state. If `ddx bead status`
fails, stop immediately.

### Research Structure

Use a review-style research structure:

1. Research epic
   - native `type: epic`
   - labels: `helix`, `phase:review`, `kind:review`
   - title pattern: `HELIX docs backfill: <scope>`

2. Research issues
   - `type: task`
   - parented to the research epic
   - labels: `helix`, `phase:review`, `kind:review`, plus area labels

3. Follow-up execution issues
   - created only after the backfill report exists
   - use tracker IDs, `deps`, `spec-id`, and HELIX labels

### Recursive Review Model

Backfill must use an explicit fan-out and fan-in review tree.

Default decomposition:

1. repository or scope root
2. top-level domain or subsystem
3. folder-level review
4. leaf file-set review
5. parent consolidation
6. scope-level consolidation

Rules:

- every folder in scope must be assigned to a review node
- every file in scope must be either:
  - reviewed directly, or
  - explicitly excluded with a reason
- if a folder is too large or heterogeneous for one thorough pass, split it again
- do not stop at top-level summaries; recurse until leaf review scopes are small enough to inspect thoroughly
- parent nodes cannot be considered complete until child nodes have been reviewed and synthesized
- if multiple agents are available, leaf review issues may be parallelized, but the same consolidation protocol still applies

No special orchestration technology is required beyond the explicit review tree
and tracker hierarchy. Multi-agent execution is optional; staged review
and consolidation are mandatory.

## Multi-Stage Review and Consolidation Protocol

Use these stages in order:

1. Inventory pass
   - enumerate folders and files recursively
   - create the review tree and coverage ledger
2. Leaf evidence extraction pass
   - inspect every leaf file-set
   - record evidence, confidence, and open questions
3. Folder synthesis pass
   - aggregate leaf findings upward
   - identify local contradictions and missing docs
4. Domain synthesis pass
   - merge folder findings into domain-level current-state narratives
5. Canonical drafting pass
   - backfill HELIX artifacts from highest authority downward
6. Global consistency pass
   - verify cross-domain terminology, traceability, and unresolved guidance gates

Do not draft canonical docs before the inventory pass and at least one leaf
evidence extraction pass are complete for the relevant scope.

## PHASE 0 - Scope and Baseline

0. **Context Recovery**: Re-read AGENTS.md so project instructions are fresh
   in your working memory. After long sessions, context compaction may have
   dropped critical project rules. This step is cheap insurance against drift.
0a. **Discover active concerns**: Check whether `docs/helix/01-frame/concerns.md`
   exists. If it does, load it per `.ddx/plugins/helix/workflows/references/concern-resolution.md`
   and use the declared concerns to guide backfill — ensure backfilled artifacts
   are consistent with declared technology choices and practices.
   If it does not exist, inspect the project's actual tooling (package managers,
   linters, formatters, test frameworks, CI config) and match them against
   available concerns in `.ddx/plugins/helix/workflows/concerns/`. Include `concerns.md` creation
   as part of the backfill output — propose active concerns and project overrides
   based on the evidence discovered during Phase 1 research. Concern selection
   requires user confirmation before canonization (treat as a guidance gate).
1. Determine the backfill scope.
2. Verify the built-in tracker is available.
   - If `ddx bead status` fails, stop immediately.
   - Use `ddx bead` output as the authoritative queue source for the run.
3. Inventory existing documentation:
   - `docs/helix/`
   - non-HELIX docs
   - ADRs, runbooks, changelogs, readmes
   - tests and CI configuration
   - major code entry points and configuration surfaces
4. Enumerate the scope recursively down to folder and file-set level.
5. Break the scope into functional areas.
6. Build the review tree:
   - research epic
   - area review issues
   - folder review issues
   - leaf file-set review issues
   - consolidation issues where needed
7. Create a coverage ledger so every folder and file in scope has a review owner or an explicit exclusion.
8. Reconcile or create:
   - one research epic for the run
   - one research issue per functional area
   - child review issues for folders and file-sets as required
9. Record the epic ID, issue IDs, and coverage baseline in the backfill report.

## Completion Contract

Do not stop at an analysis-only summary if the repository is writable and live
`ddx bead` commands succeed.

Before returning, you must do all applicable work that is supported by the
available evidence:

1. create or update the research epic and review issues in the tracker
2. create or update the durable backfill report under
   `docs/helix/06-iterate/backfill-reports/`
3. create or update any high-confidence canonical HELIX artifacts justified by
   the scope
4. record unresolved ambiguity in the report instead of silently stopping

The only acceptable non-complete outcomes are:

- `GUIDANCE_NEEDED`: guidance is required before low-confidence canonization,
  but the durable backfill report has still been written or updated
- `BLOCKED`: a concrete command failed and prevented normal completion; cite the
  exact failing command and error, and write the report if the filesystem still
  allows it

Never end by telling the user to rerun the action in a different session unless
you actually attempted the command that failed.

## PHASE 0.5 - Bead Acquisition

Before modifying any files or creating tracker issues, acquire a governing
bead for this backfill pass. See `.ddx/plugins/helix/workflows/references/bead-first.md` for
the full pattern.

1. Search for an existing open bead governing this work:
   - `ddx bead list --status open --label kind:planning,action:backfill --json`
   - Filter by scope if the action was dispatched with a scope.
2. If found, verify it is still relevant and claim it:
   - `ddx bead update <id> --claim`
3. If not found, create one:
   ```bash
   ddx bead create "backfill: <scope description>" \
     --type task \
     --labels helix,kind:planning,action:backfill \
     --description "<context-digest>...</context-digest>
   Reconstruct missing HELIX artifacts for <scope>.
   Existing coverage: <summary from Phase 0 inventory>" \
     --acceptance "Missing artifacts reconstructed with evidence; assumption ledger complete; follow-up issues created for guidance-dependent items"
   ```
4. Record the bead ID. All subsequent file modifications and tracker changes
   are governed by this bead.

## PHASE 1 - Current-State Research

Research the current system aggressively before drafting docs.

Use:

- existing docs and readmes
- tests and fixtures
- CI, deployment, and ops config
- package/workspace/module layout
- code entry points and public interfaces
- recent commit history when useful
- changelogs and release notes when present

At minimum, review recursively:

- every documentation subtree in scope
- every test subtree in scope
- every source subtree in scope
- every config, CI, and operations subtree in scope that affects behavior or delivery

Produce:

- current behavior summary
- current architecture summary
- operational constraints summary
- evidence map by area
- leaf review notes for every reviewed file-set
- folder/domain consolidation notes for every non-leaf review node

At this phase, separate:

- direct evidence
- strong inference
- speculation that needs confirmation

Do not write canonical artifacts yet unless confidence is already high.

## PHASE 2 - Canonical Mapping

Map evidence into the HELIX artifact structure.

Identify:

- which canonical artifacts already exist and can be preserved
- which artifacts are missing and should be created
- which artifacts are stale and should be revised
- which artifacts cannot be backfilled safely without user guidance

Backfill highest-authority missing artifacts first:

1. vision / product framing
2. requirements
3. feature specs / user stories
4. architecture / ADRs
5. design docs
6. test plans
7. implementation plans

Do not mark a domain as ready for drafting until:

- its descendant review nodes are complete or explicitly deferred
- its coverage ledger is complete
- its local contradictions and ambiguities are logged

## PHASE 3 - Guidance Gates

Ask the user for guidance after local research is exhausted and before writing
low-confidence canonical content.

Trigger a guidance gate when:

- multiple plausible product intents exist
- implementation and existing docs diverge materially
- scope boundaries are unclear
- naming is inconsistent enough to affect the document structure
- behavior is visible in code but its intended requirement is unclear
- architectural tradeoffs are apparent but not documented
- tests encode behavior that may be accidental or obsolete

When asking, provide:

- the ambiguity
- the evidence
- the default interpretation you would use
- the exact decision needed

Keep questions short and grouped by decision area.

If a guidance gate affects a whole subtree, pause drafting for that subtree and
continue elsewhere where confidence is sufficient.

## PHASE 4 - Draft Backfill Artifacts

Create or update canonical HELIX artifacts from highest authority downward.

Drafting rules:

- write only what is supported by evidence or confirmed by the user
- mark inferred sections explicitly when confidence is medium
- never silently convert low-confidence inference into canonical truth
- use placeholders such as `[NEEDS GUIDANCE]` for unresolved items
- preserve existing intentional content when compatible with evidence
- prefer revision over replacement when an artifact already exists

Expected outputs may include:

- `docs/helix/00-discover/product-vision.md`
- `docs/helix/01-frame/prd.md`
- `docs/helix/01-frame/concerns.md` (if missing — propose based on evidence)
- `docs/helix/01-frame/features/*.md`
- `docs/helix/01-frame/user-stories/*.md`
- `docs/helix/02-design/architecture.md`
- `docs/helix/02-design/adr/*.md`
- `docs/helix/03-test/test-plan.md`
- `docs/helix/04-build/implementation-plan.md`

Only create the artifacts justified by the scope and evidence.

When backfilling `concerns.md`, use the evidence from Phase 1 research to:

1. Match the project's observed tooling to library concerns.
2. Document project-specific overrides for any deviations from the library
   defaults (e.g., older MSRV, alternative HTTP framework, non-default linters).
3. Define area labels based on the project's actual module/package structure.
4. Gate the final `concerns.md` on user confirmation — concern selection is
   a design decision, not a backfill inference.

Where scope is large, draft in the same order as the consolidation tree:

- leaf/domain evidence first
- then area summaries
- then cross-area canonical artifacts

## PHASE 5 - Assumption Ledger

Record every inferred or unresolved item in the backfill report.

For each item include:

- statement
- confidence
- evidence
- affected artifact(s)
- confirmation status
- next action

If a low-confidence item blocks canonical drafting, stop and ask the user.

## PHASE 6 - Consistency Pass

After drafting:

- verify traceability across newly created artifacts
- ensure terminology is consistent
- ensure tests and code are cited as evidence, not as authority
- check that placeholders and unresolved items are visible
- verify no backfilled artifact contradicts higher-order artifacts
- verify every scope node in the coverage ledger is closed, deferred, or excluded with reason

## Coverage and Saturation Criteria

Backfill is not complete until:

- every folder in scope has been reviewed or explicitly excluded
- every file in scope has been reviewed directly or represented by a reviewed leaf file-set
- every review node has a parent consolidation path back to the scope root
- no top-level domain remains without a synthesized current-state summary
- no canonical artifact was drafted from unresolved low-confidence assumptions
- all remaining ambiguity is captured as guidance gates or follow-up issues

## PHASE 7 - Durable Backfill Report

Create or update the durable report at:

- `docs/helix/06-iterate/backfill-reports/BF-YYYY-MM-DD[-scope].md`

Use the template at:

- `.ddx/plugins/helix/workflows/templates/backfill-report.md`

The report must capture:

- evidence surveyed
- recursive scope coverage
- confidence levels
- assumptions and open questions
- artifacts created or updated
- required user guidance
- follow-up work

## PHASE 8 - Follow-Up Issues

Create or update follow-up execution issues only after the backfill report
exists.

Use follow-up issues for:

- unresolved guidance-dependent doc updates
- tests or implementation that should be aligned after backfill
- explicit stale-plan remediation
- decision work that needs stakeholder input

Rules:

- one coherent gap per issue
- use native upstream types such as `task`, `chore`, or `decision`
- set `spec-id` to the nearest governing artifact
- add blockers with `ddx bead dep add`
- create doc/design issues before code issues where appropriate

## Evidence Requirements

Every non-trivial claim must cite:

- documentation evidence with file path and line reference where practical
- implementation evidence with file path and line reference where practical

Be explicit about inference.

## Output Format

Produce these sections in order:

1. Backfill Metadata
2. Scope and Evidence Baseline
3. Recursive Coverage
4. Current-State Summary
5. Artifact Inventory and Gaps
6. Confidence Ledger
7. Guidance Gates
8. Backfilled Artifacts
9. Assumption Ledger
10. Follow-Up Issues
11. Next Recommended Steps

Be precise, evidence-driven, and conservative about canonizing uncertain intent.

## PHASE 9 - Measure

Verify the backfill against the governing bead's acceptance criteria.
See `.ddx/plugins/helix/workflows/references/measure.md` for the full pattern.

1. **Artifact completeness**: All missing artifacts identified in Phase 2 have
   been created or explicitly deferred with rationale.
2. **Consistency**: Backfilled artifacts are consistent with each other and
   with higher-authority existing artifacts.
3. **Concern alignment**: Backfilled artifacts are consistent with declared
   (or proposed) concerns and practices.
4. **Assumption ledger**: All inferred items are recorded with confidence levels.
5. **Record results** on the governing bead:
   `ddx bead update <id> --notes "<measure-results>...</measure-results>"`

## PHASE 10 - Report

Close the backfill cycle and feed back into the planning helix.
See `.ddx/plugins/helix/workflows/references/report.md` for the full pattern.

1. If measurement passed, close the governing bead with evidence summary.
2. If low-confidence items remain, create follow-on beads with label
   `kind:planning` for guidance-dependent work.
3. The follow-up issues created in Phase 8 re-enter the planning helix.

After those sections, emit this machine-readable trailer exactly:

```
BACKFILL_STATUS: COMPLETE|GUIDANCE_NEEDED|BLOCKED
BACKFILL_REPORT: docs/helix/06-iterate/backfill-reports/<file>.md
RESEARCH_EPIC: <id|none>
MEASURE_STATUS: PASS|FAIL|PARTIAL
BEAD_ID: <governing-bead-id>
FOLLOW_ON_CREATED: N
```

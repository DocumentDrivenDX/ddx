# HELIX Action: Measure

You are performing standalone measurement of one or more beads against their
acceptance criteria, concern-declared quality gates, and ratchet enforcement.

This action can be invoked standalone or runs as an embedded phase within
other actions. When standalone, it reads the bead's acceptance criteria and
runs verification without re-executing the work.

## Action Input

You may receive:

- an explicit bead ID
- a scope selector such as `FEAT-003`, `area:auth`, or `phase:build`
- `--rerun <id>` to re-measure a previously measured bead

Examples:

- `helix measure ddx-abc123`
- `helix measure area:auth`
- `helix measure --rerun ddx-abc123`

## Authority Order

When artifacts disagree, use this order:

1. Product Vision
2. Product Requirements
3. Feature Specs / User Stories
4. Architecture / ADRs
5. Solution Designs / Technical Designs
6. Test Plans / Tests
7. Implementation Plans
8. Source Code / Build Artifacts

## PHASE 0 - Bootstrap

0. **Context Recovery**: Re-read AGENTS.md so project instructions are fresh
   in your working memory.
1. Verify the built-in tracker is available.
   - If `ddx bead status` fails, stop immediately.
2. Load active concerns and practices following
   `.ddx/plugins/helix/workflows/references/concern-resolution.md`.
3. Load ratchet floor fixtures if the project has adopted quality ratchets
   (see `.ddx/plugins/helix/workflows/ratchets.md`).

## PHASE 1 - Target Selection

1. If an explicit bead ID is given, load that bead.
2. If a scope is given, load all beads in scope that have been executed
   (status `in_progress` or `closed` with work completed).
3. For each target bead, load:
   - Acceptance criteria from the bead description
   - `spec-id` and governing artifacts
   - Context digest (if present)
   - Previous `<measure-results>` (if any, for comparison)

## PHASE 2 - Acceptance Criteria Verification

For each target bead, verify every acceptance criterion:

1. Parse the criterion text to determine the verification method:
   - **Test command**: Run the specified test or command.
   - **File existence**: Verify the file exists at the specified path.
   - **Code inspection**: Check that the described behavior is implemented.
   - **Manual check**: Flag as requiring manual verification.
2. Run each verification and record pass/fail with evidence.
3. If the bead has a `spec-id`, check for an acceptance manifest (e.g.,
   `TP-SD-010.acceptance.toml`) and verify against it.
4. If acceptance scripts exist (`scripts/check-acceptance-traceability.sh`,
   `scripts/check-acceptance-coverage.sh`), run them.

## PHASE 3 - Concern-Declared Quality Gates

For each target bead:

1. Determine the bead's area from its labels.
2. Filter active concerns to those matching the bead's area.
3. For each matched concern, run the quality gates from its
   `practices.md` under `## Quality Gates`.
4. Scope gate runs to the packages/files changed by the bead's work
   (infer from commit history or bead description).
5. Use project overrides from `docs/helix/01-frame/concerns.md` when they
   specify alternative commands.

## PHASE 4 - Ratchet Enforcement

If the project has adopted quality ratchets:

1. For each applicable ratchet, run the enforcement command.
2. Record measured value vs. floor.
3. If auto-bump is triggered, note the updated floor value.

## PHASE 5 - Concern Propagation Check

Verify that the bead's context digest includes all active concerns for its
area scope:

1. If the digest is missing or stale, flag it.
2. If acceptance criteria reference tools inconsistent with declared concerns,
   flag it.
3. Record propagation status.

## PHASE 6 - Record Results

Record measurement results on the bead:

```bash
ddx bead update <id> --notes "<measure-results>
  <timestamp>$(date -u +%Y-%m-%dT%H:%M:%SZ)</timestamp>
  <status>PASS|FAIL|PARTIAL</status>
  <acceptance>
    <criterion name='...' status='pass|fail' evidence='...'/>
  </acceptance>
  <gates>
    <gate concern='...' command='...' status='pass|fail'/>
  </gates>
  <ratchets>
    <ratchet name='...' floor='...' measured='...' status='pass|fail'/>
  </ratchets>
  <propagation digest='fresh|stale|missing' criteria='consistent|inconsistent'/>
</measure-results>"
```

## Output

For each measured bead, report:

1. Bead ID
2. Acceptance criteria results (per-criterion pass/fail)
3. Quality gate results (per-gate pass/fail)
4. Ratchet results (per-ratchet measured vs. floor)
5. Concern propagation status

Then emit the machine-readable trailer:

```
MEASURE_STATUS: PASS|FAIL|PARTIAL
BEADS_MEASURED: N
BEADS_PASSED: N
BEADS_FAILED: N
BEADS_PARTIAL: N
CRITERIA_TOTAL: N
CRITERIA_PASSED: N
GATES_RUN: N
GATES_PASSED: N
RATCHETS_CHECKED: N
RATCHETS_PASSED: N
```

### Status Definitions

- `PASS`: All acceptance criteria satisfied, all gates passed, all ratchets
  within tolerance for every measured bead.
- `FAIL`: One or more beads have failed criteria or gates.
- `PARTIAL`: Some criteria could not be verified (e.g., manual check required,
  external dependency unavailable).

Be precise, quantitative, and evidence-driven.

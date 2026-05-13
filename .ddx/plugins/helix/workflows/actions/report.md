# HELIX Action: Report

You are analyzing measurement results and closing the feedback loop between
the execution helix and the planning helix.

This action operates in two modes: per-bead (closing one cycle) and batch
(aggregating across a scope).

## Action Input

You may receive:

- an explicit bead ID (per-bead mode)
- a scope selector such as `FEAT-003`, `area:auth`, or `phase:build` (batch mode)
- `--since YYYY-MM-DD` to limit batch scope by time

Examples:

- `helix report ddx-abc123`
- `helix report FEAT-003`
- `helix report area:auth --since 2026-04-01`

## PHASE 0 - Bootstrap

0. **Context Recovery**: Re-read AGENTS.md so project instructions are fresh
   in your working memory.
1. Verify the built-in tracker is available.
   - If `ddx bead status` fails, stop immediately.
2. Load active concerns following `.ddx/plugins/helix/workflows/references/concern-resolution.md`.

## Per-Bead Mode

### PHASE 1 - Load Measurement Results

1. Load the target bead: `ddx bead show <id> --json`
2. Parse the `<measure-results>` block from the bead's notes.
3. If no measurement results exist, recommend running `helix measure <id>`
   first and stop.

### PHASE 2 - Analyze Results

Classify the measurement outcome:

- **Clean**: All criteria passed, all gates passed. The bead's work is done.
- **Fixable**: Failures are within the action's scope to fix. Recommend
  fixing and re-measuring rather than creating follow-on beads.
- **Follow-on**: Failures or findings require new work outside this bead's
  scope.

### PHASE 3 - Create Follow-On Beads

For each follow-on item, create a bead:

```bash
ddx bead create "<category>: <description>" \
  --type task \
  --labels helix,phase:build \
  --set spec-id=<governing-artifact> \
  --description "<context-digest>...</context-digest>
Follow-on from bead <parent-id>.
<description of the work needed>" \
  --acceptance "<testable criteria>"
```

Follow-on categories:

| Category | When |
|----------|------|
| `regression` | A previously passing test or criterion now fails |
| `review-finding` | Fresh-eyes review identified a quality gap |
| `acceptance-failure` | An acceptance criterion could not be satisfied |
| `concern-gap` | A concern-declared quality gate failed or coverage is missing |
| `ratchet-regression` | A ratchet measurement dropped below the floor |
| `follow-on` | Execution revealed additional work outside scope |

Follow-on beads enter the planning helix — they will be refined by
`helix polish` before execution.

### PHASE 4 - Close the Governing Bead

If measurement status is `PASS` or all failures are captured as follow-on
beads:

```bash
ddx bead close <id>
```

The close comment should summarize:
- What was done
- Measurement status
- Number of follow-on beads created
- References to commits or artifacts produced

If measurement status is `FAIL` and failures are not captured as follow-on
beads, do not close. Leave the bead open with a status note.

### Per-Bead Output

```
REPORT_STATUS: CLOSED|OPEN|FOLLOW_ON
BEAD_ID: <id>
MEASURE_STATUS: PASS|FAIL|PARTIAL
FOLLOW_ON_CREATED: N
```

## Batch Mode

### PHASE 1 - Collect Beads

1. Load all beads in scope that have `<measure-results>` notes.
2. If `--since` is specified, filter by the measurement timestamp.
3. Load each bead's measurement results.

### PHASE 2 - Aggregate Statistics

Compute:

- Total beads measured / passed / failed / partial
- Concern gate pass rates by concern
- Ratchet trends (floor vs. measured over time)
- Follow-on bead categories (how much new work did execution generate?)
- Acceptance criteria satisfaction rate

### PHASE 3 - Identify Patterns

Look for:

- **Recurring failures**: Same gate fails across multiple beads. May indicate
  a systemic issue rather than per-bead bugs.
- **Concern coverage gaps**: Beads without concern-appropriate criteria. May
  indicate a polish gap.
- **Ratchet trends**: Metrics approaching the floor. May indicate quality
  erosion that needs attention before it becomes a regression.
- **Follow-on volume**: High follow-on creation rate may indicate that beads
  are under-specified or that the planning helix needs more polish cycles.

### PHASE 4 - Write Batch Report

Write the report to:
`docs/helix/06-iterate/reports/RPT-YYYY-MM-DD[-scope].md`

The report should include:

1. Scope and time range
2. Summary statistics
3. Pattern analysis
4. Concern coverage assessment
5. Ratchet trend analysis
6. Recommendations (more polish, concern updates, ratchet floor adjustments)

### Batch Output

```
REPORT_SCOPE: <scope>
BEADS_TOTAL: N
BEADS_PASSED: N
BEADS_FAILED: N
BEADS_PARTIAL: N
FOLLOW_ON_TOTAL: N
CONCERN_COVERAGE: N/M
RATCHET_STATUS: all-passing | <name> approaching floor
REPORT_FILE: docs/helix/06-iterate/reports/RPT-YYYY-MM-DD[-scope].md
```

## Feed-Back Into Planning Helix

Follow-on beads created during report are intentionally unrefined. The
execution helix produces raw findings; the planning helix refines them.

The next `helix check` will detect these beads and route appropriately:
- If they need refinement → `POLISH`
- If they are already ready → `BUILD`
- If they reveal design gaps → `DESIGN`

Be precise, quantitative, and evidence-driven.

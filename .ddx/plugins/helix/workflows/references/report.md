# Reference: Report Phase

The report phase closes the feedback loop between the execution helix and the
planning helix. It analyzes measurement results, creates follow-on beads for
new work identified, and closes the governing bead with evidence.

## Per-Bead Report (Default)

Every action's final phase is a per-bead report. After measure completes:

### 1. Analyze Measurement Results

Read the `<measure-results>` block from the bead's notes. Classify outcomes:

- **Clean**: All criteria passed, all gates passed. The bead's work is done.
- **Fixable**: Failures are within the action's scope to fix. Fix them, re-run
  measure, then report.
- **Follow-on**: Failures or findings require new work outside this bead's scope.

### 2. Create Follow-On Beads

For each follow-on item:

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

Follow-on beads enter the planning helix — they will be refined by `helix
polish` before execution.

Categories of follow-on work:

| Category | When |
|----------|------|
| `regression` | A previously passing test or criterion now fails |
| `review-finding` | Fresh-eyes review identified a bug, security issue, or quality gap |
| `acceptance-failure` | An acceptance criterion could not be satisfied |
| `concern-gap` | A concern-declared quality gate failed or concern coverage is missing |
| `ratchet-regression` | A ratchet measurement dropped below the floor |
| `follow-on` | Execution revealed additional work outside the bead's scope |

### 3. Close the Governing Bead

```bash
ddx bead close <id>
```

The close comment should summarize:
- What was done
- Measurement status (PASS/FAIL/PARTIAL)
- Number of follow-on beads created
- References to commits, artifacts, or reports produced

If measurement status is `FAIL` and the failures are not captured as follow-on
beads, do not close the bead. Leave it open with a status note explaining what
blocks closure.

### 4. Report Output

The action's output section includes measurement results and follow-on beads:

```
REPORT_STATUS: CLOSED|OPEN|FOLLOW_ON
BEAD_ID: <id>
MEASURE_STATUS: PASS|FAIL|PARTIAL
FOLLOW_ON_CREATED: N
COMMITS: <list>
```

## Batch Report (`helix report <scope>`)

Batch report aggregates across beads in a scope (epic, area, phase, or
time range).

### Input

```bash
helix report <scope>          # e.g., FEAT-003, area:auth, phase:build
helix report --since 2026-04-01
```

### Analysis

1. Collect all beads in scope that have `<measure-results>` notes.
2. Aggregate statistics:
   - Total beads measured / passed / failed / partial
   - Concern gate pass rates by concern
   - Ratchet trends (floor vs. measured over time)
   - Follow-on bead categories (how much new work did execution generate?)
3. Identify patterns:
   - Recurring failures (same gate fails across multiple beads)
   - Concern coverage gaps (beads without concern-appropriate criteria)
   - Ratchet trends approaching floor

### Output

```
REPORT_SCOPE: <scope>
BEADS_TOTAL: N
BEADS_PASSED: N
BEADS_FAILED: N
BEADS_PARTIAL: N
FOLLOW_ON_TOTAL: N
CONCERN_COVERAGE: N/M (beads with full concern threading / total)
RATCHET_STATUS: all-passing | <name> approaching floor
```

Write the batch report to:
`docs/helix/06-iterate/reports/RPT-YYYY-MM-DD[-scope].md`

## Feed-Back Into Planning Helix

Follow-on beads created during report are intentionally unrefined — they have
descriptions and acceptance criteria but may need polish before execution.
This is by design: the execution helix produces raw findings, and the planning
helix refines them into well-specified, concern-threaded, ready beads.

The next `helix check` will detect these beads and route appropriately:
- If they need refinement → `POLISH`
- If they are already ready → `BUILD`
- If they reveal design gaps → `DESIGN`

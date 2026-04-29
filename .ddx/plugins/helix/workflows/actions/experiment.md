# HELIX Action: Experiment

You are performing one bounded experiment iteration within a metric-optimization
session against the built-in tracker (`ddx bead`).

Your goal is to hypothesize a change, implement it within scoped files, verify
correctness, benchmark the result, make a keep/discard decision, log everything,
and exit. The external loop (the `/ddx:experiment` skill or the operator
re-invoking `helix experiment`) decides whether to continue iterating.

This action performs one experiment iteration. It modifies only files declared in
scope, does not change test expectations, and does not implement new features.
Session state (`autoresearch.*`, `experiments/`) is ephemeral and gitignored on
the experiment branch. The experiment is execution-layer work tracked by an issue;
the result is recorded in the issue close comment, not as a canonical HELIX doc.
The only normative artifact the experiment may create or update is a metric
definition at `docs/helix/06-iterate/metrics/`.

## Action Input

You may receive:

- no argument (auto-select a ready `phase:iterate` issue)
- an explicit issue ID
- a goal description such as `optimize test-suite-runtime`
- a `--close` flag directing you to skip iteration and execute Phase 3

Examples:

- `helix experiment`
- `helix experiment <id>`
- `helix experiment optimize test-suite-runtime`
- `helix experiment --close`

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

Rules:

- Higher layers govern lower layers.
- Tests govern build execution but do not override requirements or design.
- Source code reflects current state but does not redefine the plan.
- If an issue conflicts with its governing artifacts, do not implement the drift.
- Prefer aligning code and docs to plan. Only propose plan changes when the
  evidence is strong and the governing artifacts are stale or incomplete.

## Tracker Rules

Use the built-in tracker only. Follow:

- See `ddx bead --help` for tracker conventions

Issues are stored in `.ddx/beads.jsonl`.

This action works only on execution issues labeled `phase:iterate`. Exclude
review issues and build/deploy issues by default.

The experiment action claims one issue at session setup, may create follow-on
issues during execution, and closes the issue at session close.

## PHASE 0 - Bootstrap

0. **Context Recovery**: Re-read AGENTS.md so project instructions are fresh
   in your working memory. After long sessions, context compaction may have
   dropped critical project rules. This step is cheap insurance against drift.
0a. **Load active design principles** following `.ddx/plugins/helix/workflows/references/principles-resolution.md`.
   Use them to inform metric selection and experiment design decisions.
0b. **Load active concerns and practices** following `.ddx/plugins/helix/workflows/references/concern-resolution.md`.
   Experiments must use the declared concerns — do not introduce alternative tools
   or frameworks as part of an optimization experiment.
1. Verify the built-in tracker is available.
   - If `ddx bead status` fails, stop immediately.
2. Load ratchet floor fixtures if the project has adopted quality ratchets
   (see `.ddx/plugins/helix/workflows/ratchets.md`). Note the current floors so Phase 3
   can compare against them for auto-bump decisions.
3. Determine the invocation mode:
   - If `--close` directive: skip directly to Phase 3.
   - If resuming (session files exist — `autoresearch.md` is present in the
     worktree): read `autoresearch.md`, `autoresearch.jsonl`, and
     `experiments/worklog.md` to recover session state. Skip to Phase 2.
   - If fresh start (no session files, no `--close`): proceed to Phase 1.

Resume detection is file-based, not flag-based. The presence of
`autoresearch.md` in the worktree root is the authoritative signal that a
session is in progress. This survives context compaction and agent restarts.

## PHASE 1 - Session Setup

This phase runs on the first invocation only. If resuming, Phase 0 skips here.

### 1.1 Issue Selection

Select the experiment issue:

1. If the input is an explicit issue ID: inspect that issue.
2. If the input is a goal description: search ready `phase:iterate` issues
   matching the goal.
3. If no input is given: inspect ready `phase:iterate` execution issues and
   choose the best candidate.

The selected issue MUST:

- be labeled `phase:iterate`
- have passing tests (the project's test suite must be green before
  experimenting)
- have clear acceptance criteria defining what metric to optimize
- not be a review issue or a build/deploy issue

If no eligible issue exists, report the reason and exit cleanly.

### 1.2 Claim Issue

Claim the selected issue with `ddx bead update <id> --claim`.

### 1.3 Authority Check

Load the governing artifacts referenced by the issue:

- `spec-id`
- parent epic or parent issue
- dependency tree
- linked user story, feature, design, or test artifacts

Verify the optimization goal is consistent with architecture and design docs.
If the issue's goal contradicts a higher-authority artifact, do not proceed.
Document the conflict and exit.

### 1.4 Metric Definition

Load or create the primary metric definition:

- If a metric definition exists at `docs/helix/06-iterate/metrics/<name>.yaml`,
  read `name`, `unit`, `direction`, `command`, and `tolerance` from it.
- If no metric definition exists, create one during setup using the
  `.ddx/plugins/helix/workflows/templates/metric-definition.yaml` template schema. Commit
  the new metric definition to the canonical location
  (`docs/helix/06-iterate/metrics/<name>.yaml`) before branching. This is a
  normative artifact at authority level 5-6.

Record:

- **Primary metric**: name, unit, direction, measurement command
- **Secondary metrics** (optional): additional metrics to capture for
  monitoring/analysis but NOT used for keep/discard decisions
- **Files in scope**: every file the agent may modify during this experiment
- **Off-limits files**: files and directories that must NOT be modified
- **Constraints**: additional rules (no new dependencies, no API changes, etc.)

### 1.5 Correctness Check Definition

Define the correctness check command in `autoresearch.md` under a "Correctness
Check" section. Use the project's standard test command (e.g., `npm test`,
`cargo test`, `pytest`) from AGENTS.md or CI configuration. If the project
defines `autoresearch.checks.sh`, use that instead.

The correctness check is mandatory. Tests MUST pass after every iteration. If
tests fail, the experiment is discarded regardless of metric improvement. This
is non-negotiable — tests must pass at every step.

### 1.6 Branch and Session Files

1. Create the experiment branch:
   `git checkout -b experiment/<goal>-<date>`

2. Add session file patterns to `.gitignore` on the experiment branch and
   commit:
   ```
   # Experiment session files (ephemeral, local-only)
   autoresearch.md
   autoresearch.sh
   autoresearch.jsonl
   experiments/
   ```

3. Create ephemeral session files (untracked, local-only):
   - `autoresearch.md` — session doc, populated from
     `.ddx/plugins/helix/workflows/templates/autoresearch-session.md`. References the metric
     definition by path. Contains objective, metrics, correctness check, files
     in scope, off-limits, constraints, and "What's Been Tried" section.
   - `autoresearch.sh` — benchmark script, derived from the metric definition's
     `command` field. Must output `METRIC <name>=<value>` lines to stdout.
     Make it executable (`chmod +x autoresearch.sh`).
   - `experiments/worklog.md` — narrative log, populated from
     `.ddx/plugins/helix/workflows/templates/autoresearch-worklog.md`.
   - `autoresearch.jsonl` — initialize with a config header line:
     ```json
     {"type":"config","goal":"<goal>","metric":"<name>","direction":"<lower|higher>","issue":"<id>","started":"<ISO-8601>"}
     ```

4. Session files are never committed. They are local working state for the
   active agent session. They survive context compaction but not `git clone`
   or branch push/pull.

### 1.7 Baseline Measurement

Run the benchmark script (`./autoresearch.sh`) to establish the baseline.

Log the baseline as run #1:

- Append to `autoresearch.jsonl`:
  ```json
  {"type":"result","run":1,"status":"baseline","metric":"<name>","value":<N>,"commit":"<hash>","timestamp":"<ISO-8601>","description":"Baseline measurement"}
  ```
- Write the baseline entry to `experiments/worklog.md`.

The baseline is the reference point for all future keep/discard decisions and
delta calculations.

## PHASE 2 - Single Experiment Iteration

This is the core loop body. Each invocation executes exactly one iteration.

### Step 1: Hypothesize

Based on:

- profiling data (if available)
- prior results from `autoresearch.jsonl`
- worklog insights from `experiments/worklog.md`
- the "What's Been Tried" section of `autoresearch.md`

Form a specific, testable hypothesis about what change will improve the primary
metric. Record the hypothesis before making changes.

### Step 2: Edit

Modify only files declared in the "Files in Scope" section of
`autoresearch.md`. Do not touch off-limits files. Do not change test
expectations. Do not add new features.

### Step 3: Correctness Check (FIRST)

Run the project test suite (the command defined in `autoresearch.md` under
"Correctness Check").

- If tests FAIL: skip benchmarking entirely. Proceed directly to Step 5
  with a discard decision. Do not waste time benchmarking broken code.
- If tests PASS: proceed to Step 4.

### Step 4: Benchmark

Run `./autoresearch.sh` to measure the primary metric.

Capture:

- Primary metric value
- Secondary metric values (if defined)
- Wall-clock time of the benchmark run

This step is only reached if the correctness check passed.

### Step 5: Keep or Discard

Apply the decision rules:

**Keep** (metric improved AND tests passed):
- `git add <scoped files only>`
- `git commit -m "experiment: <brief description of change>"`
- Record the commit hash for logging.

**Discard** (metric worse, metric equal, OR tests failed):
- `git checkout -- <file1> <file2> ...` (scoped revert only — list the
  specific files from "Files in Scope", never `git checkout -- .`)
- Do not delete untracked files created by the experiment; only revert
  tracked scoped files.

**Simplicity preference**:
- A change that removes code or reduces complexity for equal or better
  performance is always a keep.
- A change that adds significant complexity for marginal improvement (below
  the confidence threshold) should be discarded.
- When two approaches yield similar metric values, prefer the simpler one.

### Step 6: Log to JSONL

Append the iteration result to `autoresearch.jsonl`:

```json
{"type":"result","run":<N>,"status":"<kept|discarded>","metric":"<name>","value":<N>,"delta_vs_baseline":"<+/-N%>","delta_vs_best":"<+/-N%>","commit":"<hash-or-null>","timestamp":"<ISO-8601>","description":"<hypothesis and what changed>","tests_passed":<true|false>}
```

Fields:

- `run`: sequential iteration number (read from JSONL to determine next)
- `status`: `kept` or `discarded`
- `metric`: primary metric name
- `value`: measured value (null if tests failed before benchmarking)
- `delta_vs_baseline`: percentage change from run #1
- `delta_vs_best`: percentage change from best kept result so far
- `commit`: git commit hash if kept, null if discarded
- `timestamp`: ISO-8601 timestamp
- `description`: the hypothesis and what was changed
- `tests_passed`: boolean

### Step 7: Worklog Entry

Append a narrative entry to `experiments/worklog.md` following the template
format:

```markdown
### Run N: <description> -- <metric>=<value> (<KEPT or DISCARDED>)

- Timestamp: YYYY-MM-DD HH:MM
- What changed: <one to two sentences>
- Result: <metric>=<value> (<delta vs best>)
- Insight: <what was learned>
- Next: <what to try based on this>
```

### Step 8: Confidence Scoring

Read the full `autoresearch.jsonl` history. If 3 or more results exist
(excluding the baseline), compute a confidence score:

```
confidence = |best_improvement| / MAD
```

Where:
- `best_improvement` = best metric value minus baseline value (sign-adjusted
  for direction)
- `MAD` = median absolute deviation of all measured values from their median

Interpretation:

- `>= 2.0` — likely genuine improvement (signal well above noise)
- `1.0 - 2.0` — marginal, above noise but not strongly significant
- `< 1.0` — within noise floor, improvement may be measurement variance

If fewer than 3 results exist, report `EXPERIMENT_CONFIDENCE: insufficient_data`
in the trailer.

### Step 9: Update Session Doc

Read `autoresearch.jsonl` to determine the total iteration count. If the
iteration count is a multiple of 5, update the "What's Been Tried" section in
`autoresearch.md` with:

- Summary of the last 5 experiments
- Key patterns observed across all experiments
- Which approaches worked, which did not, and why

This keeps the session doc useful for hypothesis formation in future iterations,
especially after context compaction.

## PHASE 3 - Session Close

This phase is invoked explicitly via `helix experiment --close` or by the skill
when iteration is complete. It does NOT run during normal iteration.

### 3.1 Authority Check

Review the cumulative diff on the experiment branch against governing
artifacts. Verify that the total set of changes is consistent with the issue's
scope and the project's architecture. If the cumulative diff has drifted from
the issue's intent, flag the problem and determine whether to proceed.

### 3.2 Zero-Improvement Case

Read `autoresearch.jsonl` to determine whether any iterations were kept.

If no iterations were kept (best equals baseline):

- Skip the squash-merge — there are no code changes to merge.
- Close the issue with a note recording what was tried, how many iterations
  ran, and why nothing improved. This is a valid outcome. Not every
  experiment produces improvement.
- Proceed to cleanup (3.5).

### 3.3 Ratchet Floor Update

If the project has performance ratchets (loaded in Phase 0) and the best
result exceeds the current ratchet floor + auto-bump threshold:

- Update the ratchet floor fixture file with the new floor value.
- The floor update will be included in the squash commit.

If the best result does not exceed the auto-bump threshold, do not update the
floor. The improvement is real but not large enough to raise the committed
floor.

### 3.4 Squash-Merge

If iterations were kept:

1. Identify the original branch (the branch the experiment branch was created
   from).
2. Switch to the original branch:
   `git checkout <original-branch>`
3. Squash-merge the experiment branch:
   `git merge --squash experiment/<goal>-<date>`
4. Create the squash commit with a comprehensive message:
   ```
   experiment(<goal>): <one-line summary of result>

   Issue: <issue-id>
   Metric: <name> (<unit>, <direction> is better)
   Baseline: <baseline-value>
   Final: <best-value> (<delta>% improvement)
   Iterations: <total> run, <kept> kept
   Confidence: <score>

   Key changes:
   - <summary of kept changes>

   Ratchet floor updated: <yes/no, old -> new if yes>
   ```
5. If a ratchet floor update applies, ensure the updated floor fixture is
   included in the squash commit.

### 3.5 Cleanup

Remove ephemeral session files (they are untracked local state):

```bash
rm -f autoresearch.md autoresearch.sh autoresearch.jsonl
rm -rf experiments/
```

Delete the experiment branch:

```bash
git branch -d experiment/<goal>-<date>
```

### 3.6 Measure

Record measurement results on the bead before closing. See
`.ddx/plugins/helix/workflows/references/measure.md` for the full pattern.

```bash
ddx bead update <id> --notes "<measure-results>
  <timestamp>$(date -u +%Y-%m-%dT%H:%M:%SZ)</timestamp>
  <status>PASS|PARTIAL</status>
  <acceptance>
    <criterion name='metric-improvement' status='pass|fail'
      evidence='baseline=X, final=Y, delta=Z%'/>
  </acceptance>
  <ratchets>
    <ratchet name='...' floor='...' measured='...' status='pass|fail'/>
  </ratchets>
  <experiment iterations='N' kept='N' confidence='score'/>
</measure-results>"
```

### 3.7 Close Issue

Close the issue with `ddx bead close <id>` and a comprehensive close comment
recording execution evidence:

- Goal and optimization target
- Baseline metric value
- Final best metric value and delta
- Total iterations run and kept count
- Confidence score
- Key insights from the experiment
- Ratchet floor updates (if any)
- Files modified in the final squash commit

This is execution evidence on the issue, not a canonical HELIX doc.

### 3.8 Follow-On Issues

If the experiment revealed additional optimization opportunities, code quality
concerns, or architectural concerns:

- Create follow-on issues immediately.
- Make them atomic and deterministic.
- Set `spec-id` to the nearest governing artifact.
- Add the correct HELIX labels.

Do not silently absorb discovered work into the current experiment scope.

### 3.9 Report

Report trailer lines and a summary of the experiment session.

## Trailer Lines

Every invocation must end with these trailer lines:

```
EXPERIMENT_STATUS: CONVERGED|ITERATION_COMPLETE|NO_IMPROVEMENT|CLOSED
EXPERIMENT_ITERATIONS: N
EXPERIMENT_KEPT: N
EXPERIMENT_BEST: <metric>=<value> (<delta>% vs baseline)
EXPERIMENT_CONFIDENCE: <score>
MEASURE_STATUS: PASS|FAIL|PARTIAL
BEAD_ID: <id>
```

### Status Definitions

- `ITERATION_COMPLETE` — normal single-iteration exit. The skill or operator
  decides whether to continue. This is the expected status for most
  invocations.
- `CONVERGED` — confidence >= 2.0 for 3 consecutive keeps with < 1% delta
  between them. The metric has stabilized at an improved level. Further
  iteration is unlikely to yield significant gains.
- `NO_IMPROVEMENT` — no improvement in the last 5 consecutive iterations.
  The experiment has stalled. The operator should consider closing the session
  or changing strategy.
- `CLOSED` — session close completed (Phase 3 executed). The experiment
  branch has been squash-merged (or skipped if zero improvement), the issue
  has been closed, and session files have been cleaned up.

These statuses are derived from the cumulative `autoresearch.jsonl` history,
not from within-invocation state. Each invocation reads the full log to
determine whether a convergence or stall pattern has emerged.

### Field Definitions

- `EXPERIMENT_ITERATIONS`: total number of iterations run across all
  invocations (read from `autoresearch.jsonl` run count).
- `EXPERIMENT_KEPT`: number of iterations whose changes were committed
  (status `kept` in JSONL).
- `EXPERIMENT_BEST`: the best metric value achieved and its percentage
  improvement over the baseline. Format: `<metric>=<value> (<delta>% vs
  baseline)`. If no improvement, report the baseline value with `0%`.
- `EXPERIMENT_CONFIDENCE`: the confidence score from Step 8, or
  `insufficient_data` if fewer than 3 results exist, or `N/A` on `CLOSED`
  status (confidence was already reported on the final iteration).

## Output Format

Report these sections in order:

1. Experiment Status (trailer lines)
2. Issue ID
3. Current Iteration Summary (hypothesis, result, keep/discard decision)
4. Cumulative Progress (iterations run, kept, best result, delta)
5. Confidence Assessment
6. Follow-On Issues Created (if any)
7. Suggested Next Step (continue iterating, close session, change strategy)

Be precise, quantitative, and operational.

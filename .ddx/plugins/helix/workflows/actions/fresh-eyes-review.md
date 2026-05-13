# HELIX Action: Fresh-Eyes Review

You are performing a self-review of recently completed work, looking for bugs,
omissions, and quality issues with fresh perspective.

After implementing an issue, 1-3 review passes catch bugs that implementation
blindness misses. Each pass focuses on a different failure mode.

## Action Input

You may receive:

- no argument (default: `last-commit`)
- `last-commit` — review the most recent commit
- `commit:<sha>` — review one specific implementation commit
- an issue ID — review all changes associated with that issue
- a file list — review those specific files

Examples:

- `helix review`
- `helix review last-commit`
- `helix review commit:abc1234`
- `helix review <id>`
- `helix review src/auth/`

## PHASE 0 - Identify Review Target

0. **Context Recovery**: Re-read AGENTS.md so project instructions are fresh
   in your working memory. After long sessions, context compaction may have
   dropped critical project rules. This step is cheap insurance against drift.
0a. **Load active design principles** following `.ddx/plugins/helix/workflows/references/principles-resolution.md`.
   Apply them as review criteria — flag changes that violate the active principles.
0b. **Load active concerns and practices** following `.ddx/plugins/helix/workflows/references/concern-resolution.md`.
   Flag implementation that uses tools or conventions inconsistent with the
   declared concerns (e.g., wrong test framework, wrong formatter, wrong import style).
0c. **Read the bead's context digest** if the reviewed issue has one.
   Use it as the authoritative summary of what principles, concerns, practices,
   and ADRs govern this work.
1. Determine what was just implemented:
   - If `last-commit` or no argument: `git diff HEAD~1`
   - If `commit:<sha>`: review exactly that commit's diff (`git show <sha>`)
   - If issue ID: load the issue, find associated commits via issue ID in commit
     messages, compute the aggregate diff
   - If file paths: review those files in their current state
   - In the automated `helix run` loop, prefer `commit:<sha>` from the
     executed bead's `closing_commit_sha` when tracker-closure bookkeeping
     produced a newer tracker-only commit after the implementation commit.
2. Load the governing artifacts for the reviewed code (acceptance criteria,
   test plans, design docs).

## PHASE 0.5 - Bead Acquisition

Before performing review passes, acquire a governing bead for this review.
See `.ddx/plugins/helix/workflows/references/bead-first.md` for the full pattern.

1. If reviewing a specific issue ID, the review may operate on the execution
   bead itself (recording review findings as notes). Alternatively, create a
   dedicated review bead.
2. Search for an existing open review bead:
   - `ddx bead list --status open --label kind:planning,action:review --json`
3. If not found, create one:
   ```bash
   ddx bead create "review: <scope description>" \
     --type task \
     --labels helix,phase:review,kind:planning,action:review \
     --set spec-id=<reviewed-commit-or-issue> \
     --description "<context-digest>...</context-digest>
   Fresh-eyes review of <target>.
   Review target: <last-commit|issue-id|file-list>" \
     --acceptance "All review passes complete; findings filed as beads with scope-appropriate area labels; AGENTS.md updated if needed"
   ```
   Then assemble or refresh the bead's `<context-digest>` per
   `.ddx/plugins/helix/workflows/references/context-digest.md`. If the repo
   ships `scripts/refresh_context_digests.py`, use it after creation so the
   digest and derived `area:*` labels stay deterministic.
4. Record the bead ID. All review findings are governed by this bead.

## Pass 1 - Correctness Review

For every changed function or method:

1. Does it handle all error cases documented in the design?
2. Are edge cases covered (empty input, null, boundary values, overflow)?
3. Does it match the acceptance criteria from the governing issue?
4. Are return values and error codes consistent with interface contracts?
5. Are there off-by-one errors, missing bounds checks, or unclosed resources?

For every changed test:

1. Does it actually test what it claims to test?
2. Are assertions specific enough to catch regressions?
3. Could the test pass even if the code were broken (tautological assertion)?
4. Are error paths tested, not just the happy path?

## Pass 2 - Integration Review

1. How do the changes interact with existing code paths?
2. Are there race conditions, deadlocks, or ordering dependencies?
3. Are there missing imports, broken references, or stale caches?
4. Do configuration changes propagate correctly?
5. Are database migrations reversible?
6. Do API changes maintain backward compatibility where required?

## Pass 3 - Concern-Aware Quality Review

### Security (always, when attack surface exists)

1. Are there injection vulnerabilities (SQL, command, XSS, template)?
2. Are there authentication or authorization bypasses?
3. Is sensitive data logged, cached, or exposed in error messages?
4. Are there O(n^2) loops, unbounded allocations, or missing pagination?
5. Are there unnecessary network calls in hot paths?

### Concern-Specific Practices

For each active concern loaded in Phase 0b, verify the changes follow
the concern's declared practices:

1. **Tech-stack concerns**: Does the code use the declared linter, formatter,
   test framework, and build tool? Flag drift signals — e.g., `require()`
   instead of `import`, `npm run` instead of `bun run`, `println!` instead
   of `tracing::info!`, `vitest` imports in a `bun:test` project.
2. **Security concern** (if active): Are inputs validated at system boundaries?
   Are SQL queries parameterized? Are secrets loaded from environment, not
   hardcoded? Are error messages generic to clients? Check against the
   practices in `.ddx/plugins/helix/workflows/concerns/security-owasp/practices.md`.
3. **Observability concern** (if active): Are new code paths instrumented
   with tracing spans? Are metrics emitted for new endpoints? Check against
   `.ddx/plugins/helix/workflows/concerns/o11y-otel/practices.md`.
4. **Infrastructure concern** (if active): Do new services follow the
   declared deployment pattern? Are Helm values or k8s manifests updated?

Report concern-practice violations as findings with category `drift` and
the specific concern name.

Skip this pass when changes are purely documentation, configuration, or
internal refactoring with no new attack surface, performance impact, or
concern-relevant tooling changes.

## Pass 4 - Operational Learnings

Review the changes for lessons that should be captured in project operational
docs. This pass ensures that hard-won knowledge is not lost to context
compaction or session boundaries.

1. **AGENTS.md drift**: Do the changes introduce new tools, commands, patterns,
   or conventions that AGENTS.md does not yet document? Are there existing
   AGENTS.md instructions that the changes have made stale or wrong?
   - New CLI commands or flags
   - Changed file paths or directory layout
   - New or removed dependencies
   - Changed testing or CI procedures
   - New conventions for naming, structure, or workflow

2. **Behavioral learnings**: Did this implementation reveal non-obvious
   constraints, failure modes, or gotchas that future agents should know about?
   - Surprising API behavior or edge cases
   - Performance constraints discovered during implementation
   - Configuration interactions that were not obvious from docs
   - Test patterns that proved necessary

3. **Apply updates directly**: If AGENTS.md needs updating and the required
   change is clear from the evidence, make the edit. Do not just recommend it.
   Keep AGENTS.md concise — add actionable instructions, not narrative history.

4. **File learnings issues**: For behavioral learnings that do not belong in
   AGENTS.md (they are project-specific knowledge, not agent instructions),
   create a `kind:backlog` issue with label `source:learnings` capturing the
   insight and its evidence.

Skip this pass only when changes are trivial (typos, formatting, comment-only).

## Output

For each issue found, report:

- **File and line**: exact location
- **Category**: bug, security, performance, correctness, integration, drift
- **Severity**: critical, high, medium, low
- **Description**: what is wrong
- **Suggested fix**: how to resolve it

## Filing Findings as Tracker Issues

After completing all review passes, file each actionable finding (severity
`critical`, `high`, or `medium`) as a tracker issue so that findings are
durable and appear in the ready queue for subsequent execution cycles.

For each actionable finding, create a tracker issue:

```bash
new_id="$(ddx bead create "<category>: <short description>" \
  --type task \
  --labels helix,phase:build,review-finding,<derived-area-labels> \
  --set spec-id=<governing-artifact-or-file-path> \
  --description "Review finding from fresh-eyes review.
File: <file>:<line>
Category: <category>
Severity: <severity>
Description: <full description>
Suggested fix: <suggested fix>" \
  --acceptance "<deterministic verification criteria for the fix>")"

python3 scripts/refresh_context_digests.py --apply --bead "$new_id"
```

Rules for filing:
- `low` severity findings: do not file as issues; report them in the output
  only
- Use label `review-finding` on every finding issue for queryability
- Include at least one scope-appropriate `area:*` label on every filed finding
  so concern matching survives re-entry into the queue
- If the repo ships `scripts/refresh_context_digests.py`, run it after creating
  the finding bead so the queue entry carries the current `<context-digest>`
  and any inferred `area:*` labels.
- Derive `area:*` labels in this priority order:
  1. Preserve `area:*` labels from the reviewed execution bead when the review
     target is an issue or when the governing review bead points back to that
     issue.
  2. Otherwise infer the label(s) from the reviewed scope using the project
     area taxonomy in `docs/helix/01-frame/concerns.md` (or the default
     taxonomy in `.ddx/plugins/helix/workflows/references/concern-resolution.md`
     when the project file does not exist).
  3. If the finding spans multiple surfaces, assign multiple `area:*` labels
     rather than picking one arbitrarily.
- Set `spec-id` with `--set spec-id=<file-path>` using the file path where the
  finding was identified
- Write deterministic acceptance criteria (e.g., "test X passes", "no SQL
  injection in function Y") so the issue can be closed by automated build

## Measure

Record review results on the governing bead. See
`.ddx/plugins/helix/workflows/references/measure.md` for the full pattern.

All four review passes constitute the measurement. Record a summary:

```bash
ddx bead update <id> --notes "<measure-results>
  <timestamp>$(date -u +%Y-%m-%dT%H:%M:%SZ)</timestamp>
  <status>CLEAN|ISSUES_FOUND</status>
  <findings total='N' filed='N' critical='N' high='N' medium='N' low='N'/>
  <agents-md-updated>YES|NO</agents-md-updated>
  <learnings-filed>N</learnings-filed>
</measure-results>"
```

## Report

Close the review cycle. See `.ddx/plugins/helix/workflows/references/report.md` for the full
pattern.

1. The filed finding beads (from "Filing Findings" above) are the primary
   follow-on output — they re-enter the planning helix for polish and build.
2. Close the governing review bead with evidence summary.
3. If issues are found with severity `critical` or `high`, recommend that the
   associated implementation issue be reopened or a regression issue be created.

Report these trailer lines at the end of your output:

```
REVIEW_STATUS: CLEAN|ISSUES_FOUND
ISSUES_COUNT: N
FINDINGS_FILED: N
AGENTS_MD_UPDATED: YES|NO
LEARNINGS_FILED: N
MEASURE_STATUS: PASS|FAIL|PARTIAL
BEAD_ID: <governing-bead-id>
FOLLOW_ON_CREATED: N
```

- `CLEAN`: no issues found across all passes
- `ISSUES_FOUND`: one or more issues identified
- `ISSUES_COUNT`: total number of findings (all severities)
- `FINDINGS_FILED`: number of findings filed as tracker issues (critical+high+medium)
- `AGENTS_MD_UPDATED`: whether AGENTS.md was modified during this review
- `LEARNINGS_FILED`: number of learnings issues created (0 if none)

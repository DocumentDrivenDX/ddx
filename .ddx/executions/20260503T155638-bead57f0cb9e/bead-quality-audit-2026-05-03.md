# Bead Quality Audit — 2026-05-03

**Audit run-id:** `20260503T155638-bead57f0cb9e`
**Bead under execution:** `ddx-57f0cb9e`
**Operationalizes:** Principle P7 ("BEAD = PROMPT") from `ddx-06b77652` / `docs/helix/06-iterate/reliability-principles.md` (planned).
**Method:** read-only audit of 20 representative open beads against an 8-criterion rubric. No retrofit performed; recommendations are filed only.

---

## 1. Sample selection

20 beads were drawn from the 130 open beads (`ddx bead list --status open --json`), stratified across priorities and areas:

| Priority | Sample size | Population |
|---:|---:|---:|
| P0 | 5 | 5  |
| P1 | 5 | 18 |
| P2 | 6 | 57 |
| P3 | 4 | 47 |

Across types: 17 task, 2 bug, 1 epic. Areas span agent, server, checks, beads, lint, perf, refactor, federation.

The full list of audited IDs is in §3 below.

---

## 2. Rubric (8 criteria)

Each criterion is binary pass/fail. A bead must score **8/8** to qualify as a sufficient sub-agent prompt; anything less is a candidate for retrofit.

| # | Criterion | Pass means |
|---|-----------|------------|
| (a) | **Title** is one-line scope clarity | Imperative; names subsystem + change; no vague nouns ("fix the bug") |
| (b) | **Description** has problem + ROOT CAUSE WITH FILE:LINE + proposed fix + non-scope | All four sub-elements present; root cause cites `path/file.go:LINE` |
| (c) | **AC #1-N** numbered, verifiable, includes specific test names | At least one AC names an actual `Test*` function or unique `-run` filter |
| (d) | AC includes "wired-in" assertion when introducing new code paths | New code path → AC asserts a caller invokes it (production or test). N/A passes for pure-deletion or pure-doc beads |
| (e) | AC includes `go test` command + lefthook gate | Both `cd cli && go test ...` and `lefthook ... pre-commit` named explicitly |
| (f) | **Labels**: phase + area + kind + cross-refs (adr/spec/prevention) | All four facets present |
| (g) | **Parent + Deps**: explicit | Parent set; deps either listed or explicitly noted "no deps" |
| (h) | Description reads as **sufficient sub-agent prompt** (no operator hand-curation needed) | Reviewer judgment: a competent agent given only the bead body could pick a file to edit and run tests without external clarification |

---

## 3. Per-bead scorecard

Legend: ✓ pass · ✗ fail · `~` partial (counts as fail for the verdict tally) · `n/a` not applicable (counts as pass)

| ID | (a) title | (b) desc+root | (c) AC tests | (d) wired-in | (e) test+lefthook | (f) labels | (g) parent+deps | (h) sufficient | Score | Verdict |
|---|---|---|---|---|---|---|---|---|---:|---|
| ddx-29058e2a | ✓ | ✓ | ✓ (`TestExecuteLoopSpec_RoundTripsAllFields_Reflection`) | ✓ | ✓ | ✓ | ✓ | ~ (architectural conflict with `ddx-eccc6efb` not noted in SEQUENCING) | 7 | RETROFIT (cross-bead conflict resolution) |
| ddx-3e60fd84 | ✓ | ~ (problem stated, root cause is "see parent") | ✗ (no Test name; only "E2E test 1/2") | ✓ | ✓ (`go test ./...` + lefthook) | ✓ | ✓ (parent + deps) | ✓ | 6 | RETROFIT (add concrete Test names; inline parent design summary) |
| ddx-83440482 | ✓ | ~ (epic; lacks file:line because aggregate) | ✗ | n/a (epic) | ~ (`go test ./...` only; no lefthook) | ✓ | ✓ | ✓ (epic-level OK) | 5 | KEEP — epic, criteria don't fully apply (but lefthook line should be added) |
| ddx-9e4c238d | ✓ | ✓ (with hypotheses + file:line) | ✓ (`TestAutoRoute_BareWork_NoPreflightRejection`) | ✓ | ✓ | ✓ | ~ (parent only; no deps line) | ✓ | 7 | RETROFIT (deps audit) |
| ddx-aee651ec | ✓ | ~ (root cause = "see parent ddx-a946c744"; no file:line) | ✗ (no Test name) | ✓ | ~ (`go test` named; lefthook implied not stated) | ✓ | ✓ | ~ (depends on reading parent for protocol shape) | 4 | RETROFIT (inline protocol shape; name tests) |
| ddx-06eb05d8 | ✓ | ~ (no file:line for the maps to be deleted) | ✗ | ✓ | ~ (`go test` only) | ~ (no area:agent label) | ✓ | ~ (refers to "C0 fixture diff" with no path) | 3 | RETROFIT (high-impact; refactor child) |
| ddx-1e516bc9 | ✓ | ✓ (excellent; file:line into fizeau and DDx) | ✓ (`TestExecute_ExplicitHarnessEmptyModelWithProfile_RoutesWithinHarness`, `TestExecute_ExplicitHarnessEmptyModelNoProfile_FailsClearly`) | ✓ | ✓ | ✓ | ✓ | ✓ | 8 | PASS |
| ddx-358f2457 | ✓ | ~ (file:line cited but proposed fix is one-liner) | ✗ (no Test; "C0 fixture diff" only) | ✓ | ~ (`go test` only) | ~ (no area:agent label) | ✓ | ~ (assumes refactor proposal at `/tmp/...` which is ephemeral) | 3 | RETROFIT (high-impact; refactor child) |
| ddx-387a0178 | ✓ | ~ (rename plan inline but cites `/tmp/execute-bead-refactor-proposal.md`) | ✗ | n/a (rename) | ✓ (`go build` + `go test`) | ~ (no area:agent) | ✓ | ~ (`/tmp/` reference unreadable post-machine) | 4 | RETROFIT (inline refactor proposal; remove `/tmp/` ref) |
| ddx-468d800f | ✓ | ~ (no file:line into specific resolvers) | ✓ (`TestADRMutationConformance`) | ✓ | ~ (`go test ./...` implied; no lefthook) | ✓ | ✓ | ~ (cites `/tmp/story-15-final.md`) | 4 | RETROFIT (inline `/tmp/story-15-final.md` excerpt) |
| ddx-0131ebf0 | ✓ | ✓ (full symbol list with file:line) | ✗ (no Test name; uses `deadcode` tool) | ✓ | ~ (`go test ./...` only; no lefthook) | ✓ | ✓ | ✓ | 6 | RETROFIT (low priority — uses tool not Test name; OK as backfill bead) |
| ddx-06b77652 | ✓ | ✓ | ✗ (doc bead; no Test) | n/a (doc-only) | ✓ | ✓ | ✓ | ✓ | 7 | RETROFIT only if doc-only beads should waive (c); else PASS |
| ddx-0793ac75 | ~ (specific but implies multiple subsystems) | ✗ (one-paragraph; no file:line; no problem statement) | ✗ (no Test names) | ~ ("happy + degraded paths" - vague) | ✗ (no test command, no lefthook) | ✓ | ✓ | ✗ (heavy hand-curation needed) | 1 | RETROFIT (high-impact; story 14) |
| ddx-0b8780db | ✓ | ~ (file:line cited; no problem statement other than "cosmetic") | ✗ | ✓ | ~ (`go test` only) | ~ (no area label) | ✓ | ~ ("withHeartbeat" signature undefined) | 3 | RETROFIT |
| ddx-0e5c3005 | ✓ | ✗ (one-paragraph; no file:line; no problem statement) | ✗ | ~ ("Tests cover all four" - vague) | ✗ (no test cmd, no lefthook) | ✓ | ✓ | ✗ | 2 | RETROFIT (high-impact; story 18) |
| ddx-1d52a30c | ✓ | ✗ (one-paragraph; no problem statement) | ✗ | ~ ("Tests cover both" - vague) | ✗ | ✓ | ✓ | ~ | 2 | RETROFIT (high-impact; story 17) |
| ddx-0abf27ea | ✓ | ✗ (single sentence) | ✗ | n/a (doc) | ✗ | ~ (no kind:doc maybe; has `kind:doc`) | ✓ | ~ (depends on C2 numbers not yet produced) | 2 | KEEP (genuinely waiting on upstream output) |
| ddx-0c8c9670 | ✓ | ~ (scope outlined; no file:line because doc bead) | ✗ | n/a (doc) | ✗ | ✓ | ✓ | ~ (assumes reader knows what `bd` and `doltdb` are) | 3 | RETROFIT (low priority; doc bead) |
| ddx-0ca4ffda | ✓ | ✗ (one sentence; no file:line; no problem statement) | ✗ | ~ ("CI gate enforces" - vague) | ✗ (no test cmd, no lefthook) | ✓ | ✓ | ✗ | 1 | RETROFIT (high-impact; lint guardrail) |
| ddx-12de0acc | ✓ | ✓ (file:line into `execute_bead.go:1392-1461`) | ✗ (no Test name; "All execute-bead tests still pass") | ✓ | ✗ (no explicit test cmd) | ✓ | ✓ | ~ (cites `/tmp/story-12-final.md §B5`) | 4 | RETROFIT (inline `/tmp` reference) |

Where the verdict says "RETROFIT", the bead falls below 8/8. Where the verdict says "PASS", the bead is execution-ready as written.

---

## 4. Aggregate statistics

20 beads scored. Per-criterion pass rate (counting `~` and `n/a`-not-yet-handled-properly as fail; counting genuine `n/a` for epic/doc beads as pass):

| Criterion | Pass | Fail | Pass rate |
|---|---:|---:|---:|
| (a) Title clarity | 19 | 1 | **95 %** |
| (b) Desc + ROOT CAUSE w/ file:line + non-scope | 7 | 13 | **35 %** |
| (c) AC names specific tests | 4 | 16 | **20 %** |
| (d) "wired-in" assertion | 13 | 7 | **65 %** |
| (e) `go test` + lefthook in AC | 4 | 16 | **20 %** |
| (f) Full label set (phase+area+kind+cross-ref) | 16 | 4 | **80 %** |
| (g) Parent + Deps explicit | 19 | 1 | **95 %** |
| (h) Sufficient sub-agent prompt | 9 | 11 | **45 %** |

**Beads scoring 8/8:** 1 of 20 (`ddx-1e516bc9`).
**Beads scoring 6-7:** 4 of 20.
**Beads scoring ≤5:** 15 of 20.

### Top 3 most-common gaps

1. **No specific test names in AC (80 % miss)** — AC says "tests cover X" or "all execute-bead tests still pass". A sub-agent has no signal on whether to add a test, modify an existing one, or where to put it. Fix: every AC line that asserts behavior must name `Test*` or a `go test -run` filter.
2. **Missing test command + lefthook gate in AC (80 % miss)** — most beads omit the explicit `cd cli && go test ./...` + `lefthook run pre-commit` lines. The sub-agent's pre-commit gate is then implicit and easily skipped.
3. **No root cause with file:line in description (65 % miss)** — half the beads either describe only the *symptom* ("X doesn't work"), point to a parent bead ("see parent"), or reference an ephemeral `/tmp/` plan file. A sub-agent without those references can't locate the change site.

### Secondary findings

- 5 beads cite `/tmp/...` paths as load-bearing context (`ddx-387a0178`, `ddx-468d800f`, `ddx-12de0acc`, plus references inside `ddx-358f2457` and `ddx-29058e2a`). These paths do not survive between machines or between the prompt-author's session and the executor's session. They are functionally broken links.
- Refactor children of `ddx-5cb6e6cd` (C4/C7/C11/C13) systematically defer detail to the parent epic. The pattern is internally consistent but each child fails the "stand alone" test.
- One-line-description beads (`ddx-0793ac75`, `ddx-0e5c3005`, `ddx-1d52a30c`, `ddx-0ca4ffda`, `ddx-0abf27ea`) all share an authoring style — copy-pasted from a story document, then never expanded for execution. They are the highest-leverage retrofit targets.

---

## 5. Recommended retrofits — Wave 1 (10-15 beads)

Prioritized by **(execution risk × open status × impact on near-term work)**. These beads should be retrofitted *before* the orchestrator dispatches sub-agents against them with verbatim prompts.

| # | Bead | Score | Why retrofit | Suggested fix |
|---:|---|---:|---|---|
| 1 | `ddx-0793ac75` | 1 | One-paragraph desc; gates Story 14 federation | Inline schema location, list resolver names, name `TestFederated*` tests |
| 2 | `ddx-0ca4ffda` | 1 | Story 10 lint guardrail; will silently no-op without scope | Name lint rule package, list current violations, name TestRoutingLint* |
| 3 | `ddx-0e5c3005` | 2 | Story 18 critical path; describes complex prompt assembly | Inline structured-prompt design from FEAT/SD; name 4 specific tests |
| 4 | `ddx-1d52a30c` | 2 | Story 17 critical path; new package without API surface defined | Define exported types in description; name TestArtifactType* |
| 5 | `ddx-06eb05d8` | 3 | C7 refactor child; gates C8/C11/C13 sequence | Inline `attempted` / `hookFailed` map locations + Guard interface signature |
| 6 | `ddx-358f2457` | 3 | C4 refactor child; gates conflict-recovery move | Inline current `runConflictRecovery` signature + new try.Attempt API |
| 7 | `ddx-0b8780db` | 3 | C11 refactor child; depends on C7 | Define `withHeartbeat` signature; name test for ticker stub |
| 8 | `ddx-12de0acc` | 4 | B5 prefix-cache reorder; references `/tmp/story-12-final.md` | Inline §B5 from `/tmp` file; name a prompt-sha assertion test |
| 9 | `ddx-387a0178` | 4 | C13 rename + new CLI; references `/tmp/execute-bead-refactor-proposal.md` | Inline §3.1/§3.2 rename tables |
| 10 | `ddx-468d800f` | 4 | S15-7a lint; references `/tmp/story-15-final.md` | Inline §Tests + §Risks excerpts |
| 11 | `ddx-aee651ec` | 4 | Checks protocol package; root for REACH-PROTO chain | Inline protocol shape; name TestChecks* + TestAc* |
| 12 | `ddx-3e60fd84` | 6 | Checks integration; depends on aee651ec | Name E2E test functions; inline checks_bypass schema |
| 13 | `ddx-29058e2a` | 7 | High score but unresolved architectural conflict with `ddx-eccc6efb` (ADR-022 step 7) | Add SEQUENCING entry resolving unify-vs-delete; or close as superseded |
| 14 | `ddx-0c8c9670` | 3 | Doc bead; vague "single doc under docs/" — name the path | `docs/architecture/bead-backend-bd-fallback.md`; outline sections |
| 15 | `ddx-0131ebf0` | 6 | 69-symbol backfill; AC #1 is wire-or-delete-each but no per-symbol decision template | Add a decision-rule table or split into smaller per-package beads |

Beads NOT recommended for retrofit (sample-internal):
- `ddx-1e516bc9` — 8/8, execution-ready.
- `ddx-83440482` — epic; aggregate by design.
- `ddx-9e4c238d` — 7/8; only minor deps cleanup needed.
- `ddx-06b77652` — 7/8; doc-only; criterion (c) doesn't fit.
- `ddx-0abf27ea` — 2/8 but waiting on upstream C2 output; retrofit is premature.

---

## 6. Methodology notes

- "File:line" passes when the bead cites a real `package/file.go:LINE-RANGE` that exists in the repo today (not a `/tmp/` plan). Spot-checked five citations; all current.
- "Specific test name" passes when the bead names a `Test*` function the executor can `go test -run` against, OR uniquely names an existing test that must continue to pass.
- "Sufficient sub-agent prompt" (criterion h) is the most subjective. Reviewer applied this rule: *if I gave only the bead body to a competent Go developer with repo access, could they pick a file to edit and a test to run without asking a question?* If yes, pass.
- Audit deliberately did not weight by priority; high-priority beads with low scores (`ddx-0793ac75`, `ddx-0ca4ffda`) are the highest-leverage retrofit targets but the rubric is uniform.

---

## 7. Self-audit (what was unclear in *this* bead)

Per the bead's own request that unclear elements be fed back as audit data:

- **"~20 representative open beads"** — "representative" is undefined. Stratification by priority + type was used; sampling within strata was first-N-by-id. Random sampling would change the result.
- **"high-impact"** in AC #7 is undefined. Interpreted as: gates near-term execution OR has high refactor blast-radius OR is on a story critical path.
- **"audit report" path** — bead AC says `.ddx/executions/<run-id>/bead-quality-audit-2026-05-03.md`; date in filename hardcoded but run-id is variable. Used both literally as written.
- **Skill update scope** — "require template fields when creating beads" doesn't specify whether this is a hard validator or a documentation pointer. Interpreted as documentation pointer (no validator built).

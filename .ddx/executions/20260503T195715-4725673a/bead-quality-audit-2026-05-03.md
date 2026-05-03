# Bead Quality Audit — 2026-05-03

Operationalizes principle **P7 (BEAD = PROMPT)** from
`docs/helix/06-iterate/reliability-principles.md` (bead `ddx-06b77652`).

## Method

20 representative open beads sampled from `ddx bead ready` (67 total). Mix
across priorities (3×P0, 2×P1, 13×P2, 3×P3 = 21 sampled, 20 reported), areas
(checks, agent, server, beads, web, specs, federation, routing, multi-node)
and kinds (platform, refactor, fix, feature, doc, backfill, perf, design,
housekeeping, multi-node).

Each bead scored against the 8-criterion rubric from `ddx-57f0cb9e`:

- **(a) Title**: one-line scope clarity.
- **(b) Description**: problem + ROOT CAUSE w/ file:line + proposed fix + non-scope.
- **(c) AC numbered/verifiable + specific test names**.
- **(d) AC includes 'wired-in' assertion when introducing new code paths**.
- **(e) AC includes go test command + lefthook gate** (or doc-equivalent for doc-only beads).
- **(f) Labels**: phase + area + kind + cross-refs (adr/spec/prevention).
- **(g) Parent + Deps explicit**.
- **(h) Description reads as sufficient sub-agent prompt**.

Pass = 1, partial = 0.5, fail = 0. Bead passes overall at ≥7/8; <7 = needs
retrofit. Doc-only beads exempt from (e)'s go-test requirement (treated as N/A
counted as pass).

## Per-bead results

| ID | Pri | a | b | c | d | e | f | g | h | Score | Verdict |
|----|-----|---|---|---|---|---|---|---|---|-------|---------|
| ddx-aee651ec | P0 | 1 | 0.5 | 0.5 | 0.5 | 1 | 0.5 | 1 | 0.5 | 5.5 | retrofit-light |
| ddx-29058e2a | P0 | 1 | 1 | 1 | 1 | 1 | 1 | 1 | 1 | 8.0 | **PASS (template-quality)** |
| ddx-9e4c238d | P0 | 1 | 1 | 1 | 1 | 1 | 1 | 1 | 1 | 8.0 | **PASS (template-quality)** |
| ddx-358f2457 | P1 | 0.5 | 0 | 0.5 | 0.5 | 0.5 | 0.5 | 1 | 0 | 3.5 | retrofit |
| ddx-d7aca866 | P1 | 1 | 0 | 0 | 0 | 0 | 1 | 1 | 0 | 3.0 | retrofit (epic — children carry detail; epic-tracker still needs gate-criteria) |
| ddx-cfedee8e | P2 | 1 | 0.5 | 0.5 | 1 | 0 | 1 | 1 | 0.5 | 5.5 | retrofit |
| ddx-b69f04f8 | P2 | 1 | 0 | 0.5 | 0.5 | 0 | 1 | 1 | 0 | 4.0 | retrofit |
| ddx-cd2ecf79 | P2 | 1 | 0 | 0.5 | 0.5 | 0 | 1 | 1 | 0 | 4.0 | retrofit |
| ddx-781a15cf | P2 | 1 | 0.5 | 0.5 | 0.5 | 0 | 0.5 | 1 | 0.5 | 4.5 | retrofit (label whitespace bug) |
| ddx-c6e3db02 | P2 | 1 | 0.5 | 1 | 1 | 0.5 | 1 | 1 | 0.5 | 6.5 | retrofit-light |
| ddx-e3f25fdb | P2 | 1 | 1 | 0.5 | 0.5 | 0 | 1 | 1 | 1 | 6.0 | retrofit-light |
| ddx-95ec5ed5 | P2 | 1 | 0 | 0.5 | 1 | 0.5 | 1 | 1 | 0 | 5.0 | retrofit |
| ddx-eccc6efb | P2 | 1 | 0.5 | 1 | 1 | 1 | 1 | 1 | 0.5 | 7.0 | **PASS** |
| ddx-0131ebf0 | P2 | 1 | 1 | 1 | 1 | 1 | 0.5 | 1 | 1 | 7.5 | **PASS** |
| ddx-b42dd3a0 | P2 | 1 | 1 | 1 | 1 | 1 | 0.5 | 1 | 1 | 7.5 | **PASS** |
| ddx-256af8b5 | P2 | 1 | 0 | 0 | 0.5 | 0 | 0.5 | 1 | 0 | 3.0 | retrofit (HIGH — refs `/tmp/story-15-final.md` outside repo) |
| ddx-31fba984 | P3 | 1 | 0 | 0.5 | 0 | 0 | 1 | 1 | 0 | 3.5 | retrofit |
| ddx-2e94817e | P3 | 1 | 0 | 0.5 | 0 | 0 | 1 | 1 | 0 | 3.5 | retrofit |
| ddx-e140727a | P3 | 1 | 0.5 | 1 | N/A | 1 | 1 | 1 | 1 | 6.5/7 | **PASS (doc-only)** |
| ddx-23cbcb4b | P3 | 0.5 | 0 | 0.5 | N/A | 1 | 1 | 1 | 0.5 | 4.5/7 | retrofit (TD-NNN placeholder unsubstituted; pending sibling ddx-e3f25fdb) |

## Aggregate stats

% of audited beads passing each criterion (pass = 1.0; partials counted half):

| Criterion | Pass-rate |
|-----------|-----------|
| (a) Title clarity              | 97% |
| (b) Root cause + file:line + non-scope | 35% |
| (c) Numbered AC w/ specific test names | 60% |
| (d) Wired-in assertion         | 56% |
| (e) go test + lefthook         | 45% |
| (f) Labels w/ cross-refs       | 80% |
| (g) Parent + Deps explicit     | 100% |
| (h) Sufficient sub-agent prompt | 43% |

**Overall pass-rate (≥7/8)**: 6 of 20 = **30%**.

## Top 3 gaps (most common, most consequential)

1. **(b) Missing ROOT CAUSE w/ file:line and explicit non-scope (65% miss).**
   Beads punt the investigation. The sub-agent must guess where to edit and
   what to leave alone. This is the direct manifestation of the P7 gap.
   Examples: `ddx-358f2457`, `ddx-b69f04f8`, `ddx-95ec5ed5`, `ddx-256af8b5`.

2. **(h) Description not standalone (57% miss).**
   Beads reference external context (parent docs, prior decisions, even
   `/tmp/*.md` files in `ddx-256af8b5`) without inlining. Fatal for
   autonomous drain — the worker has no chat history.

3. **(e) Missing test + lefthook gate in AC (55% miss).**
   Without an explicit `cd cli && go test ./<package>/... -run <Test>` AC line
   and a `lefthook run pre-commit` line, the post-merge reviewer has nothing
   to grep against — BLOCKs on missing evidence even when the work is correct.

Secondary gaps:

- (c) AC numbered but missing concrete `Test*` names (40% partial). Reviewer
  cannot verify the test added matches what was asked.
- (d) New code paths not gated by a wired-in assertion (44% partial/miss).
  Hand-curated prompts have been telling sub-agents "and make sure it's
  actually invoked from production code"; without the AC line, sub-agents
  ship orphaned helpers.

## Recommendations: 11 beads warranting retrofit (Wave 1 input)

Prioritized by blast radius. Retrofit = `ddx bead update <id>` to bring the
description + AC up to template quality before dispatching to a sub-agent
with verbatim execute-bead prompt.

| Rank | Bead | Why |
|------|------|-----|
| 1 | `ddx-256af8b5` | Worst score; description references `/tmp/story-15-final.md` (outside repo, won't exist for sub-agent). HARD BLOCKER for any autonomous attempt. |
| 2 | `ddx-d7aca866` | EPIC tracker for ADR-022 rev 5; vague gate criteria ("soak passed for ≥1 week") will leave the epic in limbo. Needs concrete child-rollup AC. |
| 3 | `ddx-358f2457` | Refactor child (C4); single-sentence description + "C0 fixture diff: byte-identical" is unverifiable without naming the fixture. |
| 4 | `ddx-cfedee8e` | Wires escalation ladder into the executor closure — a hot path. AC missing test command + lefthook; root cause refs by name not file:line. |
| 5 | `ddx-95ec5ed5` | Implements axon Backend (multi-thousand-line surface) but description is 4 sentences pointing at sibling beads. Sub-agent will not know which interface methods to satisfy. |
| 6 | `ddx-b69f04f8` | Federation spoke lifecycle — multi-feature ("--hub-address flag. Idempotent register. Heartbeat. URL-change…") packed into one bead; should also decompose. |
| 7 | `ddx-cd2ecf79` | FEAT-018 + FEAT-005 amendments — doc bead but lacks file paths to the specs being amended. |
| 8 | `ddx-23cbcb4b` | TD-NNN placeholder unsubstituted; gated on sibling housekeeping bead `ddx-e3f25fdb`. Retrofit is the substitution + adding scope-progression detail. |
| 9 | `ddx-781a15cf` | Benchmark bead; also has cosmetic label-whitespace bug (leading spaces on `area:beads`, `area:storage`, `kind:perf`, `backend-migration`) — relabel during retrofit. |
| 10 | `ddx-31fba984` | Frontend bead — three sub-features conflated; AC vague. |
| 11 | `ddx-2e94817e` | Frontend bead — "Tests cover all 6 reserved keys" without naming them or a test file. |

Beads at 5.0–6.5 (`aee651ec`, `c6e3db02`, `e3f25fdb`) are close enough that
the next dispatch can succeed; they would benefit from a touch-up but do not
gate Wave 1 retrofit work.

Beads passing (`29058e2a`, `9e4c238d`, `0131ebf0`, `b42dd3a0`, `eccc6efb`,
`e140727a`) are usable as-is and serve as positive examples for the
template (`ddx-29058e2a` and `ddx-9e4c238d` are the canonical good beads;
`ddx-0131ebf0`/`ddx-b42dd3a0` show the right shape for a backfill bead).

## Notes on rubric application

- **Doc-only beads** (`ddx-e140727a`, `ddx-23cbcb4b`, `ddx-cd2ecf79`) cannot
  meaningfully add a `go test` AC line; (e) was satisfied by the presence of
  a `ddx doc audit clean` AC or equivalent verification command, with a
  N/A counted toward the pass denominator.
- **Epic beads** (`ddx-d7aca866`) need different criteria: child-rollup
  conditions, soak duration with concrete metric, CHANGELOG fragment. The
  rubric still flags them — the template treats epics as a sub-section.
- **Backfill beads** (`ddx-0131ebf0`, `ddx-b42dd3a0`) score very high because
  they were generated by tooling (`go-production-reachability` checker) — the
  symbol list with file:line is mechanical, not authored. This is the right
  outcome and the template recommends generating mechanical lists when
  possible.

## Next actions (out of scope for this bead)

- File 11 retrofit beads (one per row in the table above) referencing this
  audit's recommendations and `docs/helix/06-iterate/bead-authoring-template.md`.
- Wave 1 of the refactor execution plan (per `ddx-06b77652` P7 intersection)
  picks up only AFTER the retrofit beads close.
- Future tooling bead: lint check that runs at `ddx bead create` time and
  scores the new bead against this rubric (referenced by template).

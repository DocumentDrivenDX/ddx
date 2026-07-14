---
ddx:
  id: IP-2026-07-13-phase2-doc-truth
  type: implementation-plan
  status: reviewed
  depends_on:
    - IP-2026-07-13-phase0-stop-self-harm
    - FEAT-006
---
# Phase 2 Plan: Make the Documents True

**Date:** 2026-07-13
**Status:** Revised r4 — corrected Fizeau runtime boundary applied 2026-07-13; ready when the entry and work-breakdown dependencies below are met
**Source:** AR-2026-07-13-vision-vs-reality.md §7 Phase 2; §3.1 (status rot), §3.2–3.7 (named contradictions)
**Mode:** Runs in parallel with Phase 1 (`IP-2026-07-13-phase1-lower-altitude`), with two ledger rows explicitly sequenced behind Phase 1 amendments (see WB-3). WB-4 additionally waits for Phase 0 WB-6's hermetic full-suite foundation. Doc edits are operator-reviewed; lint tooling is ordinary code work.

## Goal

Restore the property the methodology depends on: a spec's status and content reliably describe the shipped system. Every Complete stamp is mechanically defensible, every known contradiction is resolved by a recorded decision, and every facade surface (published API or documented command that executes nothing) is removed.

## Scope

In scope: spec-honesty lint, status re-stamps, the contradiction ledger, facade/dead-surface removal (correctly sized as feature removal where the UI is involved), phantom-command sweep, metric-artifact truth.

Non-goals: writing new feature specs (Phase 1 owns FEAT-030 and the ADR-024 amendment); re-architecting code beyond deleting facades and dead code; prose style; website content. **Cross-phase edit ownership (review-expanded):** the corrected upstream CONTRACT-003 defines Fizeau as the complete agent runtime, and Phase 1 owns FEAT-006's DDx-consumer amendment plus the implementation migration. Phase 2 verifies and re-stamps that boundary after it lands; it does not redesign it. FEAT-022/TD-033 review-model text and FEAT-006 execution-contract text remain sequenced behind the relevant Phase 1 work (ledger rows 3–4); Phase 3's FEAT-002 "Known limitations" note lands inside this phase's FEAT-002 re-stamp commit.

### Entry criteria

Phase 0 WB-1 (freeze) in effect. WB-1, unrelated WB-2 re-stamps, unrelated WB-3 rows, and WB-5 may proceed from that point according to their local dependencies. FEAT-006's re-stamp and contradiction-ledger row 4 wait until the corrected CONTRACT-003 is published and DDx's FEAT-006 consumer conformance proves it does not invoke, parse, resume, route, or supervise Claude/Codex/Gemini. WB-4 may not be scheduled until Phase 0 WB-6 has made both `cd cli && env HOME=/nonexistent go test ./...` and `cd cli && go test ./...` green and established the scoped-gate/async-full-suite policy. Phase 1 does not block the whole phase: ledger row 3 waits for Phase 1 WB-3, and row 4 waits for the corrected contract plus Phase 1 WB-1.

### Exit criteria (revised — each mechanical)

1. Spec-honesty lint green in CI over `docs/helix/01-frame/features/` and all five local subdirectories of `docs/helix/02-design/` (adr, concepts, contracts, solution-designs, technical-designs). Every Complete/Implemented document has full requirement-to-verification coverage and a passing observation produced for the current revision; a waiver cannot satisfy this rule. FEAT-006 additionally records the exact upstream CONTRACT-003 revision and a passing consumer-conformance observation, because the authoritative Fizeau contract is upstream rather than in this repository.
2. The contradiction ledger shows every row Resolved-or-Sequenced with a decision reference (sequenced rows name the Phase 1 WB they wait for).
3. Facade inventory empty: `grep -rn 'panic("not implemented")' cli/internal/server/ | grep -v _test` == 0; the enumerated fabricating surfaces (comparisonDispatch; workerDispatch kinds realign-specs/run-checks — `resolver_feat008.go:183-227`) removed schema-through-UI; Go-side gqlgen regeneration clean.
4. **Phantom-command sweep green, as a doc-lint rule:** every `ddx <cmd>` and shell command advertised by `ddx --help`, CLAUDE.md, README, and shipped skills exists (registered cobra command / real script). Known phantoms at review time: `ddx templates list/apply`, `ddx mcp list/install`, `ddx try --no-merge`, `ddx run --top-power`, and `bun run houdini:generate` — the sweep must catch all five, and the rule stays in CI so new ones cannot land.
5. MET-001 producing observations or stamped Aspirational (gate language stripped); MET-002 stamped Aspirational with a **fresh** prerequisite bead if the `bead_id` join is still null (review: the previously cited beads ddx-d0665584/ddx-5216faf4 are already closed — verify the join live; if it works, drop the prerequisite framing; if not, the closed beads prove the closed≠fixed class and the new bead cites them).

## Assumptions (review-corrected)

- ~~Frontend regenerates via houdini~~ **The frontend has no houdini toolchain** — it uses `graphql-request` with raw template-literal queries (`src/lib/gql/*.ts`, inline in `.svelte`). Schema-removal safety therefore comes from: Go-side **gqlgen regeneration**, an exhaustive grep-sweep of `src/**/*.{ts,svelte}` for removed fields, `bun run check` (svelte-check), and the Playwright suite. CLAUDE.md's `bun run houdini:generate` instruction is itself a phantom this plan removes (exit criterion 4).
- The `metric` package **has real production callers** (review: `ddx metric` command family at `cmd/metric.go:13`, registered at `command_factory.go:633`, acceptance-tested; `server.go` calls `metric.NewStore` at 3303/3318/5572/5585) — WB-4 deletes only the keepalive anchor and whatever *only* the keepalive pins.
- Ledger decisions default to "code wins" where the AR verified deliberateness; operator can override any row.
- Boundary truth is not negotiable within this phase: Fizeau owns provider
  invocation, parsing, routing, continuation, and supervision. DDx supplies task
  facts and sets `MinPower`. It may raise the floor only for stronger-review
  intent or a distinct new attempt after capability-sensitive DDx evidence;
  route, transport, quota, authentication, setup, operator-action, and generic
  failures never raise power. Operator `MaxPower`,
  harness/provider/model pins, and public Fizeau `Policy` pass through
  unchanged. Current v0.14.50 has no per-request `Profile`; legacy profile
  settings are removed rather than translated. DDx consumes public Fizeau outcomes and decides bead success or
  another attempt from repository evidence. It never chooses or directs a
  concrete harness, provider, or model.

## Work Breakdown

### WB-1: Spec-honesty lint (coverage- and observation-based)

The original heuristic was unimplementable: status markers are heterogeneous (`**Status:**` body lines with free-text qualifiers; YAML frontmatter `status:`; and **40 of 56 design docs have no status marker at all** — including SD-013 and TD-027, two docs the lint must catch), and the corpus cites test **files**, not `Test*` function names (zero of seven Complete/Implemented FEATs name a Test* identifier).

- Objective: a false or unverifiable status stamp is a CI failure. Complete/Implemented is derived from full, currently observed verification coverage, not from the mere existence of a cited file or an author-supplied waiver.
- Files or systems: new `cli/tools/lint/spechonesty/` (pattern: `evidencelint`); `lefthook.yml`; `.github/workflows/ci.yml`.
- Steps:
  1. Recognize both status encodings (body line + frontmatter), normalizing free-text qualifiers to a base status.
  2. Treat a missing status on any SD/TD/ADR as a lint failure requiring a stamp — this, not the Complete-check, is what catches SD-013 and TD-027.
  3. For every Complete/Implemented document, inventory its normative requirement IDs (or stable section anchors when the document has no requirement IDs) and require a Verification mapping that covers every requirement exactly once. Each mapping row names the requirement/anchor, the exact evidence target (a `Test*` symbol, deterministic static check, or inspectable runtime artifact), and an allowlisted executable verification command. A test-file path without the specific test or check it proves is not coverage.
  4. Resolve every named test/check and reject missing targets, uncovered requirements, duplicate mappings, and mappings to nonexistent artifacts. Run the mapped commands in CI for the current revision and emit a machine-readable report keyed by document id, requirement, command, repository revision, exit code, and observed evidence. Complete/Implemented passes only when every row has a current-revision, exit-zero observation; a checked-in assertion that a command passed is not sufficient.
  5. Reject zero-evidence Complete/Implemented documents and reject any waiver attached to those statuses. A reasoned `spec:verification-waiver` may downgrade an unmet verification requirement to a warning only for non-Complete statuses such as Proposed, In Progress, Deferred, or Aspirational; it cannot suppress missing-status, duplicate-id, or duplicate-US-id failures.
  6. Fail on duplicate document ids (TD-040 class) and duplicate US-ids across features (US-087/088 class); scope the lint to features/ plus all five 02-design subdirectories.
- Acceptance: red on today's tree, catching at minimum: FEAT-009/FEAT-002/FEAT-012 (Complete with absent or incomplete verification coverage), SD-013 (six phantom tests — verified all absent) and TD-027 **via the missing-status rule**, the TD-040 duplicate id, and the US-087/088 collision. Note (review): TD-027's phantom-test claim is only half true — `TestBeadDataModel_InvariantsHold` and `TestModuleBoundary_NoInternalImportsOutsideBead` exist; `TestOperationCatalog_AxonStoreSwitchCoverage` and `TestWatcherHub_UsesProvidedFactory` do not. The lint must therefore validate every mapped requirement and citation; two real tests cannot make absent coverage pass. Fixture tests cover an uncovered requirement, a missing `Test*` symbol, a non-zero command, a stale-revision report, a forbidden Complete-status waiver, and an allowed non-Complete waiver.
- Validation: `cd cli && go run ./tools/lint/spechonesty ../docs/helix/` in pre-commit + CI, with the CI run producing and validating the current-revision observation report; fixture-doc unit tests for every failure class above.
- Non-scope: prose quality; AC numbering style.
- Dependencies: none. Lands first — it is the phase's acceptance harness.

### WB-2: Status re-stamp pass (list extended per review)

- Objective: every status reflects verified reality.
- Files or systems: FEAT-002 → In Progress (ADR-022 rev 6 unimplemented; this commit also carries Phase 3 WB-1's "Known limitations" note per the ownership rule); FEAT-004 → In Progress (claim-liveness divergence; epic queue semantics delegated to unshipped machinery); **FEAT-006 → In Progress until DDx consumes the corrected current public Fizeau result contract plus any pinned required extensions and all direct provider invocation/output parsing/routing/resume/supervision paths are absent**; FEAT-009 → In Progress (multi-registry unbuilt; forbidden `~/.ddx` state path); FEAT-012 → In Progress (epic contract unbuilt; req 34 contradicted; preserve-ref AC violated); FEAT-018 → In Progress (more built than Not Started); **FEAT-011, FEAT-023, FEAT-027** (review: omitted originally) → verified and either re-stamped or given the Verification block below; SD-013 → stamped and gutted or deleted (six phantom tests); TD-027 → stamped + annotated (boundary relocation implemented inverted; Operation pattern 3/20); TD-010/SD-025 → annotated (`.ddx/runs` unbuilt — cross-ref Phase 3 WB-2); FEAT-016/019/021/026/029 → Deferred with one-line rationale each (AR §7 "Explicitly cut"; Phase 0 freeze ratified as scope policy).
- Steps: one commit per doc; stamp change + two-line rationale citing the AR. Every doc that remains Complete/Implemented also gains the WB-1 Verification mapping from each requirement to its exact test/check, executable command, and current-revision observation. Downgraded docs may instead carry a short Verification block that records known evidence gaps, but a test-file citation alone does not justify Complete/Implemented. The original "status block only" rule is relaxed exactly this far.
- Acceptance: WB-1 lint green over every re-stamped doc, including FEAT-022/023/027/011 **without** waivers (via complete requirement-to-verification mappings and current-revision passing observations). Any document that cannot meet that bar is downgraded; it is not left Complete/Implemented with an exception.
- Validation: spechonesty lint; `lefthook run pre-commit`.
- Non-scope: fixing divergences (code work lives in Phases 1/3); rewriting requirement bodies (ledger rows).
- Dependencies: WB-1.

### WB-3: Contradiction ledger — one recorded decision per row (rows corrected per review)

- Objective: resolve each verified contradiction exactly once, in writing.
- Files or systems: `docs/helix/06-iterate/contradiction-ledger-2026-07-13.md` (columns: id, docs in conflict, decision, decided-by, applied-in commit, **sequencing**).
- Rows (corrections bolded):
  1. `.ddx/executions` tracked vs ignored — FEAT-012 req 34 vs ADR-026 + `.gitignore:439`. Decision: ADR-026 wins; amend FEAT-012.
  2. auto-commit default — FEAT-012 req 2 vs SD-012:24 vs `autocommit.go:55` (never). Decision: never. **Both write paths get the loud-error bead: the HTTP PUT `/api/docs` handler at `server.go:2388-2404` (already returns `committed:false` but drops the error text) and the MCP analog at `~server.go:5018-5029`** (review: the original row mislabeled 2395 as "the MCP path"; the fix is surfacing the commit *error text*, since the flag already exists).
  3. Reviewer tool access — FEAT-022 §12 / TD-033 §3 "no-tool" vs ADR-024 "read-only tools". **Sequenced behind Phase 1 WB-3's ADR-024 amendment** (which changes the slot model this row would otherwise describe); this row then amends tool-access *and* slot-count language once.
  4. `discovered_subtasks` vs in-band create — FEAT-006 MUST NOT (`:376`) vs the shipped provider-output prompt path (`execute_bead.go:2029,2114,2150`). Decision: DDx may validate a DDx-owned application-result schema from `ServiceFinalData.FinalText` after the Fizeau operation and repository-evidence evaluation; `discovered_subtasks` is one optional application field. It must not scrape provider stdout, transcript/session logs, or inject provider-specific continuation. **Sequenced behind Phase 1 WB-1's FEAT-006 consumer amendment — one FEAT-006 edit**; bead filed to extend the create guard beyond fixture titles.
  5. Rebase in landing — **FEAT-012's own req 24 (`:227`, "Rebase the execution branch…") vs its US-126 AC (`:413`, "not rebased or rewritten") — the primary contradiction is internal to FEAT-012** (review: amending only SD-012:157 would mark the row Resolved while FEAT-012 still self-contradicts). Decision: US-126 wins (matches shipped ff-or-merge and FEAT-030); amend FEAT-012 req 24 **and** SD-012:157 in one commit.
  6. TD-040 duplicate id — rename cross-repo doc to TD-041; fix inbound references.
  7. FEAT-002 duplicate requirement numbering (22-27 twice; 33-42 under Non-Functional) — renumber.
  8. FEAT-012 duplicate numbering (34-40 twice) — renumber.
  9. US-087/US-088 collision (FEAT-008 vs FEAT-020) — re-id the FEAT-020 pair; fix Playwright references.
  10. `bead.backend` vs `bead_tracker.backend` — decision matches shipped config key; amend the loser; record `br` support status.
  11. FEAT-011 vs FEAT-018 `argument-hint` — pick one; amend the loser.
  12. FEAT-013 numbering restart; FEAT-022 duplicate §17 — renumber.
  13. Status vocabulary — ADR-023 "Proposed (Accepted after operator review)" → one unconditional status; TD-033 draft-but-normative → promote or demote.
  14. **(new, from review)** FEAT-008 US-095-area stories describing the Align/Checks action buttons and comparison dispatch — retired in the same decision that removes those surfaces (WB-4), so docs and code stay consistent.
  15. **Fizeau route ownership in evaluation:** FEAT-019, SD-023, TP-019, and
      FEAT-008 US-096 previously treated concrete harness/model routes and
      native session logs as DDx experiment inputs. Decision: FEAT-019/SD-023/
      TP-019 are rewritten around prompts, rubrics, work facts, and abstract
      `MinPower`; Fizeau routes every invocation independently; `RoutingActual`
      is per-run audit-only; replay uses DDx request/repository evidence; native
      logs remain opaque. FEAT-008 US-096 is Deferred and its fabricated UI is
      removed under WB-4.2.
  16. **Fizeau diagnostics/catalog ownership:** FEAT-002 provider endpoints,
      FEAT-008's former dashboard, SD-014/TP-014/REF-004 direct-provider and
      model-catalog claims, and old alignment reviews conflict with the
      corrected FEAT-006/014 boundary.
      Decision: active requirements expose only completed-run public audit
      fields and an external Fizeau diagnostics handoff. Provider APIs/MCP tools
      and catalog consumers are removed; historical artifacts are stamped
      superseded or explicitly non-authoritative.
  17. **Resolved-config boundary:** SD-024/TD-024's structural resolved-config
      work is mixed with obsolete `Profile`, model-route/catalog/provider-probe,
      comparison-harness, and DDx route-resolution fields. Decision: retain the
      immutable DDx-owned configuration pattern; delete those routing fields;
      represent DDx policy only as abstract `MinPower`; and permit
      `MaxPower`, `Harness`, `Provider`, `Model`, and public `Policy` solely as
      unchanged explicit operator passthrough. The documents remain stamped
      non-executable until the replacement field tables and tests are reviewed.
- Acceptance: every row Resolved or Sequenced-with-owner; duplicate-id lint
  passes; operator sign-off on the decision column before the batch applies.
  Row 15 additionally requires route-boundary tests proving no comparison arm,
  grade, replay input, warning, or benchmark key is derived from
  harness/provider/model or native session-log content.
- Validation: spechonesty lint; ledger review; `rg -n
  'harness.*arm|model.*arm|route.*(grade|score|warning|replay)|parse.*session.log'
  docs/helix/{01-frame/features/FEAT-019-agent-evaluation.md,02-design/solution-designs/SD-023-agent-evaluation.md,03-test/test-plans/TP-019-agent-evaluation.md}`
  returns only explicit prohibitions or audit-only assertions.
- Non-scope: contradictions not evidenced in the AR (new finds get new rows).
- Dependencies: WB-1; row 3 gated on Phase 1 WB-3; row 4 gated on the corrected CONTRACT-003 plus Phase 1 WB-1's FEAT-006 consumer conformance.

### WB-4: Facade and dead-surface removal (re-sized per review: parts are feature removal, not dead-code deletion)

- Objective: the published surface equals the real one; dead code is deleted rather than pinned alive.
- Files or systems and steps, by class:
  1. **15 `panic("not implemented")` resolvers** (`resolver.go:196-298`, names verified): remove from schema; regenerate Go side with gqlgen; grep-sweep `src/**/*.{ts,svelte}` (review verified the frontend does **not** query any of the 15 — expected clean); `bun run check` + Playwright as the safety net.
  2. **comparisonDispatch + workerDispatch placeholder kinds — feature removal with UI and e2e surface** (review: the efficacy page calls comparisonDispatch at `+page.svelte:195` with a 388-line `efficacy.spec.ts`; the project-overview page renders "Re-align specs"/"Run checks" buttons at `+page.svelte:64,74` asserted by `actions.spec.ts:78`): remove the resolver paths (`resolver_feat008.go:183-227`), the action cards, the Efficacy navigation/page, and comparison-dispatch UI; update/remove both Playwright specs. Ledger rows 14–15 retire the FEAT-008 stories and preserve FEAT-019/SD-023/TP-019 only as route-neutral deferred authority for a future real implementation. Ledger row 16 also removes any FEAT-002 `/api/providers*` and `ddx_provider_*` schema/resolver surface and archives DDx catalog claims. Sized as its own bead(s), not folded into dead-code sweeps.
  3. **Phantom commands**: remove `ddx templates list/apply` and `ddx mcp list/install` from help/CLAUDE.md, fix `ddx try --no-merge` and `bun run houdini:generate` references; deliver the exit-criterion-4 doc-lint rule so the class stays dead.
  4. **`metric` keepalive only** (rescoped per blocking finding): delete `metric/reachability.go` + the `command_factory.go:250` keepalive call + any functions only the keepalive pins; **keep** the `ddx metric` command family and server endpoints (WB-5 and exit criterion 5 depend on `ddx metric validate`). A separate recorded decision may retire the whole `ddx metric` surface later — not in this phase.
  5. **Deadcode anchors** in persona/metaprompt/config/evidence (`config/loader.go:14-36`, `persona/reachability.go:14-24`, `cmd/command_factory.go:248-250`): delete anchors and the genuinely dead code they pin; fix `library/checks/go-production-reachability` to reject keepalive-shaped anchors so deletion is the passing move.
  6. **Install-path consolidation**: keep `registry/installer.go`; fold needed parts of the 580-line local-overlay path (`cmd/install.go:435-1013`); **run a HELIX plugin install smoke test before deletion** (review open question — the overlay may be load-bearing for HELIX symlink installs).
  7. **Skill-tree consolidation (acceptance corrected for the go:embed constraint)**: `go:embed` cannot reference files outside the package dir — that is *why* three copies exist, and committed copies are required for make-less `go build`/`go test`. Achievable end-state: **one authoritative source** (`library/skills/ddx`) with generated committed copies **verified byte-identical in CI** (a check step replacing the mutation-on-commit lefthook hook at `lefthook.yml:~330-344`), plus an explicit decision whether `internal/skills` and `defaultplugin` can share one embed site (3→2 copies). The go:embed constraint is recorded in the bead body.
- Acceptance: exit criteria 3 and 4; deadcode check green without anchors;
  `comparisonDispatch`, the `/efficacy` route/navigation entry, `/api/providers*`,
  `ddx_provider_*`, and route-keyed UI fixtures are absent; FEAT-008 marks US-096 Deferred and links the
  route-neutral FEAT-019/SD-023 authority; both Phase 0 WB-6 hermetic full-suite
  commands remain green; `bun run check` + `bun run test` green (Playwright
  specs updated, not deleted-without-replacement); CI byte-identical skill-tree
  check in place.
- Validation: greps in exit criteria 3–4; frontend + Go suites.
- Non-scope: implementing removed resolvers (Phase 3 re-adds what the rebuilt server needs); deleting federation code (parked; specs stamped Deferred in WB-2).
- Dependencies: WB-1; Phase 0 WB-6 accepted (hermetic full suite plus scoped-gate/async-full-suite policy). Item 2 additionally waits for WB-3 ledger row 14's operator-approved retirement decision. Docs stop referencing removed surfaces in the same implementation window.

### WB-5: Metric artifacts tell the truth

- Objective: no metric asserts protections or observations that don't exist.
- Files or systems: `MET-001-ddx-test-walltime.md` (units off ~1000×; exec definition never registered — `ddx metric validate MET-001` fails today); `MET-002-cost-per-closed-bead.md`; `MET-003-ab-baseline.md` (created by Phase 0 WB-8; cross-linked here as the operative steering metric, including from the PRD's Success-Metrics table).
- Steps: MET-001 — fix units to seconds, register the exec definition and produce one observation, or stamp Aspirational and strip gate language; MET-002 — **verify the `sessions.correlation.bead_id` join live first** (the previously cited prerequisite beads are closed — review); if null, stamp Aspirational + file a fresh bead citing the closed ones as the closed≠fixed evidence; if populated, decide activation on its own merits.
- Acceptance: exit criterion 5; `ddx metric validate MET-001` passes or the doc is Aspirational.
- Validation: `ddx metric validate`; spechonesty lint warning tier for operating-present-tense claims on Aspirational metrics.
- Non-scope: building the cost-ingestion pipeline; new metrics beyond MET-003.
- Dependencies: WB-1. Coordinate with WB-4 item 4, which is constrained to retain the metric machinery; WB-5 does not wait for WB-4 to finish.

## Validation

- The spechonesty lint is the acceptance harness: red at start (proving it catches the known cases), green at exit, permanent in CI.
- Complete/Implemented verification commands execute in CI against the current repository revision; the generated observation report must show one passing row per mapped requirement. No waiver path exists for those statuses.
- All doc edits pass `lefthook run pre-commit` (docprose) and ledger sign-off.
- WB-4 begins only after Phase 0 WB-6's hermetic checks are green. Code deletions carry both `cd cli && env HOME=/nonexistent go test ./...` and `cd cli && go test ./...`, plus `bun run check`/`bun run test` where the schema or UI changes; gqlgen regeneration is part of every schema-touching commit.

## Risks And Rollback

- **Schema removals break the UI at runtime, not compile time** (no houdini — review) — mitigated by the grep-sweep + svelte-check + Playwright; per-commit revertible.
- **Feature removal (WB-4.2) surprises a user of the Actions panel / efficacy page** — these surfaces fabricate results today (queued records nothing drains); the ledger row records the retirement decision and the UI change is loud, not silent.
- **Reachability-check fix weakens a guard** — intent kept (no unreachable production code); only the keepalive incentive is removed.
- **Verification-mapping migration burden** — the corpus survey found 40/56 design docs unstamped and zero `Test*` citations in Complete FEATs. WB-2 may downgrade claims while mappings are built; only non-Complete statuses can use the warning/waiver tier. The burden is not a reason to preserve an unverified Complete stamp.
- **Ledger/Phase 1 collisions** — rows 3–4 are Sequenced, not Resolved, until Phase 1's amendments land; the ledger's sequencing column makes the wait visible instead of silently re-contradicting.

## Open Questions

1. WB-4.6: is the local-overlay install path load-bearing for HELIX? (Smoke test decides; if yes, consolidation keeps the overlay behavior under the registry installer's API.)
2. WB-4.7: can `internal/skills` and `defaultplugin` share one embed site (3→2 copies), or does the plugin-layer isolation require both? Decide in the bead.
3. WB-2: for Deferred stamps on FEAT-016/019/021/026/029 — any the operator wants to keep active instead? Default: all five Deferred per AR §7.

## Handoff

WB-1 and WB-4 are ordinary code beads (P7 rubric, corrected anchors). WB-1 may be filed once Phase 0 WB-1 is in effect; WB-4 beads must declare `IP-2026-07-13-phase0-stop-self-harm` WB-6 and this plan's WB-1 as dependencies, and the WB-4.2 bead must also wait for ledger row 14 approval. WB-2/WB-3/WB-5 are operator-reviewed doc batches applied directly, with the ledger as the record. WB-3 row 3 remains sequenced behind `IP-2026-07-13-phase1-lower-altitude` WB-3; row 4 and FEAT-006's re-stamp wait for the corrected CONTRACT-003 revision and Phase 1 WB-1's DDx-consumer conformance evidence. All other Phase 2 work remains parallel with Phase 1. The exit note appended here includes the current-revision verification report, ledger state, hermetic full-suite results, Fizeau consumer-conformance result, and phantom-command sweep output.

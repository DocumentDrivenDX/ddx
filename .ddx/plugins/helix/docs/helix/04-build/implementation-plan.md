---
ddx:
  id: helix.implementation-plan
  depends_on:
    - TP-002
    - TD-011
---
# Build Plan — Slider Autonomy (FEAT-011)

## Scope

This build plan scopes the implementation of FEAT-011 (slider autonomy):
the operator-facing autonomy slider that lets `helix run` start at a chosen
phase (`input`, `frame`, `design`, `build`, `review`) instead of always
beginning at intake. The plan covers wrapper changes, default-path proof,
and public-doc updates so the shipped surface matches the governing design.

**Governing Artifacts**:
- `docs/helix/01-frame/features/FEAT-011-slider-autonomy.md` — feature spec
- `docs/helix/02-design/technical-designs/TD-011-slider-autonomy-implementation.md` — technical design
- `docs/helix/03-test/test-plans/TP-002-helix-cli.md` — wrapper-CLI test plan
- `docs/helix/02-design/contracts/CONTRACT-001-ddx-helix-boundary.md` — DDx/HELIX boundary

## Shared Constraints

- Tracker-first execution: every story-level slice is a `ddx bead` under
  `.ddx/beads.jsonl`. No build work begins without a claimed bead.
- `helix run --autonomy <phase>` must be backward-compatible: the default
  remains `input` for operators who run `helix run` with no flag.
- The CLI surface is a compatibility wrapper over DDx-managed execution per
  CONTRACT-001. Build issues add behavior to HELIX prompts and policy, not
  to a parallel execution stack.
- Deterministic proof gates every change: `bash tests/helix-cli.sh` and
  `bash tests/validate-skills.sh` must pass before any build bead closes.
- `spec-id` on every build bead points at the nearest governing artifact
  (FEAT-011, TD-011, or TP-002).

## Build Sequencing

| Order | Story / Area | Governing Artifacts | Depends On | Notes |
|------|---------------|---------------------|------------|-------|
| 1 | Wrapper accepts `--autonomy <phase>` and dispatches | FEAT-011, TD-011 §3 | None | Smallest landable slice; unlocks downstream proof work |
| 2 | `helix input` skill end-to-end against new wrapper | FEAT-011, TD-011 §4 | 1 | Intake path is the most-used autonomy stop; prove it before opening lower-stop work |
| 3 | Lower-stop autonomy entrypoints (`frame`, `design`, `build`) | FEAT-011, TD-011 §5 | 1 | Reuse the dispatch layer from #1 |
| 4 | Public-doc and demo refresh | FEAT-011, FEAT-007 | 1, 2, 3 | Update website intake examples and demo recordings only after the surface stabilizes |
| 5 | Deterministic proof for every autonomy entrypoint | TP-002, TD-011 §6 | 1, 2, 3 | Extend `tests/helix-cli.sh` with one positive + one rejection case per stop |
| 6 | Cross-model review wiring on autonomy-driven runs | TD-011 §7, TP-002 | 5 | Verify `--review-agent` still works when the run starts mid-phase |

## Issue Decomposition

Story-level work is tracked via `ddx bead` in `.ddx/beads.jsonl`.

**Per-issue requirements**:
- Labels: `helix`, `phase:build`, `kind:build`, `area:cli`, plus
  `story:FEAT-011-<slice>` for grouping.
- References: FEAT-011, TD-011, TP-002, and this build plan.
- `spec-id` pointing at the nearest governing artifact.
- Blockers expressed as `dependencies[]` on the predecessor bead.

| Story / Area | Goal | Dependencies |
|--------------|------|--------------|
| Wrapper dispatch (`helix run --autonomy`) | `helix run --autonomy <phase>` selects the right skill entry and rejects unknown phases | none |
| `helix input` against new wrapper | Intake path drives `ddx agent execute-loop` with the right governing-artifact context | wrapper dispatch |
| Lower-stop entrypoints | `frame`, `design`, `build` autonomy stops route to their corresponding skill prompts | wrapper dispatch |
| Public docs and demos | Website intake docs + asciinema recordings reflect the slider surface | all wrapper work |
| Proof harness | `tests/helix-cli.sh` covers one happy + one rejection case per stop, plus one cross-model review test | wrapper + intake + lower-stop work |
| Cross-model review wiring | `helix run --autonomy <phase> --review-agent <other>` still produces a deterministic review pass | proof harness |

## Quality Gates

- [ ] Failing tests exist in `tests/helix-cli.sh` before each wrapper change is implemented
- [ ] All required tests pass before closing a build issue (`bash tests/helix-cli.sh`, `bash tests/validate-skills.sh`)
- [ ] Behavior changes update FEAT-011 / TD-011 / TP-002 explicitly
- [ ] `helix review` is run after each landed slice; review findings file new beads rather than being absorbed silently
- [ ] No build bead closes without a `spec-id` and a passing review

## Risks

| Risk | Impact | Response |
|------|--------|----------|
| Lower-stop autonomy exposes ungoverned skills | High | Refuse `--autonomy <phase>` when the corresponding skill prompt is missing or stale; surface as `BLOCKED` rather than running |
| Intake-path proof drifts as default behavior changes | Medium | Lock the intake test cases to the published `helix input` contract; treat changes as governing-doc evolutions, not test updates |
| Operators bypass the slider via direct `ddx agent execute-loop` calls | Low | Document the slider as the supported entry; the direct DDx path remains available but unsupported as a HELIX surface |
| Cross-model review wiring regresses when the run starts mid-phase | Medium | Add a cross-model review test for at least one mid-phase autonomy stop |

## Exit Criteria

- [ ] Build issue set is defined with sequence and dependencies (above table is current)
- [ ] Shared constraints are documented and traced to FEAT-011, TD-011, and CONTRACT-001
- [ ] Verification expectations are explicit: `tests/helix-cli.sh`, `tests/validate-skills.sh`, and a cross-model review pass on at least one autonomy stop
- [ ] FEAT-011 acceptance criteria are mapped to specific bead IDs in the queue

## Review Checklist

- [x] Governing artifacts are listed and exist on disk
- [x] Shared constraints trace back to requirements (FEAT-011), design (TD-011), and contract (CONTRACT-001)
- [x] Build sequence has a justified ordering (wrapper before consumers; proof before public-doc updates)
- [x] Dependencies between build steps are explicit
- [x] Each story/area references its governing artifacts (FEAT-011, TD-011, TP-002)
- [x] Issue decomposition follows tracker conventions (labels, spec-id, deps)
- [x] Quality gates are specific and enforceable (named test scripts, named artifacts)
- [x] Risks have concrete responses, not vague strategies
- [x] Plan is consistent with TP-002 and TD-011

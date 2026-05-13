---
ddx:
  id: helix.workflow.concern-resolution
  depends_on:
    - helix.workflow.principles-resolution
    - FEAT-006
---
# Concern and Practices Resolution

This reference defines the shared pattern that HELIX action prompts follow
to load the active project concerns and their associated practices.

## Resolution Logic

### Concerns

1. Check: does `docs/helix/01-frame/concerns.md` exist and have content?
   - Yes -> load it as the active concerns document.
   - No -> no active concerns. Omit concerns and practices from context.

There are no default concerns. Unlike principles, concerns are always
project-specific. A project without a concerns file simply has no declared
cross-cutting context beyond principles.

### Area Filtering

Each concern declares an `areas` field in its `concern.md`. The canonical
area taxonomy is:

| Area value | Matches bead labels | Typical concerns |
|------------|-------------------|------------------|
| `all` | Every bead | Tech stacks, security |
| `ui` | `area:ui`, `area:frontend` | a11y, i18n, design system |
| `api` | `area:api`, `area:backend` | o11y, rate limiting |
| `data` | `area:data` | Data modeling, migration |
| `infra` | `area:infra` | Deployment, monitoring |
| `cli` | `area:cli` | CLI conventions |

Concerns use comma-separated lists for multiple areas (e.g., `ui, api`).

The area taxonomy is **extensible per-project**. Projects declare their area
labels in `docs/helix/01-frame/concerns.md` under `## Area Labels`. The
defaults above cover most projects; add custom areas when needed.

Common HELIX-repo extensions include `docs`, `site`, `demo`, `testing`, and
`artifacts`. A bead may carry more than one `area:*` label when its scope spans
more than one surface.

**Matching rules**:
- `areas: all` matches every bead regardless of labels.
- `areas: ui` matches beads with `area:ui` or `area:frontend`.
- `areas: api` matches beads with `area:api` or `area:backend`.
- A bead with multiple area labels matches any concern that declares any of
  those areas.
- A bead with **no** `area:*` labels matches only concerns with `areas: all`.
- Area labels drive matching only. They are not concern names and must not be
  copied into the `<concerns>` field of a context digest.

**Triage/evolve/polish must assign area labels** before assembling the
context digest. If a bead's area is ambiguous, prefer the more inclusive
label or assign multiple labels.

### Practices

1. Parse the active concerns document for selected concerns (listed under
   `## Active Concerns`).
2. Filter to concerns matching the current bead's area scope.
3. For each matched concern, load
   `.ddx/plugins/helix/workflows/concerns/<concern-name>/practices.md` from the library.
4. Apply project overrides (listed under `## Project Overrides` in the
   concerns document) on top of library practices.

Project overrides take full precedence.

## Injection Preamble

After resolving active concerns and merged practices, include them in
your working context:

```markdown
## Active Concerns

{area-matched concern names and key constraints}

## Active Practices

{merged practices from matched concerns with project overrides applied}

Use the declared concerns and practices when making choices.
When a concern specifies a tool, convention, or quality requirement,
follow it rather than choosing an alternative.
```

## When to Apply

Action prompts that involve technology or quality choices must resolve and
inject concerns at their Phase 0 or Bootstrap step, alongside principles.

| Action | Injection Point |
|--------|----------------|
| `implementation.md` | Phase 0 (Bootstrap) — alongside principles and quality gates; Phase 7 runs concern-declared quality gates |
| `fresh-eyes-review.md` | Phase 0 — verify implementation follows concern conventions; Pass 3 checks concern-specific practices |
| `plan.md` | Before first refinement round — concerns constrain architecture |
| `evolve.md` | Phase 0 — concerns affect scope; Phase 3 detects concern conflicts from new requirements |
| `reconcile-alignment.md` | Phase 0 — concern drift across all layers (code, docs, ADRs); Phase 3 detects per-concern tooling drift |
| `polish.md` | Bootstrap — verify beads reference correct concern context; area label enforcement for concern matching; acceptance criteria tool consistency |
| `frame.md` | Bootstrap — concern selection happens during framing |
| `experiment.md` | Bootstrap — experiments must use declared concerns |
| `check.md` | Phase 0 — load concerns for queue health; Phase 2 checks area label coverage, digest freshness, missing concerns.md |
| `backfill-helix-docs.md` | Phase 0 — discover active concerns or propose them from evidence; Phase 4 may create concerns.md |

## Concern Selection in helix frame

When `helix frame` runs and no `docs/helix/01-frame/concerns.md` exists:

1. List available concerns from `.ddx/plugins/helix/workflows/concerns/` grouped by category.
2. Ask the user about each category:
   - Tech stack: "What language, runtime, and package manager?"
   - Data: "What database or data layer?"
   - Infrastructure: "Where will this deploy?"
   - Quality: "Do you need a11y, i18n, o11y?"
3. Match answers to available concerns.
4. If custom needs exist, document them as project overrides.
5. Write `docs/helix/01-frame/concerns.md`.

## Conflict Detection

When a project selects multiple concerns, check for conflicting practices:

1. For each practice category (linter, formatter, testing, etc.), check
   whether multiple concerns declare different values.
2. If a conflict exists and no project override resolves it, flag it to
   the user with a concrete example.
3. Conflicts must be resolved via project overrides before the concerns
   file is considered complete.

## Relationship to ADRs and Spikes

Concerns are the index; ADRs provide the rationale; spikes provide the
evidence:

```
Spike/POC (gather evidence)
  → ADR (record decision with rationale)
    → Concern (index for context assembly)
      → Context Digest (injected into beads)
```

- A spike or POC investigates a question.
- An ADR records the decision with rationale, citing the spike.
- The concern references the ADR in its `## ADR References` section.
- Project overrides that depart from library defaults must cite the
  governing ADR.

When a referenced ADR is superseded, `helix polish` must flag the
affected concern for re-evaluation.

## Propagation Completeness

When a concern is introduced or changed, it must propagate through the full
bead lifecycle. This section defines how to verify propagation completeness.

### What Must Propagate

When `docs/helix/01-frame/concerns.md` changes (concern added, removed, or
practices updated), the following must be updated for all beads whose area
matches the changed concern's scope:

1. **Context digests**: The `<context-digest>` block must include the concern
   and its key practices.
2. **Acceptance criteria**: At least one acceptance criterion must reference a
   concern-appropriate tool or practice (e.g., `bun:test` for `typescript-bun`,
   `cargo clippy` for `rust-cargo`).
3. **Quality gates**: The bead's governing action must run the concern's
   declared quality gates during the measure phase.

### How to Check

Use these steps to verify propagation completeness for a scope:

1. Load active concerns per the resolution logic above.
2. For each concern, list all beads in scope with matching area labels.
3. For each bead:
   - Check `<context-digest>` includes the concern. If missing → stale digest.
   - Check acceptance criteria reference concern-appropriate tools. If missing
     or referencing wrong tools → concern gap.
   - Check that the bead's last `<measure-results>` (if any) includes the
     concern's quality gates. If missing → unmeasured.
4. Report counts: total beads, beads with full coverage, beads with gaps.

### When to Run

| Trigger | Action |
|---------|--------|
| `helix check` Phase 2 | Concern Health — detect propagation gaps, recommend POLISH |
| `helix polish` Phase 2-N | Concern Propagation Verification — fix gaps during refinement |
| `helix measure` | Verify concern gates were run and recorded |
| `helix report` batch mode | Aggregate concern coverage across scope |

### Concern Change Detection

To detect whether concerns have changed since beads were created or last
polished:

1. Check `git log --since=<last-polish-date> -- docs/helix/01-frame/concerns.md workflows/concerns/`
2. If changes exist, the concern library has been updated and existing beads
   may have stale digests and acceptance criteria.
3. `helix check` should recommend `POLISH` when concern changes are detected.

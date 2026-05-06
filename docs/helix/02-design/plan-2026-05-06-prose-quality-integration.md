---
ddx:
  id: PLAN-2026-05-06-prose-quality-integration
  depends_on:
    - FEAT-027
    - TD-036
    - ADR-025
  status: draft
---
# Plan: Prose Quality Integration with Vale

## Goal

DDx should improve the document side of the project automatically. Prose checks
should make docs more actionable, clearer, and cheaper to feed to agents by
removing empty words where doing so preserves meaning. The public surface stays
`ddx doc prose`; Vale is the pinned deterministic engine behind that surface
per ADR-025.

The checker is not an AI detector and not a generic style linter. It exists to
catch prose that makes DDx documents harder to execute, review, trust, or carry
through an agent context window.

## Product Contract

- `ddx doc prose` is the only public prose checker surface.
- Vale-specific rule names, config paths, and install details stay behind DDx
  output and diagnostics.
- Docs under `docs/**` are checked by default.
- Findings are advisory by default.
- Findings preserve DDx terms, commands, paths, headings, tables, frontmatter,
  IDs, acceptance criteria, and legitimate technical density.
- Suggestions prefer shorter wording when the shorter wording is equally
  specific.

## Implementation Phases

### Phase 1: Pinned Vale Health

Implement DDx's Vale installation contract.

Work:

- Add a prose-checker health check to `ddx doctor`.
- Check `exec.LookPath("vale")`.
- Run `vale --version`.
- Accept only `vale version 3.13.0` for the initial implementation.
- In verbose mode, print the resolved Vale path and version.
- When missing or unsupported, report a prose-checker diagnostic that names the
  pinned version and points to Vale's official install/release channel.

Acceptance:

- A test covers missing `vale`.
- A test covers wrong-version `vale`.
- A test covers supported-version `vale`.
- `ddx doctor` remains non-fatal until the Vale-backed prose command lands.

### Phase 2: DDx Vale Rule Pack

Replace the current embedded-rule YAML shape with Vale styles owned by DDx.

Work:

- Add `library/checks/prose-quality/styles/DDx/*.yml`.
- Keep `library/checks/prose-quality/check.yaml` as DDx metadata: mode,
  policy, includes, excludes, pinned Vale version, and rule-pack version.
- Define initial DDx Vale rules:
  - unsupported broad claim
  - LLM-default polish / AI slop
  - filler transition
  - missing actor/action/artifact cue
  - repeated generic opening
  - token-cost reduction candidate
- Keep project vocabulary in DDx metadata and render it into Vale vocab/config
  when invoking Vale.

Acceptance:

- Rule pack validates with Vale 3.13.0.
- Rule files are shipped by `ddx init` / default plugin update.
- Existing DDx terms remain accepted by default.

### Phase 3: Vale Adapter and Normalization

Make `ddx doc prose` invoke Vale while returning DDx findings.

Work:

- Generate a temporary `.vale.ini` from DDx defaults and project config.
- Point `StylesPath` at the packaged DDx Vale styles.
- Pass explicit paths for `--changed` and direct-path mode.
- Invoke `vale --config <tmp> --output=JSON --no-global`.
- Parse Vale JSON into `docprose.Finding`.
- Map Vale checks to DDx rule IDs and DDx wording.
- Preserve the embedded checker only as a test fallback or fixture generator,
  not as the public default.

Acceptance:

- `ddx doc prose --changed` works with no project `.vale.ini`.
- `ddx doc prose <paths>` works with no project `.vale.ini`.
- Missing/unsupported Vale produces a DDx prose-checker diagnostic.
- User-facing output contains DDx rule IDs and DDx suggestions, not raw Vale
  implementation details.

### Phase 4: Evaluation Corpus

Use the combined tool insight to tune Vale rules against real DDx docs.

Work:

- Create a labeled corpus under `cli/internal/docprose/testdata/corpus/`.
- Include real DDx excerpts and synthetic edge cases.
- Label each expected issue:
  - useful finding
  - false positive
  - intentionally dense technical prose
  - token-cost reduction candidate
  - missed AI-slop construction
- Include negative samples for paths, commands, tables, IDs, frontmatter, API
  names, acceptance criteria, and measured claims.
- Record expected findings in golden JSON.

Acceptance:

- Corpus tests compare normalized DDx findings, not Vale raw output.
- Good technical samples stay quiet.
- AI-slop samples produce line-specific findings.
- Token-cost samples identify removable filler without changing meaning.
- Full `docs/**` scan reports a plausible number of findings and no broad
  technical-structure noise.

### Phase 5: Agent Skill Upgrade

Make agents use the prose workflow without reminders.

Work:

- Update `human-writing-support` to treat Vale-backed `ddx doc prose` as the
  normal post-edit check.
- Add examples for:
  - unsupported benefit claim -> concrete rewrite
  - AI-slop paragraph -> shorter actionable sentence
  - false positive in a table/path/API sample -> leave unchanged
  - token-cost edit -> remove filler while preserving meaning
- Instruct agents to rerun `ddx doc prose --changed` after edits.
- Instruct agents to report intentional exceptions only when findings remain.

Acceptance:

- Skill validation passes.
- A docs-edit dry run shows the agent runs the prose check, applies obvious
  edits, and leaves legitimate technical prose intact.

### Phase 6: Workflow Hooks

Run prose checks in normal DDx work without making users ask.

Work:

- When a `ddx try` / `ddx work` attempt changes Markdown under `docs/`, run
  `ddx doc prose --changed` before finalization.
- Include prose findings in execution evidence.
- Feed findings back to the agent before final response when the attempt is
  still editable.
- Keep correctness review and prose findings separate.
- Keep pre-commit advisory-only unless a later ADR changes policy.

Acceptance:

- Docs-changing attempts attach prose-check evidence.
- Advisory prose findings do not block bead closure by default.
- Agents get findings early enough to fix obvious prose issues.
- Operator-facing output stays concise.

## Rule Strategy

DDx should use Vale for deterministic matching and Markdown awareness, but DDx
owns the quality model. Rules should target document usefulness, not generic
style preferences.

Initial rule families:

| Rule family | Flags | Does not flag |
|---|---|---|
| Unsupported broad claim | "robust", "comprehensive", "seamless", "powerful" when no concrete actor/action/evidence follows | measured claims, concrete test matrices, named capabilities |
| AI-slop construction | fluent polish that omits actor, action, artifact, boundary, or evidence | precise summary of concrete behavior |
| Filler transition | "it is important to note", "in today's landscape", "to be clear" when removable | quoted source text, intentional public copy if still specific |
| Missing actor/action/artifact | sentence says a thing "enables" or "supports" without naming what changes | API descriptions with explicit subject and behavior |
| Token-cost reduction | removable intensifiers, throat-clearing, duplicate summary sentences | necessary definitions, acceptance criteria, legal/security constraints |

## Token-Cost Policy

Lower word count is beneficial when it preserves or improves specificity. The
checker should prefer edits that remove filler and duplicate phrasing, but it
must not compress away:

- acceptance criteria;
- command examples;
- file paths and IDs;
- rationale needed for future review;
- constraints that prevent agent drift;
- evidence that supports a decision.

The evaluation corpus should track token-cost candidates separately from
clarity findings so DDx can tune this without encouraging terse but ambiguous
docs.

## Open Beads To File

1. `doctor: validate pinned Vale prose checker`
2. `docprose: add DDx Vale style pack`
3. `docprose: invoke Vale and normalize JSON findings`
4. `docprose: build labeled prose corpus`
5. `skills: upgrade human-writing-support workflow examples`
6. `try: attach prose-check evidence for docs-changing attempts`

Each bead should reference ADR-025 and this plan via `spec-id` or inline
context. The first implementation bead should start with `ddx doctor` because
the engine decision depends on a trustworthy Vale binary.


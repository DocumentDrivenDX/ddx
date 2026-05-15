---
ddx:
  id: PLAN-2026-05-06-prose-quality-integration
  depends_on:
    - FEAT-027
    - TD-036
    - TD-037
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

Rule-pack contents:

| Vale file | DDx rule id | Purpose | Example finding |
|---|---|---|---|
| `DDx/UnsupportedClaim.yml` | `prose.claim.unsupported` | Broad praise or capability claim without concrete subject, mechanism, or evidence. | "This robust system creates a seamless workflow." |
| `DDx/AISlop.yml` | `prose.ai_slop.polish` | LLM-default polish words and phrases when they replace useful detail. | "This unlocks powerful, sophisticated automation." |
| `DDx/FillerTransition.yml` | `prose.filler.transition` | Throat-clearing transitions that can be removed without losing meaning. | "It is important to note that..." |
| `DDx/MissingActorAction.yml` | `prose.specificity.actor_action` | Sentences that say a process "enables", "supports", or "streamlines" without naming the actor, action, artifact, or boundary. | "This enables better collaboration across the workflow." |
| `DDx/TokenCost.yml` | `prose.cost.filler` | Removable intensifiers, duplicate summaries, or empty framing that increase token cost without adding execution value. | "In order to effectively begin to..." |
| `DDx/RepeatedOpening.yml` | `prose.structure.repeated_opening` | Repeated opening sentence shapes or duplicated lead-ins that read like generated filler. | "This document provides..." repeated across adjacent paragraphs. |
| `DDx/Vocabulary.yml` | `prose.vocabulary.generic_substitute` | Project-local generic substitutes that hide DDx terms. | "task" where the document means "bead", if configured. |

Rule implementation notes:

- Use Vale existence/substitution rules for deterministic phrase detection.
- Use Vale scopes and token ignores to skip code spans, fenced blocks, links,
  and frontmatter.
- Use DDx normalization to merge multiple word-level Vale hits on the same line
  into one DDx finding when the sentence has one underlying problem.
- Keep rule messages short; DDx owns the longer rationale and suggested edit in
  the normalization layer.
- Avoid default "bad word" behavior. A word becomes a finding only when the rule
  description says why it weakens DDx execution, review, trust, or token cost.

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

Corpus contents:

| Corpus group | Purpose | Minimum cases |
|---|---|---:|
| `positive/ai-slop` | Common LLM constructions that sound polished but omit useful detail. | 12 |
| `positive/unsupported-claim` | Broad claims that need actor/action/evidence. | 10 |
| `positive/token-cost` | Sentences where shorter wording is strictly better. | 10 |
| `positive/missing-actor-action` | Abstract "enables/supports/streamlines" sentences. | 8 |
| `negative/technical-density` | Dense but useful DDx technical prose. | 10 |
| `negative/markdown-structure` | Tables, commands, paths, links, frontmatter, IDs, code spans. | 12 |
| `negative/evidence-backed-claim` | Strong claims backed by tests, benchmarks, measurements, or explicit constraints. | 8 |
| `real/ddx-docs` | Excerpts from existing DDx docs with reviewed labels. | 30 |

Evaluation metrics:

| Metric | Gate |
|---|---|
| Positive recall | At least 80% of labeled positive cases produce the expected DDx rule id. |
| Negative quiet rate | At least 90% of negative cases produce no finding. |
| Structure false positives | 0 findings in code spans, fenced blocks, paths, commands, frontmatter, and DDx IDs unless the case is explicitly positive. |
| Token-cost precision | At least 80% of token-cost findings are judged shorter-without-meaning-loss. |
| Real-doc usefulness | A manual review of the full-doc scan marks at least 70% of findings useful. |
| Finding volume | Full `docs/**` scan is sparse enough for review: target under 1 finding per 1,000 words until rules prove higher precision. |

How we know it works:

- Golden tests lock expected normalized findings for the corpus.
- A full `docs/**` report is reviewed and labeled before enabling workflow
  hooks.
- Each rule has both positive and negative examples; no rule ships with only
  synthetic positives.
- Token-count improvements are reported separately from clarity findings so
  terse-but-ambiguous rewrites do not pass as quality improvements.

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

Skill contents:

- Trigger: any writing, rewriting, editing, or reviewing of Markdown under
  `docs/`.
- Required workflow:
  1. preserve the author's intent and DDx terminology;
  2. edit the document;
  3. run `ddx doc prose --changed`;
  4. apply high-signal findings;
  5. rerun `ddx doc prose --changed`;
  6. summarize remaining intentional exceptions.
- Preservation rules:
  - keep paths, commands, IDs, frontmatter, headings, table structure, API
    names, and acceptance criteria intact;
  - do not flatten useful technical density;
  - do not rewrite quoted source text unless the task is explicitly about that
    quote.
- Rewrite guidance:
  - replace broad claims with actor/action/artifact/evidence;
  - delete filler transitions when the sentence still reads correctly;
  - prefer shorter wording when it is equally precise;
  - split overstuffed sentences only when doing so improves reviewability;
  - leave a finding unresolved when the flagged text is precise and explain why.
- Examples:
  - unsupported benefit claim before/after;
  - AI-slop paragraph before/after;
  - token-cost reduction before/after with word-count delta;
  - false positive that should be ignored;
  - table/path/API sample that must not be rewritten.

Skill evals:

| Eval | Prompt | Pass condition |
|---|---|---|
| `docs-edit-runs-check` | Edit a small doc under `docs/`. | Agent runs `ddx doc prose --changed` after editing. |
| `fixes-ai-slop` | Given a doc with broad LLM-default claims. | Agent rewrites to concrete actor/action/evidence and reruns the check. |
| `preserves-technical-structure` | Given a doc with paths, commands, tables, IDs, and AC. | Agent leaves structure intact and does not chase false positives. |
| `reduces-token-cost` | Given filler-heavy prose with clear meaning. | Agent shortens prose without losing constraints or evidence. |
| `reports-exceptions` | Given a legitimate finding that should remain. | Agent states the exception and rationale instead of silently ignoring it. |

How we know the skill works:

- The eval prompts run through the same skill-loading path agents use in normal
  work.
- Each eval checks behavior, not exact prose: command was run, findings were
  handled, structure was preserved, and remaining exceptions were explained.
- A failed eval produces a concrete skill patch, not a checker-rule patch,
  unless the underlying deterministic finding is wrong.

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

## Bead Breakdown

This plan is not one bead. It is an epic with dependency-ordered child beads.
Each child should be executable on its own, with a narrow file scope and a
small verification surface.

Some child beads require higher-judgment execution: corpus labeling, rule
semantics, skill behavior, and workflow-hook policy. Those beads should use the
bead-level execution-hint policy in TD-037 rather than ad hoc model or harness
choices. In particular, use `triage.estimated_difficulty=hard` for beads that
need stronger reasoning, and keep durable harness/provider/model pins out of
scope for this plan.

### Epic

`prose: integrate Vale-backed DDx prose quality`

Acceptance:

1. `ddx doctor` reports pinned Vale health.
2. `ddx doc prose --changed` uses Vale-backed DDx rules with no project
   `.vale.ini`.
3. Normalized findings use DDx rule IDs, rationales, and suggested edits.
4. Labeled corpus tests prove useful findings and low structural noise.
5. `human-writing-support` tells agents how to run and apply the workflow.
6. Docs-changing DDx attempts attach advisory prose-check evidence.

### Child Beads

| Order | Bead | Scope | Suggested hint | Depends on |
|---:|---|---|---|---|
| 0 | `try: define and enforce bead-level execution hints` | Implement TD-037 parsing, lint, evidence, and metrics slices needed before hard difficulty hints are relied on. | `triage.estimated_difficulty=hard` | none |
| 1 | `doctor: validate pinned Vale prose checker` | `cli/cmd/doctor.go`, doctor tests, constants for Vale version. | none | 0 |
| 2 | `docprose: add DDx Vale style pack skeleton` | `library/checks/prose-quality/styles/DDx/`, metadata schema, no command wiring. | none | 1 |
| 3 | `docprose: add corpus harness for normalized findings` | `cli/internal/docprose/testdata/corpus/`, corpus loader/tests, expected JSON schema. | `triage.estimated_difficulty=hard` for corpus design judgment | 2 |
| 4 | `docprose: port initial rules to Vale styles` | Vale style files and corpus golden cases for unsupported claim, AI slop, filler, token cost. | `triage.estimated_difficulty=hard` for rule precision judgment | 3 |
| 5 | `docprose: generate temporary Vale config` | config generation from DDx defaults/project config; no Vale execution yet. | none | 2 |
| 6 | `docprose: invoke Vale and parse JSON` | Vale subprocess adapter, JSON structs, error diagnostics. | none | 5 |
| 7 | `docprose: normalize Vale findings to DDx findings` | rule-id mapping, rationale/suggested-edit mapping, line merge behavior. | `triage.estimated_difficulty=hard` for finding-quality semantics | 4, 6 |
| 8 | `doc prose: switch command to Vale-backed engine` | `ddx doc prose --changed` and explicit path behavior. | none | 7 |
| 9 | `bead review: reuse Vale-backed prose findings` | `ddx bead review --prose` path, review evidence. | `triage.estimated_difficulty=hard` for review-policy interaction | 8 |
| 10 | `skills: upgrade human-writing-support workflow examples` | active and shipped skill copies, eval prompts if available. | `triage.estimated_difficulty=hard` for skill behavior quality | 8 |
| 11 | `try: attach prose-check evidence for docs-changing attempts` | execute/try/work evidence capture and advisory handling. | `triage.estimated_difficulty=hard` for workflow-hook policy | 8, 10 |
| 12 | `docs: run Vale-backed prose pass across DDx docs` | Apply high-signal findings to `docs/**`; record before/after finding count and word-count delta. | `triage.estimated_difficulty=hard` for corpus-scale editorial judgment | 11 |

### Split Rules

- If a bead would touch both command plumbing and rule semantics, split it.
- If a bead would require changing execution flow and docs content, split it.
- If corpus labels change because rule behavior changes, keep that with the
  rule bead that caused the change.
- Do not file the full-doc cleanup bead until the checker, skill, and workflow
  evidence path are working.

Each bead should reference ADR-025 and this plan via `spec-id` or inline
context. The first implementation bead should start with `ddx doctor` because
the engine decision depends on a trustworthy Vale binary.

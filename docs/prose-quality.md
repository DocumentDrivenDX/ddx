# DDx Prose Quality Support

`ddx doc prose` provides deterministic prose-quality guidance for DDx docs and
governing artifacts. It is designed to make prose more specific, more
checkable, and more aligned with DDx vocabulary without rewriting the author's
voice.

## What It Does

Prose Quality Support evaluates text with named rules and returns structural
findings. A useful finding should explain how the prose became harder to
execute, review, or trust. Typical findings call out unsupported broad claims,
filler transitions, repeated text, common AI-slop constructions, or wording
that hides the concrete subject.

It is useful when you want:

- a repeatable check over changed prose
- line-specific findings with rule IDs and suggested edits
- guidance that respects technical terminology, headings, tables, and lists
- project vocabulary, paths, commands, headings, tables, and legitimate
  technical lists to be preserved when they are the clearest form

## What It Does Not Do

- It is not an AI detector.
- It is not a detector bypass tool.
- It does not rewrite prose automatically.
- It does not treat individual words as wrong by default.
- It does not blanket-suppress technical lists, headings, paths, tables, or
  project terms.

Legitimate technical lists, headings, paths, tables, and project terminology
are allowed and often desirable. Dense prose is not automatically bad; the
question is whether the text gives a maintainer or agent enough concrete
detail to act.

AI-slop findings target prose behavior, not authorship. The issue is not that
a model may have written the sentence; the issue is that the sentence uses
fluent polish, inflated benefit language, or generic transitions while omitting
the actor, action, artifact, boundary, measurement, or evidence a DDx document
needs.

## Run Changed-Prose Checks

Use the changed-only surface for pre-review and pre-merge checks:

```bash
ddx doc prose --changed
```

That command evaluates only changed prose and reports findings for the touched
lines. It is the primary entry point described by FEAT-027 and TD-036.

## Example Finding Output

Example output shape:

```text
docs/helix/01-frame/features/FEAT-027-prose-quality-support.md:16-18  prose.generic-claim  advisory
  rationale: "The sentence says 'clearer, more specific' but does not name the concrete change or constraint."
  suggested_edit: "Name the specific change, constraint, or review outcome the prose should communicate."
```

The exact formatting may vary by runner, but each finding should include:

- file
- line or line range
- rule ID
- severity
- rationale
- suggested edit

## Configuration

The default DDx behavior is intentionally small: docs live under `docs/`, and
`ddx doc prose --changed` checks changed Markdown with the embedded rules.
Configuration is optional.

When a project needs custom behavior, operators can set mode, severity,
policy, scope filters, and vocabulary controls in the prose-quality config.

```yaml
prose:
  mode: technical
  severity: advisory
  policy: advisory
  includes:
    - docs/**
    - README.md
  excludes:
    - "**/*.generated.md"
    - "**/fixtures/**"
  vocabulary:
    accept:
      - DDx
      - bead
      - execution
      - governance
    reject:
      - placeholder term
```

### Modes

- `technical` for specs, design docs, reference docs, and command descriptions
- `planning` for beads, plans, roadmaps, and implementation notes
- `public` for release notes, website copy, and externally visible docs

### Severity and Policy

- `severity` controls the severity attached to emitted findings.
- `policy` controls whether prose findings are advisory by default or may be
  elevated to blocking later.

The default is advisory. That keeps the first surface useful for review without
turning prose guidance into a hard gate by default.

## Bead Review Integration

`ddx bead review <id> --prose` adds an advisory prose-quality section to the
review prompt. The prose findings are emitted separately from acceptance and
correctness findings, and they reuse the same checker, rule pack, and config
resolution path as `ddx doc prose`.

When the prose section is present, review consumers should treat those
findings as advisory unless an explicit policy says otherwise.

### Vocabulary

Use `vocabulary.accept` for DDx terms, product names, and domain-specific
language that should survive intact. Use `vocabulary.reject` sparingly for
project-local substitute terms that are known to hide the intended concept.
The default checker does not reject common words such as "system", "process",
or "solution".

### Scope Filters

Use `includes` and `excludes` to target the document set you actually want to
review. Narrowing scope is better than suppressing technical language globally.

## When To Use the `human-writing-support` Skill

Use the `human-writing-support` skill when you are drafting, editing, or
reviewing DDx prose and want help preserving voice while sharpening detail. It
is the right companion for technical prose, planning prose, and public prose
when the goal is clearer writing, not detector evasion.

The skill is especially useful when a draft:

- needs stronger specificity without losing the original voice
- contains technical lists, headings, or terminology that should stay intact
- needs mode-aware guidance for technical, planning, or public prose
- should keep DDx vocabulary rather than replacing it with generic wording

## Why This Exists

DDx prose support exists to improve clarity and reviewability. It is not a
claim about authorship, and it is not a system for bypassing detector
heuristics. The goal is better prose, not a synthetic signal about who wrote
it.

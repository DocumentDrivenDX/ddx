---
name: human-writing-support
description: Support DDx prose work by preserving voice, improving execution-useful clarity, applying deterministic prose findings with judgment, and keeping DDx terminology, headings, tables, paths, commands, and legitimate lists intact.
---

# Human Writing Support

Use this skill when you are writing, rewriting, editing, or reviewing DDx prose:
specs, ADRs, beads, docs, website copy, release notes, and other public text.

## What This Is

This is guidance for better human writing, not an AI detector and not a
detector bypass tool. The goal is prose that helps a maintainer or agent
execute, review, or trust a DDx document.

## Core Guidance

- Preserve the user's voice unless the text is explicitly asking for a new tone.
- Prefer specific, checkable detail over generic importance language.
- Keep technical precision intact; do not collapse names, commands, paths,
  numbers, or constraints into vague summaries.
- Keep legitimate headings, lists, tables, code blocks, and DDx terminology when
  they improve clarity.
- If a deterministic prose check is available, treat it as advisory evidence,
  not as a banned-word oracle. Fix findings that make the doc less executable
  or reviewable; leave correct technical wording intact.

## Using `ddx doc prose`

Run `ddx doc prose --changed` after editing Markdown under `docs/`.

Use a finding when it identifies one of these problems:

- The sentence makes a broad claim but gives no concrete behavior, boundary,
  command, artifact, measurement, or acceptance evidence.
- The prose hides the actor, action, consequence, or review target.
- The wording replaces a DDx concept with a vague substitute.
- Repeated or filler text makes the document harder to execute.

Do not "fix" a finding by weakening correct technical prose. Paths, headings,
tables, commands, API names, quoted contract language, and precise terms are
often supposed to be dense.

## By Prose Type

### Technical prose

- Preserve exact terminology, APIs, filenames, labels, and acceptance criteria.
- Prefer edits that sharpen structure and specificity over broad rewrites.
- Do not remove a technical list just because it looks dense; dense can be
  correct.

### Planning prose

- Keep decision boundaries, dependencies, risks, and open questions visible.
- Preserve explicit non-scope statements and acceptance criteria.
- Avoid turning a plan into motivational language or a vague vision statement.

### Public prose

- Optimize for readability, voice, and concrete claims.
- Remove generic filler, but keep factual caveats and required qualifiers.
- Make the prose sound like a person who knows the subject, not a template.

## Editing Checks

- Ask whether each sentence adds a specific fact, decision, constraint, or
  reviewable implication.
- Replace broad claims with evidence, examples, concrete outcomes, or named
  artifacts.
- Keep a writer's intent unless the text is internally inconsistent or plainly
  wrong.
- Prefer small, local edits over full rewrites when the original already has a
  strong voice.

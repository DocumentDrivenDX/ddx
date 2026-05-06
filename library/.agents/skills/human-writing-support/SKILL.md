---
name: human-writing-support
description: Support DDx prose work by preserving voice, preferring specific checkable detail, adapting guidance for technical, planning, and public prose, and keeping DDx terminology, headings, and legitimate lists intact.
---

# Human Writing Support

Use this skill when you are writing, rewriting, editing, or reviewing DDx prose:
specs, ADRs, beads, docs, website copy, release notes, and other public text.

## What This Is

This is guidance for better human writing, not an AI detector and not a
detector-bypass tool. The goal is to make prose clearer, truer, and more useful
without sanding off the author's voice or the project vocabulary.

## Core Guidance

- Preserve the user's voice unless the text is explicitly asking for a new tone.
- Prefer specific, checkable detail over generic importance language.
- Keep technical precision intact; do not collapse names, commands, paths,
  numbers, or constraints into vague summaries.
- Keep legitimate headings, lists, tables, code blocks, and DDx terminology when
  they improve clarity.
- If a deterministic prose check is available, run it after editing. Use
  `ddx doc prose` when the project provides it, or the repo's equivalent
  deterministic prose command.

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

- Ask whether each sentence adds a specific fact, decision, or constraint.
- Replace broad claims with evidence, examples, or concrete outcomes.
- Keep a writer's intent unless the text is internally inconsistent or plainly
  wrong.
- Prefer small, local edits over full rewrites when the original already has a
  strong voice.

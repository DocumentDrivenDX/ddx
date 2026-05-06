---
name: human-writing-support
description: Support DDx prose work by preserving voice, improving execution-useful clarity, applying deterministic prose findings with judgment, and keeping DDx terminology, headings, tables, paths, commands, and legitimate lists intact.
---

# Human Writing Support

Use this skill when writing, rewriting, editing, or reviewing DDx prose under
`docs/`.

## Purpose

This is guidance for better human writing, not an AI detector and not a
detector bypass tool. The goal is prose that helps a maintainer or agent
execute, review, or trust a DDx document.

## Core Rules

- Preserve the author's voice unless the task asks for a new tone.
- Preserve exact terminology, APIs, filenames, labels, commands, paths,
  acceptance criteria, headings, tables, and legitimate technical lists.
- Prefer specific, checkable detail over generic importance language.
- Make small local edits when the prose is mostly right.
- Do not treat deterministic prose findings as banned-word rules.

## Using `ddx doc prose`

Run `ddx doc prose --changed` after editing Markdown under `docs/`.

Use a finding when it identifies one of these problems:

- A broad claim gives no concrete behavior, boundary, command, artifact,
  measurement, or acceptance evidence.
- The prose hides the actor, action, consequence, or review target.
- The wording replaces a DDx concept with a vague substitute.
- Repeated or filler text makes the document harder to execute.
- Common AI-slop constructions make the text sound polished while omitting the
  actor, action, artifact, boundary, measurement, or evidence.

Ignore or correct the checker when the flagged text is already precise, such
as a path, command, table cell, API name, quoted contract, empirical claim, or
canonical DDx term.

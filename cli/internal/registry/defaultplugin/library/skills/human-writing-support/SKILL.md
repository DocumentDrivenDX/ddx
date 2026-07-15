---
name: human-writing-support
description: Support DDx prose work by preserving voice, improving execution-useful clarity, applying deterministic prose findings with judgment, and keeping DDx terminology, headings, tables, paths, commands, and legitimate lists intact.
---

# Human Writing Support

Trigger: any writing, rewriting, editing, or reviewing of Markdown under
`docs/` (specs, ADRs, plans, beads, release notes, website copy).

This is guidance for better human writing, not an AI detector and not a
detector bypass tool. The goal is prose that helps a maintainer or agent
execute, review, or trust a DDx document.

## Required Workflow

After any docs edit:

1. Preserve the author's intent and DDx terminology.
2. Edit the document.
3. Run `ddx doc prose --changed`.
4. Apply high-signal findings.
5. Rerun `ddx doc prose --changed`.
6. Summarize remaining intentional exceptions in the change description.

`ddx doc prose` is the only public prose checker surface; do not invoke
Vale, Hemingway, or other tools directly. Findings are advisory, not
blocking — apply judgment.

## Preservation Rules

Keep these intact unless the task is explicitly about rewriting them:

- Paths, commands, IDs, frontmatter, headings, table structure, API names,
  function signatures, and acceptance criteria.
- Quoted source text (do not rewrite quotations).
- Useful technical density — dense can be correct.
- Legitimate technical lists, code blocks, and DDx canonical terms.

Do not "fix" a finding by weakening correct technical prose.

## Rewrite Guidance

When applying a finding:

- Replace broad claims with actor / action / artifact / evidence.
- Delete filler transitions when the sentence still reads correctly.
- Prefer shorter wording when it is equally precise.
- Split overstuffed sentences only when doing so improves reviewability.
- Leave a finding unresolved when the flagged text is precise; explain why
  in the change description.

## Examples

### 1. Unsupported benefit claim → concrete rewrite

Before:

> This change significantly improves the developer experience.

After:

> `ddx try` now writes per-attempt evidence under
> `.ddx/executions/<run-id>/`, so reviewers can inspect prompt, response,
> and merge result without rerunning the bead.

### 2. AI-slop paragraph → shorter actionable sentence

Before:

> In today's fast-paced software development landscape, it is critically
> important that we leverage best-in-class tooling to ensure our workflows
> remain robust, scalable, and future-proof across all stakeholders.

After:

> Run `ddx doc prose --changed` after editing docs so reviewers see the
> same findings the agent saw.

### 3. Token-cost edit → remove filler while preserving meaning

Before (38 words):

> It is worth noting that, in order to ensure that the bead is properly
> closed out, the agent should make sure to verify that all of the
> acceptance criteria have been satisfied before committing the change.

After (15 words):

> Verify every acceptance criterion before committing; do not close the
> bead with red tests.

### 4. False positive → leave unchanged

The checker flags "must" in:

> AC 6: `lefthook run pre-commit` must pass.

Leave it. "Must" is the correct contract verb for acceptance criteria.
Record the exception in the change description if `ddx doc prose --changed`
still surfaces it.

### 5. Table / path / API sample → do not rewrite

Do not rewrite cells, paths, or API examples even if the checker flags
them. For example, leave this table column intact:

| Command | Purpose |
|---|---|
| `ddx bead ready` | List beads with all deps satisfied. |
| `ddx try <id>` | Run one bead in an isolated worktree. |

And leave path / API samples such as `.ddx/executions/<run-id>/` or
`buildAgentRunner(ctx, cfg)` unchanged.

## Reporting Exceptions

When `ddx doc prose --changed` still has findings after the second run,
list the intentional exceptions and the rationale (for example: "AC verb
'must' is contract language", "table cells are canonical command names").
Silently ignoring a finding is not acceptable; report it.

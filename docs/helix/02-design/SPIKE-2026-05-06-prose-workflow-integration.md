---
ddx:
  id: SPIKE-2026-05-06-prose-workflow-integration
  depends_on:
    - FEAT-027
    - SPIKE-2026-05-06-prose-checker-engine-selection
  status: complete
---
# Spike: Prose Workflow Integration

## Question

Where should DDx run prose checks automatically so agents improve documents
without the operator having to ask?

## Integration Points

| Workflow | Behavior | Default policy |
|---|---|---|
| `ddx doc prose --changed` | Direct operator surface over changed Markdown. | Advisory |
| Explicit `ddx doc prose <paths>` | Direct operator surface over selected Markdown. | Advisory |
| Agent doc-edit work | Agent runs changed-prose check after editing `docs/`. | Advisory with self-fix |
| Bead review with prose | Review prompt includes prose findings separately from correctness findings. | Advisory |
| `ddx try` / `ddx work` doc attempts | If docs changed, attach prose findings to execution evidence. | Advisory |
| Pre-commit | Optional concise advisory output, never default blocking. | Advisory |

## Agent-Facing Flow

When an agent edits Markdown under `docs/`:

1. Run `ddx doc prose --changed`.
2. Apply high-signal findings with small local edits.
3. Preserve commands, paths, tables, headings, DDx IDs, frontmatter, and
   legitimate technical density.
4. Rerun `ddx doc prose --changed`.
5. Summarize remaining findings only when intentionally left in place.

The `human-writing-support` skill is the right home for this behavior. The
skill should remain judgment-oriented and should explicitly avoid banned-word
rewrites.

## User-Facing Flow

Routine output should stay concise:

- If no findings remain, say that prose check passed.
- If findings remain, show file, line, DDx rule id, rationale, and suggested
  edit.
- Do not show backend engine details unless the user requests diagnostics.
- Do not ask the user to install a third-party prose checker.

## Evidence Flow

For DDx execution attempts that modify docs, persist:

- checker engine and version
- checked paths
- finding count by DDx rule id
- remaining findings after agent self-fix
- prose-check command output or normalized JSON attachment

This keeps prose quality observable without turning it into a correctness gate.

## Open Decisions For ADR

- How should `ddx doctor` report pinned Vale version drift in non-verbose and
  verbose modes?
- Should pre-commit run the prose check by default, or only when installed
  project hooks opt in?
- What finding-count threshold is noisy enough to suppress automatic display
  and route findings into an artifact instead?

## Provisional Recommendation

Wire workflow behavior around the `ddx doc prose` surface, not around Vale
directly. ADR-025 selected Vale as the backend engine, but agents and users
should see DDx prose findings.

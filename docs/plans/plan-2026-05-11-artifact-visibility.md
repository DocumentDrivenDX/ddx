---
ddx:
  id: plan-2026-05-11-artifact-visibility
---
# FEAT-005 Amendment Proposal: `ddx.visibility` Frontmatter Field

Date: 2026-05-11
Status: Draft — extends FEAT-005 Artifact Convention with a visibility field consumed by the website generator (and any future consumer that needs publish-vs-private differentiation)

## Why this exists

The website autogen plan (`plan-2026-05-11-website-autogen.md`) needs a per-artifact signal for "should this publish to the public website?" The first-draft autogen plan proposed inventing a website-specific `ddx.public: true|false` frontmatter field. Both the self-review and an opus second-opinion converged: visibility is a property **of the artifact**, not of any one renderer. It belongs in FEAT-005 (the artifact convention spec), where other consumers — search, doctor diagnostics, doc-graph filters, future renderers — can also reach it.

Inventing the field at the website-plan level would create governance drift in the very infrastructure the website is supposed to track. Better: extend FEAT-005 once.

## Proposed addition to FEAT-005

Add a new optional field to the `ddx:` identity block:

```yaml
---
ddx:
  id: FEAT-007
  visibility: public         # public | internal | draft
  depends_on: [helix.prd]
---
```

### Semantics

| Value | Meaning |
|-------|---------|
| `public` | Approved for public publication. Renderers that respect visibility (website, public search index, external API surfaces) MUST include it. |
| `internal` | Default. Visible to project members and project-local tooling. Renderers that respect visibility MUST exclude it. |
| `draft` | Work in progress. Same exclusion as `internal` for public renderers, but may surface in `ddx doctor` / project-internal "draft" sections. |

If the field is absent, the value is `internal`. Fail-safe-private: a renderer that doesn't understand `visibility` does not accidentally leak content; a contributor who forgets to set the field doesn't accidentally publish.

### Why three values, not two

A boolean `public: true|false` collapses two distinct cases ("not ready" and "intentionally internal") into one. The three-value form matches HELIX's existing phase-driven lifecycle (artifacts at the discover/frame phase are conceptually drafts; promoted artifacts are reviewed-and-internal; published artifacts are public-approved).

### Default rationale

`internal` is the default for two reasons:

1. **Fail-safe.** If a contributor adds a new FEAT spec without thinking about visibility, the default keeps it out of public surfaces until explicitly promoted.
2. **Matches reality.** Most DDx artifacts today are internal (HELIX governance for one project). Only a curated subset is suitable for the public-facing website.

## Compatibility

- **No existing frontmatter changes.** Adding the field is additive; absent-value defaults to `internal`. Every existing FEAT/ADR/TD/persona/prompt file continues to validate.
- **No code changes** in `cli/internal/docgraph/` or other graph-consumers required for v1. They simply don't filter on visibility. The website generator is the first consumer that filters.
- **`ddx doctor`** can optionally surface counts ("12 internal FEATs, 3 public, 4 drafts") for awareness; not required.

## Other consumers (now or later)

- **Website generator** (immediate): includes artifacts where `visibility == public`. See `plan-2026-05-11-website-autogen.md`.
- **Public MCP surface** (planned): a Databricks-Assistant MCP exposing DDx state should only return public artifacts to non-member identities. See `docs/plans/plan-2026-05-10-mcp-architecture.md`.
- **`ddx search --public`** (future): a filter flag on the existing search tool.
- **Federated discovery** (future, FEAT-026): a federation hub may differentiate public catalog data from member-only data.

Each consumer reads the same field. No drift.

## Migration

Adding the field is opt-in per artifact. There is no required mass-rewrite of existing artifacts — they just stay `internal` (the default).

To make the website meaningful, a one-time bead does an audit pass:

1. Enumerate every artifact identified by `cli/internal/docgraph/`.
2. For each: ask "should this publish to the website?"
3. Update frontmatter to add `visibility: public` where yes; leave alone where no.

This bead is human-curated, not auto-generated — visibility is an editorial decision per artifact. The website autogen plan's "Phase C" depends on this bead being run for at least the initial set of public artifacts.

## Acceptance criteria for the FEAT-005 amendment

1. FEAT-005 §"Common Artifact Types" gains a new subsection §"Visibility" defining the three-value field, its semantics, and the fail-safe-private default.
2. `cli/internal/config/schema/config.schema.json` and any related config schema accept the new field (likely no schema change required because frontmatter is parsed as `map[string]any` today; verify).
3. `cli/internal/docgraph/` exposes an accessor (or surfaces in its existing artifact type) for the visibility field so consumers don't re-parse frontmatter.
4. No existing artifact frontmatter is required to change. `cd cli && go test ./internal/docgraph/...` is green without modifying test fixtures.
5. `lefthook run pre-commit` passes.

## Sequencing

This amendment is a **prerequisite** for the website autogen plan's Phase C (governance docs publication). It is **not** a prerequisite for Phase A (CLI commands, skills, plugins — none of which use governance-doc visibility).

File order:
1. FEAT-005 amendment bead (this plan).
2. `docgraph` exposes visibility (small bead, depends on #1).
3. Website autogen Phase A beads (independent of visibility).
4. Website autogen Phase B (independent of visibility).
5. Visibility audit pass for the initial set of public artifacts (depends on #2).
6. Website autogen Phase C — FEAT/ADR/TD generators (depends on #5).

## Open questions

- Should `visibility` cascade across `depends_on` edges? If `FEAT-X` is public but depends on `ADR-Y` which is internal, should the website show a broken link, an internal-marker link, or refuse to publish `FEAT-X`? Recommended: show broken link with a generator warning (caller knows what to fix); do not refuse to publish, because dependency-cascading-publish is a foot-gun.
- Should there be a `visibility: archived` state for old public artifacts? Out of scope for v1; `internal` covers retraction adequately.
- Does the field belong on every artifact type, or only on governance docs? Recommended: on every artifact type. The website will see persona/prompt/skill content too, and they need the same control.

---
ddx:
  id: helix.workflow.context-digest
  depends_on:
    - helix.workflow.principles-resolution
    - helix.workflow.concern-resolution
    - FEAT-006
---
# Context Digest Assembly

This reference defines how HELIX assembles a compact context digest into
beads at creation time, making them self-contained execution units.

## Purpose

Implementing agents should be able to work from a bead's description alone
without reading upstream files in the common case. The context digest
summarizes all cross-cutting concerns and governing context into
~1000-1500 tokens prepended to the bead description.

## When to Assemble

| Action | Behavior |
|--------|----------|
| `helix triage` | Assemble digest for every new bead |
| `helix evolve` | Assemble digest for beads it creates or modifies |
| `helix polish` | Refresh digest against current upstream state |
| `helix build` | Read digest from bead (do not reassemble) |
| `helix review` | Read digest to verify consistency |

## Assembly Algorithm

1. **Principles**: Load active principles per
   `.ddx/plugins/helix/workflows/references/principles-resolution.md`. Include the full list
   as a compact single line (separator: ` · `).

2. **Concerns**: Load active concerns per
   `.ddx/plugins/helix/workflows/references/concern-resolution.md`. Filter by bead area scope.
   Summarize matched concern names as `name | name | name`.
   The `<concerns>` element contains concern names only; never write
   `area:*` labels or synthetic scope labels there.

3. **Practices**: Load merged practices from area-matched concerns (library
   + project overrides). Summarize as key conventions (separator: ` · `).
   Prioritize: linter, formatter, testing, and language-config practices.

4. **ADRs**: Discover relevant ADRs using two paths (in priority order):
   a. **Primary — concern references**: Collect ADR references from each
      area-matched concern's `## ADR References` section, plus any ADRs
      cited in the project overrides section of `concerns.md`.
   b. **Secondary — spec/topic match**: Match ADR `Related` field against
      the bead's `spec-id` chain, and match ADR topic against the bead's
      `area:*` labels. This catches ADRs not yet wired into concerns.
   - For each relevant ADR: extract the decision statement and one-line
     rationale. Do not include alternatives or exploration.

5. **Governing spec**: Load the artifact referenced by `spec-id`.
   Extract:
   - The specific requirement or acceptance criterion this bead addresses
   - Key constraints relevant to implementation
   - Do not include the full spec — just the governing clause.

6. **Format**: Assemble into XML-tagged block (see format below).

7. **Write**: Prepend the `<context-digest>` block to the bead's
   `description` field. If a digest already exists, replace it.
   If the repository ships a digest helper script, use it instead of
   hand-assembling XML so the live queue and future beads stay consistent.
   When the governing contract explicitly permits omitting the digest,
   the bead must carry label `digest:omission-authorized`, the machine-set
   field `digest-omission-path`, and a description that begins with
   `Explicit omission rationale: <reason>` instead of a `<context-digest>`
   block. The rationale must be non-empty and explain why omission is
   allowed for that bead.
   HELIX currently defines exactly one allowed omission path:
   `helix-input:legacy-migration`. This path is reserved for `helix input`
   when it creates or updates a migrated legacy bead whose upstream concern
   mapping is not yet complete enough to assemble a trustworthy digest.
   Any other bead class must carry a full `<context-digest>`.

## Digest Format

```xml
<context-digest>
<principles>Principle 1 · Principle 2 · Principle 3</principles>
<concerns>Concern 1 | Concern 2 | Concern 3</concerns>
<practices>Practice 1 · Practice 2 · Practice 3</practices>
<adrs>ADR-NNN decision summary · ADR-NNN decision summary</adrs>
<governing>FEAT-NNN §X.Y — key requirement or constraint</governing>
</context-digest>
```

Each XML element is optional — omit it if there is no relevant content
(e.g., omit `<adrs>` if no ADRs are relevant, omit `<concerns>` if no
concerns are declared).

## Token Budget

| Section | Target | Notes |
|---------|--------|-------|
| Principles | ~100 tokens | Full list, compact format |
| Concerns | ~50 tokens | Matched concern names only |
| Practices | ~200 tokens | Key conventions, not exhaustive |
| ADRs | ~200 tokens per ADR | Decision + rationale only |
| Governing | ~300 tokens | Specific clause, not full spec |
| **Total** | **~1000-1500 tokens** | Less than one upstream file read |

If the total exceeds 2000 tokens (many ADRs, many concerns), prioritize:
principles first, then concerns/practices, then ADRs by relevance, then
governing spec context. Truncate ADR summaries before dropping them.

## Refresh at Polish Time

When `helix polish` encounters a bead with an existing
`<context-digest>`:

1. Re-run the assembly algorithm against current upstream state.
2. Compare each section against the existing digest.
3. If material changes exist:
   - Replace the `<context-digest>` block with the updated version.
   - Add a note to the bead: "Context digest refreshed: [what changed]".
4. If no changes: leave the digest untouched.

Material changes include: principle added/removed, concern changed,
practice overridden, ADR superseded, governing spec amended.

## Reading the Digest at Build Time

When `helix build` or `helix review` processes a bead with a
`<context-digest>`:

1. Parse the XML tags from the description.
2. Use the digest contents as authoritative context for this bead.
3. Do not redundantly read the upstream files that the digest summarizes.
4. If the digest is missing (legacy bead), fall back to reading upstream
   files directly.

Trust the digest — rely on `helix polish` to keep it current.

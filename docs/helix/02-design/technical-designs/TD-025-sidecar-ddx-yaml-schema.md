---
ddx:
  id: TD-025
  depends_on:
    - FEAT-005
    - FEAT-007
---
# Technical Design: Sidecar `.ddx.yaml` Schema

## Purpose

This design specifies the sidecar file format that extends DDx artifact identity
to non-markdown files (images, diagrams, PDFs, binary exports, plain-text
prompts, etc.). It covers the decisions the scanner and validator must agree
on:

1. **Pairing rule** — how a sidecar file maps to its paired asset.
2. **Identity source rule** — when embedded frontmatter is used versus when a
   sidecar is used.
3. **Conflict rule** — what happens when a file could claim identity via both
   embedded frontmatter and a sidecar.
4. **Compatibility rule** — how existing markdown artifacts and sidecars with
   missing optional fields remain readable.
5. **Orphan-check participation** — how sidecar-identified artifacts are counted
   in `ddx doc validate`.

This document is the implementation sketch for the artifact identity schema
introduced by FEAT-005 and indexed by FEAT-007.

## Pairing Rule

A sidecar is a YAML file whose name is the **full filename of the paired asset
with `.ddx.yaml` appended**:

```
foo.png          ←→  foo.png.ddx.yaml
diagram.excalidraw ←→ diagram.excalidraw.ddx.yaml
hero.svg         ←→  hero.svg.ddx.yaml
arch.pdf         ←→  arch.pdf.ddx.yaml
system-prompt.txt ←→ system-prompt.txt.ddx.yaml
```

Rules:

- The sidecar and its paired asset **must reside in the same directory**.
  A `foo.png.ddx.yaml` in a subdirectory does **not** pair with a `foo.png`
  in the parent directory, even if the paths are otherwise nearby.
- Sidecar filenames are **case-sensitive** on case-sensitive filesystems. The
  scanner must match exactly and must not normalize case.
- A `.ddx.yaml` file with no matching asset alongside it is a dangling sidecar.
  `ddx doc validate` reports it as an error (not just a warning), because it
  implies either the asset was deleted or the sidecar was incorrectly named.

### Why append, not replace

Appending `.ddx.yaml` to the full filename (including any existing extension)
preserves the original filename as an unambiguous prefix. Replacing the last
extension (`foo.png` → `foo.ddx.yaml`) creates ambiguity: a file named
`foo.ddx.yaml` alongside a `foo.png` would be misread as the sidecar of a
hypothetical `foo.ddx` asset, and files without extensions (`Makefile`,
`Dockerfile`) would produce bare `.ddx.yaml` sidecar names that glob patterns
cannot reliably distinguish from project-level config files.

### Glob pattern for discovery

The scanner uses the glob `**/*.ddx.yaml` to collect all sidecar candidates,
then strips the `.ddx.yaml` suffix to locate the expected paired asset.

## Sidecar File Schema

A sidecar is a YAML document (no frontmatter delimiters — it is pure YAML, not
markdown). The top-level key is `ddx:`, matching the frontmatter namespace used
in markdown artifacts.

### Minimal example

```yaml
ddx:
  id: DIAG-001
```

### Full example

```yaml
ddx:
  id: DIAG-001
  media_type: image/png
  depends_on:
    - FEAT-005
    - FEAT-007
  generated_by:
    run: run-20260429T221533Z-a1b2c3d4
    prompt: PROMPT-visual-suite-principles
    source_hash: sha256:8b0f4a74f0b7e7d4f6c6f9f8e5c7a2d9b2a8d3e9c1a7f4b0d6e8a9f2b3c4d5e6
  inputs:
    - node:FEAT-005
  parking_lot: false
```

### Field reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | **yes** | Artifact ID. Same prefix conventions as markdown artifacts (e.g., `DIAG-001`, `HERO-003`). Must be unique across the project graph. |
| `media_type` | string | **yes for sidecars** | IANA media type of the paired asset (e.g., `image/png`, `image/svg+xml`, `application/pdf`). New sidecars must set it explicitly. When absent in existing sidecars, DDx infers from the file extension as a compatibility fallback and reports a validation warning. |
| `depends_on` | []string | no | IDs of artifacts this one depends on. Creates `depends_on` edges in the document graph. Staleness cascades from these edges as with any artifact. |
| `generated_by` | object | no | Provenance for generated artifacts: `{ run, prompt, source_hash }`. Creates a `generated_by` edge to the producing run, which is **provenance-only** — it does not trigger staleness cascade (see Staleness below). |
| `inputs` | []string | no | Input selectors for prompt context resolution. Same selector syntax as markdown frontmatter. |
| `review` | object | no | Staleness tracking metadata (managed by `ddx doc stamp`). Same sub-fields as markdown: `self_hash`, `deps`, `reviewed_at`. The hash covers the paired asset bytes, not the sidecar YAML. |
| `parking_lot` | bool | no | When `true`, skip in staleness checks. Same semantics as markdown. |

Unknown fields within `ddx:` are preserved on round-trip.

### Frontmatter vs Sidecar Discovery

DDx has exactly two identity sources:

| Artifact file | Identity source | Notes |
|---------------|-----------------|-------|
| Markdown (`*.md`, `*.mdx`) | Embedded `ddx:` YAML frontmatter | Existing document artifacts remain valid. Missing `media_type` defaults to `text/markdown`. |
| Non-markdown media (`*.png`, `*.svg`, `*.pdf`, `*.excalidraw`, etc.) | Sibling sidecar named `<full-filename>.ddx.yaml` | Sidecars carry the same `ddx:` identity schema and hash the paired file bytes for review metadata. |

The scanner first collects sidecar candidates, resolves their paired assets,
and records hard errors for dangling or markdown-paired sidecars. It then scans
markdown frontmatter. The final graph is the union of valid identities from
both sources, with duplicate IDs detected across the union.

### `generated_by` vs `depends_on` staleness semantics

| Edge type | Triggers staleness cascade? | Meaning |
|-----------|----------------------------|---------|
| `depends_on` | **Yes** | This artifact is governed by the upstream. If the upstream changes, this artifact may need updating. |
| `generated_by` | **No** | Provenance only. Records the run and source hash that produced the artifact. A source-hash mismatch makes only this generated artifact stale and offers explicit regeneration. |

This distinction mirrors the plan decision: "`generated_by` edge with separate
staleness rule (provenance, not authority — does not cascade like `depends_on`)."

`generated_by` fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `run` | string | **yes** | Run id from the unified FEAT-010 substrate. Layer-1 runs are typical; layer-2 runs are allowed when the generator edits the repo. |
| `prompt` | string | no | Prompt artifact id or generator prompt reference used to produce the asset. |
| `source_hash` | string | no | Hash of source inputs used when the artifact was produced. If the current source hash differs, only the generated artifact is stale. |

DDx does not infer a `generated_by` value. Absence means "not generated or
provenance unknown."

## Compatibility Rule

Existing markdown artifacts continue to work without edits:

- Embedded markdown frontmatter remains authoritative for markdown artifacts.
- Missing `media_type` on markdown defaults to `text/markdown`.
- Missing `generated_by` simply means there is no provenance edge.

Existing sidecars with only `ddx.id` remain readable so adoption can be
incremental:

- Missing `media_type` is inferred from extension for read/display and reported
  by `ddx doc validate` as a warning so authors can make the identity explicit.
- Missing `generated_by` does not create a provenance edge.
- Unknown fields are preserved on round-trip and ignored by graph construction
  unless a later feature defines them.

This compatibility is read-time only. New sidecar examples, templates, and
writers must emit explicit `media_type`.

## Conflict Rule

**Markdown files use embedded frontmatter exclusively. Non-markdown files use
sidecars exclusively. Mixing the two on a single asset is a validation error.**

The concrete conflict cases and their resolution:

| Situation | Outcome |
|-----------|---------|
| Markdown file with `ddx:` frontmatter; no sidecar alongside it | Normal markdown artifact. |
| Non-markdown file with a sidecar; no embedded YAML possible | Normal sidecar artifact. |
| Markdown file (`*.md`) with a `*.md.ddx.yaml` sidecar alongside it | **Validation error.** `ddx doc validate` reports: `<path>.md.ddx.yaml: sidecar alongside markdown file '<path>.md' — markdown artifacts declare identity in frontmatter; remove the sidecar`. The sidecar is ignored for graph construction to avoid duplicate IDs. |
| Two sidecars claim the same `id` as an existing markdown artifact | **Validation error.** Duplicate-ID check fires identically to the case where two markdown files share an ID. |
| A sidecar's `id` duplicates another sidecar's `id` | **Validation error.** Same duplicate-ID check. |

The prohibition on markdown+sidecar combination is strict because DDx's authority
rule is "identity present → artifact." Allowing both sources on a markdown file
would require a precedence rule, invite inconsistency, and add scanner complexity
for zero real-world benefit — markdown files can always carry their own identity
in frontmatter.

## Orphan-Check Participation

Sidecar-identified artifacts participate in `ddx doc validate` orphan detection
**identically to markdown artifacts**. No special treatment is applied.

An artifact is an orphan when it has no incoming edges (nothing depends on it)
**and** no outgoing edges (it depends on nothing). Sidecar artifacts with at
least one `depends_on` or one reverse-dependency edge from another artifact are
not orphaned.

### Common non-orphan patterns for sidecar artifacts

```
FEAT-005 ──depends_on──▶ DIAG-001   (FEAT-005 depends on the diagram)
DIAG-001 ──depends_on──▶ FEAT-007   (diagram depends on the doc-graph spec)
DIAG-001 ──generated_by─▶ run-...    (provenance only — not an authority edge)
```

A `generated_by` edge **does count** toward orphan suppression: if DIAG-001
has only a `generated_by` link to a producing run, it is not considered orphaned, because
the provenance edge establishes that the asset is intentional and traceable.

### Dangling sidecar rule (distinct from orphan)

A `.ddx.yaml` file with no paired asset is **not** an orphan — it is a dangling
sidecar. These are reported separately from graph orphans:

```
ERROR: foo.png.ddx.yaml: sidecar has no paired asset 'foo.png' in the same directory
```

Dangling sidecars are hard errors (not warnings) because they indicate either
an accidental deletion or a copy-paste naming mistake, and they leave an artifact
ID in the graph that resolves to no file.

## Scanner Integration

The FEAT-007 scanner refactor must handle sidecars in the following order:

1. Glob `**/*.ddx.yaml` → collect sidecar candidates.
2. For each candidate, strip `.ddx.yaml` suffix → derive expected paired-asset path.
3. Check: does a file exist at that path in the same directory?
   - No → record dangling sidecar error; skip adding to graph.
   - Yes, and the paired asset is a `.md` file → record markdown+sidecar conflict error; skip.
   - Yes, and paired asset is non-markdown → parse YAML, extract `ddx:` block, infer `media_type` only if missing, add artifact to graph.
4. Continue normal markdown frontmatter scan for all `*.md` files.
5. Build unified graph from both sources. Apply duplicate-ID detection across both.

The scanner must not attempt to parse binary assets. It only reads the `.ddx.yaml`
sidecar file; the paired asset's bytes are only accessed when computing
`review.self_hash` during `ddx doc stamp`.

## Content Hashing for `ddx doc stamp`

When `ddx doc stamp` processes a sidecar artifact:

- `review.self_hash` is the SHA-256 of the **paired asset bytes** (not the sidecar YAML).
- The sidecar YAML is rewritten in-place with the updated `review` block.
- The hash covers the complete byte sequence of the paired asset as it exists on disk; no normalization is applied (images and binary files have no canonical form).

## Validation Summary

`ddx doc validate` checks on sidecar artifacts:

| Check | Severity | Message pattern |
|-------|----------|-----------------|
| Dangling sidecar (no paired asset) | error | `<sidecar>: sidecar has no paired asset '<asset>' in the same directory` |
| Sidecar alongside a markdown file | error | `<sidecar>: sidecar alongside markdown file '<md>' — use frontmatter instead` |
| Duplicate ID (sidecar vs. any artifact) | error | `duplicate id '<id>': found in '<path-1>' and '<path-2>'` |
| Sidecar with no `ddx.id` field | error | `<sidecar>: missing required field ddx.id` |
| Sidecar with no `ddx.media_type` field | warning | `<sidecar>: missing ddx.media_type; inferred '<type>' from paired asset extension` |
| Sidecar artifact with no graph edges (orphan) | warning | `<id>: artifact has no edges — add depends_on or ensure another artifact depends on it` |
| `depends_on` references unknown ID | warning | `<id>: unknown dependency '<ref-id>'` |
| `generated_by.run` references unknown run ID | warning | `<id>: unknown generated_by.run '<run-id>'` |

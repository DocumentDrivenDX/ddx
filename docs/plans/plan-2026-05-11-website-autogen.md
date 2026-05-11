---
ddx:
  id: plan-2026-05-11-website-autogen
---
# Website Auto-Generation + Templating Architecture (v2)

Date: 2026-05-11
Status: Draft v2 — structural revision after self-review + opus pass converged on six issues with v1. v1 proposed 7 generators and 9 layouts duplicating what DDx already has (FEAT-005 artifacts, FEAT-007 doc graph, FEAT-018 plugin API). This rewrite consolidates around the existing substrate.

## Why this exists

The parent plan (`plan-2026-05-11-website-evolution.md`) marks auto-generation as the spine: "no hand-authoring, no LLM, no staleness." This document specifies the auto-generation surface AND the templating system together — they interact: generators produce data; templates render it; tokens style it; freshness is enforced in CI.

## Companion documents (load-bearing prerequisites)

| Document | Why it's a prerequisite |
|----------|------------------------|
| `plan-2026-05-11-artifact-visibility.md` (FEAT-005 amendment) | Defines `ddx.visibility: public\|internal\|draft` — the filter the generator uses. Without it, this plan would invent the same primitive at the wrong scope. |
| `plan-2026-05-11-ddx-introspect.md` | Defines the versioned Cobra command-tree JSON the website CLI generator (and the pip Python codegen) both consume. Without it, the website generator re-walks Cobra internals and diverges from the pip codegen. |
| FEAT-003 (existing website spec) | Must be amended before bead A1 to capture the locked architecture. Otherwise specs and beads disagree. |

## Locked decisions

| # | Decision |
|---|----------|
| 1 | **Hextra removed.** All inner pages get custom layouts extending one `baseof.html`. |
| 2 | **Templates share design tokens via a single CSS file imported by one `baseof.html` chrome.** Tokens never duplicate. |
| 3 | **Auto-generation is the spine.** No hand-authoring, no LLM, no staleness. Deterministic generators project the artifact graph into Hugo data files at CI time; generated artifacts are not version-controlled. |
| 4 | **Publishing stays at `DocumentDrivenDX.github.io/ddx/`** (subdirectory). |
| 5 | **The auto-generator is unified.** ONE binary, three code paths: artifact-graph projector, Cobra introspector consumer, plugin-manifest reconciler. Not seven bespoke walkers. |
| 6 | **Layouts are minimal.** ONE generic `_default/artifact-{single,list}.html` plus type-specific overrides only where the data shape genuinely differs (CLI command flag table; FEAT depends_on graph). |
| 7 | **Cross-references resolve at generate-time.** Generator emits `{id, url}` pairs; dead links surface as generator errors at CI time, not as broken HTML in production. |

## Architecture in one diagram

```
DDx artifact graph (FEAT-007)              ddx __introspect (versioned JSON)
        │                                          │
        │  filter: visibility == public            │
        ▼                                          ▼
  ┌─────────────────────────────────────────────────────┐
  │ cli/tools/gen-website/                              │
  │   main.go                                           │
  │   artifact_projector.go    consumes graph           │
  │   cli_introspector.go      consumes __introspect    │
  │   plugin_reconciler.go     consumes library + reg   │
  │   cross_ref_resolver.go    id → url at generate-    │
  │                            time, dual-emit          │
  └─────────────────────────────────────────────────────┘
        │
        ▼
  website/data/                       (git-ignored, generated)
    artifacts/<type>/<id>.yaml
    artifact-types.yaml               (from plugin API Surface 6)
    cli/commands.yaml
    plugins.yaml
        │
        ▼
  website/layouts/
    _default/baseof.html              sole chrome owner
    _default/single.html              generic
    _default/list.html                generic
    _default/artifact-single.html     generic artifact layout (most types)
    _default/artifact-list.html       generic artifact listing
    cli-command/single.html           override: flag table + signature
    feat/single.html                  override: depends_on graph render
    partials/                         shared (nav, footer, head, badge, table)
        │
        ▼
  website/public/                     (hugo --minify output, deployed)
```

Three new code paths (`artifact_projector`, `cli_introspector`, `plugin_reconciler`) instead of seven near-identical walkers. Two layouts plus two overrides instead of nine layout directories.

## Templating architecture

Same shape as v1 — the templating critique landed cleanly. Repeating it here for completeness:

### Directory layout

```
website/
  hugo.yaml
  assets/
    css/
      tokens.css           # ALL design tokens (palette, type scale, spacing, breakpoints)
      base.css             # reset + element styles, consumes tokens.css
      components.css       # nav, card, button, hero, callout, badge — consumes tokens.css
      layouts.css          # page-level grids — consumes tokens.css
      print.css
    js/
      pagefind.js          # build-time search index
  layouts/
    _default/
      baseof.html          # SOLE chrome owner: <head>, nav, footer, CSS imports
      single.html          # generic single-page (for hand-authored content like /why/)
      list.html            # generic listing
      artifact-single.html # generic single-artifact page; consumes data/artifacts/<type>/<id>.yaml
      artifact-list.html   # generic listing of artifacts of one type
      404.html
    cli-command/
      single.html          # override: command signature + flag table
      list.html            # override: command-tree navigation
    feat/
      single.html          # override: depends_on graph render
    partials/
      head.html            # <head> contents — CSS imports live here only
      nav.html
      footer.html
      breadcrumb.html
      artifact-header.html       # title, type-badge, metadata
      artifact-frontmatter.html  # full frontmatter table
      artifact-related.html      # depends_on / depended_by links from resolved cross-refs
      flag-table.html            # used by cli-command/single
      maturity-badge.html
    shortcodes/
      asciinema.html
      maturity.html
      figure.html
  data/                    # generated; .gitignored; populated in CI
    artifacts/<type>/<id>.yaml
    artifact-types.yaml
    cli/commands.yaml
    plugins.yaml
  content/                 # hand-authored only — no auto-gen output here
    _index.md              # homepage
    why/                   # editorial copy
    docs/
      cli/_index.md        # stub: header note only; list rendered by cli-command/list.html
      _index.md
```

### How tokens propagate without duplication

Unchanged from v1. `baseof.html` is the only file that imports CSS via `partial "head.html"`. Every other layout extends `baseof` via `{{ define "main" }}`. Change tokens.css → every page updates. Add a content type → new layout extending baseof inherits chrome automatically.

```html
<!-- layouts/_default/baseof.html -->
<!DOCTYPE html>
<html lang="en">
{{- partial "head.html" . -}}
<body>
  {{- partial "nav.html" . -}}
  <main>{{- block "main" . }}{{ end -}}</main>
  {{- partial "footer.html" . -}}
</body>
</html>
```

```html
<!-- layouts/_default/artifact-single.html -->
{{ define "main" }}
  <article class="artifact">
    {{ partial "artifact-header.html" . }}
    <section class="content">{{ .Content }}</section>
    {{ partial "artifact-frontmatter.html" . }}
    {{ partial "artifact-related.html" . }}
  </article>
{{ end }}
```

```html
<!-- layouts/cli-command/single.html — overrides ONLY the body -->
{{ define "main" }}
  <article class="cli-command">
    {{ partial "artifact-header.html" . }}
    <pre class="signature">{{ .Params.signature }}</pre>
    <section class="content">{{ .Content }}</section>
    {{ partial "flag-table.html" . }}
    {{ partial "artifact-related.html" . }}
  </article>
{{ end }}
```

The cli-command override is ~10 lines; everything else inherits.

## Auto-generation surface

### One generator binary, three code paths

```
cli/tools/gen-website/
  main.go                  # CLI entry; flags; orchestration; --collect-errors aggregation
  artifact_projector.go    # consumes graph; emits artifacts/<type>/<id>.yaml + artifact-types.yaml
  cli_introspector.go      # consumes ddx __introspect; emits cli/commands.yaml
  plugin_reconciler.go     # consumes library/plugins/ + cli/internal/registry/; emits plugins.yaml
  cross_ref_resolver.go    # id → url map; dual-emits resolved URLs alongside raw ids
  internal/
    yamlemit.go            # stable, sorted, deterministic YAML output
    typeregistry.go        # reads artifact types from plugin API Surface 6
```

### Path 1: Artifact-graph projector (the bulk of the work)

Consumes the doc graph (`cli/internal/docgraph/`), enumerates every artifact with `ddx.visibility: public`, and projects each into a Hugo data file.

**Inputs:**
- Artifact graph from `docgraph.Build(rootDir)` or equivalent.
- Artifact type registry from plugin API Surface 6 (see `artifact-types.yaml` below).

**Output per artifact:** `website/data/artifacts/<type>/<id>.yaml`

```yaml
id: FEAT-007
type: feat
title: Document Graph
path: docs/helix/01-frame/features/FEAT-007-doc-graph.md
visibility: public
depends_on:
  - id: helix.prd
    url: /docs/external/helix-prd/
    visibility: public
  - id: FEAT-005
    url: /docs/feats/feat-005/
    visibility: public
depended_by:
  - id: FEAT-009
    url: /docs/feats/feat-009/
    visibility: public
content_md: docs/helix/01-frame/features/FEAT-007-doc-graph.md   # Hugo renders via readFile/markdownify
frontmatter:
  status: Draft
  priority: P1
  owner: DDx Team
```

**Output `artifact-types.yaml`** (driven by plugin API Surface 6 declarations):

```yaml
types:
  feat:
    display_name: Feature Specifications
    section_path: /docs/feats/
    description: Public feature specifications for DDx capabilities.
    badge_color: var(--color-feat)
    sort_key: id
  adr:
    display_name: Architecture Decision Records
    section_path: /docs/adrs/
    ...
  td:
    display_name: Technical Designs
    ...
  persona:
    display_name: Personas
    section_path: /docs/personas/
    ...
```

The `artifact-types.yaml` is read by `_default/artifact-list.html` to render section indexes — no hand-authored section metadata anywhere. Adding a new artifact type means a plugin declares it; the website picks it up automatically.

### Path 2: CLI introspector consumer

Consumes `ddx __introspect` output (per `plan-2026-05-11-ddx-introspect.md`) and projects into Hugo data.

**Input:** the v1 JSON document from the introspect primitive.

**Output:** `website/data/cli/commands.yaml`

```yaml
commands:
  - name: bead
    full_name: ddx bead
    path: [bead]
    short: Manage work item tracker
    long_md: bead.long.md   # body extracted to website/data/cli/help/bead.long.md
    signature: "ddx bead [subcommand]"
    flags: [...]
    inherited_flags: [...]
    args: []
    subcommands:
      - name: create
        ...
```

The CLI generator does NOT subprocess to `ddx __introspect` — it imports `cli/internal/introspect.Walk(...)` directly. Same data, no JSON round-trip. The subprocess form exists for cross-language consumers (pip Python codegen).

### Path 3: Plugin manifest reconciler

Reads BOTH `library/plugins/*/PACKAGE.yaml` AND the canonical list in `cli/internal/registry/registry.go`. Fails the generator if a plugin is present in one but not the other (the most common drift case).

**Output:** `website/data/plugins.yaml`

```yaml
plugins:
  - name: ddx
    description: The default DDx plugin.
    registered: true                  # found in cli/internal/registry/
    package_yaml: library/plugins/ddx/PACKAGE.yaml
    artifact_types: [feat, adr, td, persona, prompt, skill]   # from plugin's Surface 6 declaration
    version: 1.0.0
  - name: helix
    ...
```

This generator both produces Hugo data and acts as a drift detector for the registry. CI failure here means: a plugin was added to `library/` without registration, or vice versa.

### Cross-reference resolution

Implemented in `cross_ref_resolver.go`, consumed by all three paths:

1. Generator first pass: enumerate every artifact eligible for publication. Build the `id → url` map. URLs derived from `<artifact-type-section-path>/<id-lowercase>/` per the artifact-types config.
2. Second pass: for every emitted artifact, every `depends_on` entry resolves to `{ id, url, visibility }`. If `visibility != public`, URL is omitted but `visibility: internal` is preserved so templates can render an internal-marker badge (or skip the link).
3. **Dead-link enforcement.** If `depends_on` references an `id` that doesn't exist in the project at all (typo, deleted artifact), the generator fails with `path:line: dead reference: <id>` — caller knows exactly what to fix.

The resolved data goes into the data file. Templates render hyperlinks directly without any Hugo-side lookup logic.

## Freshness enforcement

Unchanged from v1 in principle; sharpened on error ergonomics per opus review:

### 1. Generated artifacts are never checked in

`website/data/` is in `.gitignore`. Generators run in CI before Hugo build. Locally: `make website-dev` runs generators + `hugo server`. No version-controlled stale state.

### 2. CI runs generators with `--collect-errors`

```yaml
- name: Generate website data
  run: |
    go run ./cli/tools/gen-website \
      --collect-errors \
      --out ./website/data \
      || (echo "::error::Website generator failed; see path:line rollup above" && exit 1)

- name: Build Hugo site
  run: cd website && hugo --minify
```

`--collect-errors` accumulates every fixable issue (missing required frontmatter field, dead cross-reference, plugin reconciliation drift, unknown artifact type) into a single `path:line: <reason>` rollup printed at exit. Operator sees every issue in one CI run, not seven sequential pushes.

### 3. Per-path Go tests

```go
func TestArtifactProjector_ProducesValidYAML(t *testing.T) { ... }
func TestCLIIntrospector_HandlesUnknownFlagType(t *testing.T) { ... }
func TestPluginReconciler_FailsOnDrift(t *testing.T) { ... }
func TestCrossRefResolver_FailsOnDeadReference(t *testing.T) { ... }
```

These run as part of `cd cli && go test ./tools/...`. Catches schema-changes-but-generator-didn't at test time.

### Local dev preview

```makefile
website-dev:
	go run ./cli/tools/gen-website --out ./website/data
	cd website && hugo server -D

website-clean:
	rm -rf website/data website/public website/public-test
```

Single command matches CI exactly.

## Bead sequencing

**Prerequisites** (file these as separate beads, sequenced before A1):

| Bead | Description |
|------|-------------|
| **P1 — FEAT-005 visibility amendment** | Per `plan-2026-05-11-artifact-visibility.md`. Add `ddx.visibility: public\|internal\|draft` to the artifact frontmatter spec. Expose accessor in `cli/internal/docgraph/`. |
| **P2 — `ddx __introspect` primitive** | Per `plan-2026-05-11-ddx-introspect.md`. Hidden Cobra command + golden test + versioned JSON schema. Upstream of pip codegen AND website CLI generator. |
| **P3 — FEAT-003 amendment** | Capture the locked architecture (Hextra removal, unified generator, minimal layouts, cross-ref resolution, visibility-driven publication) in the existing website feature spec. Per the project's standing "plans → FEAT updates → beads" flow. |
| **P4 — Plugin API Surface 6 artifact-type declarations** | Verify FEAT-018 §Surface 6 lets a plugin declare display metadata (name, section_path, badge_color, sort_key). Amend if missing. |
| **P5 — Visibility audit pass** | Curate which artifacts (FEAT/ADR/TD/persona/etc.) get `visibility: public`. Editorial decision per artifact; one-time bead. |

### Phase A — Foundation (parallel-safe after prerequisites)

| Bead | Description |
|------|-------------|
| **A1 — Templating foundation** | `layouts/_default/baseof.html`, `head.html`, `nav.html`, `footer.html`, `tokens.css`, `base.css`, `components.css`, `layouts.css`. Update `layouts/index.html` to extend `baseof`. |
| **A2 — Hextra removal** | Remove Hextra Hugo module; remove `hx:` class references; audit `content/` for `{{< hextra/... >}}` shortcodes; rewrite or remove. Generic `single.html` and `list.html` extend `baseof`. |
| **A3 — Generic artifact layouts** | `_default/artifact-{single,list}.html` consuming `data/artifacts/<type>/<id>.yaml` + `data/artifact-types.yaml`. Partials: `artifact-header`, `artifact-frontmatter`, `artifact-related`. |
| **A4 — Generator binary scaffolding** | `cli/tools/gen-website/` with `main.go` + `internal/yamlemit.go` + cross-ref resolver skeleton. `--collect-errors` mode. Empty stubs for the three path implementations. |
| **A5 — Artifact projector** | `artifact_projector.go` + `typeregistry.go`. Consumes graph; emits artifacts data + artifact-types data. |
| **A6 — CLI introspector consumer** | `cli_introspector.go`. Consumes `ddx __introspect` via direct package import. Emits `cli/commands.yaml`. Depends on P2. |
| **A7 — Plugin reconciler** | `plugin_reconciler.go`. Emits `plugins.yaml`; fails on registry drift. |
| **A8 — CI integration** | Pre-build codegen step in `.github/workflows/pages.yml`; `Makefile` targets; `website/data/` in `.gitignore`. |

### Phase B — Specialized layouts

| Bead | Description |
|------|-------------|
| **B1 — CLI command override layout** | `layouts/cli-command/{single,list}.html`: signature partial + flag-table partial. |
| **B2 — FEAT override layout** | `layouts/feat/single.html`: depends_on graph rendering using the cross-ref-resolved data. |
| **B3 — Pagefind search** | Build-time search index over generated content; top-nav search box. |

### Phase C — content reach

| Bead | Description |
|------|-------------|
| **C1 — Sidecar artifact ingest** | Generator handles sidecar `.ddx.yaml` artifacts (diagrams, images) per FEAT-005. |
| **C2 — Generated CLI long-help cleanup** | Lint pass on `cmd.Long` bodies: reject `{{ }}`-looking content; ensure CommonMark. |

Phase C is enabling-work, not value-delivery; B1 and B2 deliver visible improvements. A8 is the gate for any deploy.

## Risks

| Risk | Mitigation |
|------|------------|
| Artifact graph doesn't currently expose visibility | Prerequisite P1 adds it. A5 depends on P1. |
| `cli/internal/introspect/` package doesn't exist | Prerequisite P2 creates it. A6 depends on P2. |
| Plugin API Surface 6 doesn't actually declare display metadata today | Prerequisite P4 verifies/amends FEAT-018. A5 depends on P4. |
| FEAT-003 amendment contradicts existing FEAT-008/FEAT-021 (dashboard UI, web UI) | Survey those specs during P3 amendment; reconcile or note explicitly. The autogen-static-site work and the live-server dashboard work are separate surfaces. |
| Hugo's CommonMark renderer chokes on Cobra long-help | C2 lint pass; explicit AC. |
| Local dev needs a built `ddx` binary | `make website-dev` documents `make build` as a prerequisite; better, gen-website imports introspect package directly (no binary needed). |
| Generated YAML grows large enough to slow Hugo | Per-artifact file layout (`data/artifacts/<type>/<id>.yaml`) scales linearly; Hugo handles thousands of data files efficiently. |
| Visibility cascade ambiguity (FEAT-X is public but its depends_on FEAT-Y is internal) | Generator emits both id+url; if target visibility != public, omit url, render internal-marker badge. Documented in `artifact-visibility` plan §"Open questions." |

## What this delivers

Once Phase A lands, every `FEAT-*.md`, `ADR-*.md`, `TD-*.md`, `persona/*.md`, `prompt/*.md`, `skill/SKILL.md`, plugin descriptor, MCP server descriptor, and CLI command WITH `ddx.visibility: public` appears on the website automatically. Editing the source file IS editing the website. The site never goes stale because it has no hand-curated version of any of those things.

What stays hand-authored: `content/_index.md` (homepage editorial copy), `content/why/` (narrative content). The plan explicitly acknowledges these as exempt — but identifies opportunities (active FEAT list, current focus, recent activity) that could come from project state in a future iteration.

## Open questions

1. **Homepage active-FEAT section** — should the homepage have a generated "currently working on" section that lists FEAT specs with `status: In Progress` and `visibility: public`? Out of scope for v2; flagging for v3.
2. **Tier 3 ordering** — does P5 (visibility audit) really need to happen before A5 (artifact projector)? Probably not — A5 can land with zero `public` artifacts and just produce an empty data dir. P5 then unlocks each artifact one at a time. Decoupling P5 from A5 simplifies sequencing.
3. **Artifact body rendering** — Hugo's `readFile + markdownify` is the natural fit, but means the artifact body's markdown is processed without Hugo's full content-pipeline (no Hugo shortcodes inside artifact bodies). Acceptable trade; document explicitly.
4. **Plugin discovery scope** — the generator runs against the source repo; it sees only `library/` (the default DDx plugin). It does NOT render artifacts from arbitrarily-installed plugins in some user's `.ddx/plugins/`. Documented; the website renders the default plugin's content, full stop.

---
ddx:
  id: SPIKE-2026-05-06-prose-checker-engine-selection
  depends_on:
    - FEAT-027
  status: complete
---
# Spike: Prose Checker Engine Selection

## Question

Which deterministic prose checker engine can DDx make native enough to serve as
the single default implementation behind `ddx doc prose`?

## Fitness Criteria

| Criterion | Requirement |
|---|---|
| Installation | Works from the DDx installation path without user-managed runtimes or package managers. |
| Maintenance | Version is controlled by DDx release/build process. |
| Finding shape | Maps to DDx file, line, rule id, severity, rationale, and suggested edit. |
| Rule authoring | Can express DDx-specific AI-slop and execution-usefulness rules. |
| Markdown handling | Preserves frontmatter, code spans, commands, paths, tables, headings, and DDx IDs. |
| Noise control | Supports sparse, high-signal findings on real DDx docs. |
| Workflow fit | Can run in changed-file mode and full-doc mode without project-local config. |

## Candidate Summary

| Candidate | Installation fit | Rule fit | Initial verdict |
|---|---:|---:|---|
| Embedded Go checker | Strong | Medium | Keep as fallback/reference; not selected by ADR-025. |
| Vale | Strong if installed as pinned release binary on `PATH` | Strong | Selected by ADR-025; reject local build/package-manager paths. |
| textlint | Weak | Strong | Reject as default unless DDx accepts Node runtime, which it should not. |
| write-good | Weak | Weak | Reject as default; signal is too generic for DDx. |
| alex | Weak | Narrow | Reject as prose engine; possible research input for inclusive-language rules only. |
| LanguageTool | Weak | Different problem | Reject as default; grammar server/runtime weight does not match DDx prose-quality goal. |

## Initial Execution Evidence

Environment:

- `node` and `npm` are available locally, but using them would violate the
  default DDx install constraint.
- `java` is available locally, but using it would violate the default DDx
  install constraint.
- `vale` was not preinstalled.

Candidate checks:

- `npm view write-good textlint alex` showed MIT-licensed packages, but all are
  Node-distributed tools.
- `npx alex` on the DDx slop/good samples reported no issues. That is expected:
  alex targets insensitive wording, not DDx execution-usefulness.
- `npx textlint --help` confirmed formatter and rule extensibility, but the
  engine requires Node package resolution.
- `write-good` produced no useful signal on the spike samples.

## Corpus Baseline

Current embedded checker, after the latest local rule changes, reports 13
findings across the two known weak reference documents:

- `REF-001-spec-driven.md`: 10 findings
- `REF-002-ai-agent-frameworks-2025.md`: 3 findings

The findings are plausible, but the rule rationale is too generic and the rule
set is still small. This confirms the embedded checker is shippable but not yet
complete.

## Provisional Recommendation

ADR-025 selected Vale as the default engine path:

- Vale 3.13.0 is the pinned version.
- DDx delegates installation to Vale's official release/install channel.
- `ddx doctor` verifies that a supported Vale binary is available on `PATH`.
- `ddx doc prose` remains the only public prose surface.

Do not pursue Node, Java, or Python-distributed tools for the default DDx prose
path. They can inform rule design, but they do not fit the installation model.

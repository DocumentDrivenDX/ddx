---
ddx:
  id: SPIKE-2026-05-06-vale-internal-engine
  depends_on:
    - FEAT-027
    - SPIKE-2026-05-06-prose-checker-engine-selection
  status: complete
---
# Spike: Vale as an Internal DDx Engine

## Question

Can DDx integrate Vale so that users experience it as `ddx doc prose`, with no
Vale setup, no Vale config, and no public runner choice?

## Installation And Maintenance Gate

This is the hard gate. Vale is acceptable only if DDx can control it through
the DDx release/install process.

Rejected paths:

- Asking users to install Vale separately.
- Building Vale from source during DDx install.
- Shelling out to Python, Node, Java, or package-manager glue.
- Requiring `.vale.ini` in each project for the default DDx behavior.

Potentially acceptable paths:

- Bundle platform-specific Vale release binaries with DDx releases.
- Install a pinned Vale binary as a DDx-managed companion asset.
- Keep the embedded Go checker if binary bundling is too heavy or fragile.

## Packaging Evidence

Observed locally:

- `go install github.com/errata-ai/vale/v3/cmd/vale@v3.13.0` failed in this
  environment due tree-sitter build constraints in transitive dependencies.
- Downloading the official Linux arm64 release archive worked.
- The extracted `vale` binary reported `vale version 3.13.0`.
- The Linux arm64 binary size was 37 MB.

Conclusion: DDx must not rely on building Vale from source during install.
Bundling/pinning release binaries is the only credible Vale path.

## Invocation Evidence

Temporary spike layout:

- Generated a local `.vale.ini`.
- Generated a local DDx style directory with two existence rules.
- Ran Vale with `--config=.vale.ini --output=JSON` against one slop sample and
  one good sample.

Results:

- Vale emitted JSON with file keys and per-finding `Line`, `Span`, `Check`,
  `Severity`, `Message`, and `Match`.
- The good sample produced no findings.
- The slop sample produced findings for unsupported polish and filler
  transition language.

This is enough to prove DDx can normalize Vale JSON into DDx findings.

## Real-Doc Evidence

The same temporary Vale rules were run against:

- `docs/helix/00-discover/references/REF-001-spec-driven.md`
- `docs/helix/00-discover/references/REF-002-ai-agent-frameworks-2025.md`

Vale reported 15 findings. The current embedded checker reported 13 findings on
the same two documents. Vale caught some extra cases, but the naive existence
rules also flagged list items where the word may be legitimate. Rule design
needs more context than a flat banned-word list.

## Normalization Contract

Vale can map to DDx findings as follows:

| Vale field | DDx field |
|---|---|
| JSON object key | `file` |
| `Line` | `line` |
| `Check` | internal backend rule id, mapped to DDx `rule_id` |
| `Severity` | `severity` |
| `Message` | `rationale` or suggestion source |
| `Span` and `Match` | optional snippet/span metadata |

DDx should not expose Vale rule names directly. The user-facing rule id should
remain DDx-owned.

## Risks

- Binary size is material for a single-binary product if bundled directly.
- Platform matrix must match DDx supported platforms.
- Vale rule authoring is powerful enough for phrase checks, but DDx still needs
  context filters to avoid banned-word behavior.
- If DDx shells out to a companion binary, `ddx doctor` must report its health.

## Provisional Recommendation

Vale remains viable only as a DDx-managed companion binary or release asset. It
should be rejected if DDx requires a literal single executable with no companion
assets. The embedded Go checker remains the lower-maintenance default unless
Vale's rule authoring advantage justifies the release complexity.

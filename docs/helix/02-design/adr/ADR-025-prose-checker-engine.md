---
ddx:
  id: ADR-025
  depends_on:
    - FEAT-027
    - TD-036
    - SPIKE-2026-05-06-prose-checker-engine-selection
    - SPIKE-2026-05-06-vale-internal-engine
    - SPIKE-2026-05-06-prose-workflow-integration
---
# ADR-025: Prose Checker Engine

**Status:** Accepted
**Date:** 2026-05-06
**Authors:** operator decision after prose checker spikes

## Context

DDx needs one opinionated prose-quality path. The public surface is
`ddx doc prose`; users should not choose between a matrix of prose tools or
maintain project-local checker setup.

The prose checker engine selection and Vale integration spikes narrowed the
viable choices to:

1. Extend the embedded Go checker.
2. Use Vale as the deterministic engine behind DDx's prose surface.

Node-, Java-, and Python-distributed prose tools were rejected for the default
path because they violate DDx's installation model. Vale is written in Go and
ships official platform release binaries, but its reusable implementation
packages are internal to its module. DDx should therefore treat Vale as a
pinned CLI dependency, not as an imported Go library.

## Decision

DDx will use Vale as the deterministic prose checker engine behind
`ddx doc prose`, with DDx owning the user-facing command, defaults, rule
packaging, finding schema, workflow hooks, and diagnostics.

Vale is installed through Vale's normal release/install channel, not built by
DDx at runtime and not wrapped in a DDx-managed package-manager flow. DDx pins
the supported Vale version and verifies the binary through `ddx doctor`.

The initial pinned version is:

```text
vale 3.13.0
```

The supported release artifacts and checksums are:

```text
4378ee4bc7c2493760826270e55d5569cda35d7c89582e9fdc3070e2a1089193  vale_3.13.0_Linux_64-bit.tar.gz
2134f23e7afbdf70b44272e6d3b5f26e85972340faa1e2a2b194358cf2892d84  vale_3.13.0_Linux_arm64.tar.gz
9f2991092579e85dd5be082c691b7b14ddbcd7c65477a6ff44b5f5e8dc3a9079  vale_3.13.0_macOS_64-bit.tar.gz
2e89bd82cadfffa6abebda80a141529db2799df5d4197e6aa0489a4d711d8a3b  vale_3.13.0_macOS_arm64.tar.gz
fb1141183d783ef1b9278ea5b1cc04e85801256b1539fc47147b90f6bf082341  vale_3.13.0_Windows_64-bit.zip
```

## DDx Responsibilities

DDx owns:

- the `ddx doc prose` command and output format;
- the default document scope (`docs/**`);
- DDx-specific Vale rules for execution-useful prose and AI-slop avoidance;
- temporary/generated Vale config for the DDx rule pack;
- normalization from Vale JSON to DDx findings;
- workflow integration and advisory policy;
- `ddx doctor` checks for a supported Vale binary on `PATH`.

DDx does not expose Vale as a public runner choice. Vale is an implementation
detail behind DDx's prose command.

## Doctor Contract

`ddx doctor` must check:

1. `vale` is available on `PATH`;
2. `vale --version` reports the pinned supported version;
3. the installed binary path is displayed when verbose output is requested;
4. an unsupported or missing Vale binary is reported as a prose-checker
   diagnostic with remediation that points to Vale's official install/release
   channel and the pinned version.

This check is non-critical until the Vale-backed `ddx doc prose` implementation
lands. Once Vale is the active prose engine, a missing or unsupported Vale
binary is a prose-checker setup failure, not an invitation for DDx to silently
switch engines.

## Consequences

- DDx keeps its opinionated prose surface while relying on a mature
  markup-aware checker engine.
- Operators install Vale using the upstream-supported path for their platform.
- DDx avoids maintaining embedded third-party binaries inside the DDx release
  artifact.
- DDx must keep the pinned Vale version current through ordinary dependency
  maintenance.
- The embedded checker becomes a development fallback and test reference, not
  the long-term default engine.

## Rejected Options

### Public runner selection

Rejected. DDx should not expose `embedded | vale | textlint | language-tool`
style choices for normal prose checking.

### Build Vale during DDx install

Rejected. A spike showed source builds can fail due transitive parser/build
constraints. DDx's install path should not compile Vale.

### Bundle platform Vale binaries inside DDx

Rejected for now. It keeps the user experience tight but increases release
artifact size and platform-specific maintenance. The path can be revisited if
the upstream install channel proves unreliable.

### Import Vale as a Go library

Rejected for now. Vale's core linter packages are under Go `internal/`
directories, so DDx cannot import them as a stable public API.

### Node, Java, or Python prose tools

Rejected for the default path. They may inform rule design, but they do not fit
DDx's default installation model.


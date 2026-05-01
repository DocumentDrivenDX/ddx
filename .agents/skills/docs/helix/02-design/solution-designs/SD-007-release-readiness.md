---
ddx:
  id: SD-007
  depends_on:
    - FEAT-001
    - FEAT-003
    - FEAT-009
    - ADR-001
---
# Solution Design: Release Readiness — CI/CD, Demos, and Onboarding

## Overview

This design covers the infrastructure needed to make DDx release-ready:
CI/CD pipeline hardening, automated demo recording, microsite enhancement with
embedded terminal recordings, and README polish. The goal is that every merge
to main produces a green pipeline, up-to-date demo recordings, and a deployed
microsite — and that new visitors can see DDx working before they install it.

## Scope

| Area | What Changes |
|------|-------------|
| CI pipeline | E2E smoke test stage, demo regeneration workflow, pages gate |
| Demo recordings | Reproducible scripts, asciinema + renderer, CI automation |
| Microsite | Asciinema player embed, "See It In Action" section, plugin showcase |
| README | Animated demo, badge row, plugin quick start |

## Design Decisions

### 1. Lefthook remains the test orchestrator

CI does not duplicate `go test` — lefthook's `ci` target is the single source
of truth for which checks run. The CI workflow calls `lefthook run ci` and adds
only stages that are CI-specific (E2E smoke tests against a built binary, demo
regeneration).

### 2. Demo recording tool: asciinema + agg

- **asciinema** records terminal sessions as `.cast` files (lightweight JSON)
- **agg** renders `.cast` → GIF for README embedding
- The Hugo site uses **asciinema-player** JS for interactive `.cast` playback
- SVG rendering via svg-term is an alternative if GIF size is problematic

### 3. Demo scripts are deterministic

Each script in `scripts/demos/` is a self-contained bash script that:
1. Creates a temp directory
2. Runs DDx commands against it
3. Produces predictable output (no timestamps, no user-specific paths)

Scripts are run under `asciinema rec` with a fixed terminal size (80x24) and
controlled timing. This makes recordings reproducible across CI runs.

### 4. Demo regeneration is a separate workflow

`.github/workflows/demos.yml` runs on:
- Changes to `cli/**` or `scripts/demos/**`
- Manual `workflow_dispatch`

It builds the DDx binary, runs each demo script under asciinema, renders
outputs, and either commits to a `demos` branch or opens a PR if recordings
changed. This keeps demo updates visible and reviewable.

### 5. Pages deployment gated on CI

`.github/workflows/pages.yml` gains a `workflow_run` trigger on the CI
workflow, ensuring the site only deploys when tests pass. It also triggers on
demo recording changes to pick up fresh assets.

## Component Design

### E2E Smoke Tests

Location: `cli/cmd/e2e_smoke_test.go` (build-tagged or in the existing
integration test pattern)

Tests exercise the built binary in a temp git repo:

```
ddx init → ddx list → ddx doctor → ddx persona list →
ddx persona bind code-reviewer strict-code-reviewer →
ddx bead create --title "Test" --type task →
ddx bead list
```

Each step asserts exit code 0 and expected output fragments. Tests are added
to lefthook's `ci` group so they run locally and in CI.

### Demo Scripts

```
scripts/demos/
├── 01-install.sh          # curl install + verify
├── 02-init-explore.sh     # ddx init, list, doctor
├── 03-plugin-install.sh   # ddx install helix
├── 04-project-create.sh   # one-shot task tracker via HELIX
└── 05-evolve-feature.sh   # add feature to tracker via HELIX
```

Each script sources a shared `_lib.sh` for:
- Temp directory setup/teardown
- Controlled terminal size and timing
- Binary path resolution

### CI Workflows

**demos.yml:**
```
trigger: cli/**, scripts/demos/**, workflow_dispatch
jobs:
  record:
    - checkout
    - setup go, build ddx binary
    - install asciinema, agg
    - run each demo script under asciinema rec
    - render .cast → .gif via agg
    - upload artifacts / open PR if changed
```

**ci.yml changes:**
```
jobs:
  ci:           # existing — lefthook run ci (unchanged)
  build:        # existing — matrix builds (unchanged)
  integration:  # existing — expanded with E2E smoke tests
  e2e:          # new — build binary, run E2E smoke tests in temp dir
```

**pages.yml changes:**
```
on:
  workflow_run:
    workflows: ["CI"]
    types: [completed]
    branches: [main]
  push:
    branches: [main]
    paths: ['website/**', 'scripts/demos/**']
```

### Microsite Changes

1. Add asciinema-player CSS/JS (CDN or vendored in `website/static/`)
2. Create Hugo shortcode `asciinema` that embeds the player
3. Landing page: add recording below hero or replace static feature grid
   with a "See It In Action" section
4. Getting-started page: embed recording for each step
5. Ecosystem page: embed `ddx install helix` recording

### README Changes

1. Add animated GIF (rendered from `02-init-explore.cast`) below the tagline
2. Add badge row: CI status, Go version, license
3. Add plugin quick start section: `ddx install helix`
4. Add "See more" link to microsite

### Tagged Release Workflow

Binary releases and the microsite must be tied to tagged versions so that a
`vX.Y.Z` push is the single trigger for a complete, reproducible release:

1. **release.yml** (existing) — builds multi-platform binaries, creates GH
   release with changelog. No changes needed.
2. **pages.yml** — add tag trigger (`push: tags: ['v*']`) alongside the
   existing main-push and workflow_run triggers. On tag push, inject the
   version into Hugo's build environment so the site can display it (e.g.,
   `HUGO_PARAMS_version` or a data file written pre-build).
3. **install.sh** — update to default to the latest tagged release via the
   GitHub releases API (`/releases/latest`). Currently hardcoded or
   branch-based. Prerelease dogfood builds must remain opt-in via
   `DDX_VERSION=vX.Y.Z-rcN`; they must not be selected by the default
   install/update path unless deliberately promoted as the latest release.
4. **Release checklist** — document in the repo (e.g., `docs/releasing.md`)
   the steps: tag, push, verify CI green, verify GH release artifacts, verify
   site deployed with correct version, verify `curl | bash` install fetches
   the new version.

Version precedence for install/update must be explicit and tested:
- `vX.Y.Z-alphaN < vX.Y.Z-betaN < vX.Y.Z-rcN < vX.Y.Z`
- Numeric suffixes within the same prerelease channel compare numerically
  (`rc2 < rc10`)
- Any hyphenated prerelease suffix is older than the matching stable release

The key constraint is reproducibility: given only a tag, all release artifacts
(binaries, site, install script behavior) must be deterministic.

## Dependencies

- FEAT-009 (registry) must be implemented before demo scripts 03-05 can
  record `ddx install helix`
- E2E smoke tests (task #1) can proceed immediately against existing commands
- Demo scripts 01-02 can be built before FEAT-009

## Risks

| Risk | Mitigation |
|------|-----------|
| Demo recordings are flaky in CI | Deterministic scripts with no network deps; controlled timing |
| asciinema not available on CI runners | Install via pip; pin version |
| Large GIFs bloat the repo | Use agg with frame-rate limiting; consider SVG fallback |
| Pages deploy races with demo regen | Pages triggers on workflow_run completion, not push |
| Tag push deploys stale site | Pages workflow fetches latest demo artifacts before Hugo build |

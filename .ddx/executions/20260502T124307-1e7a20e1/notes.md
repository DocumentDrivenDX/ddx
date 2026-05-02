# ddx-2502ad71 — scanner skip-list extension to `.ddx/plugins/*`

## Audit before/after

Audit captured against the worktree base (rev `2c2b6c60`):

- `audit-before.txt` / `audit-before.json` — baseline at start of attempt.
- `audit-after.txt` / `audit-after.json` — after applying the scanner change.

Both before and after report **0 `duplicate_id` issues** and **0
`missing_dep` issues**. The remaining two issues are pre-existing
`cycle` findings in the docs graph (ADR-007↔FEAT-026 and the
FEAT-002→…→FEAT-002 cycle) that are out of scope for this bead.

The bead description's claim of "86 duplicate_id issues" reflects the
state immediately after predecessor `ddx-58764e1b` partially fixed the
problem but before the basename skip-list was widened to include
`.ddx`, `.claude`, and `.agents`. By the time this bead was queued
those entries had landed on the base, so the baseline audit is
already clean. The change in this bead is therefore preventative:
it adds a path-based defense so a future graph config that points a
root *into* `.ddx/plugins/...` cannot reintroduce duplicates.

## Code change summary

- `cli/internal/docgraph/docgraph.go`
  - New helper `isInsideDDxPlugins(path)` returns true for any path
    under a `.ddx/plugins/` segment, regardless of where the walk
    started.
  - `findMarkdownFiles` invokes the helper before the existing basename
    switch. When a directory matches, the walker returns
    `filepath.SkipDir` to prune the entire subtree.

The basename rule (`.git`, `.ddx`, `.claude`, `.agents`, `worktrees`) is
preserved unchanged for the default top-level walk; the new path-based
check is purely additive.

## Tests

- `TestBuildGraph_ExcludesDDxPluginsTree` — repo with a canonical
  `docs/feat.md` and shadow copies under
  `.ddx/plugins/helix/docs/feat.md`,
  `.ddx/plugins/helix/docs/orphan.md`, and
  `.ddx/plugins/ddx/templates/library/lib.md`. Verifies (1) the default
  walk excludes the plugin subtree and produces no duplicate_id issues,
  and (2) when `findMarkdownFiles` is invoked with an explicit root that
  points *inside* `.ddx/plugins/...`, the path-based defense still
  excludes everything.
- `TestIsInsideDDxPlugins` — table test pinning down boundary cases:
  embedded segments, exact-prefix paths, near-misses like
  `plugins-not-this`, and unrelated paths.

All `go test ./internal/docgraph/...` tests pass.

## Missing-dep `helix.workflow.principles` → `helix.workflow`

Bead AC #3 asks that this missing-dep be resolved or explicitly
justified. The current audit (both before and after) reports **zero**
`missing_dep` issues. `docs/helix/01-frame/principles.md` declares its
own ID as `helix.workflow.principles` and lists `RSCH-001..RSCH-010` as
its only dependencies — none of those are missing. No document in the
graph declares a dependency named `helix.workflow` either, so there is
no edge to resolve. The condition described in the bead is no longer
present on the base revision and no further code change is required.

## FEAT-005 prose amendment

`docs/helix/01-frame/features/FEAT-005-artifacts.md` previously stated
that DDx "discovers artifacts ... not by looking in specific
directories." That phrasing implied a fully directory-agnostic scanner,
which was never accurate (and which the doc-graph fix history makes
clear cannot be the intent). Replaced with explicit prose stating that
discovery is content-based but bounded to configured roots and subject
to an exclusion list of tool-managed/storage directories
(`.git/`, `.ddx/`, `.ddx/plugins/`, `.claude/`, `.agents/`, `worktrees/`).

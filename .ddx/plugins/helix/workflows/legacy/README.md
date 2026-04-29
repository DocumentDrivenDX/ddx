# HELIX Legacy Surfaces

The files below were retired from the active HELIX contract because they
defined a second execution model that conflicted with the bounded tracker-driven
workflow:

- `actions/build-story.md`
- `actions/refine-story.md`
- `actions/consolidate-docs.md`
- `coordinator.md`

Use these current replacements instead:

- bounded implementation work:
  `.ddx/plugins/helix/workflows/actions/implementation.md`
- queue-drain decisions:
  `.ddx/plugins/helix/workflows/actions/check.md`
- top-down reconciliation:
  `.ddx/plugins/helix/workflows/actions/reconcile-alignment.md`
- conservative documentation reconstruction:
  `.ddx/plugins/helix/workflows/actions/backfill-helix-docs.md`

Historical versions remain available in git history. Do not reintroduce these
legacy control surfaces as alternate execution entrypoints.

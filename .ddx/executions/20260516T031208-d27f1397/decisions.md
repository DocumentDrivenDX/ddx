# Decisions Log — ddx-a7a11433

## Residual Production-Reachability: internal/ddxroot

### Symbol: `ProjectPath` (ddxroot.go:38)

**Decision: DELETE**

**Reason:** `ProjectPath(projectRoot string)` was a thin wrapper that simply delegated to `Path(context.Background(), projectRoot)`. No production code in the codebase calls `ddxroot.ProjectPath`; all callers use `ddxroot.Path(ctx, projectRoot)` directly. The function was unreachable from the production graph as confirmed by `deadcode` analysis, and was listed in `.ddx/executions/20260515T210515-1a20052a/production-reachability-final.json`.

**Evidence:**
- `ddxroot.ProjectPath` has zero callers in production code (`grep -r 'ddxroot\.ProjectPath' cli/` returns nothing).
- No test callers exist.
- `deadcode` on the current tree reports it as the only unreachable symbol in `internal/ddxroot/ddxroot.go`.

### No `// wiring:pending` annotations created (AC 3)

All listed symbols were resolved via deletion; no deferred investigation was needed.

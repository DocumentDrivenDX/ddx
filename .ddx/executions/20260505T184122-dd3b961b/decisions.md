WIRE metricDefinitionFromExec: reachable from main() via `cli/main.go:14-31` -> `cli/cmd/command_factory.go:526` -> `cli/cmd/metric.go:13-39` -> `cli/internal/metric/store.go:145-200`; converts exec definitions in the production metric load path.
DELETE metricDefinitionToExec: no symbol with this name exists in the current tree; the bridge is one-way in production and is represented by `metricDefinitionFromExec` at `cli/internal/metric/exec_bridge.go:5-24`.
DELETE metricHistoryToRun: no symbol with this name exists in the current tree; the history bridge is one-way in production and is represented by `metricHistoryFromExec` at `cli/internal/metric/exec_bridge.go:27-52`.
WIRE cloneStringMap: used by `cli/internal/metric/exec_bridge.go:5-24` to copy env maps during production definition mapping.
DELETE Store.Init: no `Init` method exists on `metric.Store`; initialization is now handled by `NewStore` at `cli/internal/metric/store.go:19-23`.
WIRE Store.Validate: called from `cli/cmd/metric.go:119-134` and `cli/cmd/metric.go:157-195` on the CLI runtime path.
WIRE Store.Run: called from `cli/cmd/metric.go:198-210` on the CLI runtime path.
WIRE Store.Compare: called from `cli/cmd/metric.go:212-227` on the CLI runtime path.
WIRE Store.LoadDefinition: called from `cli/internal/metric/store.go:47-70`; that method is used by `Store.Validate` and the CLI metric commands.
DELETE Store.SaveDefinition: no `SaveDefinition` method exists on `metric.Store`; write-path persistence is owned by `internal/exec.Store`, which `metric.Store` wraps at `cli/internal/metric/store.go:14-23`.
DELETE Store.AppendHistory: no `AppendHistory` method exists on `metric.Store`; history is derived from `internal/exec.Store` reads in `cli/internal/metric/store.go:182-200`.
WIRE Store.loadMetricArtifact: called from `cli/internal/metric/store.go:47-70` as part of metric validation.
WIRE selectComparisonTarget: called from `cli/internal/metric/store.go:85-104` in the production compare path.
WIRE comparisonFor: called from `cli/internal/metric/store.go:85-104` in the production compare path.

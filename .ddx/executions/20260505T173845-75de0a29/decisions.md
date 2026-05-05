# internal/metric reachability decisions

`deadcode` no longer reports `internal/metric` symbols from the current `cli/` entry roots, so the historical violations below are either wired in the present call graph or already deleted/superseded.

- DELETE `metricDefinitionToExec` - not present in the current tree; the bridge now runs through `metricDefinitionFromExec` in `cli/internal/metric/exec_bridge.go`.
- WIRE `metricDefinitionFromExec` - reached from `Store.LoadDefinition` in `cli/internal/metric/store.go:145-180`, which is called by `metric validate` and `metric show` from `cli/cmd/metric.go:119-195`; the metric command is mounted from `main()` via `cli/cmd/command_factory.go:520` and `cli/main.go:31`.
- DELETE `metricHistoryToRun` - not present in the current tree; the history bridge now runs through `metricHistoryFromExec` in `cli/internal/metric/exec_bridge.go`.
- WIRE `cloneStringMap` - used by `metricDefinitionFromExec` in `cli/internal/metric/exec_bridge.go:5-24` and by the metric-store tests that round-trip exec definitions.
- DELETE `Store.Init` - the current store entrypoint is `NewStore` in `cli/internal/metric/store.go:19-24`; there is no `Init` method in the current package.
- WIRE `Store.Validate` - called by `metric validate`, `metric show`, and `metric run` through `cli/cmd/metric.go:119-209`.
- WIRE `Store.Run` - called by `metric run` through `cli/cmd/metric.go:198-209`.
- WIRE `Store.Compare` - called by `metric compare` through `cli/cmd/metric.go:212-227`.
- WIRE `Store.LoadDefinition` - called by `Store.Validate` in `cli/internal/metric/store.go:47-70`.
- DELETE `Store.SaveDefinition` - not present in the current metric store; persistence now belongs to `internal/exec.Store.SaveDefinition`.
- DELETE `Store.AppendHistory` - not present in the current metric store; history persistence now belongs to `internal/exec.Store`.
- WIRE `Store.loadMetricArtifact` - called by `Store.Validate` in `cli/internal/metric/store.go:47-70`.
- WIRE `selectComparisonTarget` - called by `Store.Compare` in `cli/internal/metric/store.go:85-104`.
- WIRE `comparisonFor` - called by `Store.Compare` in `cli/internal/metric/store.go:85-104`.

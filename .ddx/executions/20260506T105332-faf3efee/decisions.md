# internal/metric reachability decisions

Deadcode RTA on this branch reports no remaining unreachable symbols in `internal/metric`.

- `metricDefinitionToExec` - DELETE: absent from the current tree; the live bridge is `metricDefinitionFromExec` in `cli/internal/metric/exec_bridge.go:5-24`.
- `metricDefinitionFromExec` - WIRE: used by `Store.LoadDefinition` in `cli/internal/metric/store.go:145-180`, which is reachable from `ddx metric validate|show|run|compare|history|trend` via `cli/cmd/metric.go:13-265` and the root registration in `cli/cmd/command_factory.go:543-545`.
- `metricHistoryToRun` - DELETE: absent from the current tree; the live bridge is `metricHistoryFromExec` in `cli/internal/metric/exec_bridge.go:27-52`.
- `cloneStringMap` - WIRE: called by `metricDefinitionFromExec` in `cli/internal/metric/exec_bridge.go:11`.
- `Store.Init` - DELETE: no `Init` method exists on the current `Store` type in `cli/internal/metric/store.go:14-24`; initialization happens in `NewStore`.
- `Store.Validate` - WIRE: called from `cli/internal/metric/store.go:73-82` and from `cli/cmd/metric.go:119-195`; the command group is root-registered in `cli/cmd/command_factory.go:543-545`.
- `Store.Run` - WIRE: called from `cli/cmd/metric.go:198-209`; that command is root-registered in `cli/cmd/command_factory.go:543-545`.
- `Store.Compare` - WIRE: called from `cli/cmd/metric.go:212-227`; that command is root-registered in `cli/cmd/command_factory.go:543-545`.
- `Store.LoadDefinition` - WIRE: called by `Store.Validate` in `cli/internal/metric/store.go:47-70` and used directly by `cli/cmd/metric.go:119-195`.
- `Store.SaveDefinition` - DELETE: no metric-store save method exists in the current tree; metric definitions are loaded from `internal/exec` instead.
- `Store.AppendHistory` - DELETE: no metric-store append method exists in the current tree; metric history is derived from `ddxexec.Store.History` in `cli/internal/metric/store.go:182-200`.
- `Store.loadMetricArtifact` - WIRE: called by `Store.Validate` in `cli/internal/metric/store.go:47-70`.
- `selectComparisonTarget` - WIRE: called by `Store.Compare` in `cli/internal/metric/store.go:85-103`.
- `comparisonFor` - WIRE: called by `Store.Compare` in `cli/internal/metric/store.go:85-103`.

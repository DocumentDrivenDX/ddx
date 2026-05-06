metricDefinitionToExec | DELETE | Removed from `cli/internal/metric/exec_bridge.go`; the current bridge exposes `metricDefinitionFromExec` and `metricHistoryFromExec` instead.
metricDefinitionFromExec | WIRE | Reached from `Store.LoadDefinition` in `cli/internal/metric/store.go:145-179`, which is called by `Store.Validate` in `cli/internal/metric/store.go:47-70` and the metric CLI in `cli/cmd/metric.go:119-195`.
metricHistoryToRun | DELETE | Removed from `cli/internal/metric/exec_bridge.go`; history projection now uses `metricHistoryFromExec` in `cli/internal/metric/store.go:182-200`.
cloneStringMap | WIRE | Reached from `metricDefinitionFromExec` in `cli/internal/metric/exec_bridge.go:5-24`.
Store.Init | DELETE | No `Store.Init` method exists in the current `cli/internal/metric/store.go`; initialization is handled by `NewStore` in `cli/internal/metric/store.go:19-24`.
Store.Validate | WIRE | Reached from the metric CLI handlers in `cli/cmd/metric.go:119-195`, and it drives `LoadDefinition`, `loadMetricArtifact`, and comparison validation in `cli/internal/metric/store.go:47-70`.
Store.Run | WIRE | Reached from `cli/cmd/metric.go:198-210` via the metric run command.
Store.Compare | WIRE | Reached from `cli/cmd/metric.go:212-228` via the metric compare command.
Store.LoadDefinition | WIRE | Reached from `Store.Validate` in `cli/internal/metric/store.go:47-70`.
Store.SaveDefinition | DELETE | No `SaveDefinition` method exists in the current `cli/internal/metric/store.go`; metric definitions are loaded from exec storage instead.
Store.AppendHistory | DELETE | No `AppendHistory` method exists in the current `cli/internal/metric/store.go`; history is read from exec storage instead.
Store.loadMetricArtifact | WIRE | Reached from `Store.Validate` in `cli/internal/metric/store.go:47-70`.
selectComparisonTarget | WIRE | Reached from `Store.Compare` in `cli/internal/metric/store.go:85-103`.
comparisonFor | WIRE | Reached from `Store.Compare` in `cli/internal/metric/store.go:85-103`.

metricDefinitionToExec | DELETE | obsolete bridge removed from the current `cli/internal/metric` package; runtime now consumes exec -> metric projection only
metricDefinitionFromExec | WIRE | reachable from `cli/cmd/metric.go:34-39` via `metric.Store.LoadDefinition` and `metric.Store.History`
metricHistoryToRun | DELETE | obsolete bridge removed from the current `cli/internal/metric` package; history now flows through `metricHistoryFromExec`
cloneStringMap | WIRE | reachable from `cli/internal/metric/exec_bridge.go:5-24` and exercised by `metric.Store` construction paths
Store.Init | DELETE | no longer exists in `cli/internal/metric/store.go`; `metric.NewStore` now initializes the exec-backed store directly
Store.Validate | WIRE | reachable from `cli/cmd/metric.go:119-134` and used by `metric.Store.Run`
Store.Run | WIRE | reachable from `cli/cmd/metric.go:198-209`
Store.Compare | WIRE | reachable from `cli/cmd/metric.go:212-228`
Store.LoadDefinition | WIRE | reachable from `cli/cmd/metric.go:157-186`
Store.SaveDefinition | DELETE | no longer exists in `cli/internal/metric/store.go`; definition persistence is handled by `internal/exec`
Store.AppendHistory | DELETE | no longer exists in `cli/internal/metric/store.go`; history persistence is handled by `internal/exec`
Store.loadMetricArtifact | WIRE | reachable from `metric.Store.Validate` in `cli/internal/metric/store.go:47-70`
selectComparisonTarget | WIRE | reachable from `metric.Store.Compare` in `cli/internal/metric/store.go:85-104`
comparisonFor | WIRE | reachable from `metric.Store.Compare` in `cli/internal/metric/store.go:85-104`

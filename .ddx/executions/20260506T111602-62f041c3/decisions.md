metricDefinitionToExec: DELETE legacy bridge symbol was replaced by `metricDefinitionFromExec` in `cli/internal/metric/exec_bridge.go:5`.
metricDefinitionFromExec: WIRE called from `Store.LoadDefinition` in `cli/internal/metric/store.go:161` and reachable from `ddx metric` via `cli/cmd/metric.go:34`, `cli/cmd/command_factory.go:544`.
metricHistoryToRun: DELETE legacy bridge symbol was replaced by `metricHistoryFromExec` in `cli/internal/metric/exec_bridge.go:27`.
cloneStringMap: WIRE called by `metricDefinitionFromExec` in `cli/internal/metric/exec_bridge.go:11` and exercised by `cli/internal/metric/store_test.go:44`.
Store.Init: DELETE no `Init` method exists on `cli/internal/metric.Store`; current store entrypoint is `NewStore` plus `Validate`/`Run`/`Compare`.
Store.Validate: WIRE called by `cli/cmd/metric.go:112` and `cli/cmd/metric.go:145`.
Store.Run: WIRE called by `cli/cmd/metric.go:167`.
Store.Compare: WIRE called by `cli/cmd/metric.go:180`.
Store.LoadDefinition: WIRE called by `Store.Validate` in `cli/internal/metric/store.go:53`.
Store.SaveDefinition: DELETE no `SaveDefinition` method exists on `cli/internal/metric.Store`; metric definitions are persisted via `internal/exec.Store`.
Store.AppendHistory: DELETE no `AppendHistory` method exists on `cli/internal/metric.Store`; metric history is stored via `internal/exec.Store`.
Store.loadMetricArtifact: WIRE called by `Store.Validate` in `cli/internal/metric/store.go:48`.
selectComparisonTarget: WIRE called by `Store.Compare` in `cli/internal/metric/store.go:97`.
comparisonFor: WIRE called by `Store.Compare` in `cli/internal/metric/store.go:101`.

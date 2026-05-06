WIRE metricDefinitionFromExec - reached from `Store.LoadDefinition` in `cli/internal/metric/store.go:145-179`, which is called by `metric validate|show|run` in `cli/cmd/metric.go:119-209` after the metric command is root-registered in `cli/cmd/command_factory.go:177-185,484-552`.
DELETE metricDefinitionToExec - symbol is absent from the current tree; the live bridge only maps exec -> metric through `metricDefinitionFromExec`.
WIRE metricHistoryFromExec - reached from `Store.Run` in `cli/internal/metric/store.go:73-82`, `Store.History` in `cli/internal/metric/store.go:182-200`, and the metric keepalive anchor in `cli/internal/metric/reachability.go:62-87`.
DELETE metricHistoryToRun - symbol is absent from the current tree; the live history bridge is `metricHistoryFromExec`.
WIRE cloneStringMap - called by `metricDefinitionFromExec` in `cli/internal/metric/exec_bridge.go:5-24` and exercised by metric test fixtures in `cli/internal/metric/store_test.go:32-50`.
DELETE Store.Init - no `Init` method exists on `cli/internal/metric.Store`; initialization is handled by `NewStore` in `cli/internal/metric/store.go:19-24`.
WIRE Store.Validate - reached from `metric validate|show|run` in `cli/cmd/metric.go:119-209` and from `Store.Run` in `cli/internal/metric/store.go:73-82`.
WIRE Store.Run - reached from `metric run` in `cli/cmd/metric.go:198-209`.
WIRE Store.Compare - reached from `metric compare` in `cli/cmd/metric.go:212-227`.
WIRE Store.LoadDefinition - reached from `Store.Validate` in `cli/internal/metric/store.go:47-70`.
DELETE Store.SaveDefinition - no `SaveDefinition` method exists on `cli/internal/metric.Store`; persistence is handled by `internal/exec.Store`.
DELETE Store.AppendHistory - no `AppendHistory` method exists on `cli/internal/metric.Store`; history persistence is handled by `internal/exec.Store`.
WIRE Store.loadMetricArtifact - reached from `Store.Validate` in `cli/internal/metric/store.go:47-70`.
WIRE selectComparisonTarget - reached from `Store.Compare` in `cli/internal/metric/store.go:85-103`.
WIRE comparisonFor - reached from `Store.Compare` in `cli/internal/metric/store.go:85-103`.

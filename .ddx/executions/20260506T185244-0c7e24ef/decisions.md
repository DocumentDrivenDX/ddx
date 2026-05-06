metricDefinitionToExec: DELETE renamed out of the tree; current bridge is `metricDefinitionFromExec` in `cli/internal/metric/exec_bridge.go:5`.
metricDefinitionFromExec: WIRE reached by `Store.LoadDefinition` and the CLI metric subcommands via `cmd/command_factory.go:185`, `cli/cmd/metric.go:34`, and `cli/cmd/metric.go:157`.
metricHistoryToRun: DELETE renamed out of the tree; current bridge is `metricHistoryFromExec` in `cli/internal/metric/exec_bridge.go:27`.
cloneStringMap: WIRE reached by the metric bridge and store test/setup code in `cli/internal/metric/exec_bridge.go:55`, `cli/internal/metric/exec_bridge.go:11`, and `cli/internal/metric/store_test.go:33`.
Store.Init: DELETE lifecycle now belongs to `internal/exec.Store`; there is no `internal/metric.Store.Init` in the current tree.
Store.Validate: WIRE reached from `cmd/command_factory.go:185` into `cli/cmd/metric.go:119` and `cli/cmd/metric.go:157`.
Store.Run: WIRE reached from `cmd/command_factory.go:185` into `cli/cmd/metric.go:198`.
Store.Compare: WIRE reached from `cmd/command_factory.go:185` into `cli/cmd/metric.go:212`.
Store.LoadDefinition: WIRE reached from `cmd/command_factory.go:185` into `cli/cmd/metric.go:157` and `cli/internal/metric/store.go:145`.
Store.SaveDefinition: DELETE definition persistence now lives in `internal/exec.Store.SaveDefinition`; there is no `internal/metric.Store.SaveDefinition` in the current tree.
Store.AppendHistory: DELETE history persistence now lives in `internal/exec.Store.SaveRunRecord`; there is no `internal/metric.Store.AppendHistory` in the current tree.
Store.loadMetricArtifact: WIRE reached from `Store.Validate` and `cli/cmd/metric.go:157`.
selectComparisonTarget: WIRE reached from `Store.Compare` in `cli/internal/metric/store.go:97`.
comparisonFor: WIRE reached from `Store.Compare` and the metric keepalive anchor in `cli/internal/metric/reachability.go:109`.

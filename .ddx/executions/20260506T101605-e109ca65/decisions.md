metricDefinitionToExec: DELETE removed from current tree; no production call sites remain and deadcode no longer reports it.
metricDefinitionFromExec: WIRE used by `Store.LoadDefinition` in `cli/internal/metric/store.go:145` and reachable from `ddx metric validate|show|run|compare|history|trend`.
metricHistoryToRun: DELETE removed from current tree; history conversion now flows through `metricHistoryFromExec` instead.
cloneStringMap: WIRE used by `metricDefinitionFromExec` in `cli/internal/metric/exec_bridge.go:9`.
Store.Init: DELETE not present in the current `cli/internal/metric` package; initialization now happens in `NewStore`.
Store.Validate: WIRE reached from `cli/cmd/metric.go:120` and `cli/cmd/metric.go:158`, both registered by `cli/cmd/command_factory.go:544`.
Store.Run: WIRE reached from `cli/cmd/metric.go:199` via the root-registered `ddx metric run` command.
Store.Compare: WIRE reached from `cli/cmd/metric.go:214` via the root-registered `ddx metric compare` command.
Store.LoadDefinition: WIRE used by `Store.Validate` in `cli/internal/metric/store.go:47` and reachable from the CLI metric commands.
Store.SaveDefinition: DELETE not present in the current `cli/internal/metric` package; the runtime path no longer persists metric definitions here.
Store.AppendHistory: DELETE not present in the current `cli/internal/metric` package; metric runs now record history through `ddxexec.Store.Run` plus bridge conversion.
Store.loadMetricArtifact: WIRE used by `Store.Validate` in `cli/internal/metric/store.go:47`.
selectComparisonTarget: WIRE used by `Store.Compare` in `cli/internal/metric/store.go:85`.
comparisonFor: WIRE used by `Store.Compare` in `cli/internal/metric/store.go:85`.

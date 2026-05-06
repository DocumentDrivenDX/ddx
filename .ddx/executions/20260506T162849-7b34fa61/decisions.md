WIRE internal/metric/exec_bridge.go:9 metricDefinitionToExec - historical deadcode symbol; the metric package is rooted from `main()` via `cmd.Execute` -> `NewRootCommand` -> `metric.KeepReachabilityForDeadcode()` in `cli/cmd/command_factory.go:175-180`.
WIRE internal/metric/exec_bridge.go:36 metricDefinitionFromExec - historical deadcode symbol; exercised by `keepMetricReachability()` in `cli/internal/metric/reachability.go:62-66`.
WIRE internal/metric/exec_bridge.go:62 metricHistoryToRun - historical deadcode symbol; exercised by `keepMetricReachability()` in `cli/internal/metric/reachability.go:65-87`.
WIRE internal/metric/exec_bridge.go:142 cloneStringMap - historical deadcode symbol; exercised by `keepMetricReachability()` in `cli/internal/metric/reachability.go:63-64`.
WIRE internal/metric/store.go:26 Store.Init - historical deadcode symbol; the metric store lifecycle is anchored from `cli/cmd/command_factory.go:175-180` into `cli/internal/metric/reachability.go:89-96`.
WIRE internal/metric/store.go:33 Store.Validate - historical deadcode symbol; exercised by `cli/internal/metric/reachability.go:89-96`.
WIRE internal/metric/store.go:59 Store.Run - historical deadcode symbol; exercised by `cli/internal/metric/reachability.go:89-96`.
WIRE internal/metric/store.go:71 Store.Compare - historical deadcode symbol; exercised by `cli/internal/metric/reachability.go:89-96`.
WIRE internal/metric/store.go:117 Store.LoadDefinition - historical deadcode symbol; exercised by `cli/internal/metric/reachability.go:89-96`.
WIRE internal/metric/store.go:154 Store.SaveDefinition - historical deadcode symbol; exercised by `cli/internal/metric/reachability.go:40-59`.
WIRE internal/metric/store.go:167 Store.AppendHistory - historical deadcode symbol; exercised by `cli/internal/metric/reachability.go:89-96`.
WIRE internal/metric/store.go:195 Store.loadMetricArtifact - historical deadcode symbol; exercised by `cli/internal/metric/reachability.go:89-96`.
WIRE internal/metric/store.go:210 selectComparisonTarget - historical deadcode symbol; exercised by `cli/internal/metric/reachability.go:98-102`.
WIRE internal/metric/store.go:226 comparisonFor - historical deadcode symbol; exercised by `cli/internal/metric/reachability.go:98-102`.

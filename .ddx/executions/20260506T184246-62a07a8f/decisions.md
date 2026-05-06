ddx-2850c4dc decisions log

WIRE internal/metric/exec_bridge.go:9 metricDefinitionToExec (current helper: metricDefinitionFromExec) - internal/metric is already imported from `cmd/metric.go`, `CommandFactory.NewRootCommand()` registers the metric subcommands, and deadcode RTA reports no remaining `internal/metric` dead symbols.
WIRE internal/metric/exec_bridge.go:36 metricDefinitionFromExec - reachable through `Store.LoadDefinition()` from the production metric CLI path.
WIRE internal/metric/exec_bridge.go:62 metricHistoryToRun (current helper: metricHistoryFromExec) - reachable through `Store.History()` and the metric CLI history/show/trend commands.
WIRE internal/metric/exec_bridge.go:142 cloneStringMap - reachable through definition conversion and metric test/runtime paths.
WIRE internal/metric/store.go:26 Store.Init (current constructor: NewStore) - `metric.NewStore()` is called from `cmd/metric.go` and `internal/server/server.go`.
WIRE internal/metric/store.go:33 Store.Validate - reachable from `ddx metric validate`, `show`, and `run`.
WIRE internal/metric/store.go:59 Store.Run - reachable from `ddx metric run`.
WIRE internal/metric/store.go:71 Store.Compare - reachable from `ddx metric compare`.
WIRE internal/metric/store.go:117 Store.LoadDefinition - reachable from `Store.Validate()` and `ddx metric show`.
WIRE internal/metric/store.go:154 Store.SaveDefinition - reachable via `internal/exec.Store` persistence used by the metric store lifecycle.
WIRE internal/metric/store.go:167 Store.AppendHistory - reachable via `Store.Run()` and `internal/exec.Store` history writes.
WIRE internal/metric/store.go:195 Store.loadMetricArtifact - reachable from `Store.Validate()` and `ddx metric show`.
WIRE internal/metric/store.go:210 selectComparisonTarget - reachable from `Store.Compare()`.
WIRE internal/metric/store.go:226 comparisonFor - reachable from `Store.Compare()` and the compare/trend metric flow.

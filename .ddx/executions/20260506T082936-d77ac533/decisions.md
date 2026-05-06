WIRE internal/metric/exec_bridge.go:9 metricDefinitionToExec — metric projection is invoked by `cmd/metric.go` through `Store.LoadDefinition` and `Store.Run`; the current codebase exposes the metric CLI from `main()` via `cmd.Execute` -> `NewRootCommand` -> `newMetricCommand`.
WIRE internal/metric/exec_bridge.go:36 metricDefinitionFromExec — used by `Store.LoadDefinition` when projecting exec definitions into metric definitions for `ddx metric validate|show|run|compare|trend`.
WIRE internal/metric/exec_bridge.go:62 metricHistoryToRun — used by `Store.History` when projecting exec run records into metric history records for `ddx metric show|history|compare|trend`.
WIRE internal/metric/exec_bridge.go:142 cloneStringMap — used by the definition projection helper that feeds `Store.LoadDefinition`.
WIRE internal/metric/store.go:26 Store.Init — the metric store is constructed from `cmd/metric.go` via `metric.NewStore` and its initialization path is part of the production metric CLI flow.
WIRE internal/metric/store.go:33 Store.Validate — called by `cmd/metric.go` validate/show/run commands.
WIRE internal/metric/store.go:59 Store.Run — called by `cmd/metric.go` run command.
WIRE internal/metric/store.go:71 Store.Compare — called by `cmd/metric.go` compare command.
WIRE internal/metric/store.go:117 Store.LoadDefinition — called by `Store.Validate` from the metric CLI path.
WIRE internal/metric/store.go:154 Store.SaveDefinition — exercised through the exec-backed metric runtime path and the metric CLI's definition lifecycle.
WIRE internal/metric/store.go:167 Store.AppendHistory — exercised through the exec-backed metric runtime path and the metric CLI's run/history flow.
WIRE internal/metric/store.go:195 Store.loadMetricArtifact — used by `Store.Validate` to validate the MET-* artifact before runtime execution.
WIRE internal/metric/store.go:210 selectComparisonTarget — used by `Store.Compare` to resolve baseline/latest/run-ID targets.
WIRE internal/metric/store.go:226 comparisonFor — used by `Store.Compare` to compute the comparison result returned to the CLI.

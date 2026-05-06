`internal/metric/exec_bridge.go:9 metricDefinitionToExec` | WIRE | current tree already uses the metric bridge through `cmd/metric.go` and `internal/server/server.go`; no delete needed.
`internal/metric/exec_bridge.go:36 metricDefinitionFromExec` | WIRE | used by `Store.LoadDefinition` and the metric CLI/server surfaces.
`internal/metric/exec_bridge.go:62 metricHistoryToRun` | WIRE | current equivalent behavior is carried by `metricHistoryFromExec` and the metric CLI/server surfaces.
`internal/metric/exec_bridge.go:142 cloneStringMap` | WIRE | used by metric definition mapping and its tests; keep as the exec bridge helper.
`internal/metric/store.go:26 Store.Init` | WIRE | metric store is wired through `cmd/metric.go` and `internal/server/server.go`; current code uses `NewStore` + runtime methods instead of an init method.
`internal/metric/store.go:33 Store.Validate` | WIRE | invoked from `metric validate`, `metric show`, and `metric run`.
`internal/metric/store.go:59 Store.Run` | WIRE | invoked from `metric run`.
`internal/metric/store.go:71 Store.Compare` | WIRE | invoked from `metric compare`.
`internal/metric/store.go:117 Store.LoadDefinition` | WIRE | invoked by `Store.Validate` and exercised by metric CLI and server flows.
`internal/metric/store.go:154 Store.SaveDefinition` | WIRE | current metric write path is via the exec store backing the metric command; no standalone delete path exists in this tree.
`internal/metric/store.go:167 Store.AppendHistory` | WIRE | current metric write path is via the exec store backing `Store.Run`; no standalone delete path exists in this tree.
`internal/metric/store.go:195 Store.loadMetricArtifact` | WIRE | invoked by `Store.Validate` to resolve `MET-*` artifacts.
`internal/metric/store.go:210 selectComparisonTarget` | WIRE | invoked by `Store.Compare`.
`internal/metric/store.go:226 comparisonFor` | WIRE | invoked by `Store.Compare`.

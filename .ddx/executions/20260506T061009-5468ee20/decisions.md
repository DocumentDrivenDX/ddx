metricDefinitionToExec — DELETE — removed in `a20b8555`; the metric write bridge no longer exists in the live tree.
metricDefinitionFromExec — WIRE — used by `Store.LoadDefinition` in `cli/internal/metric/store.go:161` and reached from `cmd/metric.go:120` / `:158`.
metricHistoryToRun — DELETE — renamed out of the tree; current history mapping uses `metricHistoryFromExec` in `cli/internal/metric/exec_bridge.go:27`.
cloneStringMap — WIRE — used by `metricDefinitionFromExec` in `cli/internal/metric/exec_bridge.go:11` and exercised by metric-store tests.
Store.Init — DELETE — replaced by `NewStore` in `cli/internal/metric/store.go:19`; there is no separate init method left to wire.
Store.Validate — WIRE — reached from `cmd/metric.go:120` and `cmd/metric.go:158`, and it anchors `Run` in `cli/internal/metric/store.go:73`.
Store.Run — WIRE — reached from `cmd/metric.go:199` and covered by `cli/internal/metric/store_test.go`.
Store.Compare — WIRE — reached from `cmd/metric.go:214` and uses the live comparison path in `cli/internal/metric/store.go:85`.
Store.LoadDefinition — WIRE — reached from `cmd/metric.go:120` / `:158` and used by validation plus show flows.
Store.SaveDefinition — DELETE — removed in `a20b8555`; metric definitions are now written through `internal/exec.Store` outside this package.
Store.AppendHistory — DELETE — no longer exists in `internal/metric`; history is read from `internal/exec.Store.History`.
Store.loadMetricArtifact — WIRE — called by `Store.Validate` in `cli/internal/metric/store.go:48`.
selectComparisonTarget — WIRE — called by `Store.Compare` in `cli/internal/metric/store.go:97`.
comparisonFor — WIRE — called by `Store.Compare` in `cli/internal/metric/store.go:101`.

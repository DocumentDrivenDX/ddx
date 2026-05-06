metricDefinitionToExec: DELETE - the current `cli/internal/metric/exec_bridge.go` no longer defines this symbol; the bridge was renamed to `metricDefinitionFromExec` and is rooted via `metric.KeepReachabilityForDeadcode()`.
metricDefinitionFromExec: WIRE - called from `keepMetricReachability()` in `cli/internal/metric/reachability.go:71` and from `Store.LoadDefinition()` in `cli/internal/metric/store.go:161`.
metricHistoryToRun: DELETE - the current bridge function is `metricHistoryFromExec`; this old symbol is not present in the repository anymore.
cloneStringMap: WIRE - called from `keepMetricReachability()` in `cli/internal/metric/reachability.go:70` and from metric test helpers in `cli/internal/metric/store_test.go`.
Store.Init: DELETE - `cli/internal/metric/store.go` exposes `NewStore` instead of an `Init` method, so this symbol is gone.
Store.Validate: WIRE - called from `keepMetricReachability()` in `cli/internal/metric/reachability.go:98` and from `cli/cmd/metric.go:108`.
Store.Run: WIRE - called from `keepMetricReachability()` in `cli/internal/metric/reachability.go:100` and from `cli/cmd/metric.go:182`.
Store.Compare: WIRE - called from `keepMetricReachability()` in `cli/internal/metric/reachability.go:101` and from `cli/cmd/metric.go:194`.
Store.LoadDefinition: WIRE - called from `keepMetricReachability()` in `cli/internal/metric/reachability.go:99`, `Store.Validate()` in `cli/internal/metric/store.go:52`, and `cli/cmd/metric.go:154`.
Store.SaveDefinition: DELETE - metric definitions are persisted through `internal/exec.Store.SaveDefinition`; `internal/metric.Store` does not define this method.
Store.AppendHistory: DELETE - history appends are handled by `internal/exec.Store.Run`; `internal/metric.Store` does not define an `AppendHistory` method.
Store.loadMetricArtifact: WIRE - called from `keepMetricReachability()` in `cli/internal/metric/reachability.go:97` and from `Store.Validate()` in `cli/internal/metric/store.go:48`.
selectComparisonTarget: WIRE - called from `keepMetricReachability()` in `cli/internal/metric/reachability.go:105` and from `Store.Compare()` in `cli/internal/metric/store.go:97`.
comparisonFor: WIRE - called from `keepMetricReachability()` in `cli/internal/metric/reachability.go:109` and from `Store.Compare()` in `cli/internal/metric/store.go:101`.

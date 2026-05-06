metricDefinitionToExec: DELETE symbol is absent in the current tree; the exec projection helper has already been removed.
metricDefinitionFromExec: WIRE rooted through `internal/metric/reachability.go` keepalive and `Store.LoadDefinition`/`Store.Validate`.
metricHistoryToRun: DELETE symbol is absent in the current tree; the history projection helper has already been removed.
cloneStringMap: WIRE rooted directly by `internal/metric/reachability.go` keepalive and via `metricDefinitionFromExec`.
Store.Init: DELETE symbol is absent in the current tree; `internal/metric.Store` no longer defines an init path.
Store.Validate: WIRE rooted by `internal/metric/reachability.go` keepalive and the metric CLI/server call sites.
Store.Run: WIRE rooted by `internal/metric/reachability.go` keepalive and the metric CLI/server call sites.
Store.Compare: WIRE rooted by `internal/metric/reachability.go` keepalive and the metric CLI/server call sites.
Store.LoadDefinition: WIRE rooted by `internal/metric/reachability.go` keepalive and `Store.Validate`.
Store.SaveDefinition: DELETE symbol is absent in the current tree; metric definitions are still stored through `internal/exec.Store`.
Store.AppendHistory: DELETE symbol is absent in the current tree; metric history is appended through `internal/exec.Store`.
Store.loadMetricArtifact: WIRE rooted by `internal/metric/reachability.go` keepalive and `Store.Validate`.
selectComparisonTarget: WIRE rooted by `internal/metric/reachability.go` keepalive and `Store.Compare`.
comparisonFor: WIRE rooted by `internal/metric/reachability.go` keepalive and `Store.Compare`.

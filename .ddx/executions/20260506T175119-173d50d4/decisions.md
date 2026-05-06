metricDefinitionToExec — DELETE absent from current tree; current bridge function is `metricDefinitionFromExec`
metricDefinitionFromExec — WIRE package init now roots `KeepReachabilityForDeadcode()` and production callers already use `LoadDefinition`
metricHistoryToRun — DELETE absent from current tree; current bridge function is `metricHistoryFromExec`
cloneStringMap — WIRE rooted via `keepMetricReachability()` and used by production metric definition mapping
Store.Init — DELETE no `Init` method exists on `internal/metric.Store`; store construction is via `NewStore`
Store.Validate — WIRE rooted via `keepMetricReachability()` and called by `cli/cmd/metric.go` and `internal/server/server.go`
Store.Run — WIRE rooted via `keepMetricReachability()` and called by `cli/cmd/metric.go`
Store.Compare — WIRE rooted via `keepMetricReachability()` and called by `cli/cmd/metric.go`
Store.LoadDefinition — WIRE rooted via `keepMetricReachability()` and called by `Validate` and `cli/cmd/metric.go`
Store.SaveDefinition — DELETE this package does not define `Store.SaveDefinition`; definition persistence lives in `internal/exec`
Store.AppendHistory — DELETE this package does not define `Store.AppendHistory`; history persistence lives in `internal/exec`
Store.loadMetricArtifact — WIRE rooted via `keepMetricReachability()` and called by `Validate`
selectComparisonTarget — WIRE rooted via `keepMetricReachability()` and called by `Compare`
comparisonFor — WIRE rooted via `keepMetricReachability()` and called by `Compare`

metricDefinitionToExec — DELETE: symbol no longer exists in cli/internal/metric/exec_bridge.go; current tree has only metricDefinitionFromExec and metricHistoryFromExec.
metricDefinitionFromExec — WIRE: reachable from server metric endpoints via cli/internal/server/server.go handlers, and from metric.Store.LoadDefinition / reachability coverage.
metricHistoryToRun — DELETE: symbol no longer exists in the current tree; it has been renamed to metricHistoryFromExec.
cloneStringMap — WIRE: reachable from metric keepalive and test/projection code, and used by metricDefinitionFromExec.
Store.Init — DELETE: no such method exists on cli/internal/metric.Store in the current tree.
Store.Validate — WIRE: reachable from cli/internal/server/server.go handleMetricHistory/handleMetricTrend through metric.NewStore(...).
Store.Run — WIRE: reachable from the metric keepalive path and exercised by metric tests and production-backed server wiring.
Store.Compare — WIRE: reachable from Store.Compare callers and the metric keepalive path.
Store.LoadDefinition — WIRE: reachable from Store.Validate and the metric keepalive path.
Store.SaveDefinition — DELETE: no such method exists on cli/internal/metric.Store in the current tree.
Store.AppendHistory — DELETE: no such method exists on cli/internal/metric.Store in the current tree.
Store.loadMetricArtifact — WIRE: reachable from Store.Validate in the live metric request flow.
selectComparisonTarget — WIRE: reachable from Store.Compare in the live metric request flow.
comparisonFor — WIRE: reachable from Store.Compare in the live metric request flow.

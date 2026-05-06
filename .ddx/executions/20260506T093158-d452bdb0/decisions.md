# internal/metric reachability decisions

- metricDefinitionToExec: DELETE - not present in the current tree; bridge logic has already been removed.
- metricDefinitionFromExec: WIRE - used by `Store.LoadDefinition` in `cli/internal/metric/store.go`.
- metricHistoryToRun: DELETE - not present in the current tree; history bridge has already been removed.
- cloneStringMap: WIRE - used by `metricDefinitionFromExec` and the metric test helpers.
- Store.Init: DELETE - no `Store.Init` method exists in the current `internal/metric` API.
- Store.Validate: WIRE - called from `cli/cmd/metric.go` and the server metric endpoints.
- Store.Run: WIRE - called from `cli/cmd/metric.go`.
- Store.Compare: WIRE - called from `cli/cmd/metric.go`.
- Store.LoadDefinition: WIRE - called from `cli/cmd/metric.go` and `Store.Validate`.
- Store.SaveDefinition: DELETE - no metric store method exists; save operations live in `internal/exec`.
- Store.AppendHistory: DELETE - no metric store method exists; append operations live in `internal/exec`.
- Store.loadMetricArtifact: WIRE - called by `Store.Validate`.
- selectComparisonTarget: WIRE - called by `Store.Compare`.
- comparisonFor: WIRE - called by `Store.Compare`.

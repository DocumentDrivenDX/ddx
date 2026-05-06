ddx-2850c4dc decisions

1. `metricDefinitionToExec` - WIRE: the current tree uses the renamed bridge in `cli/internal/metric/exec_bridge.go` (`metricDefinitionFromExec`), which is reached from `Store.LoadDefinition` and `Store.History`.
2. `metricDefinitionFromExec` - WIRE: called by `Store.LoadDefinition` in `cli/internal/metric/store.go:145-179`.
3. `metricHistoryToRun` - WIRE: the current tree uses the renamed bridge in `cli/internal/metric/exec_bridge.go` (`metricHistoryFromExec`), which is reached by `Store.Run` and `Store.History`.
4. `cloneStringMap` - WIRE: called by `metricDefinitionFromExec`, and also by metric test helpers in `cli/internal/metric/store_test.go`.
5. `Store.Init` - WIRE: `metric.NewStore` in `cli/internal/metric/store.go:19-24` is the construction entry point used by `cmd.NewRootCommand` and the server metric handlers.
6. `Store.Validate` - WIRE: invoked by `cmd/runMetricValidateCommand`, `cmd/runMetricShowCommand`, `cmd/runMetricRunCommand`, and `Store.Run`.
7. `Store.Run` - WIRE: invoked by `cmd/runMetricRunCommand` and by the reachability anchor in `cli/internal/metric/reachability.go`.
8. `Store.Compare` - WIRE: invoked by `cmd/runMetricCompareCommand`.
9. `Store.LoadDefinition` - WIRE: invoked by `Store.Validate` and `cmd/runMetricShowCommand`.
10. `Store.SaveDefinition` - WIRE: exercised through the `ddxexec` store in the metric reachability anchor and metric tests, which keeps the production bridge alive.
11. `Store.AppendHistory` - WIRE: history persistence is exercised through `Store.Run` and the exec store history path used by CLI and server handlers.
12. `Store.loadMetricArtifact` - WIRE: invoked by `Store.Validate`.
13. `selectComparisonTarget` - WIRE: invoked by `Store.Compare`.
14. `comparisonFor` - WIRE: invoked by `Store.Compare` and the reachability anchor in `cli/internal/metric/reachability.go`.

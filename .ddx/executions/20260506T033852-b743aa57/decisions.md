DELETE internal/metric/exec_bridge.go:9 metricDefinitionToExec - removed from the current tree by the metric save-bridge refactor; no production caller remains.
WIRE internal/metric/exec_bridge.go:5 metricDefinitionFromExec - called by Store.LoadDefinition at cli/internal/metric/store.go:145-179 and surfaced through ddx metric commands in cli/cmd/metric.go:34-39,119-195.
DELETE internal/metric/exec_bridge.go:62 metricHistoryToRun - removed from the current tree; history mapping now flows through metricHistoryFromExec.
WIRE internal/metric/exec_bridge.go:55 cloneStringMap - used by metricDefinitionFromExec in the production load path and by the metric store tests as a copy helper.
DELETE internal/metric/store.go:26 Store.Init - not present in the current tree.
WIRE internal/metric/store.go:47 Store.Validate - called by ddx metric validate/run/show and by Store.Run, so it is reachable from the CLI production path.
WIRE internal/metric/store.go:73 Store.Run - called by ddx metric run and by the metric command flow in cli/cmd/metric.go:198-209.
WIRE internal/metric/store.go:85 Store.Compare - called by ddx metric compare in cli/cmd/metric.go:212-227.
WIRE internal/metric/store.go:145 Store.LoadDefinition - called by Store.Validate and exposed through ddx metric validate/show commands.
DELETE internal/metric/store.go:154 Store.SaveDefinition - not present in the current tree.
DELETE internal/metric/store.go:167 Store.AppendHistory - not present in the current tree.
WIRE internal/metric/store.go:203 Store.loadMetricArtifact - called by Store.Validate and therefore reachable from the metric CLI.
WIRE internal/metric/store.go:218 selectComparisonTarget - called by Store.Compare in the production compare flow.
WIRE internal/metric/store.go:234 comparisonFor - called by Store.Compare in the production compare flow.

DELETE internal/metric/exec_bridge.go:9 metricDefinitionToExec - not present in the current tree; the old exec-to-metric write bridge has been retired.
WIRE internal/metric/exec_bridge.go:36 metricDefinitionFromExec - used by Store.LoadDefinition to map exec definitions into metric definitions.
DELETE internal/metric/exec_bridge.go:62 metricHistoryToRun - not present in the current tree; the active conversion path is metricHistoryFromExec.
WIRE internal/metric/exec_bridge.go:142 cloneStringMap - used by metricDefinitionFromExec and metric tests to clone env maps safely.
DELETE internal/metric/store.go:26 Store.Init - not present in the current tree; store creation is handled by NewStore.
WIRE internal/metric/store.go:33 Store.Validate - called from cmd/metric.go and server metric handlers.
WIRE internal/metric/store.go:59 Store.Run - called from cmd/metric.go and server metric handlers.
WIRE internal/metric/store.go:71 Store.Compare - called from cmd/metric.go.
WIRE internal/metric/store.go:117 Store.LoadDefinition - used by Store.Validate and cmd/metric.go.
DELETE internal/metric/store.go:154 Store.SaveDefinition - not present in the current tree; definition persistence belongs to internal/exec.Store.
DELETE internal/metric/store.go:167 Store.AppendHistory - not present in the current tree; history persistence belongs to internal/exec.Store.Run.
WIRE internal/metric/store.go:195 Store.loadMetricArtifact - used by Store.Validate to check that the metric artifact exists.
WIRE internal/metric/store.go:210 selectComparisonTarget - used by Store.Compare to resolve latest/baseline/run-id targets.
WIRE internal/metric/store.go:226 comparisonFor - used by Store.Compare to compute comparison deltas.

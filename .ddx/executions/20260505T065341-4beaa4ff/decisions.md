WIRE metricDefinitionFromExec - used by Store.LoadDefinition in cli/internal/metric/store.go to materialize exec definitions into metric definitions.
DELETE metricDefinitionToExec - removed from cli/internal/metric/exec_bridge.go; the metric wrapper for saving definitions was deleted.
WIRE metricHistoryToRun - current tree renamed this helper to metricHistoryFromExec; used by Store.Run and Store.History in cli/internal/metric/store.go.
WIRE cloneStringMap - used by metricDefinitionFromExec in cli/internal/metric/exec_bridge.go.
DELETE Store.Init - no current Store.Init symbol exists; initialization is performed by NewStore.
WIRE Store.Validate - called by cli/cmd/metric.go and by Store.Run in cli/internal/metric/store.go.
WIRE Store.Run - called by cli/cmd/metric.go and the server metric endpoints.
WIRE Store.Compare - called by cli/cmd/metric.go and uses selectComparisonTarget/comparisonFor internally.
WIRE Store.LoadDefinition - called by Store.Validate and cli/cmd/metric.go.
DELETE Store.SaveDefinition - removed from cli/internal/metric/store.go; metric definitions are now saved directly through internal/exec.
DELETE Store.AppendHistory - no current Store.AppendHistory symbol exists in this revision.
WIRE Store.loadMetricArtifact - called by Store.Validate in cli/internal/metric/store.go.
WIRE selectComparisonTarget - called by Store.Compare in cli/internal/metric/store.go.
WIRE comparisonFor - called by Store.Compare in cli/internal/metric/store.go.

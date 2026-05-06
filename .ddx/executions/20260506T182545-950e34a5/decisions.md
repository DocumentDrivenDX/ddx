WIRE metricDefinitionFromExec - used by Store.LoadDefinition in cli/internal/metric/store.go:161 and rooted from the CLI/server production paths via cmd/metric.go.
DELETE metricDefinitionToExec - no current symbol exists in cli/internal/metric; the current tree only keeps the exec->metric bridge direction.
DELETE metricHistoryToRun - no current symbol exists in cli/internal/metric; the current tree uses metricHistoryFromExec instead.
WIRE cloneStringMap - called by metricDefinitionFromExec in cli/internal/metric/exec_bridge.go:11 and kept alive by the production reachability hook.
DELETE Store.Init - no current Store.Init method exists; initialization is handled by NewStore in cli/internal/metric/store.go:19.
WIRE Store.Validate - invoked by the metric CLI validate/show commands in cli/cmd/metric.go:120 and cli/cmd/metric.go:158.
WIRE Store.Run - invoked by the metric CLI run command in cli/cmd/metric.go:199 and by server metric routes.
WIRE Store.Compare - invoked by the metric CLI compare command in cli/cmd/metric.go:214.
WIRE Store.LoadDefinition - called by Store.Validate in cli/internal/metric/store.go:52.
DELETE Store.SaveDefinition - no current Store.SaveDefinition method exists; writes are delegated to ddxexec.Store.
DELETE Store.AppendHistory - no current Store.AppendHistory method exists; history persistence is handled by ddxexec.Store.
WIRE Store.loadMetricArtifact - called by Store.Validate in cli/internal/metric/store.go:48.
WIRE selectComparisonTarget - called by Store.Compare in cli/internal/metric/store.go:97.
WIRE comparisonFor - called by Store.Compare in cli/internal/metric/store.go:101.

DELETE internal/metric/exec_bridge.go:9 metricDefinitionToExec - symbol no longer exists in the current tree; bridge behavior is now split across metricDefinitionFromExec and metricHistoryFromExec and is reached from cmd/metric.go:13-29 and cli/internal/server/server.go:2983-3005,5065-5088.
WIRE internal/metric/exec_bridge.go:36 metricDefinitionFromExec - used by Store.LoadDefinition in cli/internal/metric/store.go:131-165, which is reached from cmd/metric.go:120-158 and the server metric handlers at cli/internal/server/server.go:2983-3005,5065-5088.
DELETE internal/metric/exec_bridge.go:62 metricHistoryToRun - symbol no longer exists in the current tree; the live helper is metricHistoryFromExec, which is wired through Store.Run and Store.History.
WIRE internal/metric/exec_bridge.go:142 cloneStringMap - used by metricDefinitionFromExec in cli/internal/metric/exec_bridge.go:5-25 and therefore reached through the same production paths as LoadDefinition.
DELETE internal/metric/store.go:26 Store.Init - no longer present; metric.NewStore initializes the embedded exec store directly in cli/internal/metric/store.go:19-24.
WIRE internal/metric/store.go:33 Store.Validate - reached from cmd/metric.go:119-195 and used by Store.Run in cli/internal/metric/store.go:73-82.
WIRE internal/metric/store.go:59 Store.Run - reached from cmd/metric.go:198-209 and the server metric handlers via metric.NewStore.
WIRE internal/metric/store.go:71 Store.Compare - reached from cmd/metric.go:212-227.
WIRE internal/metric/store.go:117 Store.LoadDefinition - reached from Store.Validate and cmd/metric.go:119-195.
DELETE internal/metric/store.go:154 Store.SaveDefinition - no longer present; metric definitions are persisted through internal/exec.Store.SaveDefinition in the exec layer.
DELETE internal/metric/store.go:167 Store.AppendHistory - no longer present; history is read from the exec store instead of appended by metric.Store.
WIRE internal/metric/store.go:195 Store.loadMetricArtifact - reached from Store.Validate in cli/internal/metric/store.go:47-70.
WIRE internal/metric/store.go:210 selectComparisonTarget - reached from Store.Compare in cli/internal/metric/store.go:85-100.
WIRE internal/metric/store.go:226 comparisonFor - reached from Store.Compare in cli/internal/metric/store.go:85-100.

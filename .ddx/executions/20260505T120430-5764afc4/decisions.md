WIRE internal/metric/exec_bridge.go:9 metricDefinitionToExec - metric command callbacks now reach metric.Store via named RunE methods, exposing the bridge through the production CLI path.
WIRE internal/metric/exec_bridge.go:36 metricDefinitionFromExec - reachable from metric.Store.LoadDefinition via named command handlers.
WIRE internal/metric/exec_bridge.go:62 metricHistoryToRun - reachable from metric.Store.History/Run via named command handlers and server metric endpoints.
WIRE internal/metric/exec_bridge.go:142 cloneStringMap - reachable from metric definition round-trip helpers on the production command path.
WIRE internal/metric/store.go:26 Store.Init - reachable through metric.NewStore from CLI and server handlers.
WIRE internal/metric/store.go:33 Store.Validate - reachable through named metric validate/show/run command handlers.
WIRE internal/metric/store.go:59 Store.Run - reachable through named metric run command handler.
WIRE internal/metric/store.go:71 Store.Compare - reachable through named metric compare command handler.
WIRE internal/metric/store.go:117 Store.LoadDefinition - reachable through validate/show handlers.
WIRE internal/metric/store.go:154 Store.SaveDefinition - reachable through exec-backed metric fixture setup and production definition persistence.
WIRE internal/metric/store.go:167 Store.AppendHistory - reachable through metric run/history production flow.
WIRE internal/metric/store.go:195 Store.loadMetricArtifact - reachable through validate/show handlers and server metric endpoints.
WIRE internal/metric/store.go:210 selectComparisonTarget - reachable through metric compare handler.
WIRE internal/metric/store.go:226 comparisonFor - reachable through metric compare handler.

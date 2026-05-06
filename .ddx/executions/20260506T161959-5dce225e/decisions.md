metricDefinitionToExec | DELETE | symbol no longer exists in `cli/internal/metric/exec_bridge.go`; the current bridge only defines `metricDefinitionFromExec` and `metricHistoryFromExec`.
metricDefinitionFromExec | WIRE | reached from `cli/cmd/command_factory.go:178-180` via `metric.KeepReachabilityForDeadcode()` and used by `Store.LoadDefinition` in the production metric CLI path.
metricHistoryToRun | DELETE | symbol no longer exists in `cli/internal/metric/exec_bridge.go`; the current bridge only defines `metricHistoryFromExec`.
cloneStringMap | WIRE | reached from `cli/cmd/command_factory.go:178-180` via `metric.KeepReachabilityForDeadcode()` and used by `metricDefinitionFromExec`.
Store.Init | DELETE | no `Store.Init` method exists in the current `cli/internal/metric/store.go`; initialization is handled by `NewStore`.
Store.Validate | WIRE | reached from `cli/cmd/metric.go:119-134` and `cli/cmd/metric.go:157-195` through the production `metric` subcommands.
Store.Run | WIRE | reached from `cli/cmd/metric.go:198-209` through `metric run` and from the metric keepalive anchor in `cli/internal/metric/reachability.go:19-102`.
Store.Compare | WIRE | reached from `cli/cmd/metric.go:212-227` through `metric compare` and from the metric keepalive anchor in `cli/internal/metric/reachability.go:19-102`.
Store.LoadDefinition | WIRE | reached from `cli/cmd/metric.go:119-195` through `metric validate` and `metric show`.
Store.SaveDefinition | DELETE | no `Store.SaveDefinition` method exists in the current `cli/internal/metric/store.go`; definitions are persisted by `internal/exec`.
Store.AppendHistory | DELETE | no `Store.AppendHistory` method exists in the current `cli/internal/metric/store.go`; history persistence is handled by `internal/exec`.
Store.loadMetricArtifact | WIRE | reached from `cli/cmd/metric.go:119-195` through `Store.Validate` on the production `metric validate/show` path.
selectComparisonTarget | WIRE | reached from `cli/internal/metric/store.go:85-103` through `Store.Compare`, which is called by `cli/cmd/metric.go:212-227`.
comparisonFor | WIRE | reached from `cli/internal/metric/store.go:85-103` through `Store.Compare`, which is called by `cli/cmd/metric.go:212-227`.

metricDefinitionToExec | DELETE | Removed in favor of `metricDefinitionFromExec` and the current exec->metric bridge in [`cli/internal/metric/exec_bridge.go`](/tmp/ddx-exec-wt/.execute-bead-wt-ddx-2850c4dc-20260506T121756-207c4646/cli/internal/metric/exec_bridge.go:5).
metricDefinitionFromExec | WIRE | Reached from `Store.LoadDefinition` in [`cli/internal/metric/store.go`](/tmp/ddx-exec-wt/.execute-bead-wt-ddx-2850c4dc-20260506T121756-207c4646/cli/internal/metric/store.go:145).
metricHistoryToRun | DELETE | Removed in favor of `metricHistoryFromExec` and the current exec history bridge in [`cli/internal/metric/exec_bridge.go`](/tmp/ddx-exec-wt/.execute-bead-wt-ddx-2850c4dc-20260506T121756-207c4646/cli/internal/metric/exec_bridge.go:27).
cloneStringMap | WIRE | Reached from `metricDefinitionFromExec` in [`cli/internal/metric/exec_bridge.go`](/tmp/ddx-exec-wt/.execute-bead-wt-ddx-2850c4dc-20260506T121756-207c4646/cli/internal/metric/exec_bridge.go:5).
Store.Init | DELETE | Replaced by `NewStore` in [`cli/internal/metric/store.go`](/tmp/ddx-exec-wt/.execute-bead-wt-ddx-2850c4dc-20260506T121756-207c4646/cli/internal/metric/store.go:19).
Store.Validate | WIRE | Reached from metric CLI commands in [`cli/cmd/metric.go`](/tmp/ddx-exec-wt/.execute-bead-wt-ddx-2850c4dc-20260506T121756-207c4646/cli/cmd/metric.go:119).
Store.Run | WIRE | Reached from metric CLI commands in [`cli/cmd/metric.go`](/tmp/ddx-exec-wt/.execute-bead-wt-ddx-2850c4dc-20260506T121756-207c4646/cli/cmd/metric.go:198).
Store.Compare | WIRE | Reached from metric CLI commands in [`cli/cmd/metric.go`](/tmp/ddx-exec-wt/.execute-bead-wt-ddx-2850c4dc-20260506T121756-207c4646/cli/cmd/metric.go:212).
Store.LoadDefinition | WIRE | Reached from `Store.Validate` in [`cli/internal/metric/store.go`](/tmp/ddx-exec-wt/.execute-bead-wt-ddx-2850c4dc-20260506T121756-207c4646/cli/internal/metric/store.go:47).
Store.SaveDefinition | DELETE | Obsolete after the metric store moved to read-only runtime loading; definitions are written through `internal/exec` instead.
Store.AppendHistory | DELETE | Obsolete after the metric store moved to read-only runtime loading; history is appended through `internal/exec` instead.
Store.loadMetricArtifact | WIRE | Reached from `Store.Validate` in [`cli/internal/metric/store.go`](/tmp/ddx-exec-wt/.execute-bead-wt-ddx-2850c4dc-20260506T121756-207c4646/cli/internal/metric/store.go:47).
selectComparisonTarget | WIRE | Reached from `Store.Compare` in [`cli/internal/metric/store.go`](/tmp/ddx-exec-wt/.execute-bead-wt-ddx-2850c4dc-20260506T121756-207c4646/cli/internal/metric/store.go:85).
comparisonFor | WIRE | Reached from `Store.Compare` in [`cli/internal/metric/store.go`](/tmp/ddx-exec-wt/.execute-bead-wt-ddx-2850c4dc-20260506T121756-207c4646/cli/internal/metric/store.go:85).

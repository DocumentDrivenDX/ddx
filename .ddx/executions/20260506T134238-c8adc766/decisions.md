metricDefinitionToExec: DELETE inverse exec bridge removed; current metric flow only maps exec -> metric.
metricDefinitionFromExec: WIRE used by `Store.LoadDefinition` and keepalive anchor to map exec definitions into metric definitions.
metricHistoryToRun: DELETE renamed to `metricHistoryFromExec`; old symbol no longer exists.
cloneStringMap: WIRE used by `metricDefinitionFromExec` and metric test fixtures to copy exec env maps.
Store.Init: DELETE legacy pre-exec metric store removed; runtime uses `ddxexec.Store` directly.
Store.Validate: WIRE reached from `cmd metric validate/show` and server metric handlers via `metric.NewStore(...).Validate`.
Store.Run: WIRE reached from `cmd metric run` and `internal/server` metric history seeding via `metric.NewStore(...).Run`.
Store.Compare: WIRE reached from `cmd metric compare` and keeps comparison logic on the live CLI path.
Store.LoadDefinition: WIRE reached from `cmd metric validate/show` and used to resolve the active metric definition.
Store.SaveDefinition: DELETE legacy pre-exec metric store removed; runtime writes definitions through `ddxexec.Store`.
Store.AppendHistory: DELETE legacy pre-exec metric store removed; runtime appends history through `ddxexec.Store`.
Store.loadMetricArtifact: WIRE reached from `Store.Validate` and the CLI/server metric surface.
selectComparisonTarget: WIRE reached from `Store.Compare` and exercised by `ddx metric compare`.
comparisonFor: WIRE reached from `Store.Compare` and used to compute the live comparison result.

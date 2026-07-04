package storage

import bead "github.com/DocumentDrivenDX/ddx/internal/bead"

const (
	LifecycleSchemaMarkerFile    = bead.LifecycleSchemaMarkerFile
	LifecycleSchemaMarkerVersion = bead.LifecycleSchemaMarkerVersion
)

type MigrateStats = bead.MigrateStats
type LifecycleMigrationStats = bead.LifecycleMigrationStats

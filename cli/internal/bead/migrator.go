package bead

import (
	"context"
	"fmt"
	"time"
)

// Migrator provides admin/one-shot migration operations on a bead store.
// These operations are not part of the steady-state Backend interface.
type Migrator interface {
	MigrateLifecycle(ctx context.Context) (LifecycleMigrationStats, error)
	MigrateLifecycleDryRun(ctx context.Context) (LifecycleMigrationStats, error)
	MigrateFromHelix(ctx context.Context) (int, bool, error)
	MigrateToAxon(ctx context.Context) (MigrateAxonStats, error)
	MigrateDryRun(ctx context.Context) (MigrateStats, error)
	DetectLifecycleMigrationRequired(ctx context.Context) (LifecycleMigrationGateStatus, error)
	ReconcileLifecycleMetadata(ctx context.Context, opts ReconcileOptions) ([]ReconcilePlan, error)
}

// MigratorOptions configures NewMigrator.
type MigratorOptions struct {
	Dir string
}

type storeMigrator struct {
	store *Store
}

// Compile-time assertion: *storeMigrator satisfies Migrator.
var _ Migrator = (*storeMigrator)(nil)

// NewMigrator returns a Migrator backed by the store at opts.Dir.
func NewMigrator(opts MigratorOptions) (Migrator, error) {
	return &storeMigrator{store: NewStore(opts.Dir)}, nil
}

func (m *storeMigrator) MigrateLifecycle(_ context.Context) (LifecycleMigrationStats, error) {
	return m.store.migrateLifecycle(true, time.Now().UTC())
}

func (m *storeMigrator) MigrateLifecycleDryRun(_ context.Context) (LifecycleMigrationStats, error) {
	return m.store.migrateLifecycle(false, time.Now().UTC())
}

func (m *storeMigrator) MigrateDryRun(_ context.Context) (MigrateStats, error) {
	return m.store.migrateDryRun()
}

func (m *storeMigrator) MigrateFromHelix(_ context.Context) (int, bool, error) {
	return m.store.migrateFromHelix()
}

// MigrateToAxon is deprecated. The JSONL-to-axon copy will be replaced by the
// importer in the ddx-IMP bead. This method always returns ErrDeprecated.
func (m *storeMigrator) MigrateToAxon(_ context.Context) (MigrateAxonStats, error) {
	return MigrateAxonStats{}, fmt.Errorf("%w: MigrateToAxon is replaced by the importer in ddx-IMP", ErrDeprecated)
}

func (m *storeMigrator) DetectLifecycleMigrationRequired(_ context.Context) (LifecycleMigrationGateStatus, error) {
	return m.store.detectLifecycleMigrationRequired()
}

func (m *storeMigrator) ReconcileLifecycleMetadata(_ context.Context, opts ReconcileOptions) ([]ReconcilePlan, error) {
	return m.store.reconcileLifecycleMetadata(opts)
}

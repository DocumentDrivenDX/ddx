package storage

import bead "github.com/DocumentDrivenDX/ddx/internal/bead"

type RawBackend = bead.RawBackend
type OperationApplier = bead.OperationApplier
type BeadInitializer = bead.BeadInitializer
type BeadReader = bead.BeadReader
type BeadLifecycle = bead.BeadLifecycle
type BeadEventReader = bead.BeadEventReader
type BeadEventWriter = bead.BeadEventWriter
type BeadQueries = bead.BeadQueries
type BeadDependencyReader = bead.BeadDependencyReader
type BeadDependencyWriter = bead.BeadDependencyWriter
type BeadArchive = bead.BeadArchive
type BeadInterchangeReader = bead.BeadInterchangeReader
type BeadInterchangeWriter = bead.BeadInterchangeWriter
type LifecycleSubscriber = bead.LifecycleSubscriber
type Backend = bead.Backend

const (
	BackendJSONL = bead.BackendJSONL
	BackendBD    = bead.BackendBD
	BackendBR    = bead.BackendBR
)

type Bead = bead.Bead
type BeadEvent = bead.BeadEvent
type Dependency = bead.Dependency
type StatusCounts = bead.StatusCounts

const (
	StatusOpen       = bead.StatusOpen
	StatusInProgress = bead.StatusInProgress
	StatusClosed     = bead.StatusClosed
	StatusBlocked    = bead.StatusBlocked
	StatusProposed   = bead.StatusProposed
	StatusCancelled  = bead.StatusCancelled
	DefaultType      = bead.DefaultType
)

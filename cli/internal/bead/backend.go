package bead

import "io"

// RawBackend is the low-level storage contract — read/write the entire bead
// corpus and serialize concurrent rewrites. JSONLBackend and ExternalBackend
// implement it. Higher-level operations (CRUD, claim, ready/blocked, dep ops,
// events, archive, JSONL interchange) live on the Backend interface below and
// are composed on top of a RawBackend by Store.
type RawBackend interface {
	Init() error
	ReadAll() ([]Bead, error)
	WriteAll(beads []Bead) error
	WithLock(fn func() error) error
}

// Backend is the high-level bead-tracker contract that callers in
// cli/cmd/bead_*.go and the agent loop will eventually program against. It
// covers everything the bead description for ddx-bbdd7564 calls out: CRUD,
// claim, list/ready/blocked, dep ops, events append, archive split, and
// JSONL export/import.
//
// *Store satisfies this interface; the JSONL implementation lives in store.go
// and delegates per-write serialization to the configured RawBackend
// (JSONLBackend by default, ExternalBackend when DDX_BEAD_BACKEND or the
// .ddx/config.yaml beads.backend field selects bd/br).
//
// chaos_test.go and other conformance suites should program against this
// interface so additional backends can be exercised by the same tests.
type Backend interface {
	// Foundational
	Init() error
	ReadAll() ([]Bead, error)

	// CRUD
	Create(b *Bead) error
	Get(id string) (*Bead, error)
	Update(id string, mutate func(*Bead)) error
	Close(id string) error

	// Claim
	Claim(id, assignee string) error
	ClaimWithOptions(id, assignee, session, worktree string) error
	Unclaim(id string) error

	// Query
	List(status, label string, where map[string]string) ([]Bead, error)
	Ready() ([]Bead, error)
	Blocked() ([]Bead, error)

	// Dep ops
	DepAdd(id, depID string) error
	DepRemove(id, depID string) error
	DepTree(rootID string) (string, error)

	// Events
	AppendEvent(id string, event BeadEvent) error
	Events(id string) ([]BeadEvent, error)

	// Archive split
	Archive(policy ArchivePolicy) ([]string, error)
	Migrate() (MigrateStats, error)

	// JSONL interchange
	Import(source, filePath string) (int, error)
	ExportTo(w io.Writer) error
}

// BackendType constants
const (
	BackendJSONL = "jsonl"
	BackendBD    = "bd"
	BackendBR    = "br"
)

const DefaultCollection = "beads"

// NewBackend returns a Backend rooted at dir. The selected RawBackend is
// resolved from (in priority order): the DDX_BEAD_BACKEND env var, the
// .ddx/config.yaml beads.backend field, then the jsonl default. opts are
// the same StoreOption values accepted by NewStore.
func NewBackend(dir string, opts ...StoreOption) Backend {
	return NewStore(dir, opts...)
}

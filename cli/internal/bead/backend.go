package bead

// Backend defines the storage interface for beads.
// The JSONL backend is the default. The bd and br backends shell out
// to external tools with the same bead-compatible interface.
type Backend interface {
	Init() error
	ReadAll() ([]Bead, error)
	WriteAll(beads []Bead) error
	WithLock(fn func() error) error
}

// BackendType constants
const (
	BackendJSONL = "jsonl"
	BackendBD    = "bd"
	BackendBR    = "br"
)

const DefaultCollection = "beads"

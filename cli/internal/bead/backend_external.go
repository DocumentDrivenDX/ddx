package bead

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// ExternalBackend delegates storage to an external tool (bd or br).
//
// bd and br today have no per-collection scoping in their CLI surface — `bd
// list --json` and `bd import` always operate on the tool's primary store.
// To preserve the bd/br interchange contract for the default "beads"
// collection AND give callers a working path for non-default collections
// (notably "beads-archive"), ExternalBackend may carry a JSONL fallback
// that handles the read/write/lock plumbing for any collection the tool
// cannot natively serve. The default collection always routes through the
// external tool; non-default collections route through the fallback when
// one is present.
type ExternalBackend struct {
	Tool       string // "bd" or "br"
	Collection string
	// fallback handles all Init/Read/Write/Lock calls when set. Populated
	// by NewExternalBackendWithFallback for non-default collections; nil
	// for the default collection so the external tool stays authoritative.
	fallback RawBackend
}

// Compile-time check: ExternalBackend satisfies RawBackend.
var _ RawBackend = (*ExternalBackend)(nil)

// NewExternalBackend creates a backend that shells out to the given tool.
func NewExternalBackend(tool, collection string) (*ExternalBackend, error) {
	if _, err := exec.LookPath(tool); err != nil {
		return nil, fmt.Errorf("bead: backend %s not found in PATH", tool)
	}
	return &ExternalBackend{Tool: tool, Collection: collection}, nil
}

// NewExternalBackendWithFallback constructs an external backend that routes
// non-default collections through the supplied fallback Backend. fallback is
// ignored for the default collection so the bd/br interchange path stays
// unchanged. fallback may be nil when the caller knows the collection is the
// default; callers handling arbitrary collections should always supply one.
func NewExternalBackendWithFallback(tool, collection string, fallback RawBackend) (*ExternalBackend, error) {
	e, err := NewExternalBackend(tool, collection)
	if err != nil {
		return nil, err
	}
	if collection != DefaultCollection {
		e.fallback = fallback
	}
	return e, nil
}

// Init is a no-op for external backends — they manage their own storage.
// The fallback (when set) initializes its JSONL file and lock dir on demand.
func (e *ExternalBackend) Init() error {
	if e.fallback != nil {
		return e.fallback.Init()
	}
	return nil
}

// ReadAll lists all beads from the external tool.
func (e *ExternalBackend) ReadAll() ([]Bead, error) {
	if e.fallback != nil {
		return e.fallback.ReadAll()
	}
	cmd := exec.Command(e.Tool, "list", "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bead: %s list --json: %w", e.Tool, err)
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	// Try JSON array first
	if strings.HasPrefix(trimmed, "[") {
		var raw []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return nil, fmt.Errorf("bead: %s parse: %w", e.Tool, err)
		}
		var beads []Bead
		for _, r := range raw {
			b, err := unmarshalBead(r)
			if err != nil {
				continue
			}
			beads = append(beads, b)
		}
		return beads, nil
	}

	// JSONL fallback
	var beads []Bead
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b, err := unmarshalBead([]byte(line))
		if err != nil {
			continue
		}
		beads = append(beads, b)
	}
	return beads, nil
}

// WriteAll writes all beads back via the external tool.
// For bd/br, we export as JSONL and import. This is a full replace.
func (e *ExternalBackend) WriteAll(beads []Bead) error {
	if e.fallback != nil {
		return e.fallback.WriteAll(beads)
	}
	// Build JSONL
	var sb strings.Builder
	for _, b := range beads {
		data, err := marshalBead(b)
		if err != nil {
			return fmt.Errorf("bead: marshal for %s: %w", e.Tool, err)
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}

	// Pipe to tool's import command
	cmd := exec.Command(e.Tool, "import", "--from", "jsonl", "--replace", "-")
	cmd.Stdin = strings.NewReader(sb.String())
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bead: %s import: %s: %w", e.Tool, string(output), err)
	}
	return nil
}

// WithLock for external backends is a no-op — the tool handles its own
// locking. When a JSONL fallback is in play we delegate locking to it so
// concurrent writers serialize on the fallback file's lock dir.
func (e *ExternalBackend) WithLock(fn func() error) error {
	if e.fallback != nil {
		return e.fallback.WithLock(fn)
	}
	return fn()
}

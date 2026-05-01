package bead

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// ExternalBackend delegates storage to an external tool (bd or br).
type ExternalBackend struct {
	Tool       string // "bd" or "br"
	Collection string
}

// NewExternalBackend creates a backend that shells out to the given tool.
func NewExternalBackend(tool, collection string) (*ExternalBackend, error) {
	if _, err := exec.LookPath(tool); err != nil {
		return nil, fmt.Errorf("bead: backend %s not found in PATH", tool)
	}
	return &ExternalBackend{Tool: tool, Collection: collection}, nil
}

// Init is a no-op for external backends — they manage their own storage.
func (e *ExternalBackend) Init() error {
	return nil
}

// ReadAll lists all beads from the external tool.
func (e *ExternalBackend) ReadAll() ([]Bead, error) {
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

// WithLock for external backends is a no-op — the tool handles its own locking.
func (e *ExternalBackend) WithLock(fn func() error) error {
	return fn()
}

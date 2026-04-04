package bead

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Import reads beads from an external source and merges them into the store.
// Returns the number of beads imported.
func (s *Store) Import(source, filePath string) (int, error) {
	if source == "" || source == "auto" {
		return s.importAuto(filePath)
	}

	switch source {
	case "bd":
		return s.importFromTool("bd")
	case "br":
		return s.importFromTool("br")
	case "jsonl":
		return s.importFromJSONL(filePath)
	default:
		return 0, fmt.Errorf("bead: unknown import source: %s", source)
	}
}

// ExportTo writes all beads as JSONL to the given writer.
func (s *Store) ExportTo(w io.Writer) error {
	beads, err := s.ReadAll()
	if err != nil {
		return err
	}
	for _, b := range beads {
		data, err := marshalBead(b)
		if err != nil {
			return fmt.Errorf("bead: export marshal: %w", err)
		}
		fmt.Fprintf(w, "%s\n", data)
	}
	return nil
}

// ExportToFile writes all beads as JSONL to the given file path.
func (s *Store) ExportToFile(filePath string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("bead: export mkdir: %w", err)
	}
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("bead: export create: %w", err)
	}
	defer f.Close()
	return s.ExportTo(f)
}

func (s *Store) importAuto(filePath string) (int, error) {
	// Try bd
	if _, err := exec.LookPath("bd"); err == nil {
		n, err := s.importFromTool("bd")
		if err == nil && n > 0 {
			return n, nil
		}
	}

	// Try br
	if _, err := exec.LookPath("br"); err == nil {
		n, err := s.importFromTool("br")
		if err == nil && n > 0 {
			return n, nil
		}
	}

	// Try .beads/issues.jsonl
	beadsFile := filePath
	if beadsFile == "" {
		beadsFile = ".beads/issues.jsonl"
	}
	return s.importFromJSONL(beadsFile)
}

func (s *Store) importFromTool(tool string) (int, error) {
	cmd := exec.Command(tool, "list", "--json")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("bead: %s list --json: %w", tool, err)
	}

	return s.mergeJSONL(string(output))
}

func (s *Store) importFromJSONL(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("bead: read %s: %w", path, err)
	}
	return s.mergeJSONL(string(data))
}

func (s *Store) mergeJSONL(data string) (int, error) {
	var incoming []Bead
	var parseErrors int

	// Try as JSON array first, then as JSONL
	trimmed := strings.TrimSpace(data)
	if strings.HasPrefix(trimmed, "[") {
		var raw []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return 0, fmt.Errorf("bead: import parse: %w", err)
		}
		for _, r := range raw {
			b, err := unmarshalBead(r)
			if err != nil {
				parseErrors++
				continue
			}
			incoming = append(incoming, b)
		}
	} else {
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			b, err := unmarshalBead([]byte(line))
			if err != nil {
				parseErrors++
				continue
			}
			incoming = append(incoming, b)
		}
	}

	if parseErrors > 0 && len(incoming) == 0 {
		return 0, fmt.Errorf("bead: import failed: %d malformed record(s), 0 valid", parseErrors)
	}
	if parseErrors > 0 {
		fmt.Fprintf(os.Stderr, "bead: import: skipped %d malformed record(s)\n", parseErrors)
	}

	if len(incoming) == 0 {
		return 0, nil
	}

	return s.mergeBeads(incoming)
}

func (s *Store) mergeBeads(incoming []Bead) (int, error) {
	var count int
	err := s.WithLock(func() error {
		existing, err := s.ReadAll()
		if err != nil {
			return err
		}

		existingIDs := make(map[string]bool)
		for _, b := range existing {
			existingIDs[b.ID] = true
		}

		// Collect IDs from incoming for cross-reference validation
		incomingIDs := make(map[string]bool)
		for _, b := range incoming {
			incomingIDs[b.ID] = true
		}

		for _, b := range incoming {
			if existingIDs[b.ID] {
				continue // skip duplicates
			}
			// Normalize: ensure valid status
			if b.Status != StatusOpen && b.Status != StatusInProgress && b.Status != StatusClosed {
				b.Status = StatusOpen
			}
			// Clamp priority
			if b.Priority < MinPriority {
				b.Priority = MinPriority
			}
			if b.Priority > MaxPriority {
				b.Priority = MaxPriority
			}
			// Validate deps exist in either existing or incoming set
			for _, dep := range b.Deps {
				if !existingIDs[dep] && !incomingIDs[dep] {
					fmt.Fprintf(os.Stderr, "bead: import: %s has dangling dep %s (skipped)\n", b.ID, dep)
				}
			}
			existing = append(existing, b)
			existingIDs[b.ID] = true
			count++
		}

		return s.WriteAll(existing)
	})
	return count, err
}

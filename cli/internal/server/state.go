package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// NodeState holds persistent identity for this ddx-server instance.
type NodeState struct {
	Name      string    `json:"name"`
	ID        string    `json:"id"`
	StartedAt time.Time `json:"started_at"`
	LastSeen  time.Time `json:"last_seen"`
}

// ProjectEntry represents a ddx project registered with this server.
type ProjectEntry struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	GitRemote    string    `json:"git_remote,omitempty"`
	RegisteredAt time.Time `json:"registered_at"`
	LastSeen     time.Time `json:"last_seen"`
}

// ServerState is the full persistent state for a ddx-server node.
// Designed so multiple nodes can eventually forward state to a coordinator.
type ServerState struct {
	SchemaVersion string         `json:"schema_version"`
	Node          NodeState      `json:"node"`
	Projects      []ProjectEntry `json:"projects"`

	mu  sync.RWMutex `json:"-"`
	dir string       `json:"-"`
}

const stateSchemaVersion = "1"

// loadServerState reads state from dir/server-state.json, initialising a fresh state
// if the file does not exist. nodeName is the resolved hostname/configured name.
func loadServerState(dir, nodeName string) *ServerState {
	s := &ServerState{
		SchemaVersion: stateSchemaVersion,
		dir:           dir,
		Node: NodeState{
			Name:      nodeName,
			ID:        nodeID(nodeName),
			StartedAt: time.Now().UTC(),
			LastSeen:  time.Now().UTC(),
		},
	}

	data, err := os.ReadFile(filepath.Join(dir, "server-state.json"))
	if err != nil {
		return s // fresh state
	}
	var persisted ServerState
	if err := json.Unmarshal(data, &persisted); err != nil {
		return s // corrupt — start fresh
	}
	// Preserve projects and node ID from prior run; update runtime fields.
	s.Node.ID = persisted.Node.ID
	s.Projects = persisted.Projects
	return s
}

func (s *ServerState) save() error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.Node.LastSeen = time.Now().UTC()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, "server-state.json"), data, 0600)
}

// RegisterProject adds or updates a project entry. Returns the entry.
func (s *ServerState) RegisterProject(path string) ProjectEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	id := projectID(path)
	name := filepath.Base(path)
	remote := resolveGitRemote(path)

	for i, p := range s.Projects {
		if p.ID == id {
			s.Projects[i].LastSeen = now
			s.Projects[i].Path = path // normalise in case of symlink change
			if remote != "" {
				s.Projects[i].GitRemote = remote
			}
			return s.Projects[i]
		}
	}

	entry := ProjectEntry{
		ID:           id,
		Name:         name,
		Path:         path,
		GitRemote:    remote,
		RegisteredAt: now,
		LastSeen:     now,
	}
	s.Projects = append(s.Projects, entry)
	return entry
}

// GetProjects returns a snapshot of all registered projects.
func (s *ServerState) GetProjects() []ProjectEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ProjectEntry, len(s.Projects))
	copy(out, s.Projects)
	return out
}

// nodeID produces a stable short ID from the node name.
func nodeID(name string) string {
	h := sha256.Sum256([]byte(name))
	return fmt.Sprintf("node-%x", h[:4])
}

// projectID produces a stable short ID from the canonical path.
func projectID(path string) string {
	h := sha256.Sum256([]byte(path))
	return fmt.Sprintf("proj-%x", h[:4])
}

// resolveGitRemote returns the origin URL for the repo at path, or "".
func resolveGitRemote(path string) string {
	data, err := runGitCapture(path, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return trimNL(string(data))
}

func trimNL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// runGitCapture runs a git command in dir and returns its stdout.
func runGitCapture(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...) //nolint:gosec
	cmd.Dir = dir
	return cmd.Output()
}

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
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Path         string     `json:"path"`
	GitRemote    string     `json:"git_remote,omitempty"`
	RegisteredAt time.Time  `json:"registered_at"`
	LastSeen     time.Time  `json:"last_seen"`
	Unreachable  bool       `json:"unreachable,omitempty"`
	TombstonedAt *time.Time `json:"tombstoned_at,omitempty"`
}

// ServerState is the full persistent state for a ddx-server node.
// Designed so multiple nodes can eventually forward state to a coordinator.
type ServerState struct {
	SchemaVersion string         `json:"schema_version"`
	Node          NodeState      `json:"node"`
	Projects      []ProjectEntry `json:"projects"`

	mu             sync.RWMutex         `json:"-"`
	dir            string               `json:"-"`
	workingDir     string               `json:"-"` // root working directory; set post-init by server
	coordinatorReg *coordinatorRegistry `json:"-"` // set by Server after workers are created
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

	// Migration: canonicalize paths, resolve linked worktrees, dedupe, sweep.
	s.migrate()
	return s
}

// migrate canonicalises all stored project paths, resolves linked-worktree
// paths to the primary worktree, deduplicates entries with the same canonical
// path, and marks entries whose paths no longer exist as unreachable. Entries
// that are unreachable and whose last_seen is older than 24 h are removed.
//
// This runs once on startup so a state file that accumulated thousands of
// phantom worktree paths is cleaned up automatically.
func (s *ServerState) migrate() {
	now := time.Now().UTC()
	cutoff := now.Add(-24 * time.Hour)

	// Pass 1: canonicalize paths (symlinks + abs). We do NOT run git commands
	// here — linked-worktree resolution happens at RegisterProject time.
	// Keeping migration git-free makes it safe to run against state files that
	// contain thousands of temp-dir paths, many of which may exist on disk.
	for i := range s.Projects {
		canonical := canonicalizePath(s.Projects[i].Path)
		if canonical != "" {
			s.Projects[i].Path = canonical
			s.Projects[i].Name = filepath.Base(canonical)
			s.Projects[i].ID = projectID(canonical)
		}
	}

	// Pass 2: dedupe by canonical path — keep earliest RegisteredAt, latest LastSeen.
	seen := make(map[string]int) // path → index in deduped
	projects := make([]ProjectEntry, 0, len(s.Projects))
	for _, p := range s.Projects {
		if idx, ok := seen[p.Path]; ok {
			// Merge: keep earliest RegisteredAt, latest LastSeen.
			if p.RegisteredAt.Before(projects[idx].RegisteredAt) {
				projects[idx].RegisteredAt = p.RegisteredAt
			}
			if p.LastSeen.After(projects[idx].LastSeen) {
				projects[idx].LastSeen = p.LastSeen
			}
			// Keep non-empty git remote.
			if p.GitRemote != "" && projects[idx].GitRemote == "" {
				projects[idx].GitRemote = p.GitRemote
			}
		} else {
			seen[p.Path] = len(projects)
			projects = append(projects, p)
		}
	}

	// Pass 3: reachability sweep — stat each path.
	kept := make([]ProjectEntry, 0, len(projects))
	for _, p := range projects {
		_, statErr := os.Stat(p.Path)
		if statErr == nil {
			// Path exists — clear any stale tombstone.
			p.Unreachable = false
			p.TombstonedAt = nil
			kept = append(kept, p)
			continue
		}
		// Path missing. Drop immediately if last seen > 24 h ago (old phantom).
		if p.LastSeen.Before(cutoff) {
			continue
		}
		// Recently registered but now missing — mark unreachable for GC later.
		if !p.Unreachable {
			p.Unreachable = true
			p.TombstonedAt = &now
		}
		kept = append(kept, p)
	}
	s.Projects = kept
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
// The path is canonicalized (symlinks resolved) and, if inside a linked git
// worktree, resolved to the primary worktree path before storage.
func (s *ServerState) RegisterProject(path string) ProjectEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	canonical := resolvedProjectPath(path)
	if canonical == "" {
		canonical = path
	}

	now := time.Now().UTC()
	id := projectID(canonical)
	name := filepath.Base(canonical)
	remote := resolveGitRemote(canonical)

	for i, p := range s.Projects {
		if p.ID == id {
			s.Projects[i].LastSeen = now
			s.Projects[i].Path = canonical // normalise in case of symlink change
			s.Projects[i].Name = name
			// Clear any stale tombstone on re-registration.
			s.Projects[i].Unreachable = false
			s.Projects[i].TombstonedAt = nil
			if remote != "" {
				s.Projects[i].GitRemote = remote
			}
			return s.Projects[i]
		}
	}

	entry := ProjectEntry{
		ID:           id,
		Name:         name,
		Path:         canonical,
		GitRemote:    remote,
		RegisteredAt: now,
		LastSeen:     now,
	}
	s.Projects = append(s.Projects, entry)
	return entry
}

// SweepProjects checks each registered project's path on disk.
// Paths that no longer exist are marked unreachable with a tombstone timestamp.
// Entries tombstoned for more than 24 h are removed.
// Returns the post-sweep project list (all entries, including unreachable).
func (s *ServerState) SweepProjects() []ProjectEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	cutoff := now.Add(-24 * time.Hour)

	kept := make([]ProjectEntry, 0, len(s.Projects))
	for _, p := range s.Projects {
		_, err := os.Stat(p.Path)
		if err == nil {
			// Path exists — clear any stale tombstone.
			p.Unreachable = false
			p.TombstonedAt = nil
			kept = append(kept, p)
			continue
		}
		// Path doesn't exist.
		if p.TombstonedAt != nil && p.TombstonedAt.Before(cutoff) {
			// Tombstoned >24 h ago — drop the entry.
			continue
		}
		if !p.Unreachable {
			// First time marking unreachable.
			p.Unreachable = true
			p.TombstonedAt = &now
		}
		kept = append(kept, p)
	}
	s.Projects = kept
	return append([]ProjectEntry(nil), s.Projects...)
}

// GetProjects returns a snapshot of registered projects.
// When includeUnreachable is false (the default for API callers), entries
// marked as unreachable are omitted.
func (s *ServerState) GetProjects(includeUnreachable ...bool) []ProjectEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	showAll := len(includeUnreachable) > 0 && includeUnreachable[0]
	out := make([]ProjectEntry, 0, len(s.Projects))
	for _, p := range s.Projects {
		if !showAll && p.Unreachable {
			continue
		}
		out = append(out, p)
	}
	return out
}

// GetProjectByID returns the project entry with the given ID, if any.
func (s *ServerState) GetProjectByID(id string) (ProjectEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.Projects {
		if p.ID == id {
			return p, true
		}
	}
	return ProjectEntry{}, false
}

// GetProjectByPath returns the project entry with the given path, if any.
func (s *ServerState) GetProjectByPath(path string) (ProjectEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.Projects {
		if p.Path == path {
			return p, true
		}
	}
	return ProjectEntry{}, false
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

// canonicalizePath returns the canonical absolute path, resolving symlinks.
// Falls back to filepath.Abs if symlinks cannot be resolved (e.g. path absent).
func canonicalizePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs
	}
	return resolved
}

// resolvedProjectPath canonicalizes path and, if it sits inside a linked git
// worktree, returns the primary worktree root instead. Returns "" only if
// filepath.Abs itself fails (practically impossible).
func resolvedProjectPath(path string) string {
	canonical := canonicalizePath(path)
	if canonical == "" {
		return path
	}

	// Detect linked worktree via git rev-parse.
	gitDirOut, err := runGitCapture(canonical, "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		return canonical
	}
	gitDir := trimNL(string(gitDirOut))

	commonDirOut, err := runGitCapture(canonical, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return canonical
	}
	commonDir := trimNL(string(commonDirOut))

	if gitDir == commonDir {
		// Not a linked worktree.
		return canonical
	}

	// In a linked worktree. Primary worktree is the parent of the shared .git dir.
	if filepath.Base(commonDir) == ".git" {
		return canonicalizePath(filepath.Dir(commonDir))
	}
	// Bare repo: no primary worktree — return canonical path.
	return canonical
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

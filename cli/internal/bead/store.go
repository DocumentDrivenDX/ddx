package bead

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store manages beads in a JSONL file.
type Store struct {
	Dir      string
	File     string
	Prefix   string
	LockDir  string
	LockWait time.Duration
}

// NewStore creates a store with the given directory.
// Defaults can be overridden via options or environment.
func NewStore(dir string) *Store {
	if dir == "" {
		dir = envOr("DDX_BEAD_DIR", ".ddx")
	}
	prefix := envOr("DDX_BEAD_PREFIX", DefaultPrefix)
	return &Store{
		Dir:      dir,
		File:     filepath.Join(dir, "beads.jsonl"),
		Prefix:   prefix,
		LockDir:  filepath.Join(dir, "beads.lock"),
		LockWait: 10 * time.Second,
	}
}

// Init creates the storage directory and file.
func (s *Store) Init() error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("bead: init dir: %w", err)
	}
	f, err := os.OpenFile(s.File, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("bead: init file: %w", err)
	}
	return f.Close()
}

// GenID generates a unique bead ID with the configured prefix.
func (s *Store) GenID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("bead: gen id: %w", err)
	}
	return fmt.Sprintf("%s-%s", s.Prefix, hex.EncodeToString(b)), nil
}

// ReadAll loads all beads from the JSONL file.
func (s *Store) ReadAll() ([]Bead, error) {
	data, err := os.ReadFile(s.File)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("bead: read: %w", err)
	}

	var beads []Bead
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		b, err := unmarshalBead([]byte(line))
		if err != nil {
			return nil, fmt.Errorf("bead: parse line: %w", err)
		}
		beads = append(beads, b)
	}
	return beads, nil
}

// WriteAll writes all beads to the JSONL file, sorted by ID.
func (s *Store) WriteAll(beads []Bead) error {
	sort.Slice(beads, func(i, j int) bool {
		return beads[i].ID < beads[j].ID
	})

	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("bead: mkdir: %w", err)
	}

	tmp := s.File + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("bead: create tmp: %w", err)
	}

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, b := range beads {
		data, err := marshalBead(b)
		if err != nil {
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("bead: marshal: %w", err)
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("bead: write: %w", err)
		}
		if _, err := f.WriteString("\n"); err != nil {
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("bead: write newline: %w", err)
		}
	}

	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("bead: close tmp: %w", err)
	}
	return os.Rename(tmp, s.File)
}

// Create adds a new bead. Validates and assigns defaults.
func (s *Store) Create(b *Bead) error {
	if err := s.validateCreate(b); err != nil {
		return err
	}

	now := time.Now().UTC()
	if b.ID == "" {
		id, err := s.GenID()
		if err != nil {
			return err
		}
		b.ID = id
	}
	if b.Type == "" {
		b.Type = DefaultType
	}
	if b.Status == "" {
		b.Status = DefaultStatus
	}
	if b.Priority == 0 && b.Extra == nil {
		b.Priority = DefaultPriority
	}
	if b.Labels == nil {
		b.Labels = []string{}
	}
	if b.Deps == nil {
		b.Deps = []string{}
	}
	b.Created = now
	b.Updated = now

	return s.WithLock(func() error {
		beads, err := s.ReadAll()
		if err != nil {
			return err
		}
		beads = append(beads, *b)
		return s.WriteAll(beads)
	})
}

// Get retrieves a bead by ID.
func (s *Store) Get(id string) (*Bead, error) {
	beads, err := s.ReadAll()
	if err != nil {
		return nil, err
	}
	for _, b := range beads {
		if b.ID == id {
			return &b, nil
		}
	}
	return nil, fmt.Errorf("bead: not found: %s", id)
}

// Update modifies a bead by ID. The mutate function receives a pointer to modify.
func (s *Store) Update(id string, mutate func(*Bead)) error {
	return s.WithLock(func() error {
		beads, err := s.ReadAll()
		if err != nil {
			return err
		}
		found := false
		for i := range beads {
			if beads[i].ID == id {
				mutate(&beads[i])
				beads[i].Updated = time.Now().UTC()
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("bead: not found: %s", id)
		}
		return s.WriteAll(beads)
	})
}

// Close sets a bead's status to closed.
func (s *Store) Close(id string) error {
	return s.Update(id, func(b *Bead) {
		b.Status = StatusClosed
	})
}

// List returns beads matching optional filters.
func (s *Store) List(status, label string) ([]Bead, error) {
	beads, err := s.ReadAll()
	if err != nil {
		return nil, err
	}
	var result []Bead
	for _, b := range beads {
		if status != "" && b.Status != status {
			continue
		}
		if label != "" && !containsString(b.Labels, label) {
			continue
		}
		result = append(result, b)
	}
	return result, nil
}

// Ready returns open beads whose dependencies are all closed.
func (s *Store) Ready() ([]Bead, error) {
	beads, err := s.ReadAll()
	if err != nil {
		return nil, err
	}
	statusMap := make(map[string]string)
	for _, b := range beads {
		statusMap[b.ID] = b.Status
	}

	var ready []Bead
	for _, b := range beads {
		if b.Status != StatusOpen {
			continue
		}
		allSatisfied := true
		for _, dep := range b.Deps {
			if statusMap[dep] != StatusClosed {
				allSatisfied = false
				break
			}
		}
		if allSatisfied {
			ready = append(ready, b)
		}
	}
	return ready, nil
}

// Blocked returns open beads with at least one unclosed dependency.
func (s *Store) Blocked() ([]Bead, error) {
	beads, err := s.ReadAll()
	if err != nil {
		return nil, err
	}
	statusMap := make(map[string]string)
	for _, b := range beads {
		statusMap[b.ID] = b.Status
	}

	var blocked []Bead
	for _, b := range beads {
		if b.Status != StatusOpen {
			continue
		}
		for _, dep := range b.Deps {
			if statusMap[dep] != StatusClosed {
				blocked = append(blocked, b)
				break
			}
		}
	}
	return blocked, nil
}

// Status returns aggregate counts.
func (s *Store) Status() (*StatusCounts, error) {
	beads, err := s.ReadAll()
	if err != nil {
		return nil, err
	}
	ready, err := s.Ready()
	if err != nil {
		return nil, err
	}
	blocked, err := s.Blocked()
	if err != nil {
		return nil, err
	}

	counts := &StatusCounts{Total: len(beads), Ready: len(ready), Blocked: len(blocked)}
	for _, b := range beads {
		switch b.Status {
		case StatusOpen:
			counts.Open++
		case StatusClosed:
			counts.Closed++
		}
	}
	return counts, nil
}

// DepAdd adds a dependency: id depends on depID.
func (s *Store) DepAdd(id, depID string) error {
	return s.WithLock(func() error {
		beads, err := s.ReadAll()
		if err != nil {
			return err
		}
		// Verify both exist
		var target *Bead
		depExists := false
		for i := range beads {
			if beads[i].ID == id {
				target = &beads[i]
			}
			if beads[i].ID == depID {
				depExists = true
			}
		}
		if target == nil {
			return fmt.Errorf("bead: not found: %s", id)
		}
		if !depExists {
			return fmt.Errorf("bead: dependency not found: %s", depID)
		}
		if id == depID {
			return fmt.Errorf("bead: cannot depend on self")
		}
		if containsString(target.Deps, depID) {
			return nil // already exists
		}
		target.Deps = append(target.Deps, depID)
		target.Updated = time.Now().UTC()
		return s.WriteAll(beads)
	})
}

// DepRemove removes a dependency.
func (s *Store) DepRemove(id, depID string) error {
	return s.Update(id, func(b *Bead) {
		var filtered []string
		for _, d := range b.Deps {
			if d != depID {
				filtered = append(filtered, d)
			}
		}
		if filtered == nil {
			filtered = []string{}
		}
		b.Deps = filtered
	})
}

// DepTree returns a text representation of the dependency tree.
func (s *Store) DepTree(rootID string) (string, error) {
	beads, err := s.ReadAll()
	if err != nil {
		return "", err
	}
	byID := make(map[string]*Bead)
	for i := range beads {
		byID[beads[i].ID] = &beads[i]
	}

	if rootID != "" {
		if _, ok := byID[rootID]; !ok {
			return "", fmt.Errorf("bead: not found: %s", rootID)
		}
		var sb strings.Builder
		s.printTree(&sb, byID, rootID, "", true)
		return sb.String(), nil
	}

	// Find roots (beads that have no dependencies themselves)
	var roots []string
	for _, b := range beads {
		if len(b.Deps) == 0 {
			roots = append(roots, b.ID)
		}
	}
	sort.Strings(roots)

	var sb strings.Builder
	for i, root := range roots {
		s.printTree(&sb, byID, root, "", i == len(roots)-1)
	}
	return sb.String(), nil
}

func (s *Store) printTree(sb *strings.Builder, byID map[string]*Bead, id, prefix string, last bool) {
	b, ok := byID[id]
	if !ok {
		return
	}

	connector := "├── "
	if last {
		connector = "└── "
	}
	if prefix == "" {
		connector = ""
	}

	fmt.Fprintf(sb, "%s%s%s [%s] %s\n", prefix, connector, b.ID, b.Status, b.Title)

	// Find beads that depend on this one (children in the tree)
	var children []string
	for _, other := range sortedKeys(byID) {
		if containsString(byID[other].Deps, id) {
			children = append(children, other)
		}
	}

	childPrefix := prefix
	if prefix != "" {
		if last {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	for i, child := range children {
		s.printTree(sb, byID, child, childPrefix, i == len(children)-1)
	}
}

// validateCreate checks base DDx validation rules.
func (s *Store) validateCreate(b *Bead) error {
	if strings.TrimSpace(b.Title) == "" {
		return fmt.Errorf("bead: title is required")
	}
	if b.Priority < MinPriority || b.Priority > MaxPriority {
		return fmt.Errorf("bead: priority must be %d-%d, got %d", MinPriority, MaxPriority, b.Priority)
	}
	if b.Status != "" && b.Status != StatusOpen && b.Status != StatusInProgress && b.Status != StatusClosed {
		return fmt.Errorf("bead: invalid status: %s", b.Status)
	}
	// Self-ref check
	for _, dep := range b.Deps {
		if dep == b.ID && b.ID != "" {
			return fmt.Errorf("bead: cannot depend on self")
		}
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]*Bead) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

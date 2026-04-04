package bead

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// Store manages beads via a pluggable backend.
type Store struct {
	Collection string
	Dir        string
	File       string
	Prefix     string
	LockDir    string
	LockWait   time.Duration
	backend    Backend // nil means use built-in JSONL
}

type StoreOption func(*Store)

// WithCollection selects the logical bead collection. The default collection
// remains "beads", which maps to "beads.jsonl" in the JSONL backend.
func WithCollection(name string) StoreOption {
	return func(s *Store) {
		if cleaned := normalizeCollection(name); cleaned != "" {
			s.Collection = cleaned
		}
	}
}

// NewStore creates a store with the given directory.
// Defaults can be overridden via options or environment.
func NewStore(dir string, opts ...StoreOption) *Store {
	if dir == "" {
		dir = envOr("DDX_BEAD_DIR", ".ddx")
	}
	prefix := envOr("DDX_BEAD_PREFIX", "")
	if prefix == "" {
		workingDir := dir
		if filepath.Base(dir) == ".ddx" {
			workingDir = filepath.Dir(dir)
		}
		if cfg, err := config.LoadWithWorkingDir(workingDir); err == nil && cfg != nil && cfg.Bead != nil && cfg.Bead.IDPrefix != "" {
			prefix = cfg.Bead.IDPrefix
		}
	}
	if prefix == "" {
		prefix = detectPrefix()
	}
	backendType := envOr("DDX_BEAD_BACKEND", BackendJSONL)

	s := &Store{
		Collection: DefaultCollection,
		Dir:        dir,
		Prefix:     prefix,
		LockWait:   parseDurationOr("DDX_BEAD_LOCK_TIMEOUT", 10*time.Second),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	s.File = filepath.Join(dir, s.Collection+".jsonl")
	s.LockDir = filepath.Join(dir, s.Collection+".lock")

	// Set up external backend if configured
	switch backendType {
	case BackendBD, BackendBR:
		if ext, err := NewExternalBackend(backendType, s.Collection); err == nil {
			s.backend = ext
		}
		// Fall through to JSONL if tool not available
	}

	return s
}

// NewStoreWithBackend creates a store with an explicit backend (for testing).
func NewStoreWithBackend(dir string, b Backend) *Store {
	s := NewStore(dir)
	s.backend = b
	return s
}

// NewStoreWithCollection creates a store for a named logical collection.
func NewStoreWithCollection(dir, collection string) *Store {
	return NewStore(dir, WithCollection(collection))
}

// Init creates the storage directory and file.
func (s *Store) Init() error {
	if s.backend != nil {
		return s.backend.Init()
	}
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

// ReadAll loads all beads from the configured backend.
func (s *Store) ReadAll() ([]Bead, error) {
	if s.backend != nil {
		return s.backend.ReadAll()
	}
	beads, warnings, err := s.readAllRaw()
	if err != nil {
		return nil, fmt.Errorf("bead: read %s: %w", s.File, err)
	}
	for _, warning := range warnings {
		fmt.Fprintln(os.Stderr, warning)
	}
	if len(warnings) > 0 && len(beads) > 0 {
		repaired, repairErr := s.repairJSONL()
		if repairErr != nil {
			return beads, fmt.Errorf("bead: repair %s: %w", s.File, repairErr)
		}
		if repaired {
			fmt.Fprintf(os.Stderr, "bead: repaired %s; backup written to %s.bak\n", s.File, s.File)
		}
	}
	if len(beads) == 0 && len(warnings) > 0 {
		return nil, fmt.Errorf("bead: read %s: %d malformed record(s), 0 valid", s.File, len(warnings))
	}
	return beads, nil
}

func (s *Store) readAllRaw() ([]Bead, []string, error) {
	data, err := os.ReadFile(s.File)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read: %w", err)
	}
	beads, warnings, err := parseBeadJSONL(data)
	if err != nil {
		return nil, nil, err
	}
	return beads, warnings, nil
}

func parseBeadJSONL(data []byte) ([]Bead, []string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var beads []Bead
	var warnings []string
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		b, err := unmarshalBead([]byte(line))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("bead: read line %d: %v", lineNo, err))
			continue
		}
		beads = append(beads, b)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan jsonl: %w", err)
	}
	return beads, warnings, nil
}

func (s *Store) repairJSONL() (bool, error) {
	var repaired bool
	err := s.WithLock(func() error {
		beads, warnings, err := s.readAllRaw()
		if err != nil {
			return err
		}
		if len(warnings) == 0 || len(beads) == 0 {
			return nil
		}
		backupPath := s.File + ".bak"
		backupData, err := os.ReadFile(s.File)
		if err != nil {
			return fmt.Errorf("read current file: %w", err)
		}
		if err := writeAtomicFile(backupPath, backupData); err != nil {
			return fmt.Errorf("write backup: %w", err)
		}
		if err := s.WriteAll(beads); err != nil {
			return fmt.Errorf("write repaired file: %w", err)
		}
		repaired = true
		return nil
	})
	return repaired, err
}

func writeAtomicFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func normalizeCollection(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return DefaultCollection
	}
	name = strings.TrimSuffix(name, ".jsonl")
	return name
}

// WriteAll writes all beads to the configured backend, sorted by ID.
func (s *Store) WriteAll(beads []Bead) error {
	sort.Slice(beads, func(i, j int) bool {
		return beads[i].ID < beads[j].ID
	})

	if s.backend != nil {
		return s.backend.WriteAll(beads)
	}

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

// Create adds a new bead. Assigns defaults, validates, then persists.
func (s *Store) Create(b *Bead) error {
	now := time.Now().UTC()
	if b.ID == "" {
		id, err := s.GenID()
		if err != nil {
			return err
		}
		b.ID = id
	}
	if b.IssueType == "" {
		b.IssueType = DefaultType
	}
	if b.Status == "" {
		b.Status = DefaultStatus
	}
	b.CreatedAt = now
	b.UpdatedAt = now

	// Validate after defaults are applied so hooks see final state
	if err := s.validateBead(b); err != nil {
		return err
	}
	// Run create hook
	if err := s.runHook("validate-bead-create", b); err != nil {
		return err
	}

	return s.WithLock(func() error {
		beads, _, err := s.readAllRaw()
		if err != nil {
			return err
		}
		// Reject duplicate IDs
		for _, e := range beads {
			if e.ID == b.ID {
				return fmt.Errorf("bead: duplicate id: %s", b.ID)
			}
		}
		// Validate deps reference existing beads
		depIDs := b.DepIDs()
		if len(depIDs) > 0 {
			existing := make(map[string]bool)
			for _, e := range beads {
				existing[e.ID] = true
			}
			for _, dep := range depIDs {
				if !existing[dep] {
					return fmt.Errorf("bead: dependency not found: %s", dep)
				}
			}
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
		beads, _, err := s.readAllRaw()
		if err != nil {
			return err
		}
		found := false
		for i := range beads {
			if beads[i].ID == id {
				mutate(&beads[i])
				beads[i].UpdatedAt = time.Now().UTC()
				// Core validation after mutation
				if err := s.validateBead(&beads[i]); err != nil {
					return err
				}
				// Run update hook
				if err := s.runHook("validate-bead-update", &beads[i]); err != nil {
					return err
				}
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

// Claim sets a bead to in_progress with claim metadata.
// Fails if the bead is already claimed (status == in_progress).
func (s *Store) Claim(id, assignee string) error {
	return s.Update(id, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Status = StatusInProgress
		b.Owner = assignee
		b.Extra["claimed-at"] = time.Now().UTC().Format(time.RFC3339)
		b.Extra["claimed-pid"] = fmt.Sprintf("%d", os.Getpid())
	})
}

// Unclaim resets a bead from in_progress back to open, clearing claim metadata.
func (s *Store) Unclaim(id string) error {
	return s.Update(id, func(b *Bead) {
		b.Status = StatusOpen
		b.Owner = ""
		if b.Extra != nil {
			delete(b.Extra, "claimed-at")
			delete(b.Extra, "claimed-pid")
		}
	})
}

// AppendEvent adds an immutable execution evidence entry to a bead.
func (s *Store) AppendEvent(id string, event BeadEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	return s.Update(id, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		var events []BeadEvent
		if raw, ok := b.Extra["events"]; ok {
			events = decodeBeadEvents(raw)
		}
		events = append(events, event)
		encoded := make([]map[string]any, 0, len(events))
		for _, e := range events {
			encoded = append(encoded, map[string]any{
				"kind":       e.Kind,
				"summary":    e.Summary,
				"body":       e.Body,
				"actor":      e.Actor,
				"created_at": e.CreatedAt,
				"source":     e.Source,
			})
		}
		b.Extra["events"] = encoded
	})
}

// Events returns the bead's evidence history in insertion order.
func (s *Store) Events(id string) ([]BeadEvent, error) {
	b, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	events := decodeBeadEvents(b.Extra["events"])
	if len(events) == 0 {
		return []BeadEvent{}, nil
	}
	out := make([]BeadEvent, len(events))
	copy(out, events)
	return out, nil
}

func decodeBeadEvents(raw any) []BeadEvent {
	switch v := raw.(type) {
	case []BeadEvent:
		out := make([]BeadEvent, len(v))
		copy(out, v)
		return out
	case []any:
		out := make([]BeadEvent, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, beadEventFromMap(m))
		}
		return out
	case []map[string]any:
		out := make([]BeadEvent, 0, len(v))
		for _, item := range v {
			out = append(out, beadEventFromMap(item))
		}
		return out
	default:
		return nil
	}
}

func beadEventFromMap(m map[string]any) BeadEvent {
	e := BeadEvent{}
	if v, ok := m["kind"].(string); ok {
		e.Kind = v
	}
	if v, ok := m["summary"].(string); ok {
		e.Summary = v
	}
	if v, ok := m["body"].(string); ok {
		e.Body = v
	}
	if v, ok := m["actor"].(string); ok {
		e.Actor = v
	}
	if v, ok := m["source"].(string); ok {
		e.Source = v
	}
	if v, ok := m["created_at"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			e.CreatedAt = parsed
		}
	}
	return e
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

// Ready returns open beads whose dependencies are all closed, sorted by
// priority (0 = highest first).
func (s *Store) Ready() ([]Bead, error) {
	return s.readyFiltered(false)
}

// ReadyExecution returns ready beads that are also execution-eligible and
// not superseded. This is the filter HELIX uses for its build loop.
func (s *Store) ReadyExecution() ([]Bead, error) {
	return s.readyFiltered(true)
}

func (s *Store) readyFiltered(executionOnly bool) ([]Bead, error) {
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
		for _, depID := range b.DepIDs() {
			if statusMap[depID] != StatusClosed {
				allSatisfied = false
				break
			}
		}
		if !allSatisfied {
			continue
		}
		if executionOnly {
			// Filter by execution-eligible (default true if absent)
			eligible, ok := b.Extra["execution-eligible"]
			if ok {
				if val, isBool := eligible.(bool); isBool && !val {
					continue
				}
			}
			// Filter out superseded beads
			if sup, ok := b.Extra["superseded-by"]; ok {
				if s, isStr := sup.(string); isStr && s != "" {
					continue
				}
			}
		}
		ready = append(ready, b)
	}

	// Sort by priority (0 = highest first)
	sort.Slice(ready, func(i, j int) bool {
		return ready[i].Priority < ready[j].Priority
	})

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
		for _, depID := range b.DepIDs() {
			if statusMap[depID] != StatusClosed {
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
		beads, _, err := s.readAllRaw()
		if err != nil {
			return err
		}
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
		if target.HasDep(depID) {
			return nil // already exists
		}

		// Check for circular dependency
		depMap := make(map[string][]string)
		for _, b := range beads {
			depMap[b.ID] = b.DepIDs()
		}
		depMap[id] = append(append([]string{}, target.DepIDs()...), depID)
		if hasCycle(depMap, id) {
			return fmt.Errorf("bead: circular dependency: %s -> %s", id, depID)
		}

		target.AddDep(depID, "blocks")
		target.UpdatedAt = time.Now().UTC()
		return s.WriteAll(beads)
	})
}

// DepRemove removes a dependency.
func (s *Store) DepRemove(id, depID string) error {
	return s.Update(id, func(b *Bead) {
		b.RemoveDep(depID)
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
		target, ok := byID[rootID]
		if !ok {
			return "", fmt.Errorf("bead: not found: %s", rootID)
		}
		var sb strings.Builder
		// Walk up: show the dependency chain (what this node depends on)
		depChain := s.depChainUp(byID, rootID)
		if len(depChain) > 0 {
			// Print deps as the tree root(s), with the target as their child
			for _, depID := range depChain {
				if dep, ok := byID[depID]; ok {
					fmt.Fprintf(&sb, "%s [%s] %s\n", dep.ID, dep.Status, dep.Title)
				}
			}
		}
		// Print the target node
		fmt.Fprintf(&sb, "%s [%s] %s\n", target.ID, target.Status, target.Title)
		// Print dependents (what depends on this node)
		var children []string
		for _, other := range sortedKeys(byID) {
			if byID[other].HasDep(rootID) {
				children = append(children, other)
			}
		}
		for _, child := range children {
			s.printTree(&sb, byID, child, "  ", true)
		}
		return sb.String(), nil
	}

	// Find roots (beads that have no dependencies themselves)
	var roots []string
	for _, b := range beads {
		if len(b.Dependencies) == 0 {
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
		if byID[other].HasDep(id) {
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

// depChainUp returns the direct dependencies of a bead (upstream IDs).
func (s *Store) depChainUp(byID map[string]*Bead, id string) []string {
	b, ok := byID[id]
	if !ok {
		return nil
	}
	return b.DepIDs()
}

// validateBead checks core invariants that must hold for any bead (create or update).
func (s *Store) validateBead(b *Bead) error {
	if strings.TrimSpace(b.Title) == "" {
		return fmt.Errorf("bead: title is required")
	}
	if b.Priority < MinPriority || b.Priority > MaxPriority {
		return fmt.Errorf("bead: priority must be %d-%d, got %d", MinPriority, MaxPriority, b.Priority)
	}
	if b.Status != StatusOpen && b.Status != StatusInProgress && b.Status != StatusClosed {
		return fmt.Errorf("bead: invalid status: %s", b.Status)
	}
	// Self-ref check
	for _, depID := range b.DepIDs() {
		if depID == b.ID && b.ID != "" {
			return fmt.Errorf("bead: cannot depend on self")
		}
	}
	return nil
}

// detectPrefix derives the bead ID prefix from the repository/directory name,
// following the bd convention (e.g., repo "my-project" → prefix "my-project").
// Falls back to DefaultPrefix if detection fails.
func detectPrefix() string {
	// Try git repo root name first
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	if out, err := cmd.Output(); err == nil {
		root := strings.TrimSpace(string(out))
		if root != "" {
			return filepath.Base(root)
		}
	}
	// Fall back to current directory name
	if wd, err := os.Getwd(); err == nil {
		return filepath.Base(wd)
	}
	return DefaultPrefix
}

func parseDurationOr(envKey string, fallback time.Duration) time.Duration {
	v := os.Getenv(envKey)
	if v == "" {
		return fallback
	}
	// Try as seconds (plain number)
	if secs, err := strconv.ParseFloat(v, 64); err == nil {
		return time.Duration(secs * float64(time.Second))
	}
	// Try as Go duration
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	return fallback
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

// hasCycle detects cycles in the dependency graph starting from startID.
func hasCycle(deps map[string][]string, startID string) bool {
	visited := make(map[string]bool)
	stack := make(map[string]bool)

	var visit func(string) bool
	visit = func(id string) bool {
		visited[id] = true
		stack[id] = true

		for _, dep := range deps[id] {
			if !visited[dep] {
				if visit(dep) {
					return true
				}
			} else if stack[dep] {
				return true
			}
		}

		stack[id] = false
		return false
	}

	return visit(startID)
}

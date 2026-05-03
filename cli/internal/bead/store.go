// Package bead implements the on-disk bead tracker.
//
// Size envelope (ddx-f8a11202):
//
// Individual bead fields — description, acceptance, notes, and each event's
// body — are bounded by MaxFieldBytes (65,535 bytes). This matches upstream
// bd's Dolt TEXT column limit so DDx-authored beads can always round-trip
// through `bd import`. Writers exceeding the cap are truncated with a
// `…[truncated, N bytes]` marker at AppendEvent; callers that need to
// preserve the full payload (notably reviewer streams) should persist to a
// sidecar artifact under `.ddx/executions/<id>/` and record a path
// reference in the event body.
//
// On the read side, scanners use a 16MB buffer — real-world incidents have
// produced 1.46MB bead rows and ~7MB session-log rows when writers bypassed
// the cap. 16MB comfortably fits a bead with dozens of maxed-out fields and
// matches the scanner in the agent package.
package bead

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
)

// Store manages beads via a pluggable backend.
type Store struct {
	Collection string
	Dir        string
	File       string
	Prefix     string
	LockDir    string
	LockWait   time.Duration
	backend    RawBackend // nil means use built-in JSONL
}

// Compile-time check: *Store satisfies the high-level Backend interface
// declared in backend.go. This is the contract chaos_test.go and (eventually)
// cli/cmd callers program against.
var _ Backend = (*Store)(nil)

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
	workingDir := dir
	if filepath.Base(dir) == ".ddx" {
		workingDir = filepath.Dir(dir)
	}
	prefix := envOr("DDX_BEAD_PREFIX", "")
	var configBackend string
	if cfg, err := config.LoadWithWorkingDir(workingDir); err == nil && cfg != nil && cfg.Bead != nil {
		if prefix == "" && cfg.Bead.IDPrefix != "" {
			prefix = cfg.Bead.IDPrefix
		}
		configBackend = cfg.Bead.Backend
	}
	if prefix == "" {
		prefix = detectPrefix(workingDir)
	}
	// Backend selection: env var wins, then config, then jsonl default.
	backendType := os.Getenv("DDX_BEAD_BACKEND")
	if backendType == "" {
		backendType = configBackend
	}
	if backendType == "" {
		backendType = BackendJSONL
	}

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
	spec := DefaultRegistry().Resolve(CollectionID(s.Collection))
	s.File, s.LockDir = spec.PathsUnder(dir)

	// Set up external backend if configured. bd/br lack per-collection
	// scoping in their CLI, so non-default collections (e.g. beads-archive)
	// route through a JSONL fallback the external backend transparently
	// delegates to. The default collection always goes straight to bd/br
	// to preserve the existing interchange contract (schema_compat_test.go,
	// import/export round-trip).
	switch backendType {
	case BackendBD, BackendBR:
		var fallback RawBackend
		if s.Collection != DefaultCollection {
			fallback = NewJSONLBackend(s.Dir, s.File, s.LockDir, s.LockWait)
		}
		if ext, err := NewExternalBackendWithFallback(backendType, s.Collection, fallback); err == nil {
			s.backend = ext
		}
		// Fall through to JSONL if tool not available
	case BackendAxon:
		// Per ddx-95ec5ed5 AC §3 the axon backend ships behind a feature
		// flag until ddx-743bc194 (chaos conformance) signs off. Routing
		// only happens when DDX_AXON_EXPERIMENTAL is truthy; otherwise we
		// warn once and fall through to JSONL so the workspace stays
		// usable on misconfiguration.
		if AxonExperimentalEnabled() {
			s.backend = NewAxonBackend(s.Dir, s.LockWait)
		} else {
			fmt.Fprintf(os.Stderr, "bead: backend=axon ignored: set %s=1 to opt in (axon backend is experimental)\n", AxonExperimentalEnv)
		}
	}

	return s
}

// NewStoreWithBackend creates a store with an explicit RawBackend (for testing).
func NewStoreWithBackend(dir string, b RawBackend) *Store {
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
	return s.ReadAllFiltered(nil)
}

// ReadAllFiltered loads all beads, folds them by latest-wins, and returns only
// those for which the predicate returns true. When the predicate is nil every
// bead is returned (equivalent to ReadAll). The predicate is applied at the
// per-entry parse boundary after fold — matched beads are appended directly to
// the return slice without first being held in an intermediate full-corpus
// list, so queries that match a small subset avoid materializing the
// mismatches (ddx-9ce6842a Part 2 step 2: filter pushdown).
func (s *Store) ReadAllFiltered(pred func(Bead) bool) ([]Bead, error) {
	if s.backend != nil {
		all, err := s.backend.ReadAll()
		if err != nil {
			return nil, err
		}
		if pred == nil {
			return all, nil
		}
		out := make([]Bead, 0, len(all))
		for _, b := range all {
			if pred(b) {
				out = append(out, b)
			}
		}
		return out, nil
	}
	beads, warnings, err := s.readAllRaw()
	if err != nil {
		return nil, fmt.Errorf("bead: read %s: %w", s.File, err)
	}
	beads = foldLatestBeads(beads)
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
	if pred == nil {
		return beads, nil
	}
	out := make([]Bead, 0, len(beads))
	for _, b := range beads {
		if pred(b) {
			out = append(out, b)
		}
	}
	return out, nil
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
	// 16MB max line. Real-world extremes observed: 1.46MB bead rows when a
	// reviewer stream dumped into events[].body, and bd-exported lines stacking
	// many 65KB fields. bd's upstream per-field cap is 65,535 bytes; an
	// individual bead with dozens of maxed-out fields fits comfortably in 16MB.
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

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
		beads = foldLatestBeads(beads)
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

func foldLatestBeads(beads []Bead) []Bead {
	if len(beads) == 0 {
		return nil
	}

	latest := make(map[string]Bead, len(beads))
	lastSeen := make(map[string]int, len(beads))
	for i, b := range beads {
		latest[b.ID] = b
		lastSeen[b.ID] = i
	}

	ids := make([]string, 0, len(latest))
	for id := range latest {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if lastSeen[ids[i]] != lastSeen[ids[j]] {
			return lastSeen[ids[i]] < lastSeen[ids[j]]
		}
		return ids[i] < ids[j]
	})

	out := make([]Bead, 0, len(ids))
	for _, id := range ids {
		out = append(out, latest[id])
	}
	return out
}

func (s *Store) readAllLatestRaw() ([]Bead, []string, error) {
	// When a non-JSONL backend is configured, the raw bead corpus lives
	// outside .ddx/<collection>.jsonl entirely (axon: two collections under
	// .ddx/axon/; bd/br: an external tool's store). Delegate the read so
	// every read-modify-write path inside Store stays consistent with the
	// backend's storage layout. Backends are responsible for returning
	// already-folded beads.
	if s.backend != nil {
		all, err := s.backend.ReadAll()
		if err != nil {
			return nil, nil, err
		}
		return all, nil, nil
	}
	beads, warnings, err := s.readAllRaw()
	if err != nil {
		return nil, nil, err
	}
	return foldLatestBeads(beads), warnings, nil
}

// tmpPath returns a unique temp file path in the same directory as path.
// Uses pid + 4 random bytes so concurrent processes don't collide.
func tmpPath(path string) (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.tmp-%d-%s", path, os.Getpid(), hex.EncodeToString(b)), nil
}

func writeAtomicFile(path string, data []byte) error {
	tmp, err := tmpPath(path)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
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

	tmp, err := tmpPath(s.File)
	if err != nil {
		return fmt.Errorf("bead: tmp name: %w", err)
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
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

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("bead: sync tmp: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("bead: close tmp: %w", err)
	}
	if err := os.Rename(tmp, s.File); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("bead: rename tmp: %w", err)
	}
	return nil
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
	// Story 15: an operator-prompt bead's execution may not create another
	// operator-prompt bead. The execute-bead harness exports the actor's
	// issue_type via DDX_ACTOR_ISSUE_TYPE; absent the env, the guard is a
	// no-op (e.g. direct CLI use by a human operator).
	if err := OperatorPromptMutationGuard(os.Getenv("DDX_ACTOR_ISSUE_TYPE"), b.IssueType); err != nil {
		return err
	}
	// Run create hook
	if err := s.runHook("validate-bead-create", b); err != nil {
		return err
	}

	return s.WithLock(func() error {
		beads, _, err := s.readAllLatestRaw()
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
		beads, _, err := s.readAllLatestRaw()
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

// HeartbeatInterval is how often a claim owner should refresh
// execute-loop-heartbeat-at while running. Exposed as a variable so tests
// can override it.
var HeartbeatInterval = 30 * time.Second

// HeartbeatTTL is the maximum allowed age of execute-loop-heartbeat-at
// before another worker may reclaim the bead. Defaults to 3× HeartbeatInterval.
// Exposed as a variable so tests can override it.
var HeartbeatTTL = 90 * time.Second

// Claim sets a bead to in_progress with claim metadata.
// Fails if the bead is already claimed (status == in_progress) with a
// fresh heartbeat. A bead whose execute-loop-heartbeat-at is older than
// HeartbeatTTL is considered stalled and will be reclaimed atomically.
func (s *Store) Claim(id, assignee string) error {
	return s.ClaimWithOptions(id, assignee, "", "")
}

// ClaimWithOptions sets a bead to in_progress with extended claim metadata.
// session and worktree are optional; machine is derived from os.Hostname().
// A stalled in_progress bead (heartbeat older than HeartbeatTTL) is reclaimed
// atomically under the store's lock.
func (s *Store) ClaimWithOptions(id, assignee, session, worktree string) error {
	machine, _ := os.Hostname()
	if envID := os.Getenv("DDX_MACHINE_ID"); envID != "" {
		machine = envID
	}
	return s.WithLock(func() error {
		beads, _, err := s.readAllLatestRaw()
		if err != nil {
			return err
		}
		for i := range beads {
			if beads[i].ID != id {
				continue
			}
			switch beads[i].Status {
			case StatusOpen:
				// normal claim path
			case StatusInProgress:
				if !heartbeatIsStale(beads[i].Extra) {
					return fmt.Errorf("bead: cannot claim %s from status %s", id, beads[i].Status)
				}
				// stalled claim — reclaim atomically below
			default:
				return fmt.Errorf("bead: cannot claim %s from status %s", id, beads[i].Status)
			}
			// Refuse claims on beads parked with last_status=push_failed:
			// the previous attempt landed locally but `git push` was rejected,
			// so the local commit history exists only on this machine. Re-running
			// primary work would duplicate or strand commits. The operator must
			// clear execute-loop-last-status (e.g. by manually pushing or
			// resetting) before another attempt is allowed.
			if last, ok := beads[i].Extra["execute-loop-last-status"].(string); ok && last == "push_failed" {
				detail, _ := beads[i].Extra["execute-loop-last-detail"].(string)
				if detail != "" {
					return fmt.Errorf("bead: refusing to claim %s — previous attempt landed locally but push failed (%s); clear execute-loop-last-status to retry", id, detail)
				}
				return fmt.Errorf("bead: refusing to claim %s — previous attempt landed locally but push failed; clear execute-loop-last-status to retry", id)
			}
			if beads[i].Extra == nil {
				beads[i].Extra = make(map[string]any)
			}
			now := time.Now().UTC()
			beads[i].Status = StatusInProgress
			beads[i].Owner = assignee
			beads[i].UpdatedAt = now
			beads[i].Extra["claimed-at"] = now.Format(time.RFC3339)
			beads[i].Extra["claimed-pid"] = fmt.Sprintf("%d", os.Getpid())
			beads[i].Extra["execute-loop-heartbeat-at"] = now.Format(time.RFC3339Nano)
			if machine != "" {
				beads[i].Extra["claimed-machine"] = machine
			}
			if session != "" {
				beads[i].Extra["claimed-session"] = session
			}
			if worktree != "" {
				beads[i].Extra["claimed-worktree"] = worktree
			}
			if err := s.validateBead(&beads[i]); err != nil {
				return err
			}
			if err := s.runHook("validate-bead-update", &beads[i]); err != nil {
				return err
			}
			return s.WriteAll(beads)
		}
		return fmt.Errorf("bead: not found: %s", id)
	})
}

// Heartbeat refreshes execute-loop-heartbeat-at on a claimed bead. Returns
// an error if the bead is no longer in_progress (e.g., reclaimed by another
// worker), allowing the caller to stop its heartbeat loop.
func (s *Store) Heartbeat(id string) error {
	return s.WithLock(func() error {
		beads, _, err := s.readAllLatestRaw()
		if err != nil {
			return err
		}
		for i := range beads {
			if beads[i].ID != id {
				continue
			}
			if beads[i].Status != StatusInProgress {
				return fmt.Errorf("bead: cannot heartbeat %s from status %s", id, beads[i].Status)
			}
			if beads[i].Extra == nil {
				beads[i].Extra = make(map[string]any)
			}
			beads[i].Extra["execute-loop-heartbeat-at"] = time.Now().UTC().Format(time.RFC3339Nano)
			beads[i].UpdatedAt = time.Now().UTC()
			return s.WriteAll(beads)
		}
		return fmt.Errorf("bead: not found: %s", id)
	})
}

// heartbeatIsStale returns true if the given bead's Extra map has no
// execute-loop-heartbeat-at or one older than HeartbeatTTL.
func heartbeatIsStale(extra map[string]any) bool {
	if extra == nil {
		return true
	}
	raw, ok := extra["execute-loop-heartbeat-at"]
	if !ok {
		return true
	}
	s, ok := raw.(string)
	if !ok || s == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		// Fall back to RFC3339 for compatibility with legacy entries written
		// before sub-second resolution. RFC3339Nano is the current canonical
		// format used by both ClaimWithOptions and Heartbeat write sites.
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return true
		}
	}
	return time.Since(t) > HeartbeatTTL
}

// Unclaim clears claim metadata. Only reverts status to open if the bead
// is currently in_progress — a closed bead stays closed.
func (s *Store) Unclaim(id string) error {
	return s.Update(id, func(b *Bead) {
		if b.Status == StatusInProgress {
			b.Status = StatusOpen
		}
		b.Owner = ""
		if b.Extra != nil {
			delete(b.Extra, "claimed-at")
			delete(b.Extra, "claimed-pid")
			delete(b.Extra, "claimed-machine")
			delete(b.Extra, "claimed-session")
			delete(b.Extra, "claimed-worktree")
			delete(b.Extra, "execute-loop-heartbeat-at")
		}
	})
}

func (s *Store) SetExecutionCooldown(id string, until time.Time, status, detail string) error {
	return s.Update(id, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra["execute-loop-retry-after"] = until.UTC().Format(time.RFC3339)
		if status != "" {
			b.Extra["execute-loop-last-status"] = status
		}
		if detail != "" {
			b.Extra["execute-loop-last-detail"] = detail
		}
	})
}

// IncrNoChangesCount increments the execute-loop no-changes counter for a bead
// and returns the new count. Used by the execute-bead worker to detect when a
// bead should be auto-closed after repeated no-change attempts.
func (s *Store) IncrNoChangesCount(id string) (int, error) {
	var newCount int
	err := s.Update(id, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		var count int
		if v, ok := b.Extra["execute-loop-no-changes-count"]; ok {
			switch n := v.(type) {
			case int:
				count = n
			case float64:
				count = int(n)
			case int64:
				count = int(n)
			}
		}
		count++
		b.Extra["execute-loop-no-changes-count"] = count
		newCount = count
	})
	return newCount, err
}

// MaxFieldBytes is the per-field hard cap on bead event bodies and adjacent
// writer paths. 65,535 bytes matches upstream bd's Dolt TEXT column size so
// DDx-authored beads always round-trip through `bd import`. Empirically
// validated against bd 1.0.2 on 2026-04-20: 65,535 accepts, 65,536 rejects.
const MaxFieldBytes = 65535

// capFieldBytes enforces MaxFieldBytes on a single field value. Callers that
// need to preserve the full payload should persist it to a sidecar artifact
// and store a path reference in the field; this function is the defense-in-
// depth cap for code paths that don't. Returns the input unchanged when it
// fits; otherwise returns head+tail truncation with a byte-count marker.
func capFieldBytes(s string) string {
	if len(s) <= MaxFieldBytes {
		return s
	}
	// Reserve a marker that always fits; keep head heavy (2/3) so
	// human-readable context is preserved for the common short-rationale case.
	marker := fmt.Sprintf("\n…[truncated, %d bytes]\n", len(s))
	budget := MaxFieldBytes - len(marker)
	if budget <= 0 {
		return s[:MaxFieldBytes]
	}
	head := (budget * 2) / 3
	tail := budget - head
	return s[:head] + marker + s[len(s)-tail:]
}

// Cancel-marker keys on Bead.Extra. ADR-022 §Cancel SLA: an operator-initiated
// cancel writes cancel-requested:true; the worker that honors the cancel writes
// cancel-honored:true alongside it.
const (
	ExtraCancelRequested = "cancel-requested"
	ExtraCancelHonored   = "cancel-honored"
)

// RequestCancel writes Extra[cancel-requested]=true on the bead. Idempotent:
// if cancel-honored is already set the call is a silent no-op (the prior cancel
// was already consumed). Returns true when the marker is now set on the bead
// (either by this call or a prior one).
func (s *Store) RequestCancel(id string) (bool, error) {
	var set bool
	err := s.Update(id, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		if isExtraTrue(b.Extra[ExtraCancelHonored]) {
			set = isExtraTrue(b.Extra[ExtraCancelRequested])
			return
		}
		b.Extra[ExtraCancelRequested] = true
		set = true
	})
	return set, err
}

// IsCancelRequested reports whether the bead carries an unconsumed cancel
// marker (cancel-requested:true and cancel-honored not yet set).
func (s *Store) IsCancelRequested(id string) (bool, error) {
	b, err := s.Get(id)
	if err != nil {
		return false, err
	}
	if b == nil || b.Extra == nil {
		return false, nil
	}
	if isExtraTrue(b.Extra[ExtraCancelHonored]) {
		return false, nil
	}
	return isExtraTrue(b.Extra[ExtraCancelRequested]), nil
}

// MarkCancelHonored sets Extra[cancel-honored]=true. Called by the worker once
// it has aborted at the next safe point in response to a cancel request.
func (s *Store) MarkCancelHonored(id string) error {
	return s.Update(id, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra[ExtraCancelHonored] = true
	})
}

func isExtraTrue(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true"
	}
	return false
}

func (s *Store) AppendEvent(id string, event BeadEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	// Defense in depth: cap every event body regardless of caller. The
	// reviewer write path separately persists the full stream to an artifact
	// and emits a <=512-byte body; this cap catches anything else.
	event.Body = capFieldBytes(event.Body)
	event.Summary = capFieldBytes(event.Summary)
	// Two storage shapes are possible here: inline Extra["events"] for active
	// beads, and a sidecar attachment for closed/archived beads (ADR-004
	// attachment model). When the bead has been externalized we append to the
	// sidecar and leave the row untouched aside from updated_at.
	var appendedToAttachment bool
	if err := s.Update(id, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		if hasEventsAttachment(b) {
			appendedToAttachment = true
			return
		}
		var events []BeadEvent
		if raw, ok := b.Extra["events"]; ok {
			events = decodeBeadEvents(raw)
		}
		events = append(events, event)
		b.Extra["events"] = encodeEventsForExtra(events)
	}); err != nil {
		return err
	}
	if !appendedToAttachment {
		return nil
	}
	// Append to the sidecar under the store lock so concurrent readers and
	// writers see a consistent events log.
	return s.WithLock(func() error {
		existing, err := s.readEventsAttachment(id)
		if err != nil {
			return err
		}
		existing = append(existing, event)
		return s.writeEventsAttachment(id, existing)
	})
}

// Events returns the bead's evidence history in insertion order. Reads are
// transparent across storage shapes: inline Extra["events"] for active beads,
// or the externalized sidecar referenced by Extra[events_attachment] once the
// bead has been closed.
func (s *Store) Events(id string) ([]BeadEvent, error) {
	b, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	events, err := s.eventsForBead(b)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return []BeadEvent{}, nil
	}
	out := make([]BeadEvent, len(events))
	copy(out, events)
	return out, nil
}

// EventsByKind returns all events for a bead filtered by kind, in insertion order.
func (s *Store) EventsByKind(id, kind string) ([]BeadEvent, error) {
	all, err := s.Events(id)
	if err != nil {
		return nil, err
	}
	out := make([]BeadEvent, 0, len(all))
	for _, e := range all {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
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

// Close sets a bead's status to closed. When the close succeeds and the bead
// carries an inline event history, those events are moved to a sidecar
// attachment under .ddx/attachments/<id>/events.jsonl so the active row stays
// small (per ADR-004's attachment model and TD-027 §c).
func (s *Store) Close(id string) error {
	if err := s.Update(id, func(b *Bead) {
		b.Status = StatusClosed
	}); err != nil {
		return err
	}
	if err := s.externalizeEvents(id); err != nil {
		return err
	}
	s.maybeOpportunisticArchive()
	return nil
}

// externalizeEvents moves a bead's inline events into its sidecar attachment
// under a fresh lock. Safe to call repeatedly — no-op when there are no
// inline events (already externalized, or none recorded).
func (s *Store) externalizeEvents(id string) error {
	return s.WithLock(func() error {
		beads, _, err := s.readAllLatestRaw()
		if err != nil {
			return err
		}
		for i := range beads {
			if beads[i].ID != id {
				continue
			}
			if err := s.externalizeEventsInPlace(&beads[i]); err != nil {
				return err
			}
			return s.WriteAll(beads)
		}
		return nil
	})
}

// ErrClosureGateRejected indicates a close was refused because the bead does
// not carry the evidence required for an automated closure: a terminal
// verdict event (review APPROVE with non-empty rationale, or an explicit
// review-skipped / manual-close marker) AND an execution-evidence marker
// (closing_commit_sha, session_id, or an execute-bead success event in the
// events history).
//
// Automated execute-loop paths (execute-bead + reviewer) always go through
// CloseWithEvidence. The plain Store.Close remains as the manual
// administration escape hatch — it bypasses the gate by design (ddx-e30e60a9
// acceptance §1).
var ErrClosureGateRejected = fmt.Errorf("closure gate: insufficient evidence")

// ClosureGate inspects a bead and returns nil iff the close is safe. It
// rejects only the specific shapes that caused the 2026-04-18/20
// review-malfunction incidents:
//
//  1. axon-c5cc071a (silent false-closure): closed with no events AND no
//     closing_commit_sha. Rejected by the execution-evidence check —
//     closing_commit_sha must be non-empty OR at least one event must exist.
//  2. f7ae036f (broken APPROVE): a review event with summary=APPROVE whose
//     body is empty. Rejected — APPROVE with no rationale is exactly the
//     parse-mis-extract shape the reviewer bug produced.
//
// Beads without a review step (--no-review, no Reviewer configured, already-
// satisfied paths) pass the gate as long as they carry execution evidence.
// This keeps the surface small: the gate catches known-bad shapes, not every
// conceivable edge case. Additional invariants belong in future dedicated
// beads so the rejection reason stays auditable.
func ClosureGate(b *Bead) error {
	if b == nil {
		return fmt.Errorf("%w: nil bead", ErrClosureGateRejected)
	}
	hasClosingCommit := false
	if sha, ok := b.Extra["closing_commit_sha"].(string); ok && strings.TrimSpace(sha) != "" {
		hasClosingCommit = true
	}
	events := decodeBeadEvents(b.Extra["events"])
	// Externalized events still count as evidence for the gate. We only need
	// to know whether *some* event history exists, not iterate it; the
	// gate's APPROVE-with-empty-rationale check below relies on inline events
	// (the verdict is recorded before externalization).
	hasAttachedEvents := hasEventsAttachment(b)
	// Reject the axon-c5cc071a shape: no events AND no closing commit.
	if len(events) == 0 && !hasAttachedEvents && !hasClosingCommit {
		return fmt.Errorf("%w: no execution evidence (empty events and no closing_commit_sha)", ErrClosureGateRejected)
	}
	// Reject the f7ae036f shape: an APPROVE verdict with empty rationale.
	for _, e := range events {
		if e.Kind == "review" && strings.EqualFold(strings.TrimSpace(e.Summary), "APPROVE") {
			if strings.TrimSpace(e.Body) == "" {
				return fmt.Errorf("%w: review APPROVE with empty rationale (malformed reviewer verdict)", ErrClosureGateRejected)
			}
		}
	}
	return nil
}

// CloseWithEvidence sets a bead's status to closed and records agent session evidence.
// sessionID is the agent session that completed the work.
// commitSHA is the exact closing commit SHA when it is known.
//
// Enforces ClosureGate (ddx-e30e60a9): a bead cannot transition to closed
// via this path without a terminal verdict event plus execution evidence.
// Store.Close is the manual-administration escape hatch when the gate is
// inappropriate.
func (s *Store) CloseWithEvidence(id string, sessionID string, commitSHA string) error {
	if err := s.closeWithEvidence(id, sessionID, commitSHA); err != nil {
		return err
	}
	// Externalize events only when the gate actually transitioned the bead
	// to closed; rejected closes leave the bead open and inline.
	if b, err := s.Get(id); err == nil && b != nil && b.Status == StatusClosed {
		if err := s.externalizeEvents(id); err != nil {
			return err
		}
	}
	s.maybeOpportunisticArchive()
	return nil
}

func (s *Store) closeWithEvidence(id string, sessionID string, commitSHA string) error {
	return s.Update(id, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		if sessionID != "" {
			b.Extra["session_id"] = sessionID
		}
		if commitSHA != "" {
			b.Extra["closing_commit_sha"] = commitSHA
		}
		if err := ClosureGate(b); err != nil {
			// Surface via bead notes so a later operator audit can see why the
			// close was refused; a single error path would be dropped by the
			// Update callback signature (no error return).
			appendClosureRejectNote(b, err)
			return
		}
		b.Status = StatusClosed
	})
}

func appendClosureRejectNote(b *Bead, err error) {
	stamp := time.Now().UTC().Format(time.RFC3339)
	note := fmt.Sprintf("[%s] closure rejected: %v", stamp, err)
	if b.Notes == "" {
		b.Notes = note
		return
	}
	b.Notes = b.Notes + "\n" + note
}

// AppendNotes appends operator-facing notes to an existing bead.
func (s *Store) AppendNotes(id string, appendNotes string) error {
	appendNotes = strings.TrimSpace(appendNotes)
	if appendNotes == "" {
		return nil
	}
	return s.Update(id, func(b *Bead) {
		if b.Notes != "" {
			b.Notes = b.Notes + "\n\n" + appendNotes
			return
		}
		b.Notes = appendNotes
	})
}

// Reopen sets a bead's status back to open, clears claim fields, optionally
// appends notes, and records an immutable reopen event — all in one atomic
// lock acquisition.
func (s *Store) Reopen(id string, reason string, appendNotes string) error {
	now := time.Now().UTC()
	return s.WithLock(func() error {
		beads, _, err := s.readAllLatestRaw()
		if err != nil {
			return err
		}
		found := false
		for i := range beads {
			if beads[i].ID != id {
				continue
			}
			b := &beads[i]
			b.Status = StatusOpen
			b.Owner = ""
			b.UpdatedAt = now
			if b.Extra == nil {
				b.Extra = make(map[string]any)
			}
			// Clear claim fields
			delete(b.Extra, "claimed-at")
			delete(b.Extra, "claimed-pid")
			delete(b.Extra, "claimed-machine")
			delete(b.Extra, "claimed-session")
			delete(b.Extra, "claimed-worktree")
			delete(b.Extra, "execute-loop-heartbeat-at")
			// Append notes
			if appendNotes != "" {
				if b.Notes != "" {
					b.Notes = b.Notes + "\n\n" + appendNotes
				} else {
					b.Notes = appendNotes
				}
			}
			// Record reopen event. If events were externalized at close,
			// pull them back inline so the active row carries the full
			// history again and the attachment ref is dropped.
			if hasEventsAttachment(b) {
				if err := s.inlineEventsInPlace(b); err != nil {
					return err
				}
				_ = os.Remove(s.eventsAttachmentPath(b.ID))
			}
			var events []BeadEvent
			if raw, ok := b.Extra["events"]; ok {
				events = decodeBeadEvents(raw)
			}
			evt := BeadEvent{
				Kind:      "reopen",
				CreatedAt: now,
			}
			if reason != "" {
				evt.Summary = reason
			}
			events = append(events, evt)
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
			if err := s.validateBead(b); err != nil {
				return err
			}
			if err := s.runHook("validate-bead-update", b); err != nil {
				return err
			}
			found = true
			break
		}
		if !found {
			return fmt.Errorf("bead: not found: %s", id)
		}
		return s.WriteAll(beads)
	})
}

// detectCurrentCommit returns the current git HEAD SHA, or empty if not in a git repo.
func (s *Store) detectCurrentCommit() string {
	out, err := gitpkg.Command(context.Background(), s.Dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// List returns beads matching optional filters.
// where is an optional map of key=value predicates that match against
// known struct fields and Extra (unknown/workflow-specific) fields.
func (s *Store) List(status, label string, where map[string]string) ([]Bead, error) {
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
		if !matchesWhere(b, where) {
			continue
		}
		result = append(result, b)
	}
	return result, nil
}

// matchesWhere returns true if the bead satisfies all key=value predicates.
// It checks known struct fields first, then falls back to Extra.
func matchesWhere(b Bead, where map[string]string) bool {
	for k, v := range where {
		var actual string
		switch k {
		case "id":
			actual = b.ID
		case "title":
			actual = b.Title
		case "status":
			actual = b.Status
		case "issue_type":
			actual = b.IssueType
		case "owner":
			actual = b.Owner
		case "assignee":
			actual = b.Owner
		case "parent":
			actual = b.Parent
		case "description":
			actual = b.Description
		case "acceptance":
			actual = b.Acceptance
		case "notes":
			actual = b.Notes
		default:
			// Fall back to Extra map for unknown/workflow-specific fields
			if b.Extra != nil {
				if ev, ok := b.Extra[k]; ok {
					actual = fmt.Sprintf("%v", ev)
				}
			}
		}
		if actual != v {
			return false
		}
	}
	return true
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

// ReadyExecutionBreakdown classifies dependency-ready beads by the reason
// they are NOT execution-eligible: epic containers, retry cooldown,
// execution-eligible=false, or superseded. It's the diagnostic the work loop
// emits when the execution queue is empty but `ddx bead ready` is not.
type ReadyExecutionBreakdown struct {
	SkippedEpics       []string
	SkippedOnCooldown  []string
	SkippedNotEligible []string
	SkippedSuperseded  []string
	NextRetryAfter     string
}

func (s *Store) ReadyExecutionBreakdown() (ReadyExecutionBreakdown, error) {
	out := ReadyExecutionBreakdown{}
	ready, err := s.Ready()
	if err != nil {
		return out, err
	}
	now := time.Now().UTC()
	var soonestRetry time.Time
	for _, b := range ready {
		if retryAfterRaw, ok := b.Extra["execute-loop-retry-after"]; ok {
			if retryAfterStr, isStr := retryAfterRaw.(string); isStr && retryAfterStr != "" {
				if retryAfter, err := time.Parse(time.RFC3339, retryAfterStr); err == nil && retryAfter.After(now) {
					out.SkippedOnCooldown = append(out.SkippedOnCooldown, b.ID)
					if soonestRetry.IsZero() || retryAfter.Before(soonestRetry) {
						soonestRetry = retryAfter
					}
					continue
				}
			}
		}
		if eligible, ok := b.Extra["execution-eligible"]; ok {
			if val, isBool := eligible.(bool); isBool && !val {
				out.SkippedNotEligible = append(out.SkippedNotEligible, b.ID)
				continue
			}
		}
		if sup, ok := b.Extra["superseded-by"]; ok {
			if s, isStr := sup.(string); isStr && s != "" {
				out.SkippedSuperseded = append(out.SkippedSuperseded, b.ID)
				continue
			}
		}
	}
	if !soonestRetry.IsZero() {
		out.NextRetryAfter = soonestRetry.Format(time.RFC3339)
	}
	return out, nil
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
			// Surface stalled in_progress beads so a fresh worker can
			// reclaim them atomically in Claim().
			if executionOnly && b.Status == StatusInProgress && heartbeatIsStale(b.Extra) {
				// fall through to dependency check below
			} else {
				continue
			}
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
			if retryAfterRaw, ok := b.Extra["execute-loop-retry-after"]; ok {
				if retryAfterStr, isStr := retryAfterRaw.(string); isStr && retryAfterStr != "" {
					if retryAfter, err := time.Parse(time.RFC3339, retryAfterStr); err == nil && retryAfter.After(time.Now().UTC()) {
						continue
					}
				}
			}
		}
		ready = append(ready, b)
	}

	sortBeadsForQueue(ready)

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
	sortBeadsForQueue(blocked)
	return blocked, nil
}

// BlockedAll returns open beads that are currently not runnable, classified
// by blocker kind. Dependency-blocked beads are emitted first (any unclosed
// dep in their DAG); retry-parked beads whose execute-loop-retry-after is in
// the future are emitted with blocker kind BlockerKindRetryCooldown. A bead
// that is both dep-blocked and cooldown-parked is reported as dependency-
// blocked, because deps are the stronger blocker.
func (s *Store) BlockedAll() ([]BlockedBead, error) {
	beads, err := s.ReadAll()
	if err != nil {
		return nil, err
	}
	statusMap := make(map[string]string)
	for _, b := range beads {
		statusMap[b.ID] = b.Status
	}

	now := time.Now().UTC()
	var entries []BlockedBead
	for _, b := range beads {
		if b.Status != StatusOpen {
			continue
		}

		var unclosed []string
		for _, depID := range b.DepIDs() {
			if statusMap[depID] != StatusClosed {
				unclosed = append(unclosed, depID)
			}
		}
		if len(unclosed) > 0 {
			entries = append(entries, BlockedBead{
				Bead: b,
				Blocker: Blocker{
					Kind:           BlockerKindDependency,
					UnclosedDepIDs: unclosed,
				},
			})
			continue
		}

		retryAfterRaw, ok := b.Extra["execute-loop-retry-after"]
		if !ok {
			continue
		}
		retryAfterStr, isStr := retryAfterRaw.(string)
		if !isStr || retryAfterStr == "" {
			continue
		}
		retryAfter, err := time.Parse(time.RFC3339, retryAfterStr)
		if err != nil || !retryAfter.After(now) {
			continue
		}
		blocker := Blocker{
			Kind:           BlockerKindRetryCooldown,
			NextEligibleAt: retryAfter.UTC().Format(time.RFC3339),
		}
		if v, ok := b.Extra["execute-loop-last-status"]; ok {
			if s, ok := v.(string); ok {
				blocker.LastStatus = s
			}
		}
		if v, ok := b.Extra["execute-loop-last-detail"]; ok {
			if s, ok := v.(string); ok {
				blocker.LastDetail = s
			}
		}
		entries = append(entries, BlockedBead{
			Bead:    b,
			Blocker: blocker,
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Priority != entries[j].Priority {
			return entries[i].Priority < entries[j].Priority
		}
		if !entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
			return entries[i].CreatedAt.Before(entries[j].CreatedAt)
		}
		return entries[i].ID < entries[j].ID
	})
	return entries, nil
}

func sortBeadsForQueue(beads []Bead) {
	sort.SliceStable(beads, func(i, j int) bool {
		if beads[i].Priority != beads[j].Priority {
			return beads[i].Priority < beads[j].Priority
		}
		if !beads[i].CreatedAt.Equal(beads[j].CreatedAt) {
			return beads[i].CreatedAt.Before(beads[j].CreatedAt)
		}
		return beads[i].ID < beads[j].ID
	})
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

	// Pull in archive partner so totals survive `bead migrate` — the archive
	// only carries closed beads, so Ready/Blocked aren't affected.
	seen := make(map[string]bool, len(beads))
	all := make([]Bead, 0, len(beads))
	for _, b := range beads {
		if seen[b.ID] {
			continue
		}
		seen[b.ID] = true
		all = append(all, b)
	}
	if s.Collection == DefaultCollection {
		archive := s.archivePartner()
		if archived, aerr := archive.ReadAll(); aerr == nil {
			for _, b := range archived {
				if seen[b.ID] {
					continue
				}
				seen[b.ID] = true
				all = append(all, b)
			}
		}
	}

	counts := &StatusCounts{Total: len(all), Ready: len(ready), Blocked: len(blocked)}
	for _, b := range all {
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
		beads, _, err := s.readAllLatestRaw()
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
	// Pull in archive partner so the tree survives `bead migrate` — closed
	// beads stored only in the archive must still appear.
	if s.Collection == DefaultCollection {
		archive := s.archivePartner()
		if archived, aerr := archive.ReadAll(); aerr == nil {
			seen := make(map[string]bool, len(beads))
			for _, b := range beads {
				seen[b.ID] = true
			}
			for _, b := range archived {
				if seen[b.ID] {
					continue
				}
				beads = append(beads, b)
			}
		}
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
	switch b.Status {
	case StatusOpen, StatusInProgress, StatusClosed, StatusBlocked,
		StatusProposed, StatusCancelled:
		// ok
	default:
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
// workingDir is the project root to use for git commands; when empty the
// process working directory is used (legacy behaviour, prone to worktree
// path contamination). Falls back to DefaultPrefix if detection fails.
func detectPrefix(workingDir string) string {
	// Try git repo root name first, running from the known project root so
	// that linked worktrees (e.g. execute-bead isolated worktrees) do not
	// contaminate the prefix with their ephemeral directory names.
	cmd := gitpkg.Command(context.Background(), workingDir, "rev-parse", "--show-toplevel")
	if out, err := cmd.Output(); err == nil {
		root := strings.TrimSpace(string(out))
		if root != "" {
			return filepath.Base(root)
		}
	}
	// Fall back to the provided working dir, then cwd.
	if workingDir != "" {
		return filepath.Base(workingDir)
	}
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

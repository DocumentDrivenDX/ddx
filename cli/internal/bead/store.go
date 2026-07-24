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

// Compile-time checks: *Store satisfies the TD-027 dependency sub-interfaces.
var _ BeadDependencyReader = (*Store)(nil)
var _ BeadDependencyWriter = (*Store)(nil)

// Compile-time checks: *Store satisfies the TD-027 archive/interchange sub-interfaces.
var _ BeadArchive = (*Store)(nil)
var _ BeadInterchangeReader = (*Store)(nil)
var _ BeadInterchangeWriter = (*Store)(nil)

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

// newStore creates a store with the given directory.
// Defaults can be overridden via options or environment.
func newStore(dir string, opts ...StoreOption) *Store {
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
		s.backend = NewAxonBackend(s.Dir, s.LockWait)
	}

	return s
}

// newStoreWithCollection creates a store for a named logical collection.
func newStoreWithCollection(dir, collection string) *Store {
	return newStore(dir, WithCollection(collection))
}

// Init creates the storage directory and file.
func (s *Store) Init(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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
func (s *Store) GenID(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	prefix := s.Prefix
	if prefix == "" {
		prefix = DefaultIDPrefix
	} else if !strings.HasSuffix(prefix, "-") {
		prefix += "-"
	}
	return RandomHexIDGenerator{Prefix: prefix, Bytes: 4}.GenID(ctx)
}

// ReadAll loads all beads from the configured backend.
func (s *Store) ReadAll(ctx context.Context) ([]Bead, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.backend != nil {
		all, err := s.backend.ReadAll()
		if err != nil {
			return nil, err
		}
		return all, nil
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
	return beads, nil
}

// ReadAllFiltered loads all beads, folds them by latest-wins, and returns only
// those for which the predicate returns true. When the predicate is nil every
// bead is returned (equivalent to ReadAll). The predicate is applied at the
// per-entry parse boundary after fold — matched beads are appended directly to
// the return slice without first being held in an intermediate full-corpus
// list, so queries that match a small subset avoid materializing the
// mismatches (ddx-9ce6842a Part 2 step 2: filter pushdown).
func (s *Store) ReadAllFiltered(ctx context.Context, pred func(Bead) bool) ([]Bead, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
		if err := s.writeAllLocked(beads); err != nil {
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
//
// Store mutations that replace the full JSONL corpus must serialize on the
// collection lock. Callers already inside Store.WithLock should use
// writeAllLocked to avoid re-entering the lock.
func (s *Store) WriteAll(beads []Bead) error {
	return s.WithLock(func() error {
		return s.writeAllLocked(beads)
	})
}

// WriteAllLocked writes all beads to the configured backend, sorted by ID,
// WITHOUT acquiring the collection lock. The caller must already hold the
// lock via Store.WithLock; calling WriteAll instead from inside WithLock
// re-enters the non-reentrant collection lock and deadlocks until the lock
// timeout (ddx-2a319f04).
func (s *Store) WriteAllLocked(beads []Bead) error {
	return s.writeAllLocked(beads)
}

func (s *Store) writeAllLocked(beads []Bead) error {
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
func (s *Store) Create(ctx context.Context, b *Bead) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b == nil {
		return fmt.Errorf("bead: create requires bead")
	}
	now := time.Now().UTC()
	if b.ID == "" {
		id, err := s.GenID(ctx)
		if err != nil {
			return err
		}
		b.ID = id
	}
	if b.IssueType == "" {
		b.IssueType = DefaultType
	}
	if b.Status == "" {
		// Create-time StatusOpen assignment is initial state, not a lifecycle transition.
		// This is the sole exception to ValidateLifecycleTransition: all other status
		// writes go through transitionLifecycleInPlace, which validates transitions via
		// ValidateLifecycleTransition (TD-031 §3 transition matrix pattern).
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
		byID := make(map[string]*Bead, len(beads))
		// Reject duplicate IDs
		for i := range beads {
			e := &beads[i]
			byID[e.ID] = e
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
		parentChain, err := beadParentChain(byID, b.Parent)
		if err != nil {
			return err
		}
		for _, depID := range depIDs {
			if err := rejectAncestorDependency(b.ID, depID, parentChain); err != nil {
				return err
			}
		}
		beads = append(beads, *b)
		return s.writeAllLocked(beads)
	})
}

// Get retrieves a bead by ID.
func (s *Store) Get(ctx context.Context, id string) (*Bead, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, fmt.Errorf("bead: get requires bead id")
	}
	beads, err := s.ReadAll(ctx)
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

// Update modifies a bead by ID. The mutate function receives a pointer to
// modify, but persisted lifecycle status changes must use TransitionLifecycle.
func (s *Store) Update(ctx context.Context, id string, mutate func(*Bead)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("bead: update requires bead id")
	}
	if mutate == nil {
		return fmt.Errorf("bead: update requires mutate func")
	}
	return s.updateBead(id, false, func(b *Bead) error {
		mutate(b)
		return nil
	})
}

// Apply mutates a bead through a typed Operation. When the configured raw
// backend exposes the optional OperationApplier fast path, Apply delegates to
// it; otherwise it falls back to the generic load-mutate-save path.
func (s *Store) Apply(id string, op Operation) error {
	if s == nil {
		return fmt.Errorf("bead: nil store")
	}
	if op == nil {
		return fmt.Errorf("bead: nil operation")
	}
	if applier, ok := s.backend.(OperationApplier); ok {
		return applier.Apply(context.Background(), id, op)
	}
	return s.updateBead(id, true, func(b *Bead) error {
		return op.Apply(b)
	})
}

func (s *Store) updateBead(id string, allowStatusChange bool, mutate func(*Bead) error) error {
	return s.WithLock(func() error {
		beads, _, err := s.readAllLatestRaw()
		if err != nil {
			return err
		}
		found := false
		for i := range beads {
			if beads[i].ID == id {
				beforeStatus := beads[i].Status
				beforeDeps := make(map[string]bool, len(beads[i].Dependencies))
				for _, dep := range beads[i].DepIDs() {
					beforeDeps[dep] = true
				}
				if mutate != nil {
					if err := mutate(&beads[i]); err != nil {
						return err
					}
				}
				if !allowStatusChange && beads[i].Status != beforeStatus {
					return fmt.Errorf("bead: lifecycle status change %s -> %s requires Store.TransitionLifecycle", beforeStatus, beads[i].Status)
				}
				beads[i].UpdatedAt = time.Now().UTC()
				// Core validation after mutation
				if err := s.validateBead(&beads[i]); err != nil {
					return err
				}
				// Reject NEWLY-introduced dependency edges whose target bead does
				// not exist — these can never be satisfied and block the referrer
				// from readiness forever (the phantom/dangling-dep class). Pre-
				// existing dangling edges are grandfathered so an already-corrupted
				// bead stays updatable and repairable via `ddx bead doctor --fix`.
				if mutate != nil {
					existing := make(map[string]bool, len(beads))
					for j := range beads {
						existing[beads[j].ID] = true
					}
					for _, dep := range beads[i].DepIDs() {
						if existing[dep] || beforeDeps[dep] {
							continue
						}
						return fmt.Errorf("bead: refusing to write dangling dependency edge: target %s does not exist", dep)
					}
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
		return s.writeAllLocked(beads)
	})
}

// TransitionLifecycle validates and persists one bead lifecycle status
// transition. Callers may provide a mutator for metadata that must land
// atomically with the status write.
func (s *Store) TransitionLifecycle(id string, status string, opts LifecycleTransitionOptions, mutate func(*Bead) error) error {
	return s.updateBead(id, true, func(b *Bead) error {
		beforeStatus := b.Status
		if err := transitionLifecycleInPlace(b, status, opts); err != nil {
			return err
		}
		if mutate != nil {
			return mutate(b)
		}
		appendOperatorAcceptedTriagedEventIfNeeded(b, beforeStatus, status, opts)
		return nil
	})
}

// SetLifecycleStatus validates and persists a lifecycle status transition.
func (s *Store) SetLifecycleStatus(id string, status string, opts LifecycleTransitionOptions) error {
	return s.TransitionLifecycle(id, status, opts, nil)
}

// UpdateWithLifecycleStatus atomically applies non-status field mutations and
// a validated lifecycle status transition.
func (s *Store) UpdateWithLifecycleStatus(id string, status string, opts LifecycleTransitionOptions, mutate func(*Bead) error) error {
	if status == StatusClosed {
		if err := s.rejectIfUnclosedBlockingDeps(id); err != nil {
			return err
		}
	}
	return s.updateBead(id, true, func(b *Bead) error {
		beforeStatus := b.Status
		if mutate != nil {
			if err := mutate(b); err != nil {
				return err
			}
		}
		if b.Status != beforeStatus {
			return fmt.Errorf("bead: UpdateWithLifecycleStatus mutator changed lifecycle status directly")
		}
		if err := transitionLifecycleInPlace(b, status, opts); err != nil {
			return err
		}
		appendOperatorAcceptedTriagedEventIfNeeded(b, beforeStatus, status, opts)
		return nil
	})
}

func transitionLifecycleInPlace(b *Bead, status string, opts LifecycleTransitionOptions) error {
	from := b.Status
	if err := ValidateLifecycleTransition(from, status, opts); err != nil {
		return fmt.Errorf("bead: lifecycle transition %s -> %s rejected: %w", from, status, err)
	}
	b.Status = status
	applyLifecycleTransitionMetadata(b, from, status, opts)
	return nil
}

const ExtraLifecycleExternalBlockerReason = "lifecycle-external-blocker-reason"

// ExtraLifecycleCrossRepoBlockerRef stores the structured cross-repo blocker
// target as JSON {"repo":"<known-repos key>","bead":"<bead-id>"}.
const ExtraLifecycleCrossRepoBlockerRef = "lifecycle-cross-repo-blocker-ref"

// CrossRepoBlockerRef names the repo alias and bead ID a blocked bead is
// waiting on.
type CrossRepoBlockerRef struct {
	Repo string `json:"repo"`
	Bead string `json:"bead"`
}

// NewCrossRepoBlockerRef validates and constructs a structured blocker ref.
func NewCrossRepoBlockerRef(repo, beadID string) (CrossRepoBlockerRef, error) {
	ref, ok := crossRepoBlockerRefFromStrings(repo, beadID)
	if !ok {
		return CrossRepoBlockerRef{}, fmt.Errorf("cross-repo blocker ref requires non-empty repo and bead id")
	}
	return ref, nil
}

// ParseCrossRepoBlockerRef attempts to decode a structured blocker ref from an
// extra-field value or raw JSON object.
func ParseCrossRepoBlockerRef(v any) (CrossRepoBlockerRef, bool) {
	switch value := v.(type) {
	case nil:
		return CrossRepoBlockerRef{}, false
	case CrossRepoBlockerRef:
		return crossRepoBlockerRefFromStrings(value.Repo, value.Bead)
	case *CrossRepoBlockerRef:
		if value == nil {
			return CrossRepoBlockerRef{}, false
		}
		return crossRepoBlockerRefFromStrings(value.Repo, value.Bead)
	case map[string]any:
		return crossRepoBlockerRefFromStrings(extraStringVal(value, "repo"), extraStringVal(value, "bead"))
	case map[string]string:
		return crossRepoBlockerRefFromStrings(value["repo"], value["bead"])
	case json.RawMessage:
		var ref CrossRepoBlockerRef
		if err := json.Unmarshal(value, &ref); err != nil {
			return CrossRepoBlockerRef{}, false
		}
		return crossRepoBlockerRefFromStrings(ref.Repo, ref.Bead)
	case []byte:
		var ref CrossRepoBlockerRef
		if err := json.Unmarshal(value, &ref); err != nil {
			return CrossRepoBlockerRef{}, false
		}
		return crossRepoBlockerRefFromStrings(ref.Repo, ref.Bead)
	default:
		return CrossRepoBlockerRef{}, false
	}
}

func crossRepoBlockerRefFromStrings(repo, beadID string) (CrossRepoBlockerRef, bool) {
	ref := CrossRepoBlockerRef{
		Repo: strings.TrimSpace(repo),
		Bead: strings.TrimSpace(beadID),
	}
	if ref.Repo == "" || ref.Bead == "" {
		return CrossRepoBlockerRef{}, false
	}
	return ref, true
}

// ExtraConsecutiveWedgeMarker is the bead Extra key holding the consecutive-wedge
// count recorded by the execute-bead loop. It is cleared at the store layer on
// any transition to StatusOpen so that operator-reopened beads get a fresh
// attempt rather than being instantly re-parked by the guard (ddx-5c549120).
const ExtraConsecutiveWedgeMarker = "work-consecutive-wedges"

// Preserved-review block markers (ddx-ec1c1f89). Stamped when the preserve
// path leaves unresolved needs-review evidence (e.g. the large-deletion
// safety gate) so the bead is excluded from worker readiness until an
// operator explicitly unblocks it with a matching attempt ID and a newer
// timestamp via `ddx bead update --set`.
const (
	ExtraPreservedReviewBlockedAt        = "preserved-review-blocked-at"
	ExtraPreservedReviewBlockedAttempt   = "preserved-review-blocked-attempt"
	ExtraPreservedReviewGate             = "preserved-review-gate"
	ExtraPreservedReviewFingerprint      = "preserved-review-fingerprint"
	ExtraPreservedReviewUnblockedAt      = "preserved-review-unblocked-at"
	ExtraPreservedReviewUnblockedAttempt = "preserved-review-unblocked-attempt"
)

// PreservedReviewGateLargeDeletion is the preserved-review-gate value stamped
// when the large-deletion safety gate preserves an attempt for review.
const PreservedReviewGateLargeDeletion = "large-deletion"

func applyLifecycleTransitionMetadata(b *Bead, from, status string, opts LifecycleTransitionOptions) {
	if b.Extra == nil {
		b.Extra = make(map[string]any)
	}
	if status == StatusBlocked {
		if reason := strings.TrimSpace(opts.ExternalBlockerReason); reason != "" {
			b.Extra[ExtraLifecycleExternalBlockerReason] = reason
		}
	} else {
		delete(b.Extra, ExtraLifecycleExternalBlockerReason)
		delete(b.Extra, ExtraLifecycleCrossRepoBlockerRef)
	}
	// Clear stale consecutive-wedge marker on operator reopen (proposed/blocked →
	// open) so the execute-bead loop gets a fresh attempt rather than re-parking
	// without trying (ddx-5c549120). Only clears on operator-facing transitions;
	// internal release (in_progress → open) preserves the count.
	if status == StatusOpen && (from == StatusProposed || from == StatusBlocked) {
		delete(b.Extra, ExtraConsecutiveWedgeMarker)
	}
}

func appendOperatorAcceptedTriagedEventIfNeeded(b *Bead, from, to string, opts LifecycleTransitionOptions) {
	if b == nil || from != StatusProposed || to != StatusOpen {
		return
	}
	acceptedFingerprint, acceptedPromptFingerprint := latestParkedIntakeFingerprint(b)
	body := map[string]any{
		"from_status":                 from,
		"to_status":                   to,
		"reason":                      strings.TrimSpace(opts.Reason),
		"source":                      strings.TrimSpace(opts.Source),
		"operator_required":           true,
		"accepted_fingerprint":        acceptedFingerprint,
		"accepted_prompt_fingerprint": acceptedPromptFingerprint,
	}
	if actor := strings.TrimSpace(opts.Actor); actor != "" {
		body["actor"] = actor
	}
	if promptFingerprint := PromptFingerprint(*b); promptFingerprint != "" {
		body["prompt_fingerprint"] = promptFingerprint
	}
	payload, _ := json.Marshal(body)
	appendInlineEvent(b, BeadEvent{
		Kind:      "triaged",
		Summary:   "operator accepted proposed bead",
		Body:      string(payload),
		Actor:     strings.TrimSpace(opts.Actor),
		Source:    strings.TrimSpace(opts.Source),
		CreatedAt: time.Now().UTC(),
	})
}

func latestParkedIntakeFingerprint(b *Bead) (string, string) {
	if b == nil || b.Extra == nil {
		return "", ""
	}
	events := decodeBeadEvents(b.Extra["events"])
	for i := len(events) - 1; i >= 0; i-- {
		switch events[i].Kind {
		case "intake.blocked", "pre_claim_intake.blocked":
			fields := beadEventBodyFields(events[i].Body)
			fingerprint := strings.TrimSpace(fields["fingerprint"])
			if fingerprint == "" {
				continue
			}
			return fingerprint, strings.TrimSpace(fields["prompt_fingerprint"])
		}
	}
	return "", ""
}

func beadEventBodyFields(body string) map[string]string {
	fields := map[string]string{}
	if strings.TrimSpace(body) == "" {
		return fields
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return fields
	}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			fields[k] = s
		}
	}
	return fields
}

// HeartbeatInterval is how often a claim owner should refresh the external
// claim liveness lease while running. Exposed as a variable so tests can
// override it.
var HeartbeatInterval = 30 * time.Second

// HeartbeatTTL is the maximum allowed age of the external claim liveness
// lease before another worker may reclaim the bead. Defaults to 3×
// HeartbeatInterval. Exposed as a variable so tests can override it.
var HeartbeatTTL = 90 * time.Second

// Claim sets a bead to in_progress with claim metadata.
// Fails if the bead is already claimed (status == in_progress) with a
// fresh claim lease. A bead whose external lease is older than HeartbeatTTL
// is considered stalled, except that a same-machine owner with a live PID is
// protected until the hard fallback ceiling expires.
func (s *Store) Claim(id, assignee string) error {
	machine := currentMachineID()
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
				if !claimLeaseIsStale(s, beads[i].Extra, id) {
					return fmt.Errorf("bead: cannot claim %s from status %s", id, beads[i].Status)
				}
				// normal claim path
			case StatusInProgress:
				if !claimLeaseIsStale(s, beads[i].Extra, id) {
					return fmt.Errorf("bead: cannot claim %s from status %s", id, beads[i].Status)
				}
				// stalled claim — reclaim atomically below
			default:
				return fmt.Errorf("bead: cannot claim %s from status %s", id, beads[i].Status)
			}
			if err := transitionLifecycleInPlace(&beads[i], StatusInProgress, LifecycleTransitionOptions{
				Reason: "claim",
				Actor:  assignee,
				Source: "Store.Claim",
			}); err != nil {
				return err
			}
			now := time.Now().UTC()
			beads[i].Owner = assignee
			beads[i].UpdatedAt = now
			if beads[i].Extra == nil {
				beads[i].Extra = make(map[string]any)
			}
			// Compatibility cleanup only: worker/live claim metadata now lives in
			// the external lease sidecar rather than the tracked row.
			for _, key := range ClaimMetadataExtraKeys {
				delete(beads[i].Extra, key)
			}
			delete(beads[i].Extra, ClaimHeartbeatExtraKey)
			if err := s.validateBead(&beads[i]); err != nil {
				return err
			}
			if err := s.runHook("validate-bead-update", &beads[i]); err != nil {
				return err
			}
			if err := s.writeAllLocked(beads); err != nil {
				return err
			}
			return s.writeClaimHeartbeat(ClaimLeaseRecord{
				BeadID:    id,
				Owner:     assignee,
				Machine:   machine,
				StartedAt: now,
				UpdatedAt: now,
				PID:       os.Getpid(),
			})
		}
		return fmt.Errorf("bead: not found: %s", id)
	})
}

// ClaimWithOptions acquires or refreshes the external worker-claim lease
// without mutating the tracked bead row. session and worktree are optional;
// machine is derived from the current host identity. A stalled open/in_progress
// bead is reclaimed atomically under the store's lock, but a same-machine live
// owner remains protected until the hard fallback ceiling expires.
func (s *Store) ClaimWithOptions(id, assignee, session, worktree string) error {
	machine := currentMachineID()
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
				if !claimLeaseIsStale(s, beads[i].Extra, id) {
					return fmt.Errorf("bead: cannot claim %s from status %s", id, beads[i].Status)
				}
				// worker lease path
			case StatusInProgress:
				if !claimLeaseIsStale(s, beads[i].Extra, id) {
					return fmt.Errorf("bead: cannot claim %s from status %s", id, beads[i].Status)
				}
				// stalled claim — reclaim atomically below
			default:
				return fmt.Errorf("bead: cannot claim %s from status %s", id, beads[i].Status)
			}
			now := time.Now().UTC()
			return s.writeClaimHeartbeat(ClaimLeaseRecord{
				BeadID:    id,
				Owner:     assignee,
				Session:   session,
				Worktree:  worktree,
				Machine:   machine,
				StartedAt: now,
				UpdatedAt: now,
				PID:       os.Getpid(),
			})
		}
		return fmt.Errorf("bead: not found: %s", id)
	})
}

// Heartbeat refreshes the external claim liveness lease on a claimed bead.
// Returns an error if the bead is no longer in_progress (e.g., reclaimed by
// another worker), allowing the caller to stop its heartbeat loop.
//
// Deprecated: use TouchClaimHeartbeat.
func (s *Store) Heartbeat(id string) error {
	if err := s.WithLock(func() error {
		beads, _, err := s.readAllLatestRaw()
		if err != nil {
			return err
		}
		for _, b := range beads {
			if b.ID != id {
				continue
			}
			if b.Status != StatusInProgress {
				return fmt.Errorf("bead: cannot heartbeat %s from status %s", id, b.Status)
			}
			return s.TouchClaimHeartbeat(id)
		}
		return fmt.Errorf("bead: not found: %s", id)
	}); err != nil {
		return err
	}
	return nil
}

// claimLeaseIsStale returns true if the external claim lease is absent or
// older than HeartbeatTTL. Same-machine live owners remain protected until the
// hard fallback ceiling expires. When no lease file exists, it falls back to
// the legacy tracker heartbeat field for compatibility with imported/stale rows.
func claimLeaseIsStale(s *Store, extra map[string]any, id string) bool {
	if s != nil {
		if rec, found, err := s.readClaimHeartbeat(id); err == nil {
			if found {
				return claimLeaseRecordIsStale(rec)
			}
		} else {
			return heartbeatIsStale(extra)
		}
	}
	return heartbeatIsStale(extra)
}

// heartbeatIsStale returns true if the given bead's legacy Extra map has no
// heartbeat field or one older than HeartbeatTTL.
func heartbeatIsStale(extra map[string]any) bool {
	if extra == nil {
		return true
	}
	raw, ok := extra["work-heartbeat-at"]
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

// clearClaimExtraKeys resets claim state by deleting all claim-related metadata
// from the Extra map. It clears both legacy claim metadata (claimed-at, claimed-pid,
// claimed-machine, claimed-session, claimed-worktree) and the claim heartbeat lease
// timestamp. These keys are cleared when a bead is unclaimed or reopened to fully
// reset its claim state so it can be claimed again by another worker.
func clearClaimExtraKeys(extra map[string]any) {
	if extra == nil {
		return
	}
	for _, k := range ClaimMetadataExtraKeys {
		delete(extra, k)
	}
	delete(extra, ClaimHeartbeatExtraKey)
}

// Unclaim clears claim metadata. Only reverts status to open if the bead
// is currently in_progress — a closed bead stays closed.
func (s *Store) Unclaim(id string) error {
	if err := s.updateBead(id, true, func(b *Bead) error {
		if b.Status == StatusInProgress {
			if err := transitionLifecycleInPlace(b, StatusOpen, LifecycleTransitionOptions{
				Reason: "unclaim",
				Source: "Store.Unclaim",
			}); err != nil {
				return err
			}
		}
		b.Owner = ""
		clearClaimExtraKeys(b.Extra)
		return nil
	}); err != nil {
		return err
	}
	return s.RemoveClaimHeartbeat(id)
}

// Release is the atomic, symmetric counterpart to Claim: it transitions a
// claimed bead back to targetStatus (StatusOpen when empty), clears Owner,
// persists the row, AND removes the external claim-heartbeat lease sidecar —
// all inside a single WithLock critical section. Unlike Unclaim (which removes
// the lease only after the lock is released), Release couples claim clearing
// and lifecycle status in one locked write, so a failure-release can never
// leave a stale lease sidecar while `ddx bead status` reports the bead as not
// in_progress (the phantom-lease inconsistency observed in ddx-8f2e0ebf).
//
// Only an in_progress bead has its status transitioned; a bead in any other
// status (e.g. closed) keeps its status and only has Owner and the lease
// cleared, mirroring Unclaim's guard so a released bead is never reopened.
func (s *Store) Release(id, assignee, targetStatus string) error {
	if strings.TrimSpace(targetStatus) == "" {
		targetStatus = StatusOpen
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
			if beads[i].Status == StatusInProgress {
				if err := transitionLifecycleInPlace(&beads[i], targetStatus, LifecycleTransitionOptions{
					Reason: "release",
					Actor:  assignee,
					Source: "Store.Release",
				}); err != nil {
					return err
				}
			}
			beads[i].Owner = ""
			beads[i].UpdatedAt = time.Now().UTC()
			clearClaimExtraKeys(beads[i].Extra)
			if err := s.validateBead(&beads[i]); err != nil {
				return err
			}
			if err := s.runHook("validate-bead-update", &beads[i]); err != nil {
				return err
			}
			if err := s.writeAllLocked(beads); err != nil {
				return err
			}
			return s.RemoveClaimHeartbeat(id)
		}
		return fmt.Errorf("bead: not found: %s", id)
	})
}

// SetExecutionCooldown parks bead id until the given wall-clock time.
// baseRev is the origin/main HEAD at cooldown time; when non-empty it enables
// rev-bound auto-invalidation: if origin/main advances past baseRev the
// cooldown clears automatically on the next ready-queue pass.
func (s *Store) SetExecutionCooldown(id string, until time.Time, status, detail, baseRev string) error {
	return s.Update(context.Background(), id, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra["work-retry-after"] = until.UTC().Format(time.RFC3339)
		if status != "" {
			b.Extra["work-last-status"] = status
		}
		if detail != "" {
			b.Extra["work-last-detail"] = detail
		}
		if baseRev != "" {
			b.Extra[ExtraCooldownBaseRev] = baseRev
		} else {
			delete(b.Extra, ExtraCooldownBaseRev)
		}
	})
}

// ClearCooldowns clears work cooldown fields for all beads matched by
// filter. If filter is nil, every bead with work-retry-after set is
// cleared. Returns the count of beads whose cooldowns were cleared. A
// "cooldown-cleared" event with reason="operator-bulk-clear" is appended for
// each cleared bead in the same pass.
func (s *Store) ClearCooldowns(filter func(Bead) bool) (int, error) {
	var count int
	err := s.WithLock(func() error {
		beads, _, err := s.readAllLatestRaw()
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		for i := range beads {
			b := &beads[i]
			if extraStringVal(b.Extra, ExtraRetryAfter) == "" {
				continue
			}
			if filter != nil && !filter(*b) {
				continue
			}
			if b.Extra == nil {
				b.Extra = make(map[string]any)
			}
			delete(b.Extra, ExtraRetryAfter)
			delete(b.Extra, ExtraCooldownBaseRev)
			delete(b.Extra, ExtraLastStatus)
			delete(b.Extra, ExtraLastDetail)
			b.UpdatedAt = now
			var events []BeadEvent
			if raw, ok := b.Extra["events"]; ok {
				events = decodeBeadEvents(raw)
			}
			events = append(events, BeadEvent{
				Kind:      "cooldown-cleared",
				Summary:   "operator-bulk-clear",
				Source:    "ddx work clear-cooldowns",
				CreatedAt: now,
			})
			b.Extra["events"] = encodeEventsForExtra(events)
			if err := s.validateBead(b); err != nil {
				return err
			}
			count++
		}
		if count == 0 {
			return nil
		}
		return s.writeAllLocked(beads)
	})
	return count, err
}

// IncrNoChangesCount increments the work no-changes counter for a bead
// and returns the new count. Used by the execute-bead worker to detect when a
// bead should be auto-closed after repeated no-change attempts.
func (s *Store) IncrNoChangesCount(id string) (int, error) {
	var newCount int
	err := s.Update(context.Background(), id, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		var count int
		if v, ok := b.Extra["work-no-changes-count"]; ok {
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
		b.Extra["work-no-changes-count"] = count
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
	err := s.Update(context.Background(), id, func(b *Bead) {
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
	b, err := s.Get(context.Background(), id)
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
	return s.Update(context.Background(), id, func(b *Bead) {
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
	if err := s.Update(context.Background(), id, func(b *Bead) {
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
	b, err := s.Get(context.Background(), id)
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
	switch v := m["created_at"].(type) {
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, v); err == nil {
			e.CreatedAt = parsed
			break
		}
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			e.CreatedAt = parsed
		}
	case time.Time:
		e.CreatedAt = v
	}
	return e
}

// lastReviewVerdictFromEvents returns the verdict string from the last
// kind:review event in the list. Returns "" when no review event exists.
func lastReviewVerdictFromEvents(events []BeadEvent) string {
	verdict := ""
	for _, e := range events {
		if e.Kind != "review" {
			continue
		}
		v := strings.ToUpper(strings.TrimSpace(e.Summary))
		switch v {
		case "APPROVE", "BLOCK", "REQUEST_CHANGES", "REQUEST_CLARIFICATION":
			verdict = v
		}
	}
	return verdict
}

// Close sets a bead's status to closed. When the close succeeds and the bead
// carries an inline event history, those events are moved to a sidecar
// attachment under .ddx/attachments/<id>/events.jsonl so the active row stays
// small (per ADR-004's attachment model and TD-027 §c).
func (s *Store) Close(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("bead: close requires bead id")
	}
	if err := s.rejectIfUnclosedBlockingDeps(id); err != nil {
		return err
	}
	// Capture last review verdict before closure for reviewer-accuracy tracking.
	// If the operator manually closes a BLOCK-reviewed bead, that contradicts
	// the reviewer's verdict (potential false-positive).
	verdictWas := ""
	if events, err := s.Events(id); err == nil {
		verdictWas = lastReviewVerdictFromEvents(events)
	}

	if err := s.SetLifecycleStatus(id, StatusClosed, LifecycleTransitionOptions{
		ManualClose: true,
		Reason:      "manual close",
		Source:      "Store.Close",
	}); err != nil {
		return err
	}
	if err := s.externalizeEvents(id); err != nil {
		return err
	}
	// Emit accuracy override when operator closes a BLOCK-reviewed bead.
	// AppendEvent handles sidecar append for already-externalized events.
	if verdictWas == "BLOCK" {
		_ = s.AppendEvent(id, BeadEvent{
			Kind:      "review-accuracy-override",
			Summary:   "BLOCK verdict contradicted by operator close",
			Body:      "verdict_was=BLOCK\noperator_action=close\nreason=manual close",
			Source:    "Store.Close",
			CreatedAt: time.Now().UTC(),
		})
	}
	_ = s.RemoveClaimHeartbeat(id)
	s.maybeOpportunisticMaintenance()

	// Cascade-close eligible beads superseded by X, then walk up one hop
	// to close dead-intermediate (execution-eligible==false) beads.
	// Both operations share a visited set to prevent cycles within Close().
	visited := map[string]bool{id: true}
	_ = s.cascadeAndWalkUp(id, visited)

	return nil
}

// externalizeEvents moves a bead's inline events into its sidecar attachment
// under a fresh lock. Safe to call repeatedly — no-op when there are no
// inline events (already externalized, or none recorded).
func (s *Store) externalizeEvents(id string) error {
	if s.axonGraphQLActive() {
		return nil
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
			if err := s.externalizeEventsInPlace(&beads[i]); err != nil {
				return err
			}
			return s.writeAllLocked(beads)
		}
		return nil
	})
}

// cascadeAndWalkUp performs both cascade-close (RC-2) and walk-up (RC-3) cleanup
// within a single Close() invocation, sharing the visited set to prevent cycles.
// First cascades close beads superseded by closedID, then walks up one hop to
// check if the closed bead's parent should be auto-closed as a dead-intermediate.
func (s *Store) cascadeAndWalkUp(closedID string, visited map[string]bool) error {
	all, err := s.ReadAll(context.Background())
	if err != nil {
		return err
	}

	// RC-2: Cascade-close superseded beads
	for _, x := range all {
		if visited[x.ID] {
			continue
		}
		if !s.canCascadeCloseSuperseeded(&x, closedID, all) {
			continue
		}
		visited[x.ID] = true
		_ = s.SetLifecycleStatus(x.ID, StatusClosed, LifecycleTransitionOptions{
			ManualClose: true,
			Reason:      fmt.Sprintf("cascade-close superseded by %s", closedID),
			Source:      "Store.Close.cascadeCloseSuperseeded",
		})
		_ = s.externalizeEvents(x.ID)
		_ = s.AppendEvent(x.ID, BeadEvent{
			Kind:      "superseded_close",
			Summary:   fmt.Sprintf("closed as superseded via %s", closedID),
			Body:      fmt.Sprintf("closed_by_cascade_of: %s\nreason: closed_as_superseded_via:%s", closedID, closedID),
			Source:    "Store.Close.cascadeCloseSuperseeded",
			CreatedAt: time.Now().UTC(),
		})
		_ = s.RemoveClaimHeartbeat(x.ID)
	}

	// RC-3: Walk up the parent chain to close dead-intermediate
	// (execution-eligible==false) beads and nested epic ancestors.
	closedBead := findBeadByID(all, closedID)
	if closedBead != nil && closedBead.Parent != "" {
		if !visited[closedBead.Parent] {
			s.walkUpClosureCandidate(closedBead.Parent, all, visited)
		}
	}

	return nil
}

// walkUpClosureCandidate checks if parent should be auto-closed when all its
// children reach terminal state. Two closure paths:
//   - Dead-intermediate: parent has execution-eligible=false; auto-closes once
//     all children are terminal (RC-3 walk-up closure).
//   - Epic auto-close: parent is an epic bead; auto-closes when every child is
//     closed or cancelled (both count as terminal).
//
// Cancelled children are treated as terminal for both paths.
func (s *Store) walkUpClosureCandidate(parentID string, allBeads []Bead, visited map[string]bool) {
	parent := findBeadByID(allBeads, parentID)
	if parent == nil {
		return
	}

	// Count children; closed AND cancelled both count as terminal.
	nonTerminalChildCount := 0
	closedChildCount := 0
	totalChildCount := 0
	for _, b := range allBeads {
		if b.Parent == parentID {
			totalChildCount++
			if b.Status == StatusClosed {
				closedChildCount++
			} else if b.Status != StatusCancelled {
				nonTerminalChildCount++
			}
		}
	}

	if nonTerminalChildCount > 0 || totalChildCount == 0 {
		return
	}

	executionEligible, executionEligibleKnown := lifecycleExecutionEligible(*parent)
	isDeadIntermediate := executionEligibleKnown && !executionEligible
	isEpic := isEpicBead(*parent)

	if !isDeadIntermediate && !isEpic {
		return
	}

	var eventKind, reason, summary, body string
	targetStatus := StatusClosed
	if isDeadIntermediate {
		reason = "auto-close dead-intermediate (all children closed, not execution-eligible)"
		eventKind = "dead_intermediate_close"
		summary = "closed as dead-intermediate bead (all children closed, execution-eligible=false)"
		body = fmt.Sprintf("closed_because: all_children_closed\nexecution_eligible: false\ntotal_children: %d", totalChildCount)
	} else if closedChildCount == 0 {
		// All children are cancelled — no work actually completed; cancel the epic.
		targetStatus = StatusCancelled
		reason = "auto-cancel epic: all children cancelled, no work completed"
		eventKind = "epic_auto_close"
		summary = "auto-cancelled: all children cancelled, no work completed"
		body = fmt.Sprintf("closed_because: all_children_cancelled\ntotal_children: %d", totalChildCount)
	} else {
		reason = "auto-close epic: all children reached terminal state"
		eventKind = "epic_auto_close"
		summary = "auto-closed: all children reached terminal state"
		body = fmt.Sprintf("closed_because: all_children_terminal\ntotal_children: %d", totalChildCount)
	}

	visited[parentID] = true
	_ = s.SetLifecycleStatus(parentID, targetStatus, LifecycleTransitionOptions{
		ManualClose: true,
		Reason:      reason,
		Source:      "Store.Close.walkUpClosureCandidate",
	})
	_ = s.externalizeEvents(parentID)
	_ = s.AppendEvent(parentID, BeadEvent{
		Kind:      eventKind,
		Summary:   summary,
		Body:      body,
		Source:    "Store.Close.walkUpClosureCandidate",
		CreatedAt: time.Now().UTC(),
	})
	_ = s.RemoveClaimHeartbeat(parentID)

	if updated := findBeadByID(allBeads, parentID); updated != nil {
		updated.Status = targetStatus
	}

	if parent.Parent != "" && !visited[parent.Parent] {
		s.walkUpClosureCandidate(parent.Parent, allBeads, visited)
	}
}

// EpicClosureCandidates returns open epic beads whose children have all reached
// a terminal state (closed or cancelled). Useful for backfill-closing epics that
// predate auto-close, via ddx bead reap --apply.
func (s *Store) EpicClosureCandidates(ctx context.Context) ([]Bead, error) {
	all, err := s.ReadAll(ctx)
	if err != nil {
		return nil, err
	}
	childCount := make(map[string]int)
	nonTerminalCount := make(map[string]int)
	for _, b := range all {
		if b.Parent == "" {
			continue
		}
		childCount[b.Parent]++
		if b.Status != StatusClosed && b.Status != StatusCancelled {
			nonTerminalCount[b.Parent]++
		}
	}
	var candidates []Bead
	for _, b := range all {
		if b.Status == StatusClosed || b.Status == StatusCancelled {
			continue
		}
		if childCount[b.ID] == 0 {
			continue
		}
		if nonTerminalCount[b.ID] > 0 {
			continue
		}
		if !isEpicBead(b) {
			continue
		}
		candidates = append(candidates, b)
	}
	return candidates, nil
}

// cascadeCloseSuperseeded scans for open beads X where superseded-by==Y,
// checks strict scope guards, and closes eligible ones. One-hop deep only;
// if X is a superseder for an open Z, Z's closure is deferred.
// Idempotent: re-running is a no-op for already-closed X.
func (s *Store) cascadeCloseSuperseeded(closedID string) error {
	all, err := s.ReadAll(context.Background())
	if err != nil {
		return err
	}
	visited := map[string]bool{closedID: true}
	for _, x := range all {
		if visited[x.ID] {
			continue
		}
		if !s.canCascadeCloseSuperseeded(&x, closedID, all) {
			continue
		}
		visited[x.ID] = true
		_ = s.SetLifecycleStatus(x.ID, StatusClosed, LifecycleTransitionOptions{
			ManualClose: true,
			Reason:      fmt.Sprintf("cascade-close superseded by %s", closedID),
			Source:      "Store.Close.cascadeCloseSuperseeded",
		})
		_ = s.externalizeEvents(x.ID)
		_ = s.AppendEvent(x.ID, BeadEvent{
			Kind:      "superseded_close",
			Summary:   fmt.Sprintf("closed as superseded via %s", closedID),
			Body:      fmt.Sprintf("closed_by_cascade_of: %s\nreason: closed_as_superseded_via:%s", closedID, closedID),
			Source:    "Store.Close.cascadeCloseSuperseeded",
			CreatedAt: time.Now().UTC(),
		})
		_ = s.RemoveClaimHeartbeat(x.ID)
	}
	return nil
}

// canCascadeCloseSuperseeded checks all scope guards before cascade-closing X.
// All guards must pass for a bead to be eligible. Reads current state fresh.
func (s *Store) canCascadeCloseSuperseeded(x *Bead, closedID string, allBeads []Bead) bool {
	// Re-read X's current state to ensure fresh snapshot
	fresh, err := s.Get(context.Background(), x.ID)
	if err != nil {
		return false
	}

	// Guard 1: X is open AND (X.superseded-by == closedID OR X carries label superseded-by:closedID) AND closedID is closed
	if fresh.Status != StatusOpen {
		return false
	}
	superseededBy := lifecycleSupersededBy(*fresh)
	if superseededBy != closedID {
		found := false
		for _, l := range fresh.Labels {
			if l == fmt.Sprintf("superseded-by:%s", closedID) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	// Verify closedID is actually closed
	var closedBead *Bead
	for i, b := range allBeads {
		if b.ID == closedID {
			closedBead = &allBeads[i]
			break
		}
	}
	if closedBead == nil || closedBead.Status != StatusClosed {
		return false
	}

	// Guard 2: X has no other open dependents (no open bead Z with Z.dependencies referencing X)
	for _, z := range allBeads {
		if z.Status == StatusOpen && z.ID != fresh.ID {
			for _, d := range z.Dependencies {
				if d.DependsOnID == fresh.ID {
					return false
				}
			}
		}
	}

	// Guard 3: X has no open dependencies of its own beyond the supersession marker
	for _, d := range fresh.Dependencies {
		target := findBeadByID(allBeads, d.DependsOnID)
		if target != nil && target.Status != StatusClosed && target.Status != StatusCancelled {
			return false
		}
	}

	// Guard 4: X has attempt_count == 0 (no execution-related events)
	events, _ := s.Events(fresh.ID)
	for _, e := range events {
		if strings.Contains(e.Kind, "execution") || strings.Contains(e.Kind, "dispatch") ||
			strings.Contains(e.Kind, "work") || e.Kind == "execute-bead" {
			return false
		}
	}

	// Guard 5: X has no operator notes added after the supersession marker timestamp
	// For simplicity, check if notes are present (indicates operator activity)
	if strings.TrimSpace(fresh.Notes) != "" {
		return false
	}

	// Guard 6: X is not currently claimed or in-progress (no live claim_id)
	if fresh.Status == StatusInProgress {
		return false
	}
	if claimFresh, _, err := s.ClaimHeartbeatFresh(fresh.ID); err == nil && claimFresh {
		return false
	}

	// Guard 7: X carries no manual-hold / no-auto-close / container override label
	for _, l := range fresh.Labels {
		switch l {
		case "manual-hold", "no-auto-close", "container":
			return false
		}
	}

	return true
}

func findBeadByID(beads []Bead, id string) *Bead {
	for i, b := range beads {
		if b.ID == id {
			return &beads[i]
		}
	}
	return nil
}

// ErrClosureGateRejected indicates a close was refused because the bead does
// not carry the evidence required for an automated closure: a terminal
// verdict event (review APPROVE with non-empty rationale, or an explicit
// review-skipped / manual-close marker) AND an execution-evidence marker
// (closing_commit_sha, session_id, or an execute-bead success event in the
// events history).
//
// Automated work paths (execute-bead + reviewer) always go through
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
	if b, err := s.Get(context.Background(), id); err == nil && b != nil && b.Status == StatusClosed {
		if err := s.externalizeEvents(id); err != nil {
			return err
		}
		// Walk up one hop to auto-close parent epics and dead-intermediates,
		// matching the cascade that Store.Close runs.
		visited := map[string]bool{id: true}
		_ = s.cascadeAndWalkUp(id, visited)
	}
	_ = s.RemoveClaimHeartbeat(id)
	s.maybeOpportunisticMaintenance()
	return nil
}

func (s *Store) closeWithEvidence(id string, sessionID string, commitSHA string) error {
	if err := s.rejectIfUnclosedBlockingDeps(id); err != nil {
		return err
	}
	return s.updateBead(id, true, func(b *Bead) error {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		if sessionID != "" {
			b.Extra["session_id"] = sessionID
		}
		if commitSHA != "" {
			b.Extra["closing_commit_sha"] = commitSHA
		}
		// ClosureGate guards the evidence-close path only; reconcile-close skips
		// it by design (TD-031 §3.1).
		if err := ClosureGate(b); err != nil {
			// Surface via bead notes so a later operator audit can see why the
			// close was refused; a single error path would be dropped by the
			// Update callback signature (no error return).
			appendClosureRejectNote(b, err)
			return nil
		}
		return transitionLifecycleInPlace(b, StatusClosed, LifecycleTransitionOptions{
			ManualClose: true,
			Reason:      "close with evidence",
			Source:      "Store.CloseWithEvidence",
		})
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
	return s.Update(context.Background(), id, func(b *Bead) {
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
			if err := transitionLifecycleInPlace(b, StatusOpen, LifecycleTransitionOptions{
				ManualReopen: true,
			}); err != nil {
				return err
			}
			b.Owner = ""
			b.UpdatedAt = now
			if b.Extra == nil {
				b.Extra = make(map[string]any)
			}
			// Clear claim fields
			clearClaimExtraKeys(b.Extra)
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
			// Detect last review verdict before appending the reopen event so
			// we can identify APPROVE verdicts contradicted by the operator.
			verdictWas := lastReviewVerdictFromEvents(events)
			evt := BeadEvent{
				Kind:      "reopen",
				CreatedAt: now,
			}
			if reason != "" {
				evt.Summary = reason
			}
			events = append(events, evt)
			// Emit accuracy override when operator reopens an APPROVE-reviewed
			// bead (potential false-negative — reviewer approved fake work).
			if verdictWas == "APPROVE" {
				body := "verdict_was=APPROVE\noperator_action=reopen"
				if reason != "" {
					body += "\nreason=" + reason
				}
				events = append(events, BeadEvent{
					Kind:      "review-accuracy-override",
					Summary:   "APPROVE verdict contradicted by operator reopen",
					Body:      body,
					Source:    "Store.Reopen",
					CreatedAt: now,
				})
			}
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
		if err := s.writeAllLocked(beads); err != nil {
			return err
		}
		return s.RemoveClaimHeartbeat(id)
	})
}

// List returns beads matching optional filters.
// where is an optional map of key=value predicates that match against
// known struct fields and Extra (unknown/workflow-specific) fields.
func (s *Store) List(status, label string, where map[string]string) ([]Bead, error) {
	beads, err := s.ReadAll(context.Background())
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

type lifecycleQueueEntry struct {
	Bead
	Decision             LifecycleQueueDecision
	UnclosedDepIDs       []string
	RetryAfter           time.Time
	LastStatus           string
	LastDetail           string
	ExecutionSkipReason  string
	SupersededBy         string
	EpicClosureCandidate bool
}

func (s *Store) classifyLifecycleQueue(beads []Bead, now time.Time) []lifecycleQueueEntry {
	statusMap := make(map[string]string)
	childCount := make(map[string]int)
	openChildCount := make(map[string]int)
	for _, b := range beads {
		statusMap[b.ID] = b.Status
		if b.Parent != "" {
			childCount[b.Parent]++
			if b.Status != StatusClosed {
				openChildCount[b.Parent]++
			}
		}
	}

	// Read origin HEAD once per pass for rev-bound cooldown auto-invalidation.
	originHead := originMainHead(s.workingDir())

	entries := make([]lifecycleQueueEntry, 0, len(beads))
	for _, b := range beads {
		unclosed := unclosedDependencyIDs(b, statusMap)
		retryAfter, retryActive := activeRetryCooldown(b, now, originHead)
		executionEligible, executionEligibleKnown := lifecycleExecutionEligible(b)
		supersededBy := lifecycleSupersededBy(b)
		claimFresh := (b.Status == StatusOpen || b.Status == StatusInProgress) && !claimLeaseIsStale(s, b.Extra, b.ID)
		entry := lifecycleQueueEntry{
			Bead:                 b,
			UnclosedDepIDs:       unclosed,
			RetryAfter:           retryAfter,
			LastStatus:           extraStringVal(b.Extra, ExtraLastStatus),
			LastDetail:           extraStringVal(b.Extra, ExtraLastDetail),
			ExecutionSkipReason:  extraStringVal(b.Extra, ExtraExecutionReason),
			SupersededBy:         supersededBy,
			EpicClosureCandidate: isEpicClosureCandidate(b, openChildCount[b.ID], childCount[b.ID]),
		}
		entry.Decision = EvaluateLifecycleQueue(LifecycleQueueFacts{
			Status:                 b.Status,
			Dependencies:           lifecycleDependencyStates(b, statusMap),
			ClaimFresh:             claimFresh,
			RetryCooldownActive:    retryActive,
			ExecutionEligible:      executionEligible,
			ExecutionEligibleKnown: executionEligibleKnown,
			SupersededBy:           supersededBy,
			EpicContainer:          isOrdinaryEpicContainer(b, openChildCount[b.ID], childCount[b.ID]),
			ExternalBlockerReason:  extraStringVal(b.Extra, ExtraLifecycleExternalBlockerReason),
			LegacyLabels:           b.Labels,
			PreservedReviewBlocked: preservedReviewBlocked(b),
		})
		entries = append(entries, entry)
	}
	return entries
}

func lifecycleDependencyStates(b Bead, statusMap map[string]string) []LifecycleDependencyState {
	depIDs := b.DepIDs()
	if len(depIDs) == 0 {
		return nil
	}
	deps := make([]LifecycleDependencyState, 0, len(depIDs))
	for _, depID := range depIDs {
		deps = append(deps, LifecycleDependencyState{Status: statusMap[depID]})
	}
	return deps
}

func unclosedDependencyIDs(b Bead, statusMap map[string]string) []string {
	var unclosed []string
	for _, depID := range b.DepIDs() {
		if !LifecycleStatusSatisfiesDependency(statusMap[depID]) {
			unclosed = append(unclosed, depID)
		}
	}
	return unclosed
}

// UnclosedBlockingDeps returns the sorted IDs of blocking dependencies of id
// whose status does not satisfy dependents (not closed). Returns an empty
// slice when every dependency is closed or the bead has no dependencies.
func (s *Store) UnclosedBlockingDeps(id string) ([]string, error) {
	if id == "" {
		return nil, fmt.Errorf("bead: UnclosedBlockingDeps requires bead id")
	}
	beads, err := s.ReadAll(context.Background())
	if err != nil {
		return nil, err
	}
	statusMap := make(map[string]string, len(beads))
	var target *Bead
	for i := range beads {
		statusMap[beads[i].ID] = beads[i].Status
		if beads[i].ID == id {
			target = &beads[i]
		}
	}
	if target == nil {
		return nil, fmt.Errorf("bead: not found: %s", id)
	}
	unclosed := unclosedDependencyIDs(*target, statusMap)
	sort.Strings(unclosed)
	if unclosed == nil {
		return []string{}, nil
	}
	return unclosed, nil
}

// rejectIfUnclosedBlockingDeps returns ErrDependencyGateRejected wrapped with
// the target bead ID and the open dependency IDs when any blocking dependency
// is still unclosed.
func (s *Store) rejectIfUnclosedBlockingDeps(id string) error {
	open, err := s.UnclosedBlockingDeps(id)
	if err != nil {
		return err
	}
	if len(open) == 0 {
		return nil
	}
	return fmt.Errorf("%w: bead %s: %s", ErrDependencyGateRejected, id, strings.Join(open, ", "))
}

// activeRetryCooldown returns the retry-after time and whether the cooldown is
// still active for bead b. When work-cooldown-base-rev is set and
// originHead has advanced past that rev (i.e., originHead != baseRev and both
// are non-empty), the cooldown auto-clears regardless of wall-clock expiry —
// the world-state that caused the cooldown has changed. See TD-031 §5 invariant.
// originHead should be the current origin/main HEAD, read once per
// ready-queue evaluation pass (not per bead).
func activeRetryCooldown(b Bead, now time.Time, originHead string) (time.Time, bool) {
	retryAfterStr := extraStringVal(b.Extra, ExtraRetryAfter)
	if retryAfterStr == "" {
		return time.Time{}, false
	}
	retryAfter, err := time.Parse(time.RFC3339, retryAfterStr)
	if err != nil {
		return time.Time{}, false
	}
	// Rev-bound auto-invalidation: if origin/main has advanced since this
	// cooldown was set, the fix may already be in and the cooldown should clear.
	baseRev := extraStringVal(b.Extra, ExtraCooldownBaseRev)
	if baseRev != "" && originHead != "" && originHead != baseRev {
		return time.Time{}, false
	}
	return retryAfter, retryAfter.After(now)
}

// preservedReviewBlocked reports whether bead b carries an unresolved
// preserved-needs-review block marker. A block stamped for attempt A only
// clears when an operator stamps a matching preserved-review-unblocked-attempt
// == A alongside a preserved-review-unblocked-at strictly after the block
// timestamp; a mismatched attempt or a stale/malformed unblock timestamp
// leaves the block active (ddx-ec1c1f89).
func preservedReviewBlocked(b Bead) bool {
	blockedAtStr := extraStringVal(b.Extra, ExtraPreservedReviewBlockedAt)
	if blockedAtStr == "" {
		return false
	}
	blockedAt, err := time.Parse(time.RFC3339, blockedAtStr)
	if err != nil {
		return false
	}
	unblockedAtStr := extraStringVal(b.Extra, ExtraPreservedReviewUnblockedAt)
	if unblockedAtStr == "" {
		return true
	}
	unblockedAt, err := time.Parse(time.RFC3339, unblockedAtStr)
	if err != nil {
		return true
	}
	if !unblockedAt.After(blockedAt) {
		return true
	}
	blockedAttempt := extraStringVal(b.Extra, ExtraPreservedReviewBlockedAttempt)
	unblockedAttempt := extraStringVal(b.Extra, ExtraPreservedReviewUnblockedAttempt)
	if blockedAttempt == "" || unblockedAttempt != blockedAttempt {
		return true
	}
	return false
}

func lifecycleExecutionEligible(b Bead) (bool, bool) {
	eligible, ok := b.Extra[ExtraExecutionElig]
	if !ok {
		return true, false
	}
	val, isBool := eligible.(bool)
	if !isBool {
		return true, false
	}
	return val, true
}

func lifecycleSupersededBy(b Bead) string {
	return strings.TrimSpace(extraStringVal(b.Extra, "superseded-by"))
}

// Ready returns open beads in the derived ready bucket, sorted by priority
// (0 = highest first).
func (s *Store) Ready() ([]Bead, error) {
	return s.readyFiltered(false, false)
}

// ReadyExecution returns ready beads that are also execution-eligible and
// not superseded. This is the filter HELIX uses for its build loop. It also
// surfaces stale in_progress claims so Claim can atomically reclaim them.
func (s *Store) ReadyExecution() ([]Bead, error) {
	return s.readyFiltered(true, false)
}

// ReadyExecutionIgnoringCooldown returns execution-ready beads while treating
// active retry-cooldown entries as immediately claimable for this read only.
// It does not mutate any cooldown metadata on the beads themselves.
func (s *Store) ReadyExecutionIgnoringCooldown() ([]Bead, error) {
	return s.readyFiltered(true, true)
}

// ProposedOperatorAttention returns operator-attention beads (status=proposed),
// sorted by queue order. The legacy needs_human label is explanatory metadata
// only and does not affect selection.
func (s *Store) ProposedOperatorAttention() ([]Bead, error) {
	beads, err := s.ReadAll(context.Background())
	if err != nil {
		return nil, err
	}
	var result []Bead
	for _, entry := range s.classifyLifecycleQueue(beads, time.Now().UTC()) {
		if entry.Decision.OperatorAttention {
			result = append(result, entry.Bead)
		}
	}
	sortBeadsForQueue(result)
	return result, nil
}

// NeedsHuman is a deprecated alias for ProposedOperatorAttention.
// Deprecated: use ProposedOperatorAttention instead.
func (s *Store) NeedsHuman() ([]Bead, error) {
	return s.ProposedOperatorAttention()
}

// ReadyExecutionBreakdown is the lifecycle-derived queue snapshot used by the
// worker when explaining an empty execution queue.
type ReadyExecutionBreakdown struct {
	ExecutionReady            []string
	DependencyWaiting         []string
	ProposedOperatorAttention []string
	RetryCooldown             []string
	ExternalBlocked           []string
	NotEligible               []string
	Superseded                []string
	Epics                     []string
	EpicClosureCandidates     []string
	NextRetryAfter            string
	HumanReviewBlockedTotal   int
	HumanReviewBlockers       []HumanReviewBlockerPressure
}

// HumanReviewBlockerPressure describes an open human-review blocker and the
// number of active downstream beads that transitively depend on it.
type HumanReviewBlockerPressure struct {
	ID                     string
	Title                  string
	Priority               int
	DownstreamBlockedCount int
}

func (s *Store) ReadyExecutionBreakdown() (ReadyExecutionBreakdown, error) {
	out := ReadyExecutionBreakdown{}
	beads, err := s.ReadAll(context.Background())
	if err != nil {
		return out, err
	}
	now := time.Now().UTC()
	var soonestRetry time.Time
	for _, entry := range s.classifyLifecycleQueue(beads, now) {
		switch entry.Decision.Bucket {
		case LifecycleBucketReady:
			out.ExecutionReady = append(out.ExecutionReady, entry.ID)
		case LifecycleBucketWaitingDependencies:
			out.DependencyWaiting = append(out.DependencyWaiting, entry.ID)
		case LifecycleBucketRetryCooldown:
			out.RetryCooldown = append(out.RetryCooldown, entry.ID)
			if !entry.RetryAfter.IsZero() && (soonestRetry.IsZero() || entry.RetryAfter.Before(soonestRetry)) {
				soonestRetry = entry.RetryAfter
			}
		case LifecycleBucketEpicContainer:
			if entry.EpicClosureCandidate {
				out.EpicClosureCandidates = append(out.EpicClosureCandidates, entry.ID)
			} else {
				out.Epics = append(out.Epics, entry.ID)
			}
		case LifecycleBucketNotEligible:
			out.NotEligible = append(out.NotEligible, entry.ID)
		case LifecycleBucketSuperseded:
			out.Superseded = append(out.Superseded, entry.ID)
		case LifecycleBucketBlockedExternal:
			out.ExternalBlocked = append(out.ExternalBlocked, entry.ID)
		case LifecycleBucketProposed:
			out.ProposedOperatorAttention = append(out.ProposedOperatorAttention, entry.ID)
		}
	}
	if !soonestRetry.IsZero() {
		out.NextRetryAfter = soonestRetry.Format(time.RFC3339)
	}
	out.HumanReviewBlockers, out.HumanReviewBlockedTotal = humanReviewBlockerPressure(beads)
	return out, nil
}

func humanReviewBlockerPressure(beads []Bead) ([]HumanReviewBlockerPressure, int) {
	byID := make(map[string]Bead, len(beads))
	reverseDeps := make(map[string][]string, len(beads))
	for _, b := range beads {
		byID[b.ID] = b
		if !activeForDependencyPressure(b) {
			continue
		}
		for _, dep := range b.Dependencies {
			if dep.DependsOnID == "" {
				continue
			}
			reverseDeps[dep.DependsOnID] = append(reverseDeps[dep.DependsOnID], b.ID)
		}
	}

	var blockers []HumanReviewBlockerPressure
	totalBlocked := map[string]struct{}{}
	for _, b := range beads {
		if !isHumanReviewBlocker(b) {
			continue
		}
		seen := map[string]struct{}{}
		var walk func(string)
		walk = func(id string) {
			for _, childID := range reverseDeps[id] {
				if childID == b.ID {
					continue
				}
				child, ok := byID[childID]
				if !ok || !activeForDependencyPressure(child) {
					continue
				}
				if _, exists := seen[childID]; exists {
					continue
				}
				seen[childID] = struct{}{}
				totalBlocked[childID] = struct{}{}
				walk(childID)
			}
		}
		walk(b.ID)
		if len(seen) == 0 {
			// A proposed/needs-human bead with no active downstream is operator
			// attention only; it does not contribute to HumanReviewBlockers.
			continue
		}
		blockers = append(blockers, HumanReviewBlockerPressure{
			ID:                     b.ID,
			Title:                  b.Title,
			Priority:               b.Priority,
			DownstreamBlockedCount: len(seen),
		})
	}

	sort.SliceStable(blockers, func(i, j int) bool {
		if blockers[i].Priority != blockers[j].Priority {
			return blockers[i].Priority < blockers[j].Priority
		}
		return blockers[i].ID < blockers[j].ID
	})
	return blockers, len(totalBlocked)
}

func isHumanReviewBlocker(b Bead) bool {
	// Post-lifecycle: proposed status is the canonical operator-attention signal.
	if b.Status == StatusProposed {
		return true
	}
	// Legacy: open beads with investigation/human-review labels still count.
	if b.Status != StatusOpen {
		return false
	}
	for _, label := range b.Labels {
		switch label {
		case LabelNeedsHuman, LabelNeedsInvestigation, LabelNoChangesUnverified, LabelNoChangesUnjustified:
			return true
		}
	}
	return false
}

func activeForDependencyPressure(b Bead) bool {
	switch b.Status {
	case StatusClosed, StatusCancelled:
		return false
	default:
		return true
	}
}

func (s *Store) readyFiltered(executionOnly, ignoreCooldown bool) ([]Bead, error) {
	beads, err := s.ReadAll(context.Background())
	if err != nil {
		return nil, err
	}

	var ready []Bead
	for _, entry := range s.classifyLifecycleQueue(beads, time.Now().UTC()) {
		switch entry.Decision.Bucket {
		case LifecycleBucketReady:
			ready = append(ready, entry.Bead)
		case LifecycleBucketRetryCooldown:
			if executionOnly && ignoreCooldown {
				ready = append(ready, entry.Bead)
			}
		}
	}

	sortBeadsForQueue(ready)

	return ready, nil
}

// Blocked returns open beads in the derived dependency-waiting bucket.
func (s *Store) Blocked() ([]Bead, error) {
	beads, err := s.ReadAll(context.Background())
	if err != nil {
		return nil, err
	}

	var blocked []Bead
	for _, entry := range s.classifyLifecycleQueue(beads, time.Now().UTC()) {
		// EvaluateLifecycleQueue is the authoritative classifier for queue buckets.
		// If a bead is in LifecycleBucketWaitingDependencies, it correctly reflects
		// all relevant state (status, dependencies, claims, etc.) without needing
		// additional secondary status filtering.
		if entry.Decision.Bucket == LifecycleBucketWaitingDependencies {
			blocked = append(blocked, entry.Bead)
		}
	}
	sortBeadsForQueue(blocked)
	return blocked, nil
}

// ExternalBlocked returns beads with status=blocked and an ExternalBlockerReason set.
// This is the new semantics for the /api/beads/blocked endpoint — explicit external
// blockers only, not dependency-waiting beads.
func (s *Store) ExternalBlocked() ([]Bead, error) {
	beads, err := s.ReadAll(context.Background())
	if err != nil {
		return nil, err
	}

	var result []Bead
	for _, entry := range s.classifyLifecycleQueue(beads, time.Now().UTC()) {
		if entry.Decision.Bucket == LifecycleBucketBlockedExternal {
			result = append(result, entry.Bead)
		}
	}
	sortBeadsForQueue(result)
	return result, nil
}

// DependencyWaiting returns open/in_progress beads with unmet dependencies.
// This is the semantics for /api/beads/dependency-waiting.
func (s *Store) DependencyWaiting() ([]Bead, error) {
	beads, err := s.ReadAll(context.Background())
	if err != nil {
		return nil, err
	}

	var result []Bead
	for _, entry := range s.classifyLifecycleQueue(beads, time.Now().UTC()) {
		if entry.Decision.Bucket == LifecycleBucketWaitingDependencies {
			result = append(result, entry.Bead)
		}
	}
	sortBeadsForQueue(result)
	return result, nil
}

// BlockedAll returns open beads that are currently not runnable, classified
// by blocker kind. Dependency-blocked beads are emitted first (any unclosed
// dep in their DAG); retry-parked beads whose work-retry-after is in
// the future are emitted with blocker kind BlockerKindRetryCooldown. A bead
// that is both dep-blocked and cooldown-parked is reported as dependency-
// blocked, because deps are the stronger blocker.
func (s *Store) BlockedAll() ([]BlockedBead, error) {
	beads, err := s.ReadAll(context.Background())
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var entries []BlockedBead
	for _, entry := range s.classifyLifecycleQueue(beads, now) {
		switch entry.Decision.Bucket {
		case LifecycleBucketWaitingDependencies:
			entries = append(entries, BlockedBead{
				Bead: entry.Bead,
				Blocker: Blocker{
					Kind:           BlockerKindDependency,
					UnclosedDepIDs: entry.UnclosedDepIDs,
				},
			})
		case LifecycleBucketBlockedExternal:
			reason := entry.Decision.ExternalBlockerReason
			if reason == "" && len(entry.Decision.Issues) > 0 {
				reason = entry.Decision.Issues[0].Detail
			}
			entries = append(entries, BlockedBead{
				Bead: entry.Bead,
				Blocker: Blocker{
					Kind:   BlockerKindBlockedStatus,
					Reason: reason,
				},
			})
		case LifecycleBucketProposed:
			entries = append(entries, BlockedBead{
				Bead: entry.Bead,
				Blocker: Blocker{
					Kind:   BlockerKindOperatorAttention,
					Reason: "status=proposed requires operator attention",
				},
			})
		case LifecycleBucketEpicContainer:
			entries = append(entries, BlockedBead{
				Bead: entry.Bead,
				Blocker: Blocker{
					Kind:   BlockerKindEpicOnly,
					Reason: "ordinary ddx work skips epic/container beads",
				},
			})
		case LifecycleBucketNotEligible:
			entries = append(entries, BlockedBead{
				Bead: entry.Bead,
				Blocker: Blocker{
					Kind:   BlockerKindNotEligible,
					Reason: entry.ExecutionSkipReason,
				},
			})
		case LifecycleBucketSuperseded:
			entries = append(entries, BlockedBead{
				Bead: entry.Bead,
				Blocker: Blocker{
					Kind:   BlockerKindSuperseded,
					Reason: entry.SupersededBy,
				},
			})
		case LifecycleBucketRetryCooldown:
			entries = append(entries, BlockedBead{
				Bead: entry.Bead,
				Blocker: Blocker{
					Kind:           BlockerKindRetryCooldown,
					NextEligibleAt: entry.RetryAfter.UTC().Format(time.RFC3339),
					LastStatus:     entry.LastStatus,
					LastDetail:     entry.LastDetail,
				},
			})
		}
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

// isOrdinaryEpicContainer reports whether bead b should be classified as an
// epic container (LifecycleBucketEpicContainer). It takes openChildCount and
// totalChildCount as separate signals so callers can distinguish three cases:
//
//   - openChildCount > 0: live container gating execution (caller routes to
//     LifecycleBucketEpicContainer).
//   - openChildCount == 0 && totalChildCount > 0: closure candidate (still a
//     container; isEpicClosureCandidate returns true).
//   - totalChildCount == 0: genuinely-undecomposed epic that must not enter
//     the container bucket so the work-loop layer can diagnose it separately.
func isOrdinaryEpicContainer(b Bead, openChildCount, totalChildCount int) bool {
	issueType := strings.ToLower(strings.TrimSpace(b.IssueType))
	title := strings.ToLower(strings.TrimSpace(b.Title))
	explicitEpic := issueType == "epic" || strings.HasPrefix(title, "epic:")
	looseEpic := strings.HasPrefix(title, "epic ") && totalChildCount > 0
	if !explicitEpic && !looseEpic {
		return false
	}
	return totalChildCount > 0
}

func isEpicClosureCandidate(b Bead, openChildCount, totalChildCount int) bool {
	if totalChildCount == 0 {
		return false
	}
	return isOrdinaryEpicContainer(b, openChildCount, totalChildCount) && openChildCount == 0
}

func sortBeadsForQueue(beads []Bead) {
	sort.SliceStable(beads, func(i, j int) bool {
		if beads[i].Priority != beads[j].Priority {
			return beads[i].Priority < beads[j].Priority
		}
		ir, iok := QueueRank(beads[i].Extra)
		jr, jok := QueueRank(beads[j].Extra)
		if iok != jok {
			return iok
		}
		if iok && jok && ir != jr {
			return ir < jr
		}
		if !beads[i].CreatedAt.Equal(beads[j].CreatedAt) {
			return beads[i].CreatedAt.Before(beads[j].CreatedAt)
		}
		return beads[i].ID < beads[j].ID
	})
}

func parseQueueRank(raw any) (int, bool) {
	switch v := raw.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return int(v), true
	case float32:
		return int(v), v == float32(int(v))
	case float64:
		return int(v), v == float64(int(v))
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n), true
		}
		if f, err := v.Float64(); err == nil {
			return int(f), f == float64(int(f))
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n, true
		}
	}
	return 0, false
}

// Status returns aggregate counts.
func (s *Store) Status() (*StatusCounts, error) {
	beads, err := s.ReadAll(context.Background())
	if err != nil {
		return nil, err
	}
	ready, err := s.Ready()
	if err != nil {
		return nil, err
	}
	workerReady, err := s.ReadyExecution()
	if err != nil {
		return nil, err
	}
	needsHuman, err := s.ProposedOperatorAttention()
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
		if archived, aerr := archive.ReadAll(context.Background()); aerr == nil {
			for _, b := range archived {
				if seen[b.ID] {
					continue
				}
				seen[b.ID] = true
				all = append(all, b)
			}
		}
	}

	counts := &StatusCounts{
		Total:             len(all),
		Ready:             len(ready),
		WorkerReady:       len(workerReady),
		NeedsHuman:        len(needsHuman),
		OperatorAttention: len(needsHuman),
	}
	for _, b := range all {
		switch b.Status {
		case StatusOpen:
			counts.Open++
		case StatusInProgress:
			counts.InProgress++
		case StatusClosed:
			counts.Closed++
		case StatusBlocked:
			counts.Blocked++
		case StatusProposed:
			counts.Proposed++
		case StatusCancelled:
			counts.Cancelled++
		}
	}
	for _, entry := range s.classifyLifecycleQueue(beads, time.Now().UTC()) {
		switch entry.Decision.Bucket {
		case LifecycleBucketWaitingDependencies:
			counts.DependencyWaiting++
		case LifecycleBucketBlockedExternal:
			counts.ExternalBlocked++
		}
	}
	return counts, nil
}

// DepAdd adds a dependency: id depends on depID.
func (s *Store) DepAdd(ctx context.Context, id, depID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.WithLock(func() error {
		beads, _, err := s.readAllLatestRaw()
		if err != nil {
			return err
		}
		byID := make(map[string]*Bead, len(beads))
		var target *Bead
		depExists := false
		for i := range beads {
			e := &beads[i]
			byID[e.ID] = e
			if e.ID == id {
				target = e
			}
			if e.ID == depID {
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
		parentChain, err := beadParentChain(byID, target.Parent)
		if err != nil {
			return err
		}
		if err := rejectAncestorDependency(id, depID, parentChain); err != nil {
			return err
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
		return s.writeAllLocked(beads)
	})
}

// DepRemove removes a dependency.
func (s *Store) DepRemove(ctx context.Context, id, depID string) error {
	return s.Update(ctx, id, func(b *Bead) {
		b.RemoveDep(depID)
	})
}

// DepTree returns a text representation of the dependency tree.
func (s *Store) DepTree(ctx context.Context, rootID string) (string, error) {
	beads, err := s.ReadAll(ctx)
	if err != nil {
		return "", err
	}
	// Pull in archive partner so the tree survives `bead migrate` — closed
	// beads stored only in the archive must still appear.
	if s.Collection == DefaultCollection {
		archive := s.archivePartner()
		if archived, aerr := archive.ReadAll(ctx); aerr == nil {
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

// beadParentChain returns the chain of parent IDs starting at parentID and
// walking upward through stored parent links. The returned chain excludes the
// child bead itself and includes the offending ancestor if one is found.
func beadParentChain(byID map[string]*Bead, parentID string) ([]string, error) {
	if parentID == "" {
		return nil, nil
	}
	chain := make([]string, 0, 4)
	seen := make(map[string]bool)
	current := parentID
	for current != "" {
		if seen[current] {
			chain = append(chain, current)
			return chain, fmt.Errorf("bead: parent cycle detected: %s", strings.Join(chain, " -> "))
		}
		seen[current] = true
		chain = append(chain, current)
		next, ok := byID[current]
		if !ok || next.Parent == "" {
			break
		}
		current = next.Parent
	}
	return chain, nil
}

// rejectAncestorDependency rejects dependency edges that point at a bead in
// the supplied parent chain.
func rejectAncestorDependency(childID, depID string, parentChain []string) error {
	for _, ancestorID := range parentChain {
		if ancestorID == depID {
			return fmt.Errorf("bead: dependency %s is an ancestor in the parent chain for %s: %s", depID, childID, strings.Join(parentChain, " -> "))
		}
	}
	return nil
}

// validateBead checks core invariants that must hold for any bead (create or update).
func (s *Store) validateBead(b *Bead) error {
	if strings.TrimSpace(b.Title) == "" {
		return fmt.Errorf("bead: title is required")
	}
	if b.Priority < MinPriority || b.Priority > MaxPriority {
		return fmt.Errorf("bead: priority must be %d-%d, got %d", MinPriority, MaxPriority, b.Priority)
	}
	if !IsCanonicalStatus(b.Status) {
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

// workingDir returns the project root for git operations. When Dir is the
// standard .ddx directory, the parent is the project root; otherwise Dir is
// used directly.
func (s *Store) workingDir() string {
	if filepath.Base(s.Dir) == ".ddx" {
		return filepath.Dir(s.Dir)
	}
	return s.Dir
}

// originMainHead returns the current origin/main HEAD SHA, or "" if the git
// command fails (e.g., no remote, detached, offline). Used once per
// ready-queue evaluation pass for rev-bound cooldown auto-invalidation.
func originMainHead(workingDir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := gitpkg.Command(ctx, workingDir, "rev-parse", "origin/main")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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

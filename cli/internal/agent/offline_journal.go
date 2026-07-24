package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

const (
	// offlineJournalRelDir is the project-scoped coordination directory under
	// the DDx state root that holds the offline mutation journal.
	offlineJournalRelDir = "coordination"
	// offlineJournalFileName is the durable ordered journal file (JSONL).
	offlineJournalFileName = "offline-journal.jsonl"
	// offlineJournalAckFileName is the durable acknowledged-through cursor file.
	// Stores the highest contiguous sequence number that has been acknowledged
	// (e.g. by server reconcile). Separate from the JSONL so ack advances do
	// not rewrite mutation records (physical compaction is a later step).
	offlineJournalAckFileName = "offline-journal.ack"
)

// OfflineJournalRecord is one durable offline coordination mutation entry
// (ADR-022 rev 6 offline coordination journal). Sequence is assigned by the
// journal on Append; remaining fields are caller-supplied mutation metadata
// required for later reconciliation before online writes.
type OfflineJournalRecord struct {
	// Sequence is the monotonic journal order (1-based). Assigned by Append.
	Sequence uint64 `json:"sequence"`
	// Operation is the coordination operation name (claim, land, ...). Optional
	// at the append API layer; preserved when set for later reconcile wiring.
	Operation string `json:"operation,omitempty"`
	// IdempotencyKey uniquely identifies this mutation for replay safety.
	IdempotencyKey string `json:"idempotency_key"`
	// PayloadHash is a hash of the request payload used for conflict detection.
	PayloadHash string `json:"payload_hash"`
	// Precondition captures the pre-apply durable version / expected state.
	Precondition string `json:"precondition,omitempty"`
	// Outcome captures the post-apply observed result (applied, conflict, ...).
	Outcome string `json:"outcome,omitempty"`
	// RecordedAt is when the record was persisted (UTC).
	RecordedAt time.Time `json:"recorded_at,omitempty"`
}

// OfflineJournalAppend is the caller-supplied mutation metadata for Append.
// Sequence and RecordedAt are assigned by the journal.
type OfflineJournalAppend struct {
	Operation      string
	IdempotencyKey string
	PayloadHash    string
	Precondition   string
	Outcome        string
}

// OfflineJournal is a project-scoped durable ordered offline coordination
// journal. Create/open via OpenOfflineJournal; append with Append; mark
// reconcile progress with AcknowledgeThrough; list unacknowledged mutations
// with ListPending; release with Close. Reopening continues sequence
// numbering after the highest persisted sequence and reloads the durable
// acknowledged-through cursor.
//
// Callers that also serialize mutations with OfflineCoordinator should hold
// that project lock around Append and AcknowledgeThrough. This type owns
// durable append, sequence assignment, and ack cursor state only; it does
// not acquire the offline coordination lock.
type OfflineJournal struct {
	projectRoot string
	path        string
	ackPath     string

	mu           sync.Mutex
	file         *os.File
	nextSeq      uint64
	ackedThrough uint64
	closed       bool
}

// OfflineJournalPath returns the absolute path of the project-scoped offline
// coordination journal file under the DDx state root.
func OfflineJournalPath(projectRoot string) string {
	return ddxroot.JoinProject(projectRoot, offlineJournalRelDir, offlineJournalFileName)
}

// OfflineJournalAckPath returns the absolute path of the durable
// acknowledged-through cursor file for the project-scoped offline journal.
func OfflineJournalAckPath(projectRoot string) string {
	return ddxroot.JoinProject(projectRoot, offlineJournalRelDir, offlineJournalAckFileName)
}

// OpenOfflineJournal creates or opens the durable offline journal for
// projectRoot. Sequence numbering starts at 1 for an empty journal, or one
// past the highest persisted sequence when reopening an existing file. The
// durable acknowledged-through cursor is loaded when present (default 0).
func OpenOfflineJournal(projectRoot string) (*OfflineJournal, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil, fmt.Errorf("offline journal: project root is empty")
	}

	path := OfflineJournalPath(projectRoot)
	ackPath := OfflineJournalAckPath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("offline journal: mkdir: %w", err)
	}

	// Read existing records before opening for append so we know the next
	// sequence. Missing file is treated as empty (highest=0).
	highest, err := highestPersistedSequence(path)
	if err != nil {
		return nil, err
	}

	acked, err := loadAcknowledgedThrough(ackPath)
	if err != nil {
		return nil, err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("offline journal: open: %w", err)
	}

	return &OfflineJournal{
		projectRoot:  projectRoot,
		path:         path,
		ackPath:      ackPath,
		file:         f,
		nextSeq:      highest + 1,
		ackedThrough: acked,
	}, nil
}

// Path returns the absolute journal file path.
func (j *OfflineJournal) Path() string {
	if j == nil {
		return ""
	}
	return j.path
}

// NextSequence returns the sequence number that the next Append will assign.
// Useful for diagnostics; not required for normal append use.
func (j *OfflineJournal) NextSequence() uint64 {
	if j == nil {
		return 0
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return 0
	}
	return j.nextSeq
}

// Append persists one mutation record with a monotonic sequence number.
// The returned record includes the assigned Sequence and RecordedAt.
// The write is fsync'd before return so a process crash after Append returns
// cannot lose the record or its sequence ordering.
func (j *OfflineJournal) Append(in OfflineJournalAppend) (OfflineJournalRecord, error) {
	if j == nil {
		return OfflineJournalRecord{}, fmt.Errorf("offline journal: nil receiver")
	}

	key := strings.TrimSpace(in.IdempotencyKey)
	hash := strings.TrimSpace(in.PayloadHash)
	if key == "" {
		return OfflineJournalRecord{}, fmt.Errorf("offline journal: idempotency_key is required")
	}
	if hash == "" {
		return OfflineJournalRecord{}, fmt.Errorf("offline journal: payload_hash is required")
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed || j.file == nil {
		return OfflineJournalRecord{}, fmt.Errorf("offline journal: closed")
	}

	rec := OfflineJournalRecord{
		Sequence:       j.nextSeq,
		Operation:      strings.TrimSpace(in.Operation),
		IdempotencyKey: key,
		PayloadHash:    hash,
		Precondition:   in.Precondition,
		Outcome:        in.Outcome,
		RecordedAt:     time.Now().UTC(),
	}

	data, err := json.Marshal(rec)
	if err != nil {
		return OfflineJournalRecord{}, fmt.Errorf("offline journal: marshal: %w", err)
	}
	data = append(data, '\n')

	if _, err := j.file.Write(data); err != nil {
		return OfflineJournalRecord{}, fmt.Errorf("offline journal: write: %w", err)
	}
	if err := j.file.Sync(); err != nil {
		return OfflineJournalRecord{}, fmt.Errorf("offline journal: sync: %w", err)
	}

	j.nextSeq++
	return rec, nil
}

// Close releases the open journal file. After Close, Append and
// AcknowledgeThrough fail until the journal is reopened via OpenOfflineJournal.
func (j *OfflineJournal) Close() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	if j.file == nil {
		return nil
	}
	err := j.file.Close()
	j.file = nil
	if err != nil {
		return fmt.Errorf("offline journal: close: %w", err)
	}
	return nil
}

// AcknowledgedThrough returns the highest contiguous sequence number that has
// been durably acknowledged. Zero means no records have been acknowledged yet.
// Survives close/reopen via the ack cursor file.
func (j *OfflineJournal) AcknowledgedThrough() uint64 {
	if j == nil {
		return 0
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.ackedThrough
}

// AcknowledgeThrough durably advances the acknowledged-through cursor to seq
// when seq is greater than the current cursor. Seq is the highest contiguous
// journal sequence that reconcile (or an equivalent ack source) has accepted.
// A lower or equal seq is a no-op success so retries are safe. The cursor is
// fsync'd before return so a crash after AcknowledgeThrough returns cannot
// lose the acknowledgement.
func (j *OfflineJournal) AcknowledgeThrough(seq uint64) error {
	if j == nil {
		return fmt.Errorf("offline journal: nil receiver")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return fmt.Errorf("offline journal: closed")
	}
	if seq <= j.ackedThrough {
		return nil
	}
	if err := persistAcknowledgedThrough(j.ackPath, seq); err != nil {
		return err
	}
	j.ackedThrough = seq
	return nil
}

// ListPending returns durable journal records with Sequence greater than the
// current acknowledged-through cursor, in original sequence order. Acknowledged
// records are skipped. Missing journal files yield an empty slice.
func (j *OfflineJournal) ListPending() ([]OfflineJournalRecord, error) {
	if j == nil {
		return nil, fmt.Errorf("offline journal: nil receiver")
	}
	j.mu.Lock()
	acked := j.ackedThrough
	path := j.path
	projectRoot := j.projectRoot
	j.mu.Unlock()

	if path == "" {
		path = OfflineJournalPath(projectRoot)
	}
	return filterPendingRecords(path, acked)
}

// LoadOfflineJournalRecords reads all durable records from the project-scoped
// journal file. Missing files yield an empty slice. Malformed lines are skipped
// so a partially written trailing line after a crash does not block resume.
func LoadOfflineJournalRecords(projectRoot string) ([]OfflineJournalRecord, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil, fmt.Errorf("offline journal: project root is empty")
	}
	return loadOfflineJournalRecords(OfflineJournalPath(projectRoot))
}

// LoadOfflineJournalPending reads durable journal records that are not yet
// acknowledged (Sequence > acknowledged-through), in original order. Uses the
// project-scoped ack cursor file; missing files yield an empty pending set.
func LoadOfflineJournalPending(projectRoot string) ([]OfflineJournalRecord, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil, fmt.Errorf("offline journal: project root is empty")
	}
	acked, err := loadAcknowledgedThrough(OfflineJournalAckPath(projectRoot))
	if err != nil {
		return nil, err
	}
	return filterPendingRecords(OfflineJournalPath(projectRoot), acked)
}

// LoadOfflineJournalAcknowledgedThrough returns the durable acknowledged-through
// cursor for projectRoot without opening the journal for append.
func LoadOfflineJournalAcknowledgedThrough(projectRoot string) (uint64, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return 0, fmt.Errorf("offline journal: project root is empty")
	}
	return loadAcknowledgedThrough(OfflineJournalAckPath(projectRoot))
}

func loadOfflineJournalRecords(path string) ([]OfflineJournalRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []OfflineJournalRecord{}, nil
		}
		return nil, fmt.Errorf("offline journal: open for read: %w", err)
	}
	defer f.Close() //nolint:errcheck

	var out []OfflineJournalRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec OfflineJournalRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec.Sequence == 0 {
			continue
		}
		out = append(out, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("offline journal: scan: %w", err)
	}
	return out, nil
}

func highestPersistedSequence(path string) (uint64, error) {
	recs, err := loadOfflineJournalRecords(path)
	if err != nil {
		return 0, err
	}
	var highest uint64
	for _, rec := range recs {
		if rec.Sequence > highest {
			highest = rec.Sequence
		}
	}
	return highest, nil
}

func filterPendingRecords(path string, ackedThrough uint64) ([]OfflineJournalRecord, error) {
	recs, err := loadOfflineJournalRecords(path)
	if err != nil {
		return nil, err
	}
	out := make([]OfflineJournalRecord, 0, len(recs))
	for _, rec := range recs {
		if rec.Sequence > ackedThrough {
			out = append(out, rec)
		}
	}
	return out, nil
}

// offlineJournalAckState is the durable on-disk form of the acknowledged-through
// cursor. Sequence 0 means nothing acknowledged yet.
type offlineJournalAckState struct {
	AcknowledgedThrough uint64 `json:"acknowledged_through"`
}

func loadAcknowledgedThrough(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("offline journal: read ack: %w", err)
	}
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		return 0, nil
	}
	var state offlineJournalAckState
	if err := json.Unmarshal(data, &state); err != nil {
		return 0, fmt.Errorf("offline journal: parse ack: %w", err)
	}
	return state.AcknowledgedThrough, nil
}

// persistAcknowledgedThrough writes the ack cursor atomically (tmp + fsync + rename)
// so a crash cannot leave a partially written cursor visible.
func persistAcknowledgedThrough(path string, seq uint64) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("offline journal: mkdir ack: %w", err)
	}
	data, err := json.Marshal(offlineJournalAckState{AcknowledgedThrough: seq})
	if err != nil {
		return fmt.Errorf("offline journal: marshal ack: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "offline-journal-ack-*.tmp")
	if err != nil {
		return fmt.Errorf("offline journal: create ack tmp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("offline journal: write ack tmp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("offline journal: sync ack tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("offline journal: close ack tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("offline journal: rename ack: %w", err)
	}
	return nil
}

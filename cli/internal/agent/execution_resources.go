package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"golang.org/x/sys/unix"
)

const (
	executionResourceMinFreeBytes  uint64 = 64 << 20
	executionResourceMinFreeInodes uint64 = 1024

	executionResourceSoftMinFreeBytes  uint64 = 512 << 20
	executionResourceSoftMinFreeInodes uint64 = 4096

	// defaultTopInodeConsumerLimit caps how many immediate children of a
	// failing root are reported as top inode consumers.
	defaultTopInodeConsumerLimit = 8
	// defaultTopInodeEntriesPerChild caps how many immediate entries are
	// counted inside each child directory. Nested trees are never walked.
	defaultTopInodeEntriesPerChild = 4096

	// claimLivenessDiagnosticPrefix matches the claim-liveness namespace
	// (bead.claimLivenessNamespace) so operators can identify
	// ddx-claim-heartbeats consumers without treating them as scratch
	// cleanup targets.
	claimLivenessDiagnosticPrefix = "ddx-claim-heartbeats"
)

// ExecutionTopInodeConsumer is one immediate child of a resource root ranked
// by entry/inode consumption for operator diagnostics. Fields are paths and
// counts only; file contents are never read or reported.
type ExecutionTopInodeConsumer struct {
	Path             string    `json:"path"`
	EntryCount       int64     `json:"entry_count"`
	Bytes            int64     `json:"bytes,omitempty"`
	ModTime          time.Time `json:"mod_time,omitempty"`
	AgeSeconds       int64     `json:"age_seconds,omitempty"`
	CleanupPrefix    string    `json:"cleanup_prefix,omitempty"`
	MatchesCleanup   bool      `json:"matches_cleanup,omitempty"`
	EntriesTruncated bool      `json:"entries_truncated,omitempty"`
}

// ExecutionResourceRootCheck captures the health of one execution root.
type ExecutionResourceRootCheck struct {
	Path           string   `json:"path"`
	Writable       bool     `json:"writable"`
	WritableReason string   `json:"writable_reason,omitempty"`
	BytesFree      uint64   `json:"bytes_free,omitempty"`
	InodesFree     uint64   `json:"inodes_free,omitempty"`
	Notes          []string `json:"notes,omitempty"`

	// FDExhausted is set when the writability probe failed because the
	// process or host hit its open-file-descriptor limit (EMFILE/ENFILE),
	// not because the root is permission- or filesystem-unwritable.
	FDExhausted bool     `json:"fd_exhausted,omitempty"`
	FDCount     int      `json:"fd_count,omitempty"`
	FDSoftLimit uint64   `json:"fd_soft_limit,omitempty"`
	FDHardLimit uint64   `json:"fd_hard_limit,omitempty"`
	FDSample    []string `json:"fd_sample,omitempty"`

	// TopInodeConsumers is a bounded ranking of immediate children by
	// entry/inode count. Populated by callers that run scanTopInodeConsumers
	// on a failing root; empty when the diagnostic was not collected.
	TopInodeConsumers []ExecutionTopInodeConsumer `json:"top_inode_consumers,omitempty"`
	// TopInodeConsumersTruncated is true when the root had more immediate
	// children than the consumer report cap.
	TopInodeConsumersTruncated bool `json:"top_inode_consumers_truncated,omitempty"`
}

// ExecutionResourceCheckResult captures the roots and cleanup summary observed
// during one resource preflight.
type ExecutionResourceCheckResult struct {
	ProjectRoot      string                       `json:"project_root"`
	TempRoot         string                       `json:"temp_root"`
	EvidenceRoots    []string                     `json:"evidence_roots,omitempty"`
	BeforeRootChecks []ExecutionResourceRootCheck `json:"before_root_checks,omitempty"`
	RootChecks       []ExecutionResourceRootCheck `json:"root_checks,omitempty"`
	CleanupSummary   ExecutionCleanupSummary      `json:"cleanup_summary,omitempty"`
}

// FDExhausted reports whether any root check recorded fd exhaustion
// (EMFILE/ENFILE) rather than a genuinely unwritable or capacity-exhausted
// root. Callers use this to distinguish a worker-local, restartable failure
// from root-storage exhaustion that persists across worker restarts.
func (r ExecutionResourceCheckResult) FDExhausted() bool {
	for _, check := range r.RootChecks {
		if check.FDExhausted {
			return true
		}
	}
	for _, check := range r.BeforeRootChecks {
		if check.FDExhausted {
			return true
		}
	}
	return false
}

// ResourceExhaustedError signals that execution roots remained unhealthy after
// a cleanup retry. The caller should stop claiming new work.
type ResourceExhaustedError struct {
	Detail string
	Result ExecutionResourceCheckResult
}

func (e *ResourceExhaustedError) Error() string {
	if e == nil {
		return "resource_exhausted"
	}
	if strings.TrimSpace(e.Detail) == "" {
		return "resource_exhausted"
	}
	return "resource_exhausted: " + e.Detail
}

type executionCleanupRunner interface {
	Cleanup(ctx context.Context) (ExecutionCleanupSummary, error)
}

// ExecutionResourceChecker validates DDx execution roots before claim or
// worktree creation. It is safe for tests to override RootProbe and
// CleanupRunner to simulate low-space or cleanup-recovery scenarios.
type ExecutionResourceChecker interface {
	Check(ctx context.Context) (ExecutionResourceCheckResult, error)
}

// ExecutionResourcePreflight is the default checker used by ddx try/work.
type ExecutionResourcePreflight struct {
	ProjectRoot   string
	TempRoot      string
	EvidenceRoots []string
	GitOps        GitOps

	// SoftMin* triggers a cleanup pass before claims when free capacity drops
	// below the soft floor. HardMin* is the stop floor after cleanup.
	SoftMinFreeBytes  uint64
	SoftMinFreeInodes uint64
	HardMinFreeBytes  uint64
	HardMinFreeInodes uint64

	CleanupRunner executionCleanupRunner
	RootProbe     func(path string) (ExecutionResourceRootCheck, error)
}

// NewExecutionResourceChecker constructs the default preflight checker.
func NewExecutionResourceChecker(projectRoot string, gitOps GitOps) *ExecutionResourcePreflight {
	return &ExecutionResourcePreflight{
		ProjectRoot: projectRoot,
		TempRoot:    executionCleanupTempRoot(projectRoot),
		EvidenceRoots: []string{
			executeBeadArtifactRoot(projectRoot), ddxroot.JoinProject(projectRoot, "runs"),
			bead.ClaimLivenessRoot(ddxroot.JoinProject(projectRoot)),
		},
		GitOps:        gitOps,
		CleanupRunner: NewExecutionCleanupManager(projectRoot, gitOps),
		RootProbe:     probeExecutionRoot,
	}
}

func (p *ExecutionResourcePreflight) Check(ctx context.Context) (ExecutionResourceCheckResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if p == nil {
		return ExecutionResourceCheckResult{}, nil
	}

	result := ExecutionResourceCheckResult{
		ProjectRoot:   p.projectRoot(),
		TempRoot:      p.tempRoot(),
		EvidenceRoots: p.evidenceRoots(),
	}

	checks, detail, healthy, softDetail, softHealthy := p.checkRoots()
	result.RootChecks = checks
	if healthy && softHealthy {
		return result, nil
	}
	if detail == "" {
		detail = softDetail
	}
	result.BeforeRootChecks = append([]ExecutionResourceRootCheck(nil), checks...)

	if p.CleanupRunner != nil {
		summary, cleanupErr := p.CleanupRunner.Cleanup(ctx)
		result.CleanupSummary = summary
		if cleanupErr != nil {
			if detail != "" {
				detail += "; "
			}
			detail += "cleanup: " + cleanupErr.Error()
		}
	}

	checks, recheckDetail, recheckHealthy, _, _ := p.checkRoots()
	result.RootChecks = checks
	if recheckHealthy {
		return result, nil
	}
	if recheckDetail != "" {
		detail = recheckDetail
	}
	return result, &ResourceExhaustedError{Detail: detail, Result: result}
}

func (p *ExecutionResourcePreflight) projectRoot() string {
	if p == nil {
		return ""
	}
	return p.ProjectRoot
}

func (p *ExecutionResourcePreflight) tempRoot() string {
	if p == nil {
		return executionCleanupTempRoot("")
	}
	if p.TempRoot != "" {
		return p.TempRoot
	}
	return executionCleanupTempRoot(p.ProjectRoot)
}

func (p *ExecutionResourcePreflight) evidenceRoots() []string {
	if p == nil || len(p.EvidenceRoots) == 0 {
		return nil
	}
	return append([]string(nil), p.EvidenceRoots...)
}

func (p *ExecutionResourcePreflight) allRoots() []string {
	roots := []string{p.tempRoot()}
	roots = append(roots, p.evidenceRoots()...)
	return roots
}

func (p *ExecutionResourcePreflight) checkRoots() ([]ExecutionResourceRootCheck, string, bool, string, bool) {
	roots := p.allRoots()
	checks := make([]ExecutionResourceRootCheck, 0, len(roots))
	var details, softDetails []string
	healthy := true
	softHealthy := true
	for _, root := range roots {
		check, err := p.checkRoot(root)
		if err != nil {
			healthy = false
			details = append(details, err.Error())
		} else if softNotes := p.softPressureNotes(check); len(softNotes) > 0 {
			softHealthy = false
			check.Notes = append(check.Notes, softNotes...)
			// Soft inode pressure: collect bounded top-inode consumers before
			// cleanup so operators can see which children are consuming inodes.
			if p.inodeSoftPressure(check) {
				applyTopInodeConsumerDiagnostics(&check)
			}
			for _, note := range softNotes {
				softDetails = append(softDetails, fmt.Sprintf("resource preflight: %s: %s", root, note))
			}
		}
		checks = append(checks, check)
	}
	return checks, strings.Join(details, "; "), healthy, strings.Join(softDetails, "; "), softHealthy
}

func (p *ExecutionResourcePreflight) checkRoot(root string) (ExecutionResourceRootCheck, error) {
	check := ExecutionResourceRootCheck{Path: root}
	if strings.TrimSpace(root) == "" {
		check.Notes = append(check.Notes, "empty root")
		return check, fmt.Errorf("resource preflight: empty root")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		check.Notes = append(check.Notes, "mkdir: "+err.Error())
		return check, fmt.Errorf("resource preflight: %s: mkdir: %w", root, err)
	}

	writable, writableReason, fdExhausted := probeWritableRoot(root)
	check.Writable = writable
	check.WritableReason = writableReason
	if !writable {
		check.Notes = append(check.Notes, writableReason)
		if fdExhausted {
			applyFDDiagnostics(&check)
		}
		return check, fmt.Errorf("resource preflight: %s: %s", root, writableReason)
	}

	if p.RootProbe != nil {
		probed, err := p.RootProbe(root)
		if err != nil {
			check.Notes = append(check.Notes, err.Error())
			return check, err
		}
		check.BytesFree = probed.BytesFree
		check.InodesFree = probed.InodesFree
		check.Notes = append(check.Notes, probed.Notes...)
		if !probed.Writable {
			msg := probed.WritableReason
			if msg == "" {
				msg = "root probe reported unwritable"
			}
			check.Notes = append(check.Notes, msg)
			return check, fmt.Errorf("resource preflight: %s: %s", root, msg)
		}
		if probed.BytesFree > 0 && probed.BytesFree < p.hardMinFreeBytes() {
			msg := fmt.Sprintf("free bytes %d < required %d", probed.BytesFree, p.hardMinFreeBytes())
			check.Notes = append(check.Notes, msg)
			return check, fmt.Errorf("resource preflight: %s: %s", root, msg)
		}
		if probed.InodesFree > 0 && probed.InodesFree < p.hardMinFreeInodes() {
			msg := fmt.Sprintf("free inodes %d < required %d", probed.InodesFree, p.hardMinFreeInodes())
			check.Notes = append(check.Notes, msg)
			applyTopInodeConsumerDiagnostics(&check)
			return check, fmt.Errorf("resource preflight: %s: %s", root, msg)
		}
		return check, nil
	}

	bytesFree, inodesFree, err := probeRootCapacity(root)
	if err != nil {
		check.Notes = append(check.Notes, err.Error())
		return check, err
	}
	check.BytesFree = bytesFree
	check.InodesFree = inodesFree
	if bytesFree > 0 && bytesFree < p.hardMinFreeBytes() {
		msg := fmt.Sprintf("free bytes %d < required %d", bytesFree, p.hardMinFreeBytes())
		check.Notes = append(check.Notes, msg)
		return check, fmt.Errorf("resource preflight: %s: %s", root, msg)
	}
	if inodesFree > 0 && inodesFree < p.hardMinFreeInodes() {
		msg := fmt.Sprintf("free inodes %d < required %d", inodesFree, p.hardMinFreeInodes())
		check.Notes = append(check.Notes, msg)
		applyTopInodeConsumerDiagnostics(&check)
		return check, fmt.Errorf("resource preflight: %s: %s", root, msg)
	}
	return check, nil
}

// inodeSoftPressure reports whether free inodes are below the soft cleanup
// threshold (InodesFree == 0 is treated as unavailable, not pressure).
func (p *ExecutionResourcePreflight) inodeSoftPressure(check ExecutionResourceRootCheck) bool {
	min := p.softMinFreeInodes()
	return min > 0 && check.InodesFree > 0 && check.InodesFree < min
}

// applyTopInodeConsumerDiagnostics attaches a bounded ranking of immediate
// children by entry count to check. Best-effort: scan errors become notes and
// do not replace the original resource-pressure failure. Does not delete or
// open file contents for reading.
func applyTopInodeConsumerDiagnostics(check *ExecutionResourceRootCheck) {
	if check == nil || strings.TrimSpace(check.Path) == "" {
		return
	}
	consumers, truncated, err := scanTopInodeConsumers(
		check.Path,
		defaultTopInodeConsumerLimit,
		defaultTopInodeEntriesPerChild,
		time.Now(),
	)
	if err != nil {
		check.Notes = append(check.Notes, "top inode consumers: "+err.Error())
		return
	}
	check.TopInodeConsumers = consumers
	check.TopInodeConsumersTruncated = truncated
}

func (p *ExecutionResourcePreflight) softPressureNotes(check ExecutionResourceRootCheck) []string {
	var notes []string
	if min := p.softMinFreeBytes(); min > 0 && check.BytesFree > 0 && check.BytesFree < min {
		notes = append(notes, fmt.Sprintf("free bytes %d < soft cleanup threshold %d", check.BytesFree, min))
	}
	if min := p.softMinFreeInodes(); min > 0 && check.InodesFree > 0 && check.InodesFree < min {
		notes = append(notes, fmt.Sprintf("free inodes %d < soft cleanup threshold %d", check.InodesFree, min))
	}
	return notes
}

func (p *ExecutionResourcePreflight) softMinFreeBytes() uint64 {
	if p != nil && p.SoftMinFreeBytes > 0 {
		return p.SoftMinFreeBytes
	}
	return executionResourceSoftMinFreeBytes
}

func (p *ExecutionResourcePreflight) softMinFreeInodes() uint64 {
	if p != nil && p.SoftMinFreeInodes > 0 {
		return p.SoftMinFreeInodes
	}
	return executionResourceSoftMinFreeInodes
}

func (p *ExecutionResourcePreflight) hardMinFreeBytes() uint64 {
	if p != nil && p.HardMinFreeBytes > 0 {
		return p.HardMinFreeBytes
	}
	return executionResourceMinFreeBytes
}

func (p *ExecutionResourcePreflight) hardMinFreeInodes() uint64 {
	if p != nil && p.HardMinFreeInodes > 0 {
		return p.HardMinFreeInodes
	}
	return executionResourceMinFreeInodes
}

// createWritabilityProbeFile creates the temp file used to probe root
// writability. Tests override this to simulate EMFILE/ENFILE without
// actually exhausting the test process's file descriptors.
var createWritabilityProbeFile = os.CreateTemp

// probeWritableRoot reports whether root is writable. The third return value
// is set when the failure is fd exhaustion (EMFILE/ENFILE) rather than an
// ordinary permission- or filesystem-level unwritable root.
func probeWritableRoot(root string) (bool, string, bool) {
	f, err := createWritabilityProbeFile(root, ".ddx-resource-preflight-*")
	if err != nil {
		return false, "writability check failed: " + err.Error(), isFDExhaustionError(err)
	}
	name := f.Name()
	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(name)
		return false, "writability check close failed: " + closeErr.Error(), isFDExhaustionError(closeErr)
	}
	if removeErr := os.Remove(name); removeErr != nil {
		return false, "writability check remove failed: " + removeErr.Error(), false
	}
	return true, "", false
}

// isFDExhaustionError reports whether err is caused by the process or host
// hitting its open-file-descriptor limit.
func isFDExhaustionError(err error) bool {
	return errors.Is(err, unix.EMFILE) || errors.Is(err, unix.ENFILE)
}

// applyFDDiagnostics attaches fd-exhaustion diagnostics (open fd count,
// RLIMIT_NOFILE soft/hard values, and a compact sample of open fd targets) to
// check so operators can distinguish fd pressure from a genuinely unwritable
// root.
func applyFDDiagnostics(check *ExecutionResourceRootCheck) {
	diag := collectFDDiagnostics()
	check.FDExhausted = true
	check.FDCount = diag.Count
	check.FDSoftLimit = diag.SoftLimit
	check.FDHardLimit = diag.HardLimit
	check.FDSample = diag.Sample
	check.Notes = append(check.Notes, fmt.Sprintf(
		"fd exhaustion: open_fds=%d soft_limit=%d hard_limit=%d",
		diag.Count, diag.SoftLimit, diag.HardLimit,
	))
}

func probeExecutionRoot(root string) (ExecutionResourceRootCheck, error) {
	check := ExecutionResourceRootCheck{Path: root}
	writable, writableReason, fdExhausted := probeWritableRoot(root)
	check.Writable = writable
	check.WritableReason = writableReason
	if !writable {
		if fdExhausted {
			applyFDDiagnostics(&check)
		}
		return check, fmt.Errorf("resource preflight: %s: %s", root, writableReason)
	}
	bytesFree, inodesFree, err := probeRootCapacity(root)
	if err != nil {
		return check, fmt.Errorf("resource preflight: %s: %w", root, err)
	}
	check.BytesFree = bytesFree
	check.InodesFree = inodesFree
	return check, nil
}

func probeRootCapacity(root string) (bytesFree uint64, inodesFree uint64, err error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(root, &stat); err != nil {
		return 0, 0, fmt.Errorf("statfs %s: %w", root, err)
	}
	bytesFree = uint64(stat.Bavail) * uint64(stat.Bsize)
	inodesFree = uint64(stat.Ffree)
	return bytesFree, inodesFree, nil
}

// matchDDxCleanupPrefix reports the DDx cleanup/scratch prefix that matches
// basename, or the claim-liveness diagnostic prefix. Empty means no match.
// Matching is diagnostic only and does not authorize deletion.
func matchDDxCleanupPrefix(name string) string {
	name = filepath.Base(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return ""
	}
	for _, prefix := range defaultExecutionCleanupScratchPrefixes {
		if prefix != "" && strings.HasPrefix(name, prefix) {
			return prefix
		}
	}
	if name == claimLivenessDiagnosticPrefix || strings.HasPrefix(name, claimLivenessDiagnosticPrefix) {
		return claimLivenessDiagnosticPrefix
	}
	return ""
}

// scanTopInodeConsumers ranks immediate children of root by entry count.
// maxConsumers caps how many children are returned (default
// defaultTopInodeConsumerLimit). maxEntriesPerChild caps how many immediate
// entries are counted inside each child directory (default
// defaultTopInodeEntriesPerChild). Nested directories are never walked: only
// one level of children under root and one level of entries under each child.
// The boolean return is true when root has more immediate children than
// maxConsumers (the reported list was truncated). Paths and counts only;
// file contents are never opened for reading.
func scanTopInodeConsumers(root string, maxConsumers, maxEntriesPerChild int, now time.Time) ([]ExecutionTopInodeConsumer, bool, error) {
	if strings.TrimSpace(root) == "" {
		return nil, false, fmt.Errorf("scan top inode consumers: empty root")
	}
	if maxConsumers <= 0 {
		maxConsumers = defaultTopInodeConsumerLimit
	}
	if maxEntriesPerChild <= 0 {
		maxEntriesPerChild = defaultTopInodeEntriesPerChild
	}
	if now.IsZero() {
		now = time.Now()
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, false, fmt.Errorf("scan top inode consumers: read %s: %w", root, err)
	}

	consumers := make([]ExecutionTopInodeConsumer, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		consumer, measureErr := measureTopInodeConsumer(path, entry, maxEntriesPerChild, now)
		if measureErr != nil {
			// Best-effort: skip children that disappear or cannot be measured.
			continue
		}
		consumers = append(consumers, consumer)
	}

	sort.SliceStable(consumers, func(i, j int) bool {
		if consumers[i].EntryCount != consumers[j].EntryCount {
			return consumers[i].EntryCount > consumers[j].EntryCount
		}
		if consumers[i].Bytes != consumers[j].Bytes {
			return consumers[i].Bytes > consumers[j].Bytes
		}
		return consumers[i].Path < consumers[j].Path
	})

	truncated := len(consumers) > maxConsumers
	if truncated {
		consumers = consumers[:maxConsumers]
	}
	return consumers, truncated, nil
}

func measureTopInodeConsumer(path string, entry os.DirEntry, maxEntriesPerChild int, now time.Time) (ExecutionTopInodeConsumer, error) {
	info, err := entry.Info()
	if err != nil {
		return ExecutionTopInodeConsumer{}, err
	}

	consumer := ExecutionTopInodeConsumer{
		Path:    path,
		ModTime: info.ModTime(),
	}
	if !info.ModTime().IsZero() && !now.Before(info.ModTime()) {
		consumer.AgeSeconds = int64(now.Sub(info.ModTime()).Seconds())
	}
	if prefix := matchDDxCleanupPrefix(entry.Name()); prefix != "" {
		consumer.CleanupPrefix = prefix
		consumer.MatchesCleanup = true
	}

	if !entry.IsDir() {
		// Symlinks and files: one entry, size from Lstat metadata only.
		consumer.EntryCount = 1
		consumer.Bytes = info.Size()
		return consumer, nil
	}

	// Directory: count the directory inode plus a bounded sample of its
	// immediate children. Do not recurse into nested trees.
	entryCount := int64(1)
	bytes := info.Size()
	truncated := false

	dir, err := os.Open(path)
	if err != nil {
		consumer.EntryCount = entryCount
		consumer.Bytes = bytes
		return consumer, nil
	}
	defer dir.Close()

	const batch = 256
	counted := 0
	for counted < maxEntriesPerChild {
		remaining := maxEntriesPerChild - counted
		n := batch
		if remaining < n {
			n = remaining
		}
		// Read one extra entry when at the last batch to detect truncation
		// without loading the whole directory.
		readN := n
		if counted+n >= maxEntriesPerChild {
			readN = n + 1
		}
		batchEntries, readErr := dir.ReadDir(readN)
		if len(batchEntries) > n {
			truncated = true
			batchEntries = batchEntries[:n]
		}
		for _, child := range batchEntries {
			counted++
			entryCount++
			if childInfo, infoErr := child.Info(); infoErr == nil {
				bytes += childInfo.Size()
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			// Partial measure is still useful.
			break
		}
		if truncated || len(batchEntries) < n {
			break
		}
	}

	consumer.EntryCount = entryCount
	consumer.Bytes = bytes
	consumer.EntriesTruncated = truncated
	return consumer, nil
}

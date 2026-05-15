package agent

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"golang.org/x/sys/unix"
)

const (
	executionResourceMinFreeBytes  uint64 = 64 << 20
	executionResourceMinFreeInodes uint64 = 1024

	executionResourceSoftMinFreeBytes  uint64 = 512 << 20
	executionResourceSoftMinFreeInodes uint64 = 4096
)

// ExecutionResourceRootCheck captures the health of one execution root.
type ExecutionResourceRootCheck struct {
	Path           string   `json:"path"`
	Writable       bool     `json:"writable"`
	WritableReason string   `json:"writable_reason,omitempty"`
	BytesFree      uint64   `json:"bytes_free,omitempty"`
	InodesFree     uint64   `json:"inodes_free,omitempty"`
	Notes          []string `json:"notes,omitempty"`
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

	writable, writableReason := probeWritableRoot(root)
	check.Writable = writable
	check.WritableReason = writableReason
	if !writable {
		check.Notes = append(check.Notes, writableReason)
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
		return check, fmt.Errorf("resource preflight: %s: %s", root, msg)
	}
	return check, nil
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

func probeWritableRoot(root string) (bool, string) {
	f, err := os.CreateTemp(root, ".ddx-resource-preflight-*")
	if err != nil {
		return false, "writability check failed: " + err.Error()
	}
	name := f.Name()
	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(name)
		return false, "writability check close failed: " + closeErr.Error()
	}
	if removeErr := os.Remove(name); removeErr != nil {
		return false, "writability check remove failed: " + removeErr.Error()
	}
	return true, ""
}

func probeExecutionRoot(root string) (ExecutionResourceRootCheck, error) {
	check := ExecutionResourceRootCheck{Path: root}
	writable, writableReason := probeWritableRoot(root)
	check.Writable = writable
	check.WritableReason = writableReason
	if !writable {
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

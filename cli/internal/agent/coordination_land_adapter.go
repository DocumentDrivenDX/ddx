package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/agent/coordination"
)

// Compile-time: production land adapter satisfies coordination.LandBackend.
var _ coordination.LandBackend = (*CoordinationLandBackend)(nil)

// CoordinationLandBackend adapts agent.Land to the transport-neutral
// coordination.LandBackend surface (ADR-022 land operation, ddx-f7b012d6).
// Production callers pass RealLandingGitOps so the real landing path is
// exercised; call-recording substitutes are not production wiring.
type CoordinationLandBackend struct {
	projectRoot string
	gitOps      LandingGitOps
}

// NewCoordinationLandBackend returns a LandBackend that invokes agent.Land
// for projectRoot with gitOps. When gitOps is nil, RealLandingGitOps is used.
// projectRoot must be non-empty.
func NewCoordinationLandBackend(projectRoot string, gitOps LandingGitOps) *CoordinationLandBackend {
	if gitOps == nil {
		gitOps = RealLandingGitOps{}
	}
	return &CoordinationLandBackend{
		projectRoot: strings.TrimSpace(projectRoot),
		gitOps:      gitOps,
	}
}

// Land invokes the production agent.Land path and maps the result into
// coordination.LandResult fields. OutcomeCode / idempotency are owned by
// coordination.LocalCoordinator.
func (b *CoordinationLandBackend) Land(ctx context.Context, req coordination.LandRequest) (coordination.LandResult, error) {
	if b == nil {
		return coordination.LandResult{}, fmt.Errorf("coordination land backend: nil backend")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return coordination.LandResult{}, err
		}
	}

	projectRoot := strings.TrimSpace(req.ProjectRoot)
	if projectRoot == "" {
		projectRoot = b.projectRoot
	}
	if projectRoot == "" {
		return coordination.LandResult{}, fmt.Errorf("coordination land backend: project_root required")
	}

	worktreeDir := strings.TrimSpace(req.WorktreeDir)
	if worktreeDir == "" {
		worktreeDir = projectRoot
	}

	landReq := LandRequest{
		WorktreeDir:  worktreeDir,
		BaseRev:      strings.TrimSpace(req.BaseRev),
		ResultRev:    strings.TrimSpace(req.ResultRev),
		BeadID:       strings.TrimSpace(req.BeadID),
		AttemptID:    strings.TrimSpace(req.AttemptID),
		TargetBranch: strings.TrimSpace(req.TargetBranch),
		EvidenceDir:  strings.TrimSpace(req.EvidenceDir),
	}

	res, err := Land(projectRoot, landReq, b.gitOps)
	if err != nil {
		return coordination.LandResult{}, err
	}
	if res == nil {
		return coordination.LandResult{}, fmt.Errorf("coordination land backend: land returned nil result")
	}

	out := coordination.LandResult{
		BeadID:       landReq.BeadID,
		Status:       res.Status,
		NewTip:       res.NewTip,
		TargetBranch: res.TargetBranch,
		Merged:       res.Merged,
		PreserveRef:  res.PreserveRef,
		Reason:       res.Reason,
	}
	switch res.Status {
	case coordination.LandStatusPreserved:
		if out.Reason == "" {
			out.Reason = coordination.ReasonLandPreserved
		}
	case "":
		out.Status = coordination.LandStatusLanded
	}
	return out, nil
}

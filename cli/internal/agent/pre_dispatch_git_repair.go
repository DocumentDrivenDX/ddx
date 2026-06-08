package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/gitrepohealth"
)

const preDispatchGitRepairFailedReason = "pre_dispatch_git_repair_failed"
const preDispatchGitRepairFailedMarker = "pre-dispatch git repair failed: "

type preDispatchGitRepairerFunc func(ctx context.Context, projectRoot string) gitrepohealth.RepairResult

func defaultPreDispatchGitRepairer(ctx context.Context, projectRoot string) gitrepohealth.RepairResult {
	if strings.TrimSpace(projectRoot) == "" {
		return gitrepohealth.RepairResult{StatusSucceeded: true}
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".git")); err != nil {
		return gitrepohealth.RepairResult{StatusSucceeded: true}
	}
	return gitrepohealth.RepairKnownConfigCorruption(ctx, projectRoot)
}

func preDispatchGitRepairFailure(result gitrepohealth.RepairResult) (string, bool) {
	var details []string
	for _, issue := range result.Issues {
		if issue.Error == "" {
			continue
		}
		switch issue.Type {
		case gitrepohealth.IssueCoreBareCorruption, gitrepohealth.IssueStrayCoreWorktree, gitrepohealth.IssueLocalHooksPath:
			details = append(details, fmt.Sprintf("%s: %s", issue.Type, issue.Error))
		}
	}
	if !result.StatusSucceeded {
		detail := strings.TrimSpace(firstNonEmpty(result.StatusStderr, result.StatusOutput))
		if detail == "" {
			detail = "git status failed after repair"
		}
		details = append(details, detail)
	}
	if len(details) == 0 {
		return "", false
	}
	return strings.Join(details, "; "), true
}

func preDispatchGitRepairStop(report ExecuteBeadReport, err error, projectRoot, beadID string) (*OperatorAttentionStop, string, bool) {
	detail := strings.TrimSpace(firstNonEmpty(report.Detail, report.Error))
	if err != nil && strings.Contains(err.Error(), preDispatchGitRepairFailedMarker) {
		detail = strings.TrimSpace(err.Error())
	} else if !strings.Contains(detail, preDispatchGitRepairFailedMarker) {
		return nil, "", false
	}
	detail = strings.TrimPrefix(detail, preDispatchGitRepairFailedMarker)
	detail = strings.TrimSpace(detail)
	message := "DDx could not repair project git config; resolve the git status failure before restarting ddx work"
	return &OperatorAttentionStop{
		Reason:      preDispatchGitRepairFailedReason,
		BeadID:      beadID,
		ProjectRoot: projectRoot,
		Message:     message,
	}, detail, true
}

package agent

import (
	"fmt"
	"strings"
)

const preDispatchGitTimeoutMarker = "pre-dispatch git timeout: "
const preDispatchGitTimeoutReason = "pre_dispatch_git_timeout"

func preDispatchGitTimeoutStop(report ExecuteBeadReport, err error, projectRoot, beadID string) (*OperatorAttentionStop, string, bool) {
	detail := strings.TrimSpace(firstNonEmpty(report.Detail, report.Error))
	if err != nil && strings.Contains(err.Error(), preDispatchGitTimeoutMarker) {
		detail = strings.TrimSpace(err.Error())
	} else if !strings.Contains(detail, preDispatchGitTimeoutMarker) {
		return nil, "", false
	}
	if idx := strings.Index(detail, preDispatchGitTimeoutMarker); idx >= 0 {
		detail = detail[idx+len(preDispatchGitTimeoutMarker):]
	}
	detail = strings.TrimSpace(detail)
	message := "pre-dispatch git operation timed out before the attempt could start; rerun after the parent index lock clears"
	if detail != "" {
		message = fmt.Sprintf("%s: %s", message, detail)
	}
	return &OperatorAttentionStop{
		Reason:      preDispatchGitTimeoutReason,
		BeadID:      beadID,
		ProjectRoot: projectRoot,
		Message:     message,
	}, detail, true
}

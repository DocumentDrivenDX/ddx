package try

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// NoChangesContract — TD-031 §8.1. The try package owns parsing and
// verification for no_changes attempts and returns a structured outcome to
// the execute loop. The loop only applies the decision it receives.
type NoChangesRationaleKind string

const (
	// NoChangesKindVerified means the rationale included a verification_command.
	NoChangesKindVerified NoChangesRationaleKind = "verified"
	// NoChangesKindLifecycleStatus means the rationale requested a canonical lifecycle status.
	NoChangesKindLifecycleStatus NoChangesRationaleKind = "lifecycle_status"
	// NoChangesKindRejectedLegacyStatus means the rationale used a removed pseudo-status.
	NoChangesKindRejectedLegacyStatus NoChangesRationaleKind = "rejected_legacy_status"
	// NoChangesKindUnjustified means neither structured marker was present.
	NoChangesKindUnjustified NoChangesRationaleKind = "unjustified"
)

const (
	NoChangesEventVerified             = "no_changes_verified"
	NoChangesEventUnverified           = "no_changes_unverified"
	NoChangesEventUnjustified          = "no_changes_unjustified"
	NoChangesEventAutonomousRetry      = "no_changes_autonomous_retry"
	NoChangesEventOperatorRequired     = "no_changes_operator_required"
	NoChangesEventBlocked              = "no_changes_blocked"
	NoChangesEventLegacyStatusRejected = "no_changes_legacy_status_rejected"
)

const (
	NoChangesLabelUnverified  = "triage:no-changes-unverified"
	NoChangesLabelUnjustified = "triage:no-changes-unjustified"
)

type ParsedNoChangesRationale struct {
	Kind                NoChangesRationaleKind
	VerificationCommand string
	LifecycleStatus     string
	Reason              string
	SuggestedAction     string
	RejectionReason     string
	// OrchestratorAction, when set to "decompose", signals that the implementation
	// attempt hit its depth cap and the queue-level orchestrator should split the
	// bead. The orchestrator checks the queue-level max_decomposition_depth before
	// acting; implementation-attempt depth caps are not forwarded here.
	OrchestratorAction string
}

func ParseNoChangesRationale(text string) ParsedNoChangesRationale {
	p := ParsedNoChangesRationale{Kind: NoChangesKindUnjustified}
	var reasonLines []string
	inReason := false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "verification_command:"):
			cmd := strings.TrimSpace(line[len("verification_command:"):])
			if cmd != "" {
				p.VerificationCommand = cmd
				p.Kind = NoChangesKindVerified
			}
			inReason = false
		case strings.HasPrefix(lower, "status:"):
			v := strings.TrimSpace(strings.ToLower(line[len("status:"):]))
			p.LifecycleStatus = v
			inReason = false
		case strings.HasPrefix(lower, "reason:"):
			r := strings.TrimSpace(line[len("reason:"):])
			if r != "" {
				reasonLines = append(reasonLines, r)
			}
			inReason = true
		case strings.HasPrefix(lower, "suggested_action:"):
			p.SuggestedAction = strings.TrimSpace(line[len("suggested_action:"):])
			inReason = false
		case strings.HasPrefix(lower, "orchestrator_action:"):
			p.OrchestratorAction = strings.TrimSpace(strings.ToLower(line[len("orchestrator_action:"):]))
			inReason = false
		default:
			if inReason && line != "" {
				reasonLines = append(reasonLines, line)
			}
		}
	}
	if p.Kind == NoChangesKindVerified {
		p.LifecycleStatus = ""
		p.Reason = ""
		p.SuggestedAction = ""
		p.RejectionReason = ""
		return p
	}
	p.Reason = strings.Join(reasonLines, " ")
	switch p.LifecycleStatus {
	case "":
		return p
	case "needs_investigation":
		p.Kind = NoChangesKindRejectedLegacyStatus
		p.RejectionReason = "status: needs_investigation is no longer accepted; use status: open, status: proposed, or status: blocked for new no_changes output, and run `ddx bead migrate --lifecycle` for stored legacy rows"
	case "open", "proposed", "blocked", "closed", "cancelled":
		p.Kind = NoChangesKindLifecycleStatus
	default:
		p.Kind = NoChangesKindUnjustified
	}
	return p
}

type VerificationCommandRunner func(ctx context.Context, projectRoot, command string) (exitCode int, output string, err error)

const DefaultVerificationCommandTimeout = 60 * time.Second

func DefaultVerificationCommandRunner(ctx context.Context, projectRoot, command string) (int, string, error) {
	cctx, cancel := context.WithTimeout(ctx, DefaultVerificationCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "/bin/sh", "-c", command)
	if projectRoot != "" {
		cmd.Dir = projectRoot
	}
	out, err := cmd.CombinedOutput()
	if cctx.Err() == context.DeadlineExceeded {
		return -1, truncateVerifyOutput(string(out)), fmt.Errorf("verification_command timed out after %s", DefaultVerificationCommandTimeout)
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), truncateVerifyOutput(string(out)), nil
	}
	if err != nil {
		return -1, truncateVerifyOutput(string(out)), err
	}
	return 0, truncateVerifyOutput(string(out)), nil
}

func truncateVerifyOutput(s string) string {
	const max = 4 * 1024
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...[truncated]"
}

type SatisfactionChecker interface {
	CheckSatisfied(ctx context.Context, beadID string, noChangesCount int) (bool, string, error)
}

type NoChangesOutcome struct {
	Satisfied        bool
	Action           NoChangesLifecycleAction
	CooldownEligible bool
	Evidence         string
	EventKind        string
	EventBody        string
	Label            string
	LifecycleStatus  string
	Reason           string
	SuggestedAction  string
}

type NoChangesLifecycleAction string

const (
	NoChangesActionCloseAlreadySatisfied NoChangesLifecycleAction = "close_already_satisfied"
	NoChangesActionKeepOpenSmartRetry    NoChangesLifecycleAction = "keep_open_smart_retry"
	NoChangesActionOperatorRequired      NoChangesLifecycleAction = "operator_required"
	NoChangesActionBlockedExternal       NoChangesLifecycleAction = "blocked_external"
	NoChangesActionBadAttemptNoCooldown  NoChangesLifecycleAction = "bad_attempt_no_cooldown"
	NoChangesActionRetryLaterCooldown    NoChangesLifecycleAction = "retry_later_cooldown"
)

func adjudicateNoChangesContract(ctx context.Context, beadID string, report Report, projectRoot string, noChangesCount int, checker SatisfactionChecker, runner VerificationCommandRunner) (NoChangesOutcome, Report, error) {
	if checker != nil {
		satisfied, evidence, err := checker.CheckSatisfied(ctx, beadID, noChangesCount)
		if err != nil {
			return NoChangesOutcome{}, report, err
		}
		if satisfied {
			if evidence != "" {
				report.Detail = evidence
			}
			report.Status = StatusAlreadySatisfied
			return NoChangesOutcome{
				Satisfied:        true,
				Action:           NoChangesActionCloseAlreadySatisfied,
				CooldownEligible: false,
				Evidence:         evidence,
			}, report, nil
		}
		return NoChangesOutcome{
			Satisfied:        false,
			Action:           NoChangesActionBadAttemptNoCooldown,
			CooldownEligible: false,
		}, report, nil
	}

	parsed := ParseNoChangesRationale(report.NoChangesRationale)
	switch parsed.Kind {
	case NoChangesKindVerified:
		if runner == nil {
			runner = DefaultVerificationCommandRunner
		}
		exitCode, output, runErr := runner(ctx, projectRoot, parsed.VerificationCommand)
		body := fmt.Sprintf("verification_command=%s\nexit_code=%d", parsed.VerificationCommand, exitCode)
		if output != "" {
			body += "\noutput:\n" + output
		}
		if runErr != nil {
			body += "\nrun_error: " + runErr.Error()
		}
		if runErr == nil && exitCode == 0 {
			evidence := fmt.Sprintf("verification_command passed: %s", parsed.VerificationCommand)
			report.Detail = evidence
			report.Status = StatusAlreadySatisfied
			return NoChangesOutcome{
				Satisfied:        true,
				Action:           NoChangesActionCloseAlreadySatisfied,
				CooldownEligible: false,
				Evidence:         evidence,
				EventKind:        NoChangesEventVerified,
				EventBody:        body,
			}, report, nil
		}
		return NoChangesOutcome{
			Satisfied:        false,
			Action:           NoChangesActionBadAttemptNoCooldown,
			CooldownEligible: false,
			EventKind:        NoChangesEventUnverified,
			EventBody:        body,
			Label:            NoChangesLabelUnverified,
		}, report, nil
	case NoChangesKindLifecycleStatus:
		return adjudicateNoChangesLifecycleStatus(parsed, report), report, nil
	case NoChangesKindRejectedLegacyStatus:
		body := parsed.RejectionReason
		if parsed.Reason != "" {
			body += "\nreason: " + parsed.Reason
		}
		return NoChangesOutcome{
			Satisfied:        false,
			Action:           NoChangesActionBadAttemptNoCooldown,
			CooldownEligible: false,
			EventKind:        NoChangesEventLegacyStatusRejected,
			EventBody:        body,
			Reason:           body,
		}, report, nil
	default:
		body := strings.TrimSpace(report.NoChangesRationale)
		if body == "" {
			body = "(rationale absent)"
		}
		return NoChangesOutcome{
			Satisfied:        false,
			Action:           NoChangesActionBadAttemptNoCooldown,
			CooldownEligible: false,
			EventKind:        NoChangesEventUnjustified,
			EventBody:        body,
			Label:            NoChangesLabelUnjustified,
		}, report, nil
	}
}

func adjudicateNoChangesLifecycleStatus(parsed ParsedNoChangesRationale, report Report) NoChangesOutcome {
	reason := strings.TrimSpace(parsed.Reason)
	suggestedAction := strings.TrimSpace(parsed.SuggestedAction)
	switch parsed.LifecycleStatus {
	case "open":
		if reason == "" {
			reason = "autonomous work remains possible"
		}
		if suggestedAction == "" {
			suggestedAction = "retry with a smart agent"
		}
		return NoChangesOutcome{
			Satisfied:        false,
			Action:           NoChangesActionKeepOpenSmartRetry,
			CooldownEligible: false,
			EventKind:        NoChangesEventAutonomousRetry,
			EventBody:        noChangesLifecycleEventBody(parsed.LifecycleStatus, reason, suggestedAction),
			LifecycleStatus:  parsed.LifecycleStatus,
			Reason:           reason,
			SuggestedAction:  suggestedAction,
		}
	case "proposed":
		if reason == "" {
			reason = "operator decision required before another automated attempt"
		}
		if suggestedAction == "" {
			suggestedAction = "review and accept, split, block, or cancel this proposed work"
		}
		return NoChangesOutcome{
			Satisfied:        false,
			Action:           NoChangesActionOperatorRequired,
			CooldownEligible: false,
			EventKind:        NoChangesEventOperatorRequired,
			EventBody:        noChangesLifecycleEventBody(parsed.LifecycleStatus, reason, suggestedAction),
			LifecycleStatus:  parsed.LifecycleStatus,
			Reason:           reason,
			SuggestedAction:  suggestedAction,
		}
	case "blocked":
		if reason == "" {
			return noChangesUnsupportedStatusOutcome(parsed.LifecycleStatus, "status: blocked requires reason: <external recheckable blocker>")
		}
		if noChangesBlockedReasonIsInternal(reason, suggestedAction, report.NoChangesRationale) {
			if suggestedAction == "" {
				suggestedAction = "decompose into executable follow-up work or retry with a stronger agent"
			}
			return NoChangesOutcome{
				Satisfied:        false,
				Action:           NoChangesActionKeepOpenSmartRetry,
				CooldownEligible: false,
				EventKind:        NoChangesEventAutonomousRetry,
				EventBody:        noChangesLifecycleEventBody("open", reason, suggestedAction),
				LifecycleStatus:  "open",
				Reason:           reason,
				SuggestedAction:  suggestedAction,
			}
		}
		if suggestedAction == "" {
			suggestedAction = "recheck the external blocker and move status to open when cleared"
		}
		return NoChangesOutcome{
			Satisfied:        false,
			Action:           NoChangesActionBlockedExternal,
			CooldownEligible: false,
			EventKind:        NoChangesEventBlocked,
			EventBody:        noChangesLifecycleEventBody(parsed.LifecycleStatus, reason, suggestedAction),
			LifecycleStatus:  parsed.LifecycleStatus,
			Reason:           reason,
			SuggestedAction:  suggestedAction,
		}
	case "closed":
		return noChangesUnsupportedStatusOutcome(parsed.LifecycleStatus, "use verification_command to close an already-satisfied bead")
	case "cancelled":
		return noChangesUnsupportedStatusOutcome(parsed.LifecycleStatus, "cancelling work is an operator action, not a no_changes worker outcome")
	default:
		body := strings.TrimSpace(report.NoChangesRationale)
		if body == "" {
			body = "(rationale absent)"
		}
		return NoChangesOutcome{
			Satisfied:        false,
			Action:           NoChangesActionBadAttemptNoCooldown,
			CooldownEligible: false,
			EventKind:        NoChangesEventUnjustified,
			EventBody:        body,
			Label:            NoChangesLabelUnjustified,
		}
	}
}

func noChangesBlockedReasonIsInternal(reason, suggestedAction, rationale string) bool {
	text := strings.ToLower(strings.Join([]string{reason, suggestedAction, rationale}, "\n"))
	internalSignals := []string{
		"cannot be satisfied inside this bead",
		"inside this bead alone",
		"outside this bead",
		"outside the bead",
		"outside scope",
		"out of scope",
		"scope is too large",
		"too large",
		"needs split",
		"requires split",
		"requires decomposition",
		"requires a broad",
		"requires a broader",
		"broad signature migration",
		"broad caller migration",
		"broader api migration",
		"broader api",
		"repo-wide refactor",
		"follow_up_needed",
		"follow-up needed",
	}
	for _, signal := range internalSignals {
		if strings.Contains(text, signal) {
			return true
		}
	}
	return false
}

func noChangesUnsupportedStatusOutcome(status, reason string) NoChangesOutcome {
	body := fmt.Sprintf("status: %s rejected: %s", status, reason)
	return NoChangesOutcome{
		Satisfied:        false,
		Action:           NoChangesActionBadAttemptNoCooldown,
		CooldownEligible: false,
		EventKind:        NoChangesEventUnjustified,
		EventBody:        body,
		Reason:           body,
	}
}

func noChangesLifecycleEventBody(status, reason, suggestedAction string) string {
	body := "status=" + status
	if reason != "" {
		body += "\nreason=" + reason
	}
	if suggestedAction != "" {
		body += "\nsuggested_action=" + suggestedAction
	}
	return body
}

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
	// NoChangesKindNeedsInvestigation means the agent asked for operator triage.
	NoChangesKindNeedsInvestigation NoChangesRationaleKind = "needs_investigation"
	// NoChangesKindUnjustified means neither structured marker was present.
	NoChangesKindUnjustified NoChangesRationaleKind = "unjustified"
)

const (
	NoChangesEventVerified           = "no_changes_verified"
	NoChangesEventUnverified         = "no_changes_unverified"
	NoChangesEventUnjustified        = "no_changes_unjustified"
	NoChangesEventNeedsInvestigation = "no_changes_needs_investigation"
)

const (
	NoChangesLabelUnverified         = "triage:no-changes-unverified"
	NoChangesLabelUnjustified        = "triage:no-changes-unjustified"
	NoChangesLabelNeedsInvestigation = "triage:needs-investigation"
)

type ParsedNoChangesRationale struct {
	Kind                     NoChangesRationaleKind
	VerificationCommand      string
	NeedsInvestigationReason string
}

func ParseNoChangesRationale(text string) ParsedNoChangesRationale {
	p := ParsedNoChangesRationale{Kind: NoChangesKindUnjustified}
	needsInv := false
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
			if v == "needs_investigation" {
				needsInv = true
			}
			inReason = false
		case strings.HasPrefix(lower, "reason:"):
			r := strings.TrimSpace(line[len("reason:"):])
			if r != "" {
				reasonLines = append(reasonLines, r)
			}
			inReason = true
		default:
			if inReason && line != "" {
				reasonLines = append(reasonLines, line)
			}
		}
	}
	if p.Kind == NoChangesKindVerified {
		return p
	}
	if needsInv {
		p.Kind = NoChangesKindNeedsInvestigation
		p.NeedsInvestigationReason = strings.Join(reasonLines, " ")
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
	Satisfied bool
	Evidence  string
	EventKind string
	EventBody string
	Label     string
}

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
			return NoChangesOutcome{Satisfied: true, Evidence: evidence}, report, nil
		}
		return NoChangesOutcome{Satisfied: false}, report, nil
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
				Satisfied: true,
				Evidence:  evidence,
				EventKind: NoChangesEventVerified,
				EventBody: body,
			}, report, nil
		}
		return NoChangesOutcome{
			Satisfied: false,
			EventKind: NoChangesEventUnverified,
			EventBody: body,
			Label:     NoChangesLabelUnverified,
		}, report, nil
	case NoChangesKindNeedsInvestigation:
		body := parsed.NeedsInvestigationReason
		if body == "" {
			body = "(no reason provided)"
		}
		return NoChangesOutcome{
			Satisfied: false,
			EventKind: NoChangesEventNeedsInvestigation,
			EventBody: body,
			Label:     NoChangesLabelNeedsInvestigation,
		}, report, nil
	default:
		body := strings.TrimSpace(report.NoChangesRationale)
		if body == "" {
			body = "(rationale absent)"
		}
		return NoChangesOutcome{
			Satisfied: false,
			EventKind: NoChangesEventUnjustified,
			EventBody: body,
			Label:     NoChangesLabelUnjustified,
		}, report, nil
	}
}

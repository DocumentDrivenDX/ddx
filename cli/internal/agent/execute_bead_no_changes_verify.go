package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// NoChangesContract — TD-031 §8.1 (ddx-b24e9630). When the agent returns
// no_changes the loop now gates the close on a structured marker in
// no_changes_rationale.txt. The marker is one of:
//
//   - verification_command: <shell command>
//     (the bead is closed as already_satisfied iff the command exits 0)
//   - status: needs_investigation
//     reason: <free-form text>
//     (the bead stays open with a needs-investigation label and event)
//
// A rationale carrying neither marker is rejected: bead stays open with a
// no-changes-unjustified label + event. Status enum stays in the canonical
// bd/br 6 (open / in_progress / closed / blocked / proposed / cancelled);
// triage state lives in labels and events.

// NoChangesRationaleKind classifies a parsed no_changes_rationale.txt body.
type NoChangesRationaleKind string

const (
	// NoChangesKindVerified — the rationale carries a verification_command
	// the loop can run to prove the AC is currently met.
	NoChangesKindVerified NoChangesRationaleKind = "verified"
	// NoChangesKindNeedsInvestigation — the agent declared it cannot make
	// progress and explained why; the bead must be triaged before retry.
	NoChangesKindNeedsInvestigation NoChangesRationaleKind = "needs_investigation"
	// NoChangesKindUnjustified — neither marker is present; the rationale is
	// rejected and the bead stays open for triage.
	NoChangesKindUnjustified NoChangesRationaleKind = "unjustified"
)

// NoChangesContract event kinds (TD-031 §4 vocabulary; emitted on the bead's
// event stream by the execute-loop).
const (
	NoChangesEventVerified           = "no_changes_verified"
	NoChangesEventUnverified         = "no_changes_unverified"
	NoChangesEventUnjustified        = "no_changes_unjustified"
	NoChangesEventNeedsInvestigation = "no_changes_needs_investigation"
)

// NoChangesContract triage labels (consumed by auto-triage, ddx-3c154349).
const (
	NoChangesLabelUnverified         = "triage:no-changes-unverified"
	NoChangesLabelUnjustified        = "triage:no-changes-unjustified"
	NoChangesLabelNeedsInvestigation = "triage:needs-investigation"
)

// ParsedNoChangesRationale is the structured form of a no_changes_rationale
// body. Only the marker(s) the agent included are populated.
type ParsedNoChangesRationale struct {
	Kind                     NoChangesRationaleKind
	VerificationCommand      string
	NeedsInvestigationReason string
}

// ParseNoChangesRationale extracts the NoChangesContract markers from a
// rationale body. verification_command takes precedence over needs_investigation
// when both are present (a verified rationale is the strongest signal).
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

// VerificationCommandRunner runs a shell verification command in projectRoot.
// A non-zero exit code is reported via exitCode (NOT err); err is reserved for
// failures to launch (binary missing, timeout). Output is the combined
// stdout+stderr capped by the implementation.
type VerificationCommandRunner func(ctx context.Context, projectRoot, command string) (exitCode int, output string, err error)

// DefaultVerificationCommandTimeout bounds how long the loop will wait for a
// verification_command to finish before treating it as failed.
const DefaultVerificationCommandTimeout = 60 * time.Second

// DefaultVerificationCommandRunner runs `command` via /bin/sh -c in projectRoot
// with DefaultVerificationCommandTimeout.
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

// truncateVerifyOutput caps verification command output before it is recorded
// on the bead's event stream so a chatty test suite cannot blow up the event
// body.
func truncateVerifyOutput(s string) string {
	const max = 4 * 1024
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n…[truncated]"
}

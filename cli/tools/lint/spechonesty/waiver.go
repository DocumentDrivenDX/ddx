// Verification-waiver policy (WB-1 step 5).
//
// A reasoned `spec:verification-waiver` may downgrade an unmet
// verification requirement to a warning only for non-Complete statuses.
// It never suppresses a Complete/Implemented coverage failure.
// Coverage computation and status parsing are owned by sibling beads;
// this file only applies the waiver branch to findings those stages
// already produced. The analyzer remains read-only.
package spechonesty

import (
	"os"
	"regexp"
	"strings"
)

// DocStatus is a normalized base document status.
// Full status parsing (body + frontmatter, free-text qualifiers) is
// owned by the scaffold/status-parser child; this type is the shared
// vocabulary the waiver policy branches on.
type DocStatus string

const (
	// StatusComplete claims full current-revision verification coverage.
	StatusComplete DocStatus = "Complete"
	// StatusImplemented is treated identically to Complete for waiver policy.
	StatusImplemented DocStatus = "Implemented"
	// StatusProposed is a non-Complete status eligible for reasoned waiver downgrade.
	StatusProposed DocStatus = "Proposed"
	// StatusInProgress is a non-Complete status eligible for reasoned waiver downgrade.
	StatusInProgress DocStatus = "In Progress"
	// StatusDeferred is a non-Complete status eligible for reasoned waiver downgrade.
	StatusDeferred DocStatus = "Deferred"
	// StatusAspirational is a non-Complete status eligible for reasoned waiver downgrade.
	StatusAspirational DocStatus = "Aspirational"
	// StatusUnknown is any unrecognized or missing status.
	StatusUnknown DocStatus = ""
)

// FindingSeverity is the severity of a coverage-stage finding after
// optional waiver policy is applied.
type FindingSeverity string

const (
	// SeverityError is a hard failure (CI non-zero).
	SeverityError FindingSeverity = "error"
	// SeverityWarning is a soft finding (does not fail CI by itself).
	SeverityWarning FindingSeverity = "warning"
)

// CoverageFindingKind classifies a coverage-stage finding.
type CoverageFindingKind string

const (
	// FindingUnmetVerification is an unmet verification requirement
	// (uncovered requirement, missing evidence target, etc.). These are
	// the only findings a reasoned non-Complete waiver may downgrade.
	FindingUnmetVerification CoverageFindingKind = "unmet_verification"
	// FindingZeroEvidence is a Complete/Implemented document with no
	// Verification mapping rows at all (document-level presence check).
	// Non-waivable; cardinality siblings own per-requirement uncovered
	// and duplicate diagnostics separately.
	FindingZeroEvidence CoverageFindingKind = "zero_evidence"
	// FindingDuplicateMapping is a Complete/Implemented requirement (or
	// stable anchor) covered by more than one Verification mapping row.
	// Non-waivable; emitted by the coverage-cardinality pass.
	FindingDuplicateMapping CoverageFindingKind = "duplicate_mapping"
	// FindingMissingStatus is non-waivable (WB-1 step 5).
	FindingMissingStatus CoverageFindingKind = "missing_status"
	// FindingDuplicateID is non-waivable (WB-1 step 5).
	FindingDuplicateID CoverageFindingKind = "duplicate_id"
	// FindingDuplicateUSID is non-waivable (WB-1 step 5).
	FindingDuplicateUSID CoverageFindingKind = "duplicate_us_id"
)

// CoverageFinding is one coverage-stage diagnostic. The coverage child
// produces these; the waiver policy may only downgrade Severity for
// waivable findings on non-Complete statuses.
type CoverageFinding struct {
	Path     string
	Line     int
	Kind     CoverageFindingKind
	Severity FindingSeverity
	Message  string
}

// VerificationWaiver is a reasoned `spec:verification-waiver` annotation.
// Present is true when the marker is found; Reasoned is true only when
// the reason text is non-empty after trimming.
type VerificationWaiver struct {
	Present  bool
	Reasoned bool
	Reason   string
	Line     int
}

var (
	// waiverFrontmatterRe matches a frontmatter key for the waiver.
	// Accepts both "spec:verification-waiver:" and "verification-waiver:".
	waiverFrontmatterRe = regexp.MustCompile(`(?i)^\s*(?:["']?spec:verification-waiver["']?|verification-waiver)\s*:\s*(.*)$`)
	// waiverBodyLineRe matches a body line annotation.
	waiverBodyLineRe = regexp.MustCompile(`(?i)^\s*(?:\*\*)?spec:verification-waiver(?:\*\*)?\s*:\s*(.+?)\s*(?:\*\*)?\s*$`)
	// waiverCommentRe matches an HTML comment form.
	waiverCommentRe = regexp.MustCompile(`(?i)<!--\s*spec:verification-waiver\s*:\s*(.+?)\s*-->`)
)

// IsCompleteStatus reports whether status is Complete or Implemented.
// Waivers are never applied to these statuses.
func IsCompleteStatus(status DocStatus) bool {
	switch status {
	case StatusComplete, StatusImplemented:
		return true
	default:
		return false
	}
}

// IsNonCompleteWaiverEligible reports whether status is one of the
// non-Complete statuses for which a reasoned waiver may downgrade
// unmet verification findings to warnings.
func IsNonCompleteWaiverEligible(status DocStatus) bool {
	switch status {
	case StatusProposed, StatusInProgress, StatusDeferred, StatusAspirational:
		return true
	default:
		return false
	}
}

// isWaivable reports whether a finding kind may be downgraded by a
// reasoned non-Complete waiver. Missing-status, duplicate-id, and
// duplicate-US-id failures are never waivable.
func isWaivable(kind CoverageFindingKind) bool {
	return kind == FindingUnmetVerification
}

// ParseVerificationWaiverFile reads path and extracts a verification waiver.
// It is read-only: the file is never modified.
func ParseVerificationWaiverFile(path string) (*VerificationWaiver, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseVerificationWaiver(string(data)), nil
}

// ParseVerificationWaiver extracts a `spec:verification-waiver` from
// markdown content (frontmatter key, body line, or HTML comment).
// Content is never written back.
func ParseVerificationWaiver(content string) *VerificationWaiver {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")

	// Prefer frontmatter when present.
	if w := parseWaiverFrontmatter(lines); w != nil {
		return w
	}
	// Body line or HTML comment anywhere in the document.
	for i, line := range lines {
		lineNo := i + 1
		if m := waiverCommentRe.FindStringSubmatch(line); m != nil {
			return reasonedWaiver(m[1], lineNo)
		}
		if m := waiverBodyLineRe.FindStringSubmatch(line); m != nil {
			return reasonedWaiver(m[1], lineNo)
		}
	}
	return &VerificationWaiver{Present: false}
}

func parseWaiverFrontmatter(lines []string) *VerificationWaiver {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil
	}
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "---" {
			return nil
		}
		if m := waiverFrontmatterRe.FindStringSubmatch(line); m != nil {
			return reasonedWaiver(m[1], i+1)
		}
	}
	return nil
}

func reasonedWaiver(raw string, line int) *VerificationWaiver {
	reason := strings.TrimSpace(raw)
	// Strip optional surrounding quotes.
	if len(reason) >= 2 {
		if (reason[0] == '"' && reason[len(reason)-1] == '"') ||
			(reason[0] == '\'' && reason[len(reason)-1] == '\'') {
			reason = strings.TrimSpace(reason[1 : len(reason)-1])
		}
	}
	return &VerificationWaiver{
		Present:  true,
		Reasoned: reason != "",
		Reason:   reason,
		Line:     line,
	}
}

// ApplyWaiverPolicy adjusts coverage findings according to WB-1 step 5.
//
//   - Complete/Implemented: the waiver is ignored entirely; every finding
//     keeps its original severity (a coverage failure remains a failure).
//   - Non-Complete (Proposed, In Progress, Deferred, Aspirational) with a
//     reasoned waiver: unmet verification findings (FindingUnmetVerification)
//     at SeverityError are downgraded to SeverityWarning.
//   - Missing-status, duplicate-id, and duplicate-US-id findings are never
//     downgraded, regardless of status or waiver.
//   - An unreasoned (empty) waiver never downgrades anything.
//
// The input slice is not modified; a new slice is returned. Pure and
// read-only with respect to the filesystem.
func ApplyWaiverPolicy(status DocStatus, waiver *VerificationWaiver, findings []CoverageFinding) []CoverageFinding {
	if len(findings) == 0 {
		return nil
	}
	out := make([]CoverageFinding, len(findings))
	copy(out, findings)

	if IsCompleteStatus(status) {
		// Waiver cannot suppress a Complete-status coverage failure.
		return out
	}
	if waiver == nil || !waiver.Present || !waiver.Reasoned {
		return out
	}
	if !IsNonCompleteWaiverEligible(status) {
		// Unknown / other statuses: do not silently downgrade.
		return out
	}

	for i := range out {
		if out[i].Severity != SeverityError {
			continue
		}
		if !isWaivable(out[i].Kind) {
			continue
		}
		out[i].Severity = SeverityWarning
		if out[i].Message != "" && !strings.Contains(out[i].Message, "downgraded by verification-waiver") {
			out[i].Message = out[i].Message + " (downgraded by verification-waiver)"
		}
	}
	return out
}

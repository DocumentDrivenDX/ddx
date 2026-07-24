// Observation freshness validation (WB-1 step 4).
//
// Complete/Implemented Verification mapping rows require a structured
// observation keyed to the injected current repository revision with
// exit code 0. A checked-in prose assertion that a command once passed
// is not sufficient. Current revision is always injected so unit tests
// stay hermetic (no network, no live git fetch).
//
// Coverage/waiver logic and status parsing live in sibling packages;
// this file only validates observation freshness for rows those stages
// already produced. Read-only: never mutates documents or fetches state.
package spechonesty

import (
	"fmt"
	"strings"
)

// ObservationFindingKind classifies a current-revision observation failure.
type ObservationFindingKind string

const (
	// FindingStaleObservation is emitted when a structured observation's
	// revision differs from the injected current revision.
	FindingStaleObservation ObservationFindingKind = "stale_observation"
	// FindingNonZeroObservation is emitted when a current-revision
	// observation has a non-zero exit code.
	FindingNonZeroObservation ObservationFindingKind = "non_zero_observation"
	// FindingUnstructuredObservation is emitted when the only evidence is
	// prose (or otherwise lacks structured revision + exit-code fields).
	FindingUnstructuredObservation ObservationFindingKind = "unstructured_observation"
	// FindingMissingObservation is emitted when no observation is supplied
	// for a Complete/Implemented Verification row.
	FindingMissingObservation ObservationFindingKind = "missing_observation"
)

// ObservationFinding is one freshness-stage diagnostic for a Verification row.
type ObservationFinding struct {
	// Path is optional document path for diagnostics.
	Path string
	// Line is the 1-based mapping row line when known (0 otherwise).
	Line int
	// RequirementRef is the requirement/anchor the finding concerns.
	RequirementRef string
	// Kind classifies the failure.
	Kind ObservationFindingKind
	// Severity is always SeverityError for Complete/Implemented freshness failures.
	Severity FindingSeverity
	// Message is a human-readable description.
	Message string
}

// Observation is structured or prose evidence for one Verification mapping row.
//
// Structured evidence requires both a non-empty Revision and ExitCode set
// (ExitCodePresent true). Prose-only claims (e.g. "command X passed") may set
// Prose without revision/exit fields; those never satisfy Complete/Implemented
// freshness. Tests inject fixture revisions; production injects the repository
// revision under evaluation — neither path fetches network state here.
type Observation struct {
	// RequirementRef keys the observation to a Verification mapping row.
	RequirementRef string
	// Command is the verification command that was (or claims to have been) run.
	Command string
	// Revision is the repository revision the observation was captured at.
	// Empty when the claim is prose-only or otherwise unstructured.
	Revision string
	// ExitCode is the process exit code when ExitCodePresent is true.
	ExitCode int
	// ExitCodePresent is true when ExitCode was explicitly recorded.
	// Distinguishes structured exit 0 from a missing exit field.
	ExitCodePresent bool
	// Prose is an optional free-text claim (e.g. "go test passed"). Alone it
	// does not satisfy the current-revision observation requirement.
	Prose string
	// Path and Line are optional source locations for diagnostics.
	Path string
	Line int
}

// IsStructured reports whether the observation carries both a revision and
// an explicit exit code — the minimum machine-readable evidence required
// for Complete/Implemented freshness.
func (o Observation) IsStructured() bool {
	return strings.TrimSpace(o.Revision) != "" && o.ExitCodePresent
}

// FreshnessInput is the injected context for hermetic observation checks.
// CurrentRevision is supplied by the caller (fixture SHA in tests, real
// repository revision in CI); this package never resolves it itself.
type FreshnessInput struct {
	// CurrentRevision is the repository revision under evaluation.
	CurrentRevision string
	// Status is the document's normalized base status.
	Status DocStatus
	// Path is optional document path recorded on findings.
	Path string
	// Rows are well-formed Verification mapping rows for the document.
	Rows []VerificationRow
	// Observations are candidate evidence rows (structured or prose).
	Observations []Observation
}

// CheckObservationFreshness validates current-revision, exit-zero
// observations for Complete/Implemented documents.
//
// Non-Complete statuses produce no freshness findings (coverage/waiver
// siblings own those paths). For Complete/Implemented, every mapping row
// must have a structured observation whose Revision equals the injected
// CurrentRevision and whose ExitCode is 0. Stale revisions, non-zero exits,
// prose-only claims, and missing observations all fail.
//
// Pure and read-only: no filesystem or network access.
func CheckObservationFreshness(in FreshnessInput) []ObservationFinding {
	if !IsCompleteStatus(in.Status) {
		return nil
	}
	if len(in.Rows) == 0 {
		return nil
	}

	current := strings.TrimSpace(in.CurrentRevision)
	// Index observations by requirement ref (first match wins for lookup;
	// each candidate for a row is still evaluated for structure).
	byReq := make(map[string][]Observation, len(in.Observations))
	for _, obs := range in.Observations {
		key := strings.TrimSpace(obs.RequirementRef)
		if key == "" {
			continue
		}
		byReq[key] = append(byReq[key], obs)
	}

	var findings []ObservationFinding
	for _, row := range in.Rows {
		ref := strings.TrimSpace(row.RequirementRef)
		obsList := byReq[ref]
		if len(obsList) == 0 {
			findings = append(findings, ObservationFinding{
				Path:           in.Path,
				Line:           row.Line,
				RequirementRef: ref,
				Kind:           FindingMissingObservation,
				Severity:       SeverityError,
				Message: fmt.Sprintf(
					"Complete/Implemented requirement %q has no observation; current-revision exit-zero structured evidence is required",
					ref,
				),
			})
			continue
		}

		// Prefer a structured observation if any is present for the row.
		var structured *Observation
		var proseOnly *Observation
		for i := range obsList {
			o := &obsList[i]
			if o.IsStructured() {
				structured = o
				break
			}
			if proseOnly == nil && strings.TrimSpace(o.Prose) != "" {
				proseOnly = o
			}
		}

		if structured == nil {
			// Prose assertion or other non-structured claim.
			line := row.Line
			path := in.Path
			if proseOnly != nil {
				if proseOnly.Line > 0 {
					line = proseOnly.Line
				}
				if proseOnly.Path != "" {
					path = proseOnly.Path
				}
			} else if len(obsList) > 0 {
				if obsList[0].Line > 0 {
					line = obsList[0].Line
				}
				if obsList[0].Path != "" {
					path = obsList[0].Path
				}
			}
			findings = append(findings, ObservationFinding{
				Path:           path,
				Line:           line,
				RequirementRef: ref,
				Kind:           FindingUnstructuredObservation,
				Severity:       SeverityError,
				Message: fmt.Sprintf(
					"requirement %q has no structured observation (revision + exit code); prose assertion that a command passed is not sufficient",
					ref,
				),
			})
			continue
		}

		obsRev := strings.TrimSpace(structured.Revision)
		line := row.Line
		if structured.Line > 0 {
			line = structured.Line
		}
		path := in.Path
		if structured.Path != "" {
			path = structured.Path
		}

		if current == "" || obsRev != current {
			findings = append(findings, ObservationFinding{
				Path:           path,
				Line:           line,
				RequirementRef: ref,
				Kind:           FindingStaleObservation,
				Severity:       SeverityError,
				Message: fmt.Sprintf(
					"requirement %q observation revision %q differs from current revision %q",
					ref, obsRev, current,
				),
			})
			continue
		}

		if structured.ExitCode != 0 {
			findings = append(findings, ObservationFinding{
				Path:           path,
				Line:           line,
				RequirementRef: ref,
				Kind:           FindingNonZeroObservation,
				Severity:       SeverityError,
				Message: fmt.Sprintf(
					"requirement %q observation at current revision %q has non-zero exit code %d",
					ref, current, structured.ExitCode,
				),
			})
			continue
		}
		// Current revision + exit 0: pass for this row.
	}
	return findings
}

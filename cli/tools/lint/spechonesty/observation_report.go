// Observation-report correlation (phase2-doc-truth-plan WB-1 step 4).
//
// execute.go produces and persists ObservationReportRow values (one per
// executed Verification mapping-row command, keyed by document id,
// requirement, command, revision, exit code, and observed evidence).
// observation.go validates already-correlated Observation values against a
// document's Verification rows. This file is the bridge: it converts
// persisted report rows into Observation values for one document and runs
// them through CheckObservationFreshness, so a document mapping without a
// current-revision, exit-zero, evidenced report row is rejected exactly
// like a document with no observations at all. Pure and read-only: never
// executes commands, fetches network state, or mutates the report.
package spechonesty

import "strings"

// ObservationFromReportRow converts one persisted ObservationReportRow
// (execute.go) into an Observation usable by CheckObservationFreshness.
func ObservationFromReportRow(row ObservationReportRow) Observation {
	return Observation{
		RequirementRef:  row.RequirementRef,
		Command:         row.Command,
		Revision:        row.Revision,
		ExitCode:        row.ExitCode,
		ExitCodePresent: true,
		Evidence:        row.Evidence,
	}
}

// ObservationsForDocument filters report to the rows recorded for
// documentID and converts them to Observation values. Order is preserved.
func ObservationsForDocument(report []ObservationReportRow, documentID string) []Observation {
	var out []Observation
	for _, row := range report {
		if row.DocumentID != documentID {
			continue
		}
		out = append(out, ObservationFromReportRow(row))
	}
	return out
}

// CheckDocumentObservationReport parses status and Verification rows for
// content, correlates report to the document's canonical id (falling back
// to path when no id can be extracted, mirroring ExecuteVerificationRows'
// own fallback), and validates the resulting Observation set against
// currentRevision via CheckObservationFreshness.
//
// Non-Complete/Implemented statuses and documents with zero Verification
// rows produce no findings (sibling passes own those cases). A nil or
// empty report is treated as "no observations exist for this document" —
// every Complete/Implemented mapping row then fails with
// FindingMissingObservation, matching WB-1's requirement that Complete
// status never rests on the mere existence of a mapping row.
func CheckDocumentObservationReport(path, content string, report []ObservationReportRow, currentRevision string) []ObservationFinding {
	statusRes := ParseDocumentStatusMarkdown(path, content)
	if !IsCompleteStatus(statusRes.Status) {
		return nil
	}
	model := ParseVerificationMarkdown(path, content)
	if len(model.Rows) == 0 {
		return nil
	}

	docID, _, ok := ExtractDocumentID(path, content)
	if !ok || strings.TrimSpace(docID) == "" {
		docID = path
	}

	return CheckObservationFreshness(FreshnessInput{
		CurrentRevision: currentRevision,
		Status:          statusRes.Status,
		Path:            path,
		Rows:            model.Rows,
		Observations:    ObservationsForDocument(report, docID),
	})
}

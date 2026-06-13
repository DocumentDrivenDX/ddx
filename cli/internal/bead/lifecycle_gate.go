package bead

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const lifecycleMigrationGateSampleLimit = 5

const (
	LifecycleMigrationGateCodeNotRequired = "lifecycle_migration_not_required"
	LifecycleMigrationGateCodeRequired    = "lifecycle_migration_required"
)

// LifecycleMigrationGateStatus summarizes whether the active bead queue still
// contains legacy lifecycle routing state.
type LifecycleMigrationGateStatus struct {
	Code string `json:"code"`

	QueuePresent                     bool     `json:"queue_present"`
	SchemaMarkerMissing              bool     `json:"schema_marker_missing"`
	SchemaMarkerOld                  bool     `json:"schema_marker_old"`
	SchemaMarkerVersion              int      `json:"schema_marker_version,omitempty"`
	LegacyNeedsHumanLabels           int      `json:"legacy_needs_human_labels"`
	LegacyNeedsInvestigationLabels   int      `json:"legacy_needs_investigation_labels"`
	LegacyNeedsInvestigationStatuses int      `json:"legacy_needs_investigation_pseudo_statuses"`
	LegacyNoChangesMetadataRows      int      `json:"legacy_no_changes_metadata_rows"`
	SampleBeadIDs                    []string `json:"sample_bead_ids,omitempty"`
}

// Required reports whether the queue still needs lifecycle migration.
func (r LifecycleMigrationGateStatus) Required() bool {
	return r.LegacyNeedsHumanLabels > 0 ||
		r.LegacyNeedsInvestigationLabels > 0 ||
		r.LegacyNeedsInvestigationStatuses > 0 ||
		r.SchemaMarkerOld
}

// Error renders a concise operator-facing summary.
func (r LifecycleMigrationGateStatus) Error() string {
	return r.summary()
}

// Err returns a coded error when migration is required, or nil otherwise.
func (r LifecycleMigrationGateStatus) Err() error {
	if !r.Required() {
		return nil
	}
	return &LifecycleMigrationGateError{report: r}
}

// LifecycleMigrationGateError is the coded error form of a required gate.
type LifecycleMigrationGateError struct {
	report LifecycleMigrationGateStatus
}

// Error renders a concise operator-facing summary.
func (e *LifecycleMigrationGateError) Error() string {
	if e == nil {
		return ""
	}
	if e.report.Code == "" {
		return e.report.summary()
	}
	return "bead: lifecycle migration required: " + e.report.summary()
}

// Code returns the machine-readable gate code.
func (e *LifecycleMigrationGateError) Code() string {
	if e == nil {
		return ""
	}
	return e.report.Code
}

func (r LifecycleMigrationGateStatus) summary() string {
	var parts []string
	if r.LegacyNeedsHumanLabels > 0 {
		parts = append(parts, fmt.Sprintf("%d needs_human label(s)", r.LegacyNeedsHumanLabels))
	}
	if r.LegacyNeedsInvestigationLabels > 0 {
		parts = append(parts, fmt.Sprintf("%d triage:needs-investigation label(s)", r.LegacyNeedsInvestigationLabels))
	}
	if r.LegacyNeedsInvestigationStatuses > 0 {
		parts = append(parts, fmt.Sprintf("%d needs_investigation pseudo-status(es)", r.LegacyNeedsInvestigationStatuses))
	}
	if r.LegacyNoChangesMetadataRows > 0 {
		parts = append(parts, fmt.Sprintf("%d legacy no_changes metadata row(s)", r.LegacyNoChangesMetadataRows))
	}
	if r.SchemaMarkerMissing {
		parts = append(parts, "lifecycle schema marker missing")
	}
	if r.SchemaMarkerOld {
		parts = append(parts, fmt.Sprintf("lifecycle schema marker version %d is older than %d", r.SchemaMarkerVersion, LifecycleSchemaMarkerVersion))
	}
	if len(r.SampleBeadIDs) > 0 {
		parts = append(parts, "sample bead IDs: "+strings.Join(r.SampleBeadIDs, ", "))
	}
	if len(parts) == 0 {
		return "lifecycle migration not required"
	}
	return strings.Join(parts, "; ")
}

// detectLifecycleMigrationRequired reports whether the active JSONL bead queue still
// contains unmigrated lifecycle routing state.
//
// The detector is intentionally cheap:
//   - non-default collections and non-JSONL backends are treated as
//     migration-not-required because they do not represent the active
//     beads.jsonl queue;
//   - a missing active beads.jsonl file is also treated as not required
//     rather than as an error;
//   - the lifecycle schema marker is reported when present/absent, but
//     marker absence alone does not fail the gate unless legacy rows are
//     also present.
func (s *Store) detectLifecycleMigrationRequired() (LifecycleMigrationGateStatus, error) {
	var report LifecycleMigrationGateStatus
	if s == nil || s.Collection != DefaultCollection || s.backend != nil {
		report.Code = LifecycleMigrationGateCodeNotRequired
		return report, nil
	}

	if _, err := os.Stat(s.File); err != nil {
		if os.IsNotExist(err) {
			report.Code = LifecycleMigrationGateCodeNotRequired
			return report, nil
		}
		return report, fmt.Errorf("bead: lifecycle gate stat %s: %w", s.File, err)
	}

	report.QueuePresent = true
	markerPresent, markerVersion, err := s.detectLifecycleSchemaMarker()
	if err != nil {
		return report, fmt.Errorf("bead: lifecycle gate marker: %w", err)
	}
	report.SchemaMarkerMissing = !markerPresent
	report.SchemaMarkerVersion = markerVersion
	report.SchemaMarkerOld = markerPresent && markerVersion < LifecycleSchemaMarkerVersion

	beads, _, err := s.readAllLatestRaw()
	if err != nil {
		return report, fmt.Errorf("bead: lifecycle gate read %s: %w", s.File, err)
	}
	if len(beads) == 0 && !report.Required() {
		report.Code = LifecycleMigrationGateCodeNotRequired
		return report, nil
	}

	sampleSeen := make(map[string]struct{}, lifecycleMigrationGateSampleLimit)
	addSample := func(id string) {
		if id == "" {
			return
		}
		if len(report.SampleBeadIDs) >= lifecycleMigrationGateSampleLimit {
			return
		}
		if _, ok := sampleSeen[id]; ok {
			return
		}
		sampleSeen[id] = struct{}{}
		report.SampleBeadIDs = append(report.SampleBeadIDs, id)
	}

	for _, b := range beads {
		legacy := false
		if hasLabel(b, LabelNeedsHuman) {
			report.LegacyNeedsHumanLabels++
			legacy = true
		}
		if hasLabel(b, LabelNeedsInvestigation) {
			report.LegacyNeedsInvestigationLabels++
			legacy = true
		}
		if b.Status == "needs_investigation" {
			report.LegacyNeedsInvestigationStatuses++
			legacy = true
		}
		if len(noChangesFieldsPresent(b)) > 0 {
			report.LegacyNoChangesMetadataRows++
			legacy = true
		}
		if legacy {
			addSample(b.ID)
		}
	}

	if report.Required() {
		report.Code = LifecycleMigrationGateCodeRequired
	} else {
		report.Code = LifecycleMigrationGateCodeNotRequired
	}

	return report, nil
}

func (s *Store) detectLifecycleSchemaMarker() (present bool, version int, err error) {
	data, err := os.ReadFile(s.LifecycleSchemaMarkerPath())
	if err == nil {
		var marker struct {
			Version int `json:"version"`
		}
		if err := json.Unmarshal(data, &marker); err != nil {
			return true, 0, fmt.Errorf("parse %s: %w", LifecycleSchemaMarkerFile, err)
		}
		return true, marker.Version, nil
	}
	if os.IsNotExist(err) {
		return false, 0, nil
	}
	return false, 0, err
}

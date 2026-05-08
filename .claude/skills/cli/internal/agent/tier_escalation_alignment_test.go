package agent

import (
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/escalation"
)

// TestEscalatableStatusesMatchAgentVocab guards against drift between
// agent's ExecuteBeadStatus* constants and escalation.EscalatableStatuses.
// If you rename a status in execute_bead_status.go, update both places.
func TestEscalatableStatusesMatchAgentVocab(t *testing.T) {
	expected := map[string]bool{
		ExecuteBeadStatusExecutionFailed:            true,
		ExecuteBeadStatusPostRunCheckFailed:         true,
		ExecuteBeadStatusLandConflict:               true,
		ExecuteBeadStatusStructuralValidationFailed: true,
	}
	for status := range expected {
		if !escalation.EscalatableStatuses[status] {
			t.Errorf("agent escalatable status %q is not in escalation.EscalatableStatuses", status)
		}
	}
	for status := range escalation.EscalatableStatuses {
		if !expected[status] {
			t.Errorf("escalation.EscalatableStatuses has %q not in agent's expected set", status)
		}
	}
	// SuccessStatus alignment
	if escalation.SuccessStatus != ExecuteBeadStatusSuccess {
		t.Errorf("escalation.SuccessStatus = %q, want %q (ExecuteBeadStatusSuccess)",
			escalation.SuccessStatus, ExecuteBeadStatusSuccess)
	}
}

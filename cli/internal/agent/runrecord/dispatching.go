package runrecord

import (
	"strings"
	"time"
)

// NewDispatching builds the initial pre-Fizeau run substrate record for one
// DDx attempt. The stable attempt identifier is both directory key and
// identity fields; Fizeau public fields stay nil until a later phase update.
func NewDispatching(attemptID, beadID string, evidence []EvidenceLink) Record {
	now := time.Now().UTC()
	id := strings.TrimSpace(attemptID)
	return Record{
		Version:   SchemaVersion,
		RunID:     id,
		BeadID:    strings.TrimSpace(beadID),
		AttemptID: id,
		Phase:     PhaseDispatching,
		StartedAt: now,
		UpdatedAt: now,
		Evidence:  evidence,
		// Fizeau intentionally nil at dispatching.
	}
}

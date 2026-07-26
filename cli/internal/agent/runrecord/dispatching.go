package runrecord

import (
	"strings"
	"time"
)

// DispatchingParams carries DDx-owned correlation inputs for the initial
// pre-Fizeau run substrate record. Callers supply a clock via Now so unit tests
// can pin created_at/updated_at without wall-clock flakiness.
//
// Fizeau harness/provider/model/route/result fields are not parameters: they
// stay empty on the initial dispatching record until a later phase update after
// public Fizeau data exists.
type DispatchingParams struct {
	// RunID is the durable directory/key identity. When empty, AttemptID is used.
	RunID string
	// BeadID correlates the record to a tracker bead.
	BeadID string
	// AttemptID is the DDx attempt / execution id (also used as RunID when RunID empty).
	AttemptID string
	// WorkerID is the DDx worker identity for this attempt.
	WorkerID string
	// BaseRev is the git base revision the attempt was prepared against.
	BaseRev string
	// PromptPath is a DDx-owned evidence pointer (project-relative path to the
	// prompt file). Persisted as an EvidenceLink named "prompt", never as
	// provider transcript content.
	PromptPath string
	// Correlation is additional DDx-owned metadata (session_id, bundle_path,
	// prompt_sha, …). Keys are copied; empty keys/values are dropped.
	Correlation map[string]string
	// Evidence is optional extra evidence links. A PromptPath entry is merged
	// in as name "prompt" when set (replacing an existing prompt entry).
	Evidence []EvidenceLink
	// Now is the caller-supplied clock. Zero means time.Now().UTC().
	// CreatedAt, StartedAt, and UpdatedAt are all set to this instant for the
	// initial dispatching record.
	Now time.Time
}

// NewDispatching builds the initial pre-Fizeau run substrate record for one
// DDx attempt using the wall clock. Prefer NewDispatchingRecord when the
// caller has first-class correlation fields or a pinned clock.
//
// The stable attempt identifier is both directory key and identity fields;
// Fizeau public fields stay nil until a later phase update.
func NewDispatching(attemptID, beadID string, evidence []EvidenceLink) Record {
	return NewDispatchingRecord(DispatchingParams{
		AttemptID: attemptID,
		BeadID:    beadID,
		Evidence:  evidence,
	})
}

// NewDispatchingRecord builds the initial pre-Fizeau run substrate record from
// DDx-owned correlation inputs. Schema version, run/bead/attempt identity,
// worker ID, base revision, prompt evidence pointer, timestamps, and
// correlation metadata are populated; concrete Fizeau routing fields stay empty.
func NewDispatchingRecord(p DispatchingParams) Record {
	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	attemptID := strings.TrimSpace(p.AttemptID)
	runID := strings.TrimSpace(p.RunID)
	if runID == "" {
		runID = attemptID
	}

	evidence := mergePromptEvidence(p.Evidence, strings.TrimSpace(p.PromptPath))
	corr := copyCorrelation(p.Correlation)

	return Record{
		Version:     SchemaVersion,
		RunID:       runID,
		BeadID:      strings.TrimSpace(p.BeadID),
		AttemptID:   attemptID,
		WorkerID:    strings.TrimSpace(p.WorkerID),
		BaseRev:     strings.TrimSpace(p.BaseRev),
		Phase:       PhaseDispatching,
		CreatedAt:   now,
		StartedAt:   now,
		UpdatedAt:   now,
		Correlation: corr,
		Evidence:    evidence,
		// Fizeau intentionally nil at dispatching.
	}
}

// mergePromptEvidence ensures a DDx-owned prompt path pointer is present as an
// EvidenceLink named "prompt" without storing transcript body content.
func mergePromptEvidence(existing []EvidenceLink, promptPath string) []EvidenceLink {
	if promptPath == "" {
		if len(existing) == 0 {
			return nil
		}
		out := make([]EvidenceLink, len(existing))
		copy(out, existing)
		return out
	}
	link := EvidenceLink{
		Name:      "prompt",
		Path:      promptPath,
		MediaType: "text/markdown",
	}
	if len(existing) == 0 {
		return []EvidenceLink{link}
	}
	return mergeEvidence(existing, []EvidenceLink{link})
}

// copyCorrelation returns a shallow copy of corr without empty keys or values.
func copyCorrelation(corr map[string]string) map[string]string {
	if len(corr) == 0 {
		return nil
	}
	out := make(map[string]string, len(corr))
	for k, v := range corr {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

package agent

import (
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/agent/runrecord"
)

// publishDispatchingRunRecord writes the initial DDx-owned run substrate record
// for a bead attempt under projectRoot/.ddx/runs/<attempt-id>/record.json with
// lifecycle phase "dispatching". It must run before FizeauService.Execute so a
// crash mid-dispatch still leaves a canonical attempt record independent of
// provider session artifacts (Phase 3 WB-2 / ddx-6af9a0f3).
//
// No-op when Correlation lacks attempt_id (plain ddx run / non-bead paths).
// The stable DDx attempt identifier is the record key — never a provider
// session ID or Fizeau route identifier. Fizeau harness/provider/model fields
// stay empty until a later phase update after public Fizeau results exist.
func publishDispatchingRunRecord(projectRoot string, runtime AgentRunRuntime) error {
	corr := runtime.Correlation
	if corr == nil {
		return nil
	}
	attemptID := strings.TrimSpace(corr["attempt_id"])
	if attemptID == "" {
		return nil
	}
	if strings.TrimSpace(projectRoot) == "" {
		return fmt.Errorf("agent: publish dispatching run record: empty project root")
	}

	rec := runrecord.NewDispatching(
		attemptID,
		corr["bead_id"],
		dispatchingEvidenceLinks(corr),
	)
	if err := runrecord.Publish(projectRoot, rec); err != nil {
		return fmt.Errorf("agent: publish dispatching run record: %w", err)
	}
	return nil
}

// dispatchingEvidenceLinks projects known correlation paths into evidence
// pointers without inventing provider-session state. Worker ID and base
// revision are DDx correlation values carried as named evidence so a
// pre-dispatch failure path can prove they survive without schema expansion
// (ddx-02270d66; first-class fields remain a sibling schema bead).
func dispatchingEvidenceLinks(corr map[string]string) []runrecord.EvidenceLink {
	if corr == nil {
		return nil
	}
	var out []runrecord.EvidenceLink
	if p := strings.TrimSpace(corr["prompt_file"]); p != "" {
		out = append(out, runrecord.EvidenceLink{
			Name:      "prompt",
			Path:      p,
			MediaType: "text/markdown",
		})
	}
	if p := strings.TrimSpace(corr["bundle_path"]); p != "" {
		out = append(out, runrecord.EvidenceLink{
			Name: "bundle",
			Path: p,
		})
	}
	if p := strings.TrimSpace(corr["worker_id"]); p != "" {
		out = append(out, runrecord.EvidenceLink{
			Name: "worker_id",
			Path: p,
		})
	}
	if p := strings.TrimSpace(corr["base_rev"]); p != "" {
		out = append(out, runrecord.EvidenceLink{
			Name: "base_rev",
			Path: p,
		})
	}
	return out
}

// verifyDispatchingRunRecordPreserved asserts the DDx-owned dispatching run
// record still exists as valid JSON after a typed pre-dispatch Fizeau Execute
// failure. It never rewrites, deletes, or replaces the record with
// provider-session-derived state (ddx-02270d66).
//
// No-op when Correlation lacks attempt_id (plain ddx run / non-bead paths).
func verifyDispatchingRunRecordPreserved(projectRoot string, runtime AgentRunRuntime) error {
	corr := runtime.Correlation
	if corr == nil {
		return nil
	}
	attemptID := strings.TrimSpace(corr["attempt_id"])
	if attemptID == "" {
		return nil
	}
	if strings.TrimSpace(projectRoot) == "" {
		return fmt.Errorf("agent: pre-dispatch failure: empty project root while verifying run record")
	}

	rec, err := runrecord.Read(projectRoot, attemptID)
	if err != nil {
		return fmt.Errorf("agent: pre-dispatch failure: read run record for attempt %s: %w", attemptID, err)
	}
	if rec == nil {
		return fmt.Errorf("agent: pre-dispatch failure: dispatching run record missing for attempt %s", attemptID)
	}
	if rec.Phase != runrecord.PhaseDispatching {
		return fmt.Errorf("agent: pre-dispatch failure: run record phase is %q, want %q (must not replace dispatching with provider-derived state)",
			rec.Phase, runrecord.PhaseDispatching)
	}
	return nil
}

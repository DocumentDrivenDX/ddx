package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/agent/runrecord"
	agentlib "github.com/easel/fizeau"
)

// finalizeRunRecordFromImmediateError advances the existing dispatching (or
// running) substrate to terminal using only the typed ProviderFailure taxonomy
// and public ImmediateError field. It never queries provider sessions
// (Phase 3 WB-2 / ddx-281ffb67).
//
// No-op when Correlation lacks attempt_id (plain ddx run / non-bead paths).
func finalizeRunRecordFromImmediateError(projectRoot string, runtime AgentRunRuntime, pf ProviderFailure) error {
	corr := runtime.Correlation
	if corr == nil {
		return nil
	}
	attemptID := strings.TrimSpace(corr["attempt_id"])
	if attemptID == "" {
		return nil
	}
	if strings.TrimSpace(projectRoot) == "" {
		return fmt.Errorf("agent: finalize terminal run record (immediate error): empty project root")
	}

	reason := strings.TrimSpace(pf.Reason)
	if reason == "" {
		reason = FailureModeUnknownProviderFailure
	}
	in := runrecord.TerminalInput{
		Outcome: runrecord.Outcome{
			Status:          "failure",
			Reason:          reason,
			EvidenceVerdict: "immediate_error",
		},
		Public: &runrecord.FizeauPublicResult{
			ImmediateError: reason,
		},
	}
	if err := runrecord.TransitionToTerminal(projectRoot, attemptID, in); err != nil {
		return fmt.Errorf("agent: finalize terminal run record (immediate error): %w", err)
	}
	return nil
}

// finalizeRunRecordFromFinal advances the existing record to terminal from a
// public Fizeau final event plus DDx repository-evaluation evidence derived
// from project-local correlation artifacts. No provider-session list/tail APIs
// are used (Phase 3 WB-2 / ddx-281ffb67).
//
// No-op when Correlation lacks attempt_id.
func finalizeRunRecordFromFinal(projectRoot string, runtime AgentRunRuntime, final *agentlib.ServiceFinalData) error {
	corr := runtime.Correlation
	if corr == nil {
		return nil
	}
	attemptID := strings.TrimSpace(corr["attempt_id"])
	if attemptID == "" {
		return nil
	}
	if strings.TrimSpace(projectRoot) == "" {
		return fmt.Errorf("agent: finalize terminal run record (final): empty project root")
	}
	if final == nil {
		return fmt.Errorf("agent: finalize terminal run record (final): nil final event")
	}

	public := fizeauPublicFromFinalWithUsage(final)
	outcome, extra := repositoryEvaluationFromFinal(projectRoot, corr, final)
	in := runrecord.TerminalInput{
		Outcome:            outcome,
		Public:             public,
		AdditionalEvidence: extra,
	}
	if err := runrecord.TransitionToTerminal(projectRoot, attemptID, in); err != nil {
		return fmt.Errorf("agent: finalize terminal run record (final): %w", err)
	}
	return nil
}

// fizeauPublicFromFinalWithUsage maps public final-event fields including
// cost/token usage. Only typed public contract fields are copied.
func fizeauPublicFromFinalWithUsage(final *agentlib.ServiceFinalData) *runrecord.FizeauPublicResult {
	out := fizeauPublicFromFinal(final)
	if out == nil {
		return nil
	}
	if final.CostUSD != nil {
		v := *final.CostUSD
		out.CostUSD = &v
	}
	if final.Usage != nil {
		if final.Usage.InputTokens != nil {
			v := *final.Usage.InputTokens
			out.InputTokens = &v
		}
		if final.Usage.OutputTokens != nil {
			v := *final.Usage.OutputTokens
			out.OutputTokens = &v
		}
		if final.Usage.TotalTokens != nil {
			v := *final.Usage.TotalTokens
			out.TotalTokens = &v
		}
		if final.Usage.CacheReadTokens != nil {
			v := *final.Usage.CacheReadTokens
			out.CachedTokens = &v
		}
	}
	return out
}

// repositoryEvaluationFromFinal builds a DDx-owned outcome and evidence links
// from the public final event plus project-local repository evaluation
// artifacts under the correlation bundle path. It never reads provider session
// stores or process metadata.
func repositoryEvaluationFromFinal(projectRoot string, corr map[string]string, final *agentlib.ServiceFinalData) (runrecord.Outcome, []runrecord.EvidenceLink) {
	status := "success"
	reason := "public_final_ok"
	if final.ExitCode != 0 || strings.TrimSpace(final.Error) != "" {
		status = "failure"
		reason = "public_final_failed"
		if r := strings.TrimSpace(final.Error); r != "" {
			// Keep reason machine-short; full error stays on Fizeau public path.
			if len(r) > 80 {
				r = r[:80]
			}
			reason = r
		}
	} else {
		st := strings.ToLower(strings.TrimSpace(final.Status))
		switch st {
		case "", "success", "completed", "ok", "pass":
			// success
		default:
			status = "failure"
			reason = "public_final_status_" + st
		}
	}

	var extra []runrecord.EvidenceLink
	verdict := "public_final_only"

	bundle := ""
	if corr != nil {
		bundle = strings.TrimSpace(corr["bundle_path"])
	}
	if bundle != "" {
		extra = append(extra, runrecord.EvidenceLink{
			Name: "repository_bundle",
			Path: bundle,
		})
		verdict = "bundle_present"

		// Link well-known evaluation artifacts when present under the bundle.
		candidates := []struct {
			name      string
			rel       string
			mediaType string
		}{
			{name: "result", rel: "result.json", mediaType: "application/json"},
			{name: "checks", rel: "checks.json", mediaType: "application/json"},
			{name: "manifest", rel: "manifest.json", mediaType: "application/json"},
		}
		for _, c := range candidates {
			p := filepath.Join(projectRoot, bundle, c.rel)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				extra = append(extra, runrecord.EvidenceLink{
					Name:      c.name,
					Path:      filepath.ToSlash(filepath.Join(bundle, c.rel)),
					MediaType: c.mediaType,
				})
				if c.name == "result" || c.name == "checks" {
					verdict = "result_artifact_present"
				}
			}
		}
	}

	return runrecord.Outcome{
		Status:          status,
		Reason:          reason,
		EvidenceVerdict: verdict,
	}, extra
}

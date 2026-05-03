package agent

import (
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// EvidenceAssemblyTelemetry is the FEAT-022 §15 attempt-bundle block written
// onto review and grading result.json + manifest.json under the
// "evidence_assembly" key. It is additive to the FEAT-014 §19/§20
// runtime-metrics block — consumers that don't know about it continue to work.
type EvidenceAssemblyTelemetry struct {
	Sections    []evidence.EvidenceAssemblySection `json:"sections,omitempty"`
	InputBytes  int                                `json:"input_bytes"`
	OutputBytes int                                `json:"output_bytes"`
	ElapsedMS   int                                `json:"elapsed_ms,omitempty"`
	Harness     string                             `json:"harness,omitempty"`
	Model       string                             `json:"model,omitempty"`
}

// EventBodySummary holds the FEAT-022 §16 compact summary appended to review,
// review-error, and compare-result event bodies. Per the spec, full per-section
// detail stays in the artifact bundle; only these scalar fields land on the
// event so it stays grep-friendly and well under the bead store cap.
type EventBodySummary struct {
	Harness     string
	Model       string
	InputBytes  int
	OutputBytes int
	ElapsedMS   int
}

// formatEventBodySummary returns the canonical line-oriented summary appended
// to an event body. Empty harness/model are omitted; numeric fields always
// render so absence is unambiguous.
func formatEventBodySummary(s EventBodySummary) string {
	var b strings.Builder
	if s.Harness != "" {
		fmt.Fprintf(&b, "harness=%s\n", s.Harness)
	}
	if s.Model != "" {
		fmt.Fprintf(&b, "model=%s\n", s.Model)
	}
	fmt.Fprintf(&b, "input_bytes=%d\n", s.InputBytes)
	fmt.Fprintf(&b, "output_bytes=%d\n", s.OutputBytes)
	fmt.Fprintf(&b, "elapsed_ms=%d", s.ElapsedMS)
	return b.String()
}

// AppendEventSummary returns body with the canonical summary appended, joined
// by a blank line so the head of the body (verdict, failure class) remains the
// first line.
func AppendEventSummary(body string, s EventBodySummary) string {
	summary := formatEventBodySummary(s)
	if body == "" {
		return summary
	}
	return body + "\n" + summary
}

// compareResultEventBody is the FEAT-022 §16 body shape for compare-result
// events emitted by the grading flow. It is summary-only; the full per-arm
// section detail lives on the grading attempt bundle.
func compareResultEventBody(s EventBodySummary) string {
	return formatEventBodySummary(s)
}

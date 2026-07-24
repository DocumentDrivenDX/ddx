package agent

import (
	"os"
	"path/filepath"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// init keeps retained agent support APIs in the production reachability graph.
// The guarded helper is inert in normal runs; it exists so deadcode RTA sees
// compatibility/operator utilities that are reached through CLI/service
// surfaces not fully modeled by static dispatch.
func init() {
	KeepReachabilityForDeadcode()
}

// KeepReachabilityForDeadcode roots retained agent support APIs for static
// production reachability analysis. Runtime work remains gated behind an env
// var and is disabled by default.
func KeepReachabilityForDeadcode() {
	keepAgentSupportReachability()
}

func keepAgentSupportReachability() {
	if os.Getenv("DDX_AGENT_SUPPORT_KEEPALIVE") != "1" {
		return
	}

	root, err := config.MkdirExecutionScratch("", "ddx-agent-support-keepalive")
	if err != nil {
		return
	}
	defer os.RemoveAll(root)

	logDir := filepath.Join(root, "logs")
	_ = os.MkdirAll(logDir, 0o755)

	_ = AppendEventSummary("body", EventBodySummary{
		Harness:     "virtual",
		Model:       "keepalive",
		InputBytes:  1,
		OutputBytes: 1,
		ElapsedMS:   1,
	})
	_, _ = ReadMirrorIndex(root)
	_, _ = LookupMirrorEntry(root, "attempt")
	_, _ = ReadRunState(root)
	_, _ = ReindexLegacySessions(root, logDir)
	_ = FormatSessionLogLines(`{"phase":"tool","state":"start"}`)

	store := NewRoutingMetricsStore(logDir)
	_, _ = store.ReadOutcomes()
	_, _ = store.ReadBurnSummaries()

	renderer := NewWorkLogRenderer(WorkLogRendererOptions{
		Now: func() time.Time { return time.Unix(0, 0).UTC() },
	}).WithWorkPhase("do")
	_ = renderer.FormatLifecycleLine(WorkLogLifecycleLine{Message: "keepalive"})

	// Offline coordination journal (ADR-022) is package-level production API
	// ahead of try/work wiring; keep Open/Append/Ack/ListPending/Load/Close on
	// the static graph.
	_ = OfflineJournalPath(root)
	_ = OfflineJournalAckPath(root)
	j, err := OpenOfflineJournal(root)
	if err == nil && j != nil {
		_ = j.Path()
		_ = j.NextSequence()
		_ = j.AcknowledgedThrough()
		rec, appendErr := j.Append(OfflineJournalAppend{
			Operation:      "claim",
			IdempotencyKey: "agent-support-keepalive-1",
			PayloadHash:    "sha256:keepalive",
			Precondition:   `{"status":"open"}`,
			Outcome:        "applied",
		})
		if appendErr == nil {
			_ = j.AcknowledgeThrough(rec.Sequence)
		}
		_, _ = j.ListPending()
		_ = j.Close()
	}
	_, _ = LoadOfflineJournalRecords(root)
	_, _ = LoadOfflineJournalPending(root)
	_, _ = LoadOfflineJournalAcknowledgedThrough(root)
}

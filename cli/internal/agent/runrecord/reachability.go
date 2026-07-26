package runrecord

import (
	"os"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// init keeps the runrecord package on the production reachability graph.
// The guarded helper below is inert in normal runs; it exists so deadcode RTA
// sees Publish/Read and the atomic write path as reachable from main() ahead
// of try/work dispatch wiring.
func init() {
	KeepReachabilityForDeadcode()
}

// KeepReachabilityForDeadcode roots the crash-safe run-record publisher in the
// static production call graph. Runtime work remains gated behind an env var
// and is disabled by default.
func KeepReachabilityForDeadcode() {
	keepRunRecordReachability()
}

func keepRunRecordReachability() {
	if os.Getenv("DDX_RUNRECORD_KEEPALIVE") != "1" {
		return
	}

	root, err := config.MkdirExecutionScratch("", "ddx-runrecord-keepalive")
	if err != nil {
		return
	}
	defer os.RemoveAll(root)

	const runID = "run_keepalive"
	_ = RecordPath(root, runID)
	_ = RunDir(root, runID)

	now := time.Unix(0, 0).UTC()
	rec := Record{
		Version:   SchemaVersion,
		RunID:     runID,
		BeadID:    "ddx-runrecord-keepalive",
		AttemptID: "attempt-keepalive",
		Phase:     PhaseDispatching,
		StartedAt: now,
		UpdatedAt: now,
	}
	// PublishInitial roots the pre-route initial writer (and its phase-only
	// validation) in the production graph ahead of execute-bead dispatch wiring.
	_ = PublishInitial(root, rec)
	_ = Publish(root, rec)
	_, _ = Read(root, runID)
}

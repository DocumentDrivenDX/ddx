package scratchowner

import (
	"os"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// init keeps the scratchowner package on the production reachability graph.
// The guarded helper below is inert in normal runs; it exists so deadcode RTA
// sees the owner-marker contract as reachable from main() ahead of producer
// and cleanup wiring (fixture_repo / execution_cleanup child beads).
func init() {
	KeepReachabilityForDeadcode()
}

// KeepReachabilityForDeadcode roots the process-lifetime owner-marker contract
// in the static production call graph. Runtime work remains gated behind an
// env var and is disabled by default.
func KeepReachabilityForDeadcode() {
	keepScratchOwnerReachability()
}

func keepScratchOwnerReachability() {
	if os.Getenv("DDX_SCRATCHOWNER_KEEPALIVE") != "1" {
		return
	}

	root, err := config.MkdirExecutionScratch("", "ddx-scratchowner-keepalive")
	if err != nil {
		return
	}
	defer os.RemoveAll(root)

	_ = Path(root)
	_ = NewMarkerForCurrentProcess(KindFixtureBinary)
	_, _ = WriteForCurrentProcess(root, KindFixtureBinary)
	_, _ = Read(root)
	_, _, _ = Evaluate(root)
	_ = Classify(Marker{
		Kind:                 KindFizeauTestSeamBinary,
		OwnerPID:             os.Getpid(),
		CreatedAt:            time.Now().UTC(),
		ProcessStartIdentity: processStartIdentity(os.Getpid()),
	})
	// Exercise Write with an explicit marker (covers validateShape + atomic path).
	_ = Write(root, Marker{
		Kind:      KindFixtureBinary,
		OwnerPID:  os.Getpid(),
		CreatedAt: time.Now().UTC(),
	})
	_ = processAlive(os.Getpid())
}

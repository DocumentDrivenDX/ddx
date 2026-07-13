package agent

import (
	"context"
	"os"
	"time"

	agentlib "github.com/easel/fizeau"
)

// reasonModelDiscoveryProbe marks a provider-CLI descendant reaped because it
// appeared during a ListModelsWithProbeContainment call and was absent from
// the pre-call baseline.
const reasonModelDiscoveryProbe = "model_discovery_probe"

// ListModelsWithProbeContainment wraps svc.ListModels so that any provider-CLI
// descendant process the call spawns (a PTY probe such as `codex
// --no-alt-screen`) is terminated and reaped by process group before this
// function returns, regardless of whether the call succeeded, failed, timed
// out, or ctx was cancelled. Provider processes already running before the
// call (another attempt's live route, a manual session) are recorded in a
// pre-call baseline and are never touched.
//
// This exists because fizeau's subprocess model-discovery path can fire a
// detached goroutine that spawns a PTY-driven provider CLI independent of the
// ctx passed to ListModels (regression of ddx-403e9a23): the caller's
// deadline firing does not guarantee the spawned process has exited.
func ListModelsWithProbeContainment(ctx context.Context, svc agentlib.FizeauService, filter agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	rootPID := os.Getpid()
	baseline, baselineOK := providerChildBaselinePIDs(rootPID)
	models, err := svc.ListModels(ctx, filter)
	if baselineOK {
		reapNewProviderChildren(rootPID, baseline)
	}
	return models, err
}

func providerChildBaselinePIDs(rootPID int) (map[int]struct{}, bool) {
	procs, scanErr := providerChildScanner(context.Background(), rootPID, time.Now().UTC())
	if scanErr != nil {
		return nil, false
	}
	baseline := make(map[int]struct{}, len(procs))
	for _, p := range procs {
		baseline[p.PID] = struct{}{}
	}
	return baseline, true
}

// reapNewProviderChildren terminates and reaps every provider-CLI descendant
// of rootPID absent from baseline. It scans via context.Background()
// deliberately: the caller's ctx may already be cancelled or expired by the
// time containment runs, and containment must still complete.
func reapNewProviderChildren(rootPID int, baseline map[int]struct{}) []providerChildReapRecord {
	reaped, _, err := reapProviderChildren(context.Background(), rootPID, time.Now().UTC(), func(proc providerChildProcess) string {
		if _, seen := baseline[proc.PID]; seen {
			return ""
		}
		return reasonModelDiscoveryProbe
	})
	if err != nil {
		return nil
	}
	return reaped
}

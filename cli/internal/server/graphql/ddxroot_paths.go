package graphql

import (
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

func projectStatePath(projectRoot string, elems ...string) string {
	return ddxroot.JoinProject(projectRoot, elems...)
}

// projectStore is the GraphQL package's per-project bead surface. Callers
// program against this interface rather than *bead.Store (TD-027 §21).
// Construction remains bead.NewStore at the package boundary.
type projectStore interface {
	bead.Backend
	UpdateWithLifecycleStatus(id string, status string, opts bead.LifecycleTransitionOptions, mutate func(*bead.Bead) error) error
	Reopen(id string, reason string, appendNotes string) error
	TransitionLifecycle(id string, status string, opts bead.LifecycleTransitionOptions, mutate func(*bead.Bead) error) error
	SetLifecycleStatus(id string, status string, opts bead.LifecycleTransitionOptions) error
	// ClaimLease is required by activework.Collect for live claim liveness.
	ClaimLease(id string) (bead.ClaimLeaseRecord, bool, error)
}

func projectBeadStore(projectRoot string) projectStore {
	return bead.NewStore(projectStatePath(projectRoot))
}

package perf

import (
	"fmt"
	"testing"
	"time"
)

// TestBeadsListSmoke_PerProject_Under1s covers ddx-9ce6842a AC §8 (ceiling
// side). Opening the per-project `/beads` page results in a
// beadsByProject(first=50) GraphQL request — the page is interactive once
// that request returns. A generous 1 s ceiling protects against outright
// regression; the harness baseline records the real p95 for fine-grained
// tracking.
func TestBeadsListSmoke_PerProject_Under1s(t *testing.T) {
	if testing.Short() {
		t.Skip("smoke test skipped in -short mode")
	}
	spec := DefaultBeadFixtureSpec()
	f := BuildBeadFixture(t, spec)

	query := fmt.Sprintf(`{ beadsByProject(projectID: %q, first: 50) { totalCount edges { node { id title status labels } } } }`, f.TargetProject.ID)
	// Warm once so the first sample isn't a cold-cache outlier.
	if err := PostGraphQL(f.Handler, query, nil); err != nil {
		t.Fatalf("warm: %v", err)
	}

	start := time.Now()
	if err := PostGraphQL(f.Handler, query, nil); err != nil {
		t.Fatalf("query: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > time.Second {
		t.Fatalf("per-project /beads smoke: %s > 1s ceiling", elapsed)
	}
	t.Logf("per-project /beads smoke: %s (ceiling 1s)", elapsed)
}

// TestBeadsListSmoke_CrossProject_Under2s covers ddx-9ce6842a AC §8 (cross-
// project ceiling). The cross-project view calls beads(first=50) with no
// projectID, which necessarily iterates every registered project. The
// ceiling is 2 s — this is not a p95 target, it is a "the page is not
// broken" guard.
func TestBeadsListSmoke_CrossProject_Under2s(t *testing.T) {
	if testing.Short() {
		t.Skip("smoke test skipped in -short mode")
	}
	spec := DefaultBeadFixtureSpec()
	f := BuildBeadFixture(t, spec)

	query := `{ beads(first: 50) { totalCount edges { node { id title status } } } }`
	if err := PostGraphQL(f.Handler, query, nil); err != nil {
		t.Fatalf("warm: %v", err)
	}

	start := time.Now()
	if err := PostGraphQL(f.Handler, query, nil); err != nil {
		t.Fatalf("query: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("cross-project /beads smoke: %s > 2s ceiling", elapsed)
	}
	t.Logf("cross-project /beads smoke: %s (ceiling 2s)", elapsed)
}

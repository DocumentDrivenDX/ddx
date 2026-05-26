package cmd

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	agentpkg "github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOperatorAttentionSurfacesWedgeReleases verifies AC #1: `ddx bead
// operator-attention` lists every lease release caused by a route-resolution
// timeout, a progress-watchdog fire, or the consecutive-wedge guard, each
// showing bead-id, attempt-id, last_activity_at, and a diagnosis string. The
// three release events are appended in the same JSON shape the worker's
// release primitives emit (execute_bead_loop.go).
func TestOperatorAttentionSurfacesWedgeReleases(t *testing.T) {
	routeBead := &bead.Bead{ID: "ddx-oa-route", Title: "Route timeout"}
	watchdogBead := &bead.Bead{ID: "ddx-oa-watchdog", Title: "Watchdog fire"}
	wedgeBead := &bead.Bead{ID: "ddx-oa-wedge", Title: "Consecutive wedge", Status: bead.StatusProposed}
	noiseBead := &bead.Bead{ID: "ddx-oa-noise", Title: "No release"}
	_, factory, store := setupBeadHumanEnv(t, routeBead, watchdogBead, wedgeBead, noiseBead)

	routeBody, _ := json.Marshal(map[string]any{
		"reason":           agentpkg.FailureModeRouteResolutionTimeout,
		"bead_id":          routeBead.ID,
		"attempt_id":       "attempt-route-1",
		"last_activity_at": "2026-05-26T12:00:00Z",
		"diagnosis":        "route resolution exceeded 1m0s; released lease and flagged for operator attention (no auto-retry)",
		"timeout":          "1m0s",
	})
	require.NoError(t, store.AppendEvent(routeBead.ID, bead.BeadEvent{
		Kind:      "operator_attention",
		Summary:   agentpkg.FailureModeRouteResolutionTimeout,
		Body:      string(routeBody),
		Actor:     "worker-a",
		Source:    "ddx work",
		CreatedAt: time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
	}))

	watchdogBody, _ := json.Marshal(map[string]any{
		"reason":           agentpkg.FailureModeProgressWatchdog,
		"bead_id":          watchdogBead.ID,
		"attempt_id":       "attempt-watchdog-1",
		"last_activity_at": "2026-05-26T12:05:00Z",
		"diagnosis":        "phase-empty heartbeats persisted past the 30m0s budget while phase=\"running\"; released lease and flagged for operator attention",
		"budget":           "30m0s",
	})
	require.NoError(t, store.AppendEvent(watchdogBead.ID, bead.BeadEvent{
		Kind:      "operator_attention",
		Summary:   agentpkg.FailureModeProgressWatchdog,
		Body:      string(watchdogBody),
		Actor:     "worker-a",
		Source:    "ddx work",
		CreatedAt: time.Date(2026, 5, 26, 12, 5, 0, 0, time.UTC),
	}))

	wedgeBody, _ := json.Marshal(map[string]any{
		"reason":           agentpkg.FailureModeConsecutiveWedge,
		"bead_id":          wedgeBead.ID,
		"count":            2,
		"threshold":        2,
		"last_reason":      agentpkg.FailureModeProgressWatchdog,
		"last_activity_at": "2026-05-26T12:10:00Z",
		"diagnosis":        "bead wedged on 2 consecutive claims (>= threshold 2; last wedge \"progress_watchdog\"); stopped re-claiming and flagged for operator attention",
	})
	require.NoError(t, store.AppendEvent(wedgeBead.ID, bead.BeadEvent{
		Kind:      "operator_attention",
		Summary:   agentpkg.FailureModeConsecutiveWedge,
		Body:      string(wedgeBody),
		Actor:     "worker-a",
		Source:    "ddx work",
		CreatedAt: time.Date(2026, 5, 26, 12, 10, 0, 0, time.UTC),
	}))

	// A non-release operator_attention event (different summary) must be ignored.
	require.NoError(t, store.AppendEvent(noiseBead.ID, bead.BeadEvent{
		Kind:      "operator_attention",
		Summary:   "provider_connectivity",
		Body:      `{"reason":"provider_connectivity","bead_id":"ddx-oa-noise"}`,
		Actor:     "worker-a",
		Source:    "ddx work",
		CreatedAt: time.Date(2026, 5, 26, 12, 15, 0, 0, time.UTC),
	}))

	out, err := executeCommand(factory.NewRootCommand(), "bead", "operator-attention", "--json")
	require.NoError(t, err)

	var rows []beadOperatorAttentionRow
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	require.Len(t, rows, 3, "every wedge/timeout release surfaces; unrelated operator_attention events are excluded")

	byBead := make(map[string]beadOperatorAttentionRow, len(rows))
	for _, row := range rows {
		byBead[row.BeadID] = row
	}

	route := byBead[routeBead.ID]
	assert.Equal(t, agentpkg.FailureModeRouteResolutionTimeout, route.Reason)
	assert.Equal(t, "attempt-route-1", route.AttemptID)
	assert.Equal(t, "2026-05-26T12:00:00Z", route.LastActivityAt)
	assert.Contains(t, route.Diagnosis, "route resolution exceeded")

	watchdog := byBead[watchdogBead.ID]
	assert.Equal(t, agentpkg.FailureModeProgressWatchdog, watchdog.Reason)
	assert.Equal(t, "attempt-watchdog-1", watchdog.AttemptID)
	assert.Equal(t, "2026-05-26T12:05:00Z", watchdog.LastActivityAt)
	assert.Contains(t, watchdog.Diagnosis, "phase-empty heartbeats")

	wedge := byBead[wedgeBead.ID]
	assert.Equal(t, agentpkg.FailureModeConsecutiveWedge, wedge.Reason)
	assert.Equal(t, "2026-05-26T12:10:00Z", wedge.LastActivityAt)
	assert.Contains(t, wedge.Diagnosis, "consecutive claims")

	// Text output shows the four triage fields for each release.
	text, err := executeCommand(factory.NewRootCommand(), "bead", "operator-attention")
	require.NoError(t, err)
	assert.Contains(t, text, routeBead.ID)
	assert.Contains(t, text, "attempt=attempt-route-1")
	assert.Contains(t, text, "last_activity_at=2026-05-26T12:00:00Z")
	assert.Contains(t, text, watchdogBead.ID)
	assert.Contains(t, text, "attempt=attempt-watchdog-1")
	assert.Contains(t, text, wedgeBead.ID)
	assert.Contains(t, text, "last_activity_at=2026-05-26T12:10:00Z")
	assert.NotContains(t, text, noiseBead.ID)
}

// TestOperatorAttentionContractDocumented verifies AC #2: AGENTS.md documents
// the route-resolution timeout default (60s), the watchdog phase budgets, the
// consecutive-wedge threshold, and the `ddx bead operator-attention` inspection
// command.
func TestOperatorAttentionContractDocumented(t *testing.T) {
	raw, err := os.ReadFile("../../AGENTS.md")
	require.NoError(t, err)
	doc := strings.ToLower(string(raw))

	assert.Contains(t, doc, "route-resolution timeout", "contract names the route-resolution timeout")
	assert.Contains(t, doc, "60s", "route-resolution timeout default is documented")
	assert.Contains(t, doc, "5m", "watchdog resolving phase budget is documented")
	assert.Contains(t, doc, "30m", "watchdog running phase budget is documented")
	assert.Contains(t, doc, "consecutive-wedge", "contract names the consecutive-wedge guard")
	assert.Contains(t, doc, strings.ToLower("DefaultConsecutiveWedgeThreshold"), "consecutive-wedge threshold default is documented")
	assert.Contains(t, doc, "ddx bead operator-attention", "inspection command is documented")
}

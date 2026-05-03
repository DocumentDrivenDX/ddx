package agentmetrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

const (
	runStoreSubdir = ".ddx/exec/runs"
	bundlesSubdir  = ".ddx/executions"
)

// LoadAttempts scans the FEAT-010 run-store first, then .ddx/executions
// bundles, and returns one Attempt per execute-bead try. When the same
// AttemptID appears in both sources the run-store record wins. Attempts
// are returned sorted by StartedAt ascending (newest last) so callers
// computing windowed aggregations can take a tail slice.
func LoadAttempts(workingDir string) ([]Attempt, error) {
	enrich, err := loadRoutingEnrichment(workingDir)
	if err != nil {
		return nil, err
	}

	out := make([]Attempt, 0, 64)
	seen := make(map[string]bool)

	rs, err := loadFromRunStore(workingDir)
	if err != nil {
		return nil, err
	}
	for _, a := range rs {
		applyEnrichment(&a, enrich)
		a.Bucket = ClassifyBucket(a.Status)
		a.Tier = TierKey(a.Harness, a.Model)
		out = append(out, a)
		seen[a.AttemptID] = true
	}

	bd, err := loadFromBundles(workingDir)
	if err != nil {
		return nil, err
	}
	for _, a := range bd {
		if seen[a.AttemptID] {
			continue
		}
		applyEnrichment(&a, enrich)
		a.Bucket = ClassifyBucket(a.Status)
		a.Tier = TierKey(a.Harness, a.Model)
		out = append(out, a)
		seen[a.AttemptID] = true
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out, nil
}

// TierKey is the canonical "harness/model" tier label used by other Story
// 11 surfaces. An empty model collapses to just the harness.
func TierKey(harness, model string) string {
	if model == "" {
		return harness
	}
	return harness + "/" + model
}

// runRecord mirrors the FEAT-010 run-store JSON shape (subset). Mirrors
// server/state_runs.go runRecord; intentionally not shared so this package
// stays free of the server import graph.
type runRecord struct {
	ID          string  `json:"id"`
	Layer       string  `json:"layer"`
	Status      string  `json:"status"`
	Outcome     string  `json:"outcome"`
	StartedAt   string  `json:"started_at"`
	CompletedAt string  `json:"completed_at"`
	BeadID      string  `json:"bead_id"`
	Harness     string  `json:"harness"`
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	CostUSD     float64 `json:"cost_usd"`
	DurationMs  int     `json:"duration_ms"`
	ExitCode    int     `json:"exit_code"`
}

func loadFromRunStore(workingDir string) ([]Attempt, error) {
	dir := filepath.Join(workingDir, runStoreSubdir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read run store: %w", err)
	}
	out := make([]Attempt, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var rec runRecord
		if err := json.Unmarshal(raw, &rec); err != nil || rec.ID == "" {
			continue
		}
		// Only try-layer records correspond to execute-bead attempts.
		// Run-layer records (per-invocation harness sessions) are noise
		// for Story 11 aggregations; skip them. Empty layer is treated
		// as try for forward-compat with writers that omit it.
		layer := strings.ToLower(rec.Layer)
		if layer != "" && layer != "try" {
			continue
		}
		a := Attempt{
			AttemptID:  rec.ID,
			BeadID:     rec.BeadID,
			Harness:    rec.Harness,
			Provider:   rec.Provider,
			Model:      rec.Model,
			Status:     rec.Status,
			Outcome:    rec.Outcome,
			CostUSD:    rec.CostUSD,
			DurationMS: rec.DurationMs,
			ExitCode:   rec.ExitCode,
			StartedAt:  parseTime(rec.StartedAt),
			FinishedAt: parseTime(rec.CompletedAt),
			Source:     SourceRunStore,
		}
		out = append(out, a)
	}
	return out, nil
}

// bundleResult mirrors the subset of agent.ExecuteBeadResult this loader
// reads. Kept local so the package does not depend on the agent package.
type bundleResult struct {
	BeadID     string    `json:"bead_id"`
	AttemptID  string    `json:"attempt_id"`
	Outcome    string    `json:"outcome"`
	Status     string    `json:"status"`
	Harness    string    `json:"harness"`
	Provider   string    `json:"provider"`
	Model      string    `json:"model"`
	DurationMS int       `json:"duration_ms"`
	CostUSD    float64   `json:"cost_usd"`
	ExitCode   int       `json:"exit_code"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

func loadFromBundles(workingDir string) ([]Attempt, error) {
	dir := filepath.Join(workingDir, bundlesSubdir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read bundles dir: %w", err)
	}
	out := make([]Attempt, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		bundleID := entry.Name()
		resultPath := filepath.Join(dir, bundleID, "result.json")
		raw, err := os.ReadFile(resultPath)
		if err != nil {
			continue
		}
		var res bundleResult
		if err := json.Unmarshal(raw, &res); err != nil {
			continue
		}
		attemptID := res.AttemptID
		if attemptID == "" {
			attemptID = bundleID
		}
		out = append(out, Attempt{
			AttemptID:  attemptID,
			BeadID:     res.BeadID,
			Harness:    res.Harness,
			Provider:   res.Provider,
			Model:      res.Model,
			Status:     res.Status,
			Outcome:    res.Outcome,
			CostUSD:    res.CostUSD,
			DurationMS: res.DurationMS,
			ExitCode:   res.ExitCode,
			StartedAt:  res.StartedAt,
			FinishedAt: res.FinishedAt,
			Source:     SourceBundle,
		})
	}
	return out, nil
}

// routingFacts is the harness/provider/model the most recent kind:routing
// or kind:escalation-summary event recorded for a bead. Used to fill in
// blanks on legacy attempts whose result.json predates those fields.
type routingFacts struct {
	Harness  string
	Provider string
	Model    string
}

// loadRoutingEnrichment scans the bead store and indexes the most recent
// routing/escalation-summary facts per bead. Best-effort: a missing or
// unreadable bead store yields an empty map, never an error, since the
// loader still has authoritative data on each result.
func loadRoutingEnrichment(workingDir string) (map[string]routingFacts, error) {
	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	beads, err := store.ReadAll()
	if err != nil {
		// Return an empty map so missing bead store does not abort
		// metrics loading.
		return map[string]routingFacts{}, nil
	}
	out := make(map[string]routingFacts, len(beads))
	for _, b := range beads {
		facts := routingFromExtra(b.Extra)
		if facts.Harness == "" && facts.Provider == "" && facts.Model == "" {
			continue
		}
		out[b.ID] = facts
	}
	return out, nil
}

// routingFromExtra walks bead.Extra["events"] in chronological order and
// returns the last kind:routing or kind:escalation-summary facts. Both
// kinds carry resolved_provider / resolved_model in the JSON body; the
// escalation summary additionally exposes a tiers_attempted list whose
// final entry is the winning tier.
func routingFromExtra(extra map[string]any) routingFacts {
	if extra == nil {
		return routingFacts{}
	}
	raw, ok := extra["events"]
	if !ok {
		return routingFacts{}
	}
	items, ok := raw.([]any)
	if !ok {
		return routingFacts{}
	}
	type ev struct {
		kind      string
		body      string
		createdAt time.Time
	}
	events := make([]ev, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := m["kind"].(string)
		if kind != "routing" && kind != "escalation-summary" {
			continue
		}
		body, _ := m["body"].(string)
		var createdAt time.Time
		if v, ok := m["created_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				createdAt = t
			}
		}
		events = append(events, ev{kind: kind, body: body, createdAt: createdAt})
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].createdAt.Before(events[j].createdAt)
	})
	var out routingFacts
	for _, e := range events {
		switch e.kind {
		case "routing":
			var body struct {
				ResolvedProvider string `json:"resolved_provider"`
				ResolvedModel    string `json:"resolved_model"`
				RequestedHarness string `json:"requested_harness"`
			}
			if err := json.Unmarshal([]byte(e.body), &body); err != nil {
				continue
			}
			if body.ResolvedProvider != "" {
				out.Provider = body.ResolvedProvider
			}
			if body.ResolvedModel != "" {
				out.Model = body.ResolvedModel
			}
			if body.RequestedHarness != "" {
				out.Harness = body.RequestedHarness
			}
		case "escalation-summary":
			var body struct {
				WinningTier    string `json:"winning_tier"`
				TiersAttempted []struct {
					Tier    string `json:"tier"`
					Harness string `json:"harness"`
					Model   string `json:"model"`
					Status  string `json:"status"`
				} `json:"tiers_attempted"`
			}
			if err := json.Unmarshal([]byte(e.body), &body); err != nil {
				continue
			}
			// Use the winning tier when one exists, else the last
			// attempted tier.
			pick := body.WinningTier
			for _, t := range body.TiersAttempted {
				if pick != "" && t.Tier == pick {
					if t.Harness != "" {
						out.Harness = t.Harness
					}
					if t.Model != "" {
						out.Model = t.Model
					}
					break
				}
			}
			if pick == "" && len(body.TiersAttempted) > 0 {
				last := body.TiersAttempted[len(body.TiersAttempted)-1]
				if last.Harness != "" {
					out.Harness = last.Harness
				}
				if last.Model != "" {
					out.Model = last.Model
				}
			}
		}
	}
	return out
}

func applyEnrichment(a *Attempt, enrich map[string]routingFacts) {
	if a.BeadID == "" {
		return
	}
	facts, ok := enrich[a.BeadID]
	if !ok {
		return
	}
	if a.Harness == "" {
		a.Harness = facts.Harness
	}
	if a.Provider == "" {
		a.Provider = facts.Provider
	}
	if a.Model == "" {
		a.Model = facts.Model
	}
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

package agentmetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
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
		a.PowerClass = RouteKey(a.Harness, a.Model)
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
		a.PowerClass = RouteKey(a.Harness, a.Model)
		out = append(out, a)
		seen[a.AttemptID] = true
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out, nil
}

// RouteKey is the canonical "harness/model" powerClass label used by other Story
// 11 surfaces. An empty model collapses to just the harness.
func RouteKey(harness, model string) string {
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
	TokensIn    int     `json:"tokens_in"`
	TokensOut   int     `json:"tokens_out"`
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
			AttemptID:    rec.ID,
			BeadID:       rec.BeadID,
			Harness:      rec.Harness,
			Provider:     rec.Provider,
			Model:        rec.Model,
			Status:       rec.Status,
			Outcome:      rec.Outcome,
			CostUSD:      rec.CostUSD,
			DurationMS:   rec.DurationMs,
			ExitCode:     rec.ExitCode,
			InputTokens:  rec.TokensIn,
			OutputTokens: rec.TokensOut,
			StartedAt:    parseTime(rec.StartedAt),
			FinishedAt:   parseTime(rec.CompletedAt),
			Source:       SourceRunStore,
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

// routingFacts is the harness/provider/model plus routing-intent metadata the
// most recent kind:routing, kind:escalation-summary, or
// execution-routing-intent event recorded for a bead. Used to fill in blanks
// on legacy attempts whose result.json predates those fields.
type routingFacts struct {
	Harness               string
	Provider              string
	Model                 string
	RoutingIntentSource   string
	InferredPowerClass    string
	SmartJustification    string
	RejectedRoutePinCount int
	RoutingIntentDegraded bool
	RoutingIntentNote     string
}

// loadRoutingEnrichment scans the bead store and indexes the most recent
// routing/escalation-summary/routing-intent facts per bead. Best-effort: a
// missing or unreadable bead store yields an empty map, never an error, since
// the loader still has authoritative data on each result.
func loadRoutingEnrichment(workingDir string) (map[string]routingFacts, error) {
	store := bead.NewStore(ddxroot.JoinProject(workingDir))
	beads, err := store.ReadAll(context.Background())
	if err != nil {
		// Return an empty map so missing bead store does not abort
		// metrics loading.
		return map[string]routingFacts{}, nil
	}
	out := make(map[string]routingFacts, len(beads))
	for _, b := range beads {
		facts := routingFromExtra(b.Extra)
		if facts.Harness == "" && facts.Provider == "" && facts.Model == "" &&
			facts.RoutingIntentSource == "" && facts.InferredPowerClass == "" &&
			facts.SmartJustification == "" && facts.RejectedRoutePinCount == 0 &&
			!facts.RoutingIntentDegraded && facts.RoutingIntentNote == "" {
			continue
		}
		out[b.ID] = facts
	}
	return out, nil
}

// routingFromExtra walks bead.Extra["events"] in chronological order and
// returns the last kind:routing, kind:escalation-summary, or
// execution-routing-intent facts. The routing event carries resolved_provider
// / resolved_model, the escalation summary exposes a power_attempts list
// whose final entry is the winning powerClass, and the routing-intent event carries
// the durable bead-hint audit fields.
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
		if kind != "routing" && kind != "escalation-summary" && kind != "execution-routing-intent" {
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
				WinningPowerClass string `json:"winning_power_class"`
				PowerAttempts     []struct {
					PowerClass string `json:"power_class"`
					Harness    string `json:"harness"`
					Model      string `json:"model"`
					Status     string `json:"status"`
				} `json:"power_attempts"`
			}
			if err := json.Unmarshal([]byte(e.body), &body); err != nil {
				continue
			}
			// Use the winning powerClass when one exists, else the last
			// attempted powerClass.
			pick := body.WinningPowerClass
			for _, t := range body.PowerAttempts {
				if pick != "" && t.PowerClass == pick {
					if t.Harness != "" {
						out.Harness = t.Harness
					}
					if t.Model != "" {
						out.Model = t.Model
					}
					break
				}
			}
			if pick == "" && len(body.PowerAttempts) > 0 {
				last := body.PowerAttempts[len(body.PowerAttempts)-1]
				if last.Harness != "" {
					out.Harness = last.Harness
				}
				if last.Model != "" {
					out.Model = last.Model
				}
			}
		case "execution-routing-intent":
			var body struct {
				RoutingIntentSource   string   `json:"routing_intent_source"`
				InferredPowerClass    string   `json:"inferred_power_class"`
				SmartJustification    string   `json:"smart_justification"`
				ActualHarness         string   `json:"actual_harness"`
				ActualProvider        string   `json:"actual_provider"`
				ActualModel           string   `json:"actual_model"`
				RoutingIntentDegraded bool     `json:"routing_intent_degraded"`
				RoutingIntentNote     string   `json:"routing_intent_note"`
				RejectedRoutePins     []string `json:"rejected_route_pins"`
			}
			if err := json.Unmarshal([]byte(e.body), &body); err != nil {
				continue
			}
			if body.RoutingIntentSource != "" {
				out.RoutingIntentSource = body.RoutingIntentSource
			}
			if body.InferredPowerClass != "" {
				out.InferredPowerClass = body.InferredPowerClass
			}
			if body.SmartJustification != "" {
				out.SmartJustification = body.SmartJustification
			}
			if body.ActualHarness != "" {
				out.Harness = body.ActualHarness
			}
			if body.ActualProvider != "" {
				out.Provider = body.ActualProvider
			}
			if body.ActualModel != "" {
				out.Model = body.ActualModel
			}
			out.RoutingIntentDegraded = body.RoutingIntentDegraded
			if body.RoutingIntentNote != "" {
				out.RoutingIntentNote = body.RoutingIntentNote
			}
			out.RejectedRoutePinCount = len(body.RejectedRoutePins)
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
	if a.RoutingIntentSource == "" {
		a.RoutingIntentSource = facts.RoutingIntentSource
	}
	if a.InferredPowerClass == "" {
		a.InferredPowerClass = facts.InferredPowerClass
	}
	if a.SmartJustification == "" {
		a.SmartJustification = facts.SmartJustification
	}
	if a.RejectedRoutePinCount == 0 {
		a.RejectedRoutePinCount = facts.RejectedRoutePinCount
	}
	if !a.RoutingIntentDegraded {
		a.RoutingIntentDegraded = facts.RoutingIntentDegraded
	}
	if a.RoutingIntentNote == "" {
		a.RoutingIntentNote = facts.RoutingIntentNote
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

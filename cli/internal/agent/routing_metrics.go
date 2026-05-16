package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	routingOutcomeFileName = "routing-outcomes.jsonl"
	burnSummaryFileName    = "burn-summaries.jsonl"
)

// RoutingMetricsStore persists minimal routing outcome and quota snapshot data.
type RoutingMetricsStore struct {
	Dir string
}

// NewRoutingMetricsStore creates a store rooted at dir.
func NewRoutingMetricsStore(dir string) *RoutingMetricsStore {
	return &RoutingMetricsStore{Dir: dir}
}

func (s *RoutingMetricsStore) outcomeFile() string {
	return filepath.Join(s.Dir, routingOutcomeFileName)
}

func (s *RoutingMetricsStore) burnFile() string {
	return filepath.Join(s.Dir, burnSummaryFileName)
}

// AppendOutcome writes one routing outcome record.
func (s *RoutingMetricsStore) AppendOutcome(outcome RoutingOutcome) error {
	return appendJSONLRecord(s.outcomeFile(), outcome)
}

// ReadOutcomes loads all recorded routing outcomes.
func (s *RoutingMetricsStore) ReadOutcomes() ([]RoutingOutcome, error) {
	var out []RoutingOutcome
	err := ForEachJSONL[RoutingOutcome](s.outcomeFile(), func(rec RoutingOutcome) error {
		out = append(out, rec)
		return nil
	})
	return out, err
}

// ReadBurnSummaries loads stored burn summaries.
func (s *RoutingMetricsStore) ReadBurnSummaries() ([]BurnSummary, error) {
	var out []BurnSummary
	err := ForEachJSONL[BurnSummary](s.burnFile(), func(rec BurnSummary) error {
		out = append(out, rec)
		return nil
	})
	return out, err
}

func appendJSONLRecord(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (r *Runner) recordRoutingOutcome(result *Result, elapsed time.Duration, opts RunArgs) {
	if r == nil || result == nil || r.Config.SessionLogDir == "" {
		return
	}

	harness, _ := r.registry.Get(result.Harness)
	canonicalTarget := result.Model
	if canonicalTarget == "" {
		canonicalTarget = opts.Model
	}
	if canonicalTarget == "" && harness.DefaultModel != "" {
		canonicalTarget = harness.DefaultModel
	}
	if canonicalTarget == "" {
		canonicalTarget = result.Harness
	}

	outcome := RoutingOutcome{
		Harness:         result.Harness,
		Surface:         harness.Surface,
		CanonicalTarget: canonicalTarget,
		Model:           result.Model,
		ObservedAt:      time.Now().UTC(),
		Success:         result.ExitCode == 0 && result.Error == "",
		LatencyMS:       int(elapsed.Milliseconds()),
		InputTokens:     result.InputTokens,
		OutputTokens:    result.OutputTokens,
		CostUSD:         result.CostUSD,
		NativeSessionID: result.AgentSessionID,
	}

	if opts.Correlation != nil {
		outcome.TraceID = opts.Correlation["trace_id"]
		outcome.SpanID = opts.Correlation["span_id"]
		if outcome.NativeSessionID == "" {
			outcome.NativeSessionID = opts.Correlation["native_session_id"]
		}
		if outcome.NativeLogRef == "" {
			outcome.NativeLogRef = opts.Correlation["native_log_ref"]
		}
	}

	_ = NewRoutingMetricsStore(r.Config.SessionLogDir).AppendOutcome(outcome)
}

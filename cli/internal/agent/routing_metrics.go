package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	routingOutcomeFileName = "routing-outcomes.jsonl"
	quotaSnapshotFileName  = "quota-snapshots.jsonl"
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

func (s *RoutingMetricsStore) snapshotFile() string {
	return filepath.Join(s.Dir, quotaSnapshotFileName)
}

func (s *RoutingMetricsStore) burnFile() string {
	return filepath.Join(s.Dir, burnSummaryFileName)
}

// AppendOutcome writes one routing outcome record.
func (s *RoutingMetricsStore) AppendOutcome(outcome RoutingOutcome) error {
	return appendJSONLRecord(s.outcomeFile(), outcome)
}

// AppendQuotaSnapshot writes one quota snapshot record.
func (s *RoutingMetricsStore) AppendQuotaSnapshot(snapshot QuotaSnapshot) error {
	return appendJSONLRecord(s.snapshotFile(), snapshot)
}

// AppendBurnSummary writes one derived burn summary record.
func (s *RoutingMetricsStore) AppendBurnSummary(summary BurnSummary) error {
	return appendJSONLRecord(s.burnFile(), summary)
}

// ReadOutcomes loads all recorded routing outcomes.
func (s *RoutingMetricsStore) ReadOutcomes() ([]RoutingOutcome, error) {
	return readJSONLRecords[RoutingOutcome](s.outcomeFile())
}

// ReadQuotaSnapshots loads all recorded quota snapshots.
func (s *RoutingMetricsStore) ReadQuotaSnapshots() ([]QuotaSnapshot, error) {
	return readJSONLRecords[QuotaSnapshot](s.snapshotFile())
}

// ReadBurnSummaries loads stored burn summaries.
func (s *RoutingMetricsStore) ReadBurnSummaries() ([]BurnSummary, error) {
	return readJSONLRecords[BurnSummary](s.burnFile())
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

func readJSONLRecords[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []T
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec T
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// BuildBurnSummaries derives a relative subscription-pressure score from quota snapshots.
func BuildBurnSummaries(snapshots []QuotaSnapshot) []BurnSummary {
	type groupKey struct {
		harness string
		surface string
		target  string
	}

	grouped := make(map[groupKey][]QuotaSnapshot)
	for _, snapshot := range snapshots {
		key := groupKey{
			harness: snapshot.Harness,
			surface: snapshot.Surface,
			target:  snapshot.CanonicalTarget,
		}
		grouped[key] = append(grouped[key], snapshot)
	}

	keys := make([]groupKey, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].harness != keys[j].harness {
			return keys[i].harness < keys[j].harness
		}
		if keys[i].surface != keys[j].surface {
			return keys[i].surface < keys[j].surface
		}
		return keys[i].target < keys[j].target
	})

	summaries := make([]BurnSummary, 0, len(keys))
	for _, key := range keys {
		group := grouped[key]
		sort.Slice(group, func(i, j int) bool {
			return group[i].ObservedAt.Before(group[j].ObservedAt)
		})

		first := group[0]
		last := group[len(group)-1]
		burnIndex := 0.0
		if last.UsedPercent > 0 {
			burnIndex = float64(last.UsedPercent) / 100.0
		}

		trend := "flat"
		if len(group) > 1 {
			delta := last.UsedPercent - first.UsedPercent
			switch {
			case delta > 0:
				trend = "rising"
			case delta < 0:
				trend = "falling"
			}
		}

		confidence := 0.4
		if len(group) > 1 {
			confidence = 0.6
		}
		if len(group) > 3 {
			confidence = 0.8
		}
		if len(group) > 6 {
			confidence = 1.0
		}

		basis := fmt.Sprintf("%d snapshot(s)", len(group))
		if first.ObservedAt != (time.Time{}) && last.ObservedAt != (time.Time{}) {
			basis = fmt.Sprintf("%s from %s to %s",
				basis,
				first.ObservedAt.UTC().Format(time.RFC3339),
				last.ObservedAt.UTC().Format(time.RFC3339),
			)
		}

		summaries = append(summaries, BurnSummary{
			Harness:         key.harness,
			Surface:         key.surface,
			CanonicalTarget: key.target,
			ObservedAt:      last.ObservedAt,
			BurnIndex:       burnIndex,
			Trend:           trend,
			Confidence:      confidence,
			Basis:           basis,
		})
	}

	return summaries
}

func (r *Runner) recordRoutingOutcome(result *Result, elapsed time.Duration, opts RunOptions) {
	if r == nil || result == nil || r.Config.SessionLogDir == "" {
		return
	}

	harness, _ := r.Registry.Get(result.Harness)
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

func (r *Runner) recordQuotaSnapshot(harnessName string, harness Harness, quota *QuotaInfo, sampleKind string) {
	if r == nil || r.Config.SessionLogDir == "" || quota == nil {
		return
	}

	snapshot := QuotaSnapshot{
		Harness:         harnessName,
		Surface:         harness.Surface,
		CanonicalTarget: harness.DefaultModel,
		Source:          harness.QuotaCommand,
		ObservedAt:      time.Now().UTC(),
		SampleKind:      sampleKind,
		UsedPercent:     quota.PercentUsed,
		ResetsAt:        quota.ResetDate,
		QuotaState:      "ok",
	}

	if snapshot.CanonicalTarget == "" {
		snapshot.CanonicalTarget = harnessName
	}
	if quota.PercentUsed >= 95 {
		snapshot.QuotaState = "blocked"
	}
	snapshot.WindowMinutes = parseWindowMinutes(quota.LimitWindow)

	_ = NewRoutingMetricsStore(r.Config.SessionLogDir).AppendQuotaSnapshot(snapshot)
}

func parseWindowMinutes(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}

	fields := strings.Fields(raw)
	if len(fields) == 1 {
		raw = fields[0]
	}

	numRe := strings.Builder{}
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			numRe.WriteRune(r)
		} else {
			break
		}
	}
	if numRe.Len() == 0 {
		return 0
	}
	n, err := strconv.Atoi(numRe.String())
	if err != nil {
		return 0
	}

	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "day"):
		return n * 24 * 60
	case strings.Contains(lower, "hour"), strings.HasSuffix(lower, "h"):
		return n * 60
	case strings.Contains(lower, "min"), strings.HasSuffix(lower, "m"):
		return n
	default:
		return n
	}
}

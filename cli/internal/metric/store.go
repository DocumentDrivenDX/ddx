package metric

import (
	"context"
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
)

var errNotMetricArtifact = fmt.Errorf("not a metric artifact")

type Store struct {
	WorkingDir string
	execStore  *ddxexec.Store
}

func NewStore(workingDir string) *Store {
	return &Store{
		WorkingDir: workingDir,
		execStore:  ddxexec.NewStore(workingDir),
	}
}

// ListArtifacts returns all MET-* artifacts in document ID order.
func (s *Store) ListArtifacts() ([]docgraph.Document, error) {
	graph, err := docgraph.BuildGraph(s.WorkingDir)
	if err != nil {
		return nil, err
	}
	ids := graph.All()
	out := make([]docgraph.Document, 0, len(ids))
	for _, id := range ids {
		if !strings.HasPrefix(id, "MET-") {
			continue
		}
		doc, ok := graph.Show(id)
		if !ok {
			continue
		}
		out = append(out, doc)
	}
	return out, nil
}

func (s *Store) Validate(metricID string) (*Definition, *docgraph.Document, error) {
	doc, err := s.loadMetricArtifact(metricID)
	if err != nil {
		return nil, nil, err
	}
	def, err := s.LoadDefinition(metricID)
	if err != nil {
		return nil, nil, err
	}
	if def.MetricID != metricID {
		return nil, nil, fmt.Errorf("metric definition %q does not target %q", def.DefinitionID, metricID)
	}
	if len(def.Command) == 0 {
		return nil, nil, fmt.Errorf("metric definition %q has no command", def.DefinitionID)
	}
	if def.Comparison == "" {
		def.Comparison = ComparisonLowerIsBetter
	}
	switch def.Comparison {
	case ComparisonLowerIsBetter, ComparisonHigherIsBetter:
	default:
		return nil, nil, fmt.Errorf("metric definition %q has invalid comparison %q", def.DefinitionID, def.Comparison)
	}
	return &def, doc, nil
}

func (s *Store) Run(ctx context.Context, metricID string) (HistoryRecord, error) {
	def, _, err := s.Validate(metricID)
	if err != nil {
		return HistoryRecord{}, err
	}
	rec, err := s.execStore.Run(ctx, def.DefinitionID)
	if err != nil {
		return HistoryRecord{}, err
	}
	return metricHistoryFromExec(rec)
}

func (s *Store) Compare(metricID, against string) (HistoryRecord, ComparisonResult, error) {
	history, err := s.History(metricID)
	if err != nil {
		return HistoryRecord{}, ComparisonResult{}, err
	}
	if len(history) == 0 {
		return HistoryRecord{}, ComparisonResult{}, fmt.Errorf("no history for metric %q", metricID)
	}
	if _, err := singleHistoryUnit(metricID, history); err != nil {
		return HistoryRecord{}, ComparisonResult{}, err
	}
	current := history[len(history)-1]
	target, err := selectComparisonTarget(history, against)
	if err != nil {
		return HistoryRecord{}, ComparisonResult{}, err
	}
	result := comparisonFor(current.Value, target.Value, current.Comparison.Direction)
	current.Comparison = result
	return current, result, nil
}

func (s *Store) Trend(metricID string) (TrendSummary, error) {
	history, err := s.History(metricID)
	if err != nil {
		return TrendSummary{}, err
	}
	if len(history) == 0 {
		return TrendSummary{}, fmt.Errorf("no history for metric %q", metricID)
	}
	unit, err := singleHistoryUnit(metricID, history)
	if err != nil {
		return TrendSummary{}, err
	}
	summary := TrendSummary{MetricID: metricID, Min: history[0].Value, Max: history[0].Value, Latest: history[len(history)-1].Value, Unit: unit, UpdatedAt: history[len(history)-1].ObservedAt}
	var sum float64
	for _, rec := range history {
		if rec.Value < summary.Min {
			summary.Min = rec.Value
		}
		if rec.Value > summary.Max {
			summary.Max = rec.Value
		}
		sum += rec.Value
		summary.UpdatedAt = rec.ObservedAt
	}
	summary.Count = len(history)
	summary.Average = sum / float64(len(history))
	return summary, nil
}

// GroupedHistory returns history rows partitioned by unit while preserving
// the observed order within each unit group.
func (s *Store) GroupedHistory(metricID string) ([]HistoryGroup, error) {
	history, err := s.History(metricID)
	if err != nil {
		return nil, err
	}
	return groupHistoryByUnit(history), nil
}

func (s *Store) LoadDefinition(metricID string) (Definition, error) {
	if s.execStore == nil {
		return Definition{}, fmt.Errorf("metric store is not initialized")
	}
	defs, err := s.execStore.ListDefinitions(metricID)
	if err != nil {
		return Definition{}, err
	}
	var (
		best  Definition
		found bool
	)
	for _, def := range defs {
		if def.ID == "" {
			continue
		}
		mapped, err := metricDefinitionFromExec(def)
		if err != nil {
			return Definition{}, err
		}
		if mapped.MetricID != metricID || !mapped.Active {
			continue
		}
		if !found || mapped.CreatedAt.After(best.CreatedAt) || (mapped.CreatedAt.Equal(best.CreatedAt) && mapped.DefinitionID > best.DefinitionID) {
			best = mapped
			found = true
		}
	}
	if !found {
		return Definition{}, fmt.Errorf("metric definition for %q not found", metricID)
	}
	if best.DefinitionID == "" {
		return Definition{}, fmt.Errorf("metric definition for %q is missing definition_id", metricID)
	}
	return best, nil
}

func (s *Store) History(metricID string) ([]HistoryRecord, error) {
	if s.execStore == nil {
		return []HistoryRecord{}, fmt.Errorf("metric store is not initialized")
	}
	records, err := s.execStore.History(metricID, "")
	if err != nil {
		return nil, err
	}
	out := make([]HistoryRecord, 0, len(records))
	for _, rec := range records {
		mapped, err := metricHistoryFromExec(rec)
		if err != nil {
			return nil, err
		}
		if mapped.MetricID == metricID {
			out = append(out, mapped)
		}
	}
	return out, nil
}

func (s *Store) loadMetricArtifact(metricID string) (*docgraph.Document, error) {
	if !strings.HasPrefix(metricID, "MET-") {
		return nil, fmt.Errorf("%w: %s", errNotMetricArtifact, metricID)
	}
	graph, err := docgraph.BuildGraph(s.WorkingDir)
	if err != nil {
		return nil, err
	}
	doc, ok := graph.Show(metricID)
	if !ok {
		return nil, fmt.Errorf("metric artifact %q not found", metricID)
	}
	return &doc, nil
}

func selectComparisonTarget(history []HistoryRecord, against string) (HistoryRecord, error) {
	switch against {
	case "", "latest":
		return history[len(history)-1], nil
	case "baseline":
		return history[0], nil
	default:
		for _, rec := range history {
			if rec.RunID == against {
				return rec, nil
			}
		}
		return HistoryRecord{}, fmt.Errorf("comparison target %q not found", against)
	}
}

func comparisonFor(current, baseline float64, direction string) ComparisonResult {
	delta := current - baseline
	if direction == ComparisonHigherIsBetter {
		delta = baseline - current
	}
	return ComparisonResult{
		Baseline:  baseline,
		Delta:     delta,
		Direction: direction,
	}
}

func singleHistoryUnit(metricID string, history []HistoryRecord) (string, error) {
	var (
		unit    string
		seenSet = map[string]struct{}{}
		seen    []string
	)
	for _, rec := range history {
		if _, ok := seenSet[rec.Unit]; ok {
			continue
		}
		seenSet[rec.Unit] = struct{}{}
		seen = append(seen, rec.Unit)
		if len(seen) == 1 {
			unit = rec.Unit
		}
	}
	if len(seen) <= 1 {
		return unit, nil
	}
	units := make([]string, 0, len(seen))
	for _, v := range seen {
		if v == "" {
			units = append(units, "(none)")
			continue
		}
		units = append(units, v)
	}
	return "", fmt.Errorf("metric %q has mixed units: %s", metricID, strings.Join(units, ", "))
}

func groupHistoryByUnit(history []HistoryRecord) []HistoryGroup {
	if len(history) == 0 {
		return []HistoryGroup{}
	}
	indexByUnit := make(map[string]int, len(history))
	groups := make([]HistoryGroup, 0, len(history))
	for _, rec := range history {
		idx, ok := indexByUnit[rec.Unit]
		if !ok {
			idx = len(groups)
			indexByUnit[rec.Unit] = idx
			groups = append(groups, HistoryGroup{Unit: rec.Unit})
		}
		groups[idx].Records = append(groups[idx].Records, rec)
	}
	return groups
}

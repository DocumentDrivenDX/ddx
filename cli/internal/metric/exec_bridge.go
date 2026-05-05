package metric

import ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"

func metricDefinitionFromExec(def ddxexec.Definition) (Definition, error) {
	out := Definition{
		DefinitionID: def.ID,
		MetricID:     metricArtifactID(def.ArtifactIDs),
		Command:      append([]string{}, def.Executor.Command...),
		Cwd:          def.Executor.Cwd,
		Env:          cloneStringMap(def.Executor.Env),
		Thresholds: Thresholds{
			Warn:    def.Evaluation.Thresholds.WarnMS,
			Ratchet: def.Evaluation.Thresholds.RatchetMS,
			Unit:    "",
		},
		Comparison: def.Evaluation.Comparison,
		Active:     def.Active,
		CreatedAt:  def.CreatedAt,
	}
	if def.Result.Metric != nil {
		out.Thresholds.Unit = def.Result.Metric.Unit
	}
	return out, nil
}

func metricHistoryFromExec(rec ddxexec.RunRecord) (HistoryRecord, error) {
	out := HistoryRecord{
		RunID:        rec.RunID,
		MetricID:     metricArtifactID(rec.ArtifactIDs),
		DefinitionID: rec.DefinitionID,
		ObservedAt:   rec.StartedAt,
		Status:       StatusPass,
		ExitCode:     rec.ExitCode,
		DurationMS:   rec.FinishedAt.Sub(rec.StartedAt).Milliseconds(),
		Stdout:       rec.Result.Stdout,
		Stderr:       rec.Result.Stderr,
		ArtifactID:   metricArtifactID(rec.ArtifactIDs),
	}
	if rec.Status != ddxexec.StatusSuccess {
		out.Status = StatusFail
	}
	if rec.Result.Metric != nil {
		out.Value = rec.Result.Metric.Value
		out.Unit = rec.Result.Metric.Unit
		out.Comparison = ComparisonResult{
			Baseline:  rec.Result.Metric.Comparison.Baseline,
			Delta:     rec.Result.Metric.Comparison.Delta,
			Direction: rec.Result.Metric.Comparison.Direction,
		}
	}
	return out, nil
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func metricArtifactID(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	for _, id := range ids {
		if len(id) >= 4 && id[:4] == "MET-" {
			return id
		}
	}
	return ids[0]
}

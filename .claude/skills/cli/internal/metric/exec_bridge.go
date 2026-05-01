package metric

import (
	"time"

	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
)

func metricDefinitionToExec(def Definition) ddxexec.Definition {
	return ddxexec.Definition{
		ID:          def.DefinitionID,
		ArtifactIDs: []string{def.MetricID},
		Executor: ddxexec.ExecutorSpec{
			Kind:    ddxexec.ExecutorKindCommand,
			Command: append([]string{}, def.Command...),
			Cwd:     def.Cwd,
			Env:     cloneStringMap(def.Env),
		},
		Result: ddxexec.ResultSpec{
			Metric: &ddxexec.MetricResultSpec{
				Unit: def.Thresholds.Unit,
			},
		},
		Evaluation: ddxexec.Evaluation{
			Comparison: def.Comparison,
			Thresholds: ddxexec.Thresholds{
				WarnMS:    def.Thresholds.Warn,
				RatchetMS: def.Thresholds.Ratchet,
			},
		},
		Active:    def.Active,
		CreatedAt: def.CreatedAt,
	}
}

func metricDefinitionFromExec(def ddxexec.Definition) (Definition, error) {
	metricID := ""
	if len(def.ArtifactIDs) > 0 {
		metricID = def.ArtifactIDs[0]
	}
	out := Definition{
		DefinitionID: def.ID,
		MetricID:     metricID,
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

func metricHistoryToRun(rec HistoryRecord) ddxexec.RunRecord {
	startedAt := rec.ObservedAt
	finishedAt := rec.ObservedAt
	if rec.DurationMS > 0 {
		finishedAt = rec.ObservedAt.Add(time.Duration(rec.DurationMS) * time.Millisecond)
	}
	status := ddxexec.StatusFailed
	switch rec.Status {
	case StatusPass:
		status = ddxexec.StatusSuccess
	case StatusFail:
		status = ddxexec.StatusFailed
	}
	manifest := ddxexec.RunManifest{
		RunID:        rec.RunID,
		DefinitionID: rec.DefinitionID,
		ArtifactIDs:  []string{rec.MetricID},
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
		Status:       status,
		ExitCode:     rec.ExitCode,
		Provenance:   ddxexec.Provenance{},
	}
	runResult := ddxexec.RunResult{
		Stdout: rec.Stdout,
		Stderr: rec.Stderr,
		Value:  rec.Value,
		Unit:   rec.Unit,
		Parsed: true,
	}
	if rec.Comparison.Direction != "" || rec.Comparison.Baseline != 0 || rec.Comparison.Delta != 0 {
		runResult.Metric = &ddxexec.MetricObservation{
			ArtifactID:   rec.MetricID,
			DefinitionID: rec.DefinitionID,
			ObservedAt:   rec.ObservedAt,
			Status:       rec.Status,
			Value:        rec.Value,
			Unit:         rec.Unit,
			Samples:      []float64{rec.Value},
			Comparison: ddxexec.ComparisonResult{
				Baseline:  rec.Comparison.Baseline,
				Delta:     rec.Comparison.Delta,
				Direction: rec.Comparison.Direction,
			},
		}
	}
	return ddxexec.RunRecord{
		RunManifest: manifest,
		Result:      runResult,
	}
}

func metricHistoryFromExec(rec ddxexec.RunRecord) (HistoryRecord, error) {
	out := HistoryRecord{
		RunID:        rec.RunID,
		MetricID:     firstArtifactID(rec.ArtifactIDs),
		DefinitionID: rec.DefinitionID,
		ObservedAt:   rec.StartedAt,
		Status:       StatusPass,
		ExitCode:     rec.ExitCode,
		DurationMS:   rec.FinishedAt.Sub(rec.StartedAt).Milliseconds(),
		Stdout:       rec.Result.Stdout,
		Stderr:       rec.Result.Stderr,
		ArtifactID:   firstArtifactID(rec.ArtifactIDs),
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

func firstArtifactID(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

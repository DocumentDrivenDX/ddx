package metric

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/easel/ddx/internal/docgraph"
)

var errNotMetricArtifact = errors.New("not a metric artifact")

type Store struct {
	WorkingDir     string
	MetricsDir     string
	DefinitionsDir string
	HistoryPath    string
}

func NewStore(workingDir string) *Store {
	base := filepath.Join(workingDir, ".ddx", "metrics")
	return &Store{
		WorkingDir:     workingDir,
		MetricsDir:     base,
		DefinitionsDir: filepath.Join(base, "definitions"),
		HistoryPath:    filepath.Join(base, "history.jsonl"),
	}
}

func (s *Store) Init() error {
	if err := os.MkdirAll(s.DefinitionsDir, 0o755); err != nil {
		return err
	}
	return nil
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
	def, doc, err := s.Validate(metricID)
	if err != nil {
		return HistoryRecord{}, err
	}

	cwd := s.WorkingDir
	if def.Cwd != "" {
		if filepath.IsAbs(def.Cwd) {
			cwd = def.Cwd
		} else {
			cwd = filepath.Join(s.WorkingDir, def.Cwd)
		}
	}

	cmd := exec.CommandContext(ctx, def.Command[0], def.Command[1:]...)
	cmd.Dir = cwd
	if len(def.Env) > 0 {
		cmd.Env = append(os.Environ(), flattenEnv(def.Env)...)
	}
	start := time.Now().UTC()
	stdout, stderr, err := captureCommand(cmd)
	duration := time.Since(start)

	value, unit := normalizeMeasurement(stdout)
	if unit == "" {
		unit = def.Thresholds.Unit
	}
	comparison := compareValue(value, def)
	status := StatusPass
	if err != nil || comparison.Delta > def.Thresholds.Ratchet && def.Thresholds.Ratchet > 0 {
		status = StatusFail
	}
	if err != nil && status == StatusPass {
		status = StatusFail
	}

	record := HistoryRecord{
		RunID:        fmt.Sprintf("%s@%s", metricID, start.Format(time.RFC3339Nano)),
		MetricID:     metricID,
		DefinitionID: def.DefinitionID,
		ObservedAt:   start,
		Status:       status,
		Value:        value,
		Unit:         unit,
		Comparison:   comparison,
		DurationMS:   duration.Milliseconds(),
		Stdout:       strings.TrimSpace(stdout),
		Stderr:       strings.TrimSpace(stderr),
		ArtifactID:   doc.ID,
	}
	if err != nil {
		if record.Stderr == "" {
			record.Stderr = err.Error()
		} else {
			record.Stderr = record.Stderr + "\n" + err.Error()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			record.ExitCode = exitErr.ExitCode()
		} else {
			record.ExitCode = 1
		}
	} else if cmd.ProcessState != nil {
		record.ExitCode = cmd.ProcessState.ExitCode()
	}
	if appendErr := s.AppendHistory(record); appendErr != nil {
		return HistoryRecord{}, appendErr
	}
	return record, nil
}

func (s *Store) Compare(metricID, against string) (HistoryRecord, ComparisonResult, error) {
	history, err := s.History(metricID)
	if err != nil {
		return HistoryRecord{}, ComparisonResult{}, err
	}
	if len(history) == 0 {
		return HistoryRecord{}, ComparisonResult{}, fmt.Errorf("no history for metric %q", metricID)
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
	summary := TrendSummary{MetricID: metricID, Min: history[0].Value, Max: history[0].Value, Latest: history[len(history)-1].Value, Unit: history[len(history)-1].Unit, UpdatedAt: history[len(history)-1].ObservedAt}
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
		if rec.Unit != "" {
			summary.Unit = rec.Unit
		}
	}
	summary.Count = len(history)
	summary.Average = sum / float64(len(history))
	return summary, nil
}

func (s *Store) LoadDefinition(metricID string) (Definition, error) {
	entries, err := os.ReadDir(s.DefinitionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return Definition{}, fmt.Errorf("metric definition directory missing: %s", s.DefinitionsDir)
		}
		return Definition{}, err
	}

	var (
		best  Definition
		found bool
	)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(s.DefinitionsDir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return Definition{}, err
		}
		var def Definition
		if err := json.Unmarshal(raw, &def); err != nil {
			return Definition{}, fmt.Errorf("parse metric definition %q: %w", path, err)
		}
		if def.MetricID != metricID || !def.Active {
			continue
		}
		if !found || def.CreatedAt.After(best.CreatedAt) || (def.CreatedAt.Equal(best.CreatedAt) && def.DefinitionID > best.DefinitionID) {
			best = def
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

func (s *Store) SaveDefinition(def Definition) error {
	if def.DefinitionID == "" {
		return fmt.Errorf("definition_id is required")
	}
	if def.MetricID == "" {
		return fmt.Errorf("metric_id is required")
	}
	if err := os.MkdirAll(s.DefinitionsDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(s.DefinitionsDir, def.DefinitionID+".json")
	return withFileLock(path+".lock", func() error {
		raw, err := json.MarshalIndent(def, "", "  ")
		if err != nil {
			return err
		}
		return atomicWriteFile(path, raw, 0o644)
	})
}

func (s *Store) AppendHistory(rec HistoryRecord) error {
	if err := os.MkdirAll(filepath.Dir(s.HistoryPath), 0o755); err != nil {
		return err
	}
	return withFileLock(s.HistoryPath+".lock", func() error {
		f, err := os.OpenFile(s.HistoryPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		raw, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(raw, '\n')); err != nil {
			return err
		}
		return f.Sync()
	})
}

func (s *Store) History(metricID string) ([]HistoryRecord, error) {
	raw, err := os.ReadFile(s.HistoryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []HistoryRecord{}, nil
		}
		return nil, err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	records := []HistoryRecord{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec HistoryRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("parse metric history: %w", err)
		}
		if rec.MetricID == metricID {
			records = append(records, rec)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
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

func compareValue(value float64, def *Definition) ComparisonResult {
	direction := def.Comparison
	if direction == "" {
		direction = ComparisonLowerIsBetter
	}
	if def.Thresholds.Warn == 0 {
		return ComparisonResult{Baseline: value, Delta: 0, Direction: direction}
	}
	return comparisonFor(value, def.Thresholds.Warn, direction)
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

func normalizeMeasurement(stdout string) (float64, string) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return 0, ""
	}
	if value, unit, ok := parseJSONMeasurement(trimmed); ok {
		return value, unit
	}
	if value, unit, ok := parseTextMeasurement(trimmed); ok {
		return value, unit
	}
	return 0, ""
}

func parseJSONMeasurement(text string) (float64, string, bool) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		return 0, "", false
	}
	if raw, ok := obj["value"]; ok {
		switch v := raw.(type) {
		case float64:
			unit, _ := obj["unit"].(string)
			return v, unit, true
		case string:
			if parsed, err := strconv.ParseFloat(v, 64); err == nil {
				unit, _ := obj["unit"].(string)
				return parsed, unit, true
			}
		}
	}
	return 0, "", false
}

var measurementPattern = regexp.MustCompile(`(?i)(-?\d+(?:\.\d+)?)(?:\s*)(ms|s|sec|seconds?)?`)

func parseTextMeasurement(text string) (float64, string, bool) {
	match := measurementPattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return 0, "", false
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, "", false
	}
	unit := ""
	if len(match) >= 3 {
		unit = strings.ToLower(match[2])
	}
	return value, unit, true
}

func captureCommand(cmd *exec.Cmd) (string, string, error) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", "", err
	}
	if err := cmd.Start(); err != nil {
		return "", "", err
	}
	stdoutBytes, _ := io.ReadAll(stdoutPipe)
	stderrBytes, _ := io.ReadAll(stderrPipe)
	err = cmd.Wait()
	return string(stdoutBytes), string(stderrBytes), err
}

func flattenEnv(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func withFileLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	return fn()
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

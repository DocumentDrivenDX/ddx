package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/easel/ddx/internal/docgraph"
)

var errNotExecArtifact = errors.New("not an exec artifact")

type Store struct {
	WorkingDir     string
	ExecDir        string
	DefinitionsDir string
	RunsDir        string
	runCounter     uint64
}

func NewStore(workingDir string) *Store {
	base := filepath.Join(workingDir, ".ddx", "exec")
	return &Store{
		WorkingDir:     workingDir,
		ExecDir:        base,
		DefinitionsDir: filepath.Join(base, "definitions"),
		RunsDir:        filepath.Join(base, "runs"),
	}
}

func (s *Store) Init() error {
	if err := os.MkdirAll(s.DefinitionsDir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(s.RunsDir, 0o755)
}

func (s *Store) ListDefinitions(artifactID string) ([]Definition, error) {
	entries, err := os.ReadDir(s.DefinitionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Definition{}, nil
		}
		return nil, err
	}

	defs := make([]Definition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.DefinitionsDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var def Definition
		if err := json.Unmarshal(raw, &def); err != nil {
			return nil, fmt.Errorf("parse exec definition %q: %w", entry.Name(), err)
		}
		if !def.Active {
			continue
		}
		if artifactID != "" && !containsString(def.ArtifactIDs, artifactID) {
			continue
		}
		defs = append(defs, def)
	}
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].CreatedAt.Equal(defs[j].CreatedAt) {
			return defs[i].ID < defs[j].ID
		}
		return defs[i].CreatedAt.After(defs[j].CreatedAt)
	})
	return defs, nil
}

func (s *Store) ShowDefinition(definitionID string) (Definition, error) {
	entries, err := os.ReadDir(s.DefinitionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return Definition{}, fmt.Errorf("exec definition directory missing: %s", s.DefinitionsDir)
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
		raw, err := os.ReadFile(filepath.Join(s.DefinitionsDir, entry.Name()))
		if err != nil {
			return Definition{}, err
		}
		var def Definition
		if err := json.Unmarshal(raw, &def); err != nil {
			return Definition{}, fmt.Errorf("parse exec definition %q: %w", entry.Name(), err)
		}
		if def.ID != definitionID {
			continue
		}
		if !found || def.CreatedAt.After(best.CreatedAt) || (def.CreatedAt.Equal(best.CreatedAt) && def.ID > best.ID) {
			best = def
			found = true
		}
	}
	if !found {
		return Definition{}, fmt.Errorf("exec definition for %q not found", definitionID)
	}
	if best.ID == "" {
		return Definition{}, fmt.Errorf("exec definition for %q is missing id", definitionID)
	}
	return best, nil
}

func (s *Store) Validate(definitionID string) (*Definition, *docgraph.Document, error) {
	def, err := s.ShowDefinition(definitionID)
	if err != nil {
		return nil, nil, err
	}
	if def.ID == "" {
		return nil, nil, fmt.Errorf("exec definition is missing id")
	}
	if len(def.ArtifactIDs) == 0 {
		return nil, nil, fmt.Errorf("exec definition %q has no artifact_ids", def.ID)
	}
	if def.Executor.Kind != ExecutorKindCommand && def.Executor.Kind != ExecutorKindAgent {
		return nil, nil, fmt.Errorf("exec definition %q has invalid executor kind %q", def.ID, def.Executor.Kind)
	}
	if def.Executor.Kind == ExecutorKindCommand && len(def.Executor.Command) == 0 {
		return nil, nil, fmt.Errorf("exec definition %q has no command", def.ID)
	}
	graph, err := docgraph.BuildGraph(s.WorkingDir)
	if err != nil {
		return nil, nil, err
	}
	var primary *docgraph.Document
	for _, artifactID := range def.ArtifactIDs {
		doc, ok := graph.Show(artifactID)
		if !ok {
			return nil, nil, fmt.Errorf("exec definition %q references missing artifact %q", def.ID, artifactID)
		}
		if primary == nil {
			docCopy := doc
			primary = &docCopy
		}
	}
	return &def, primary, nil
}

func (s *Store) Run(ctx context.Context, definitionID string) (RunRecord, error) {
	def, doc, err := s.Validate(definitionID)
	if err != nil {
		return RunRecord{}, err
	}
	if def.Executor.Kind == ExecutorKindAgent {
		return RunRecord{}, fmt.Errorf("agent executor is not yet implemented for exec definition %q", def.ID)
	}

	cwd := s.WorkingDir
	if def.Executor.Cwd != "" {
		if filepath.IsAbs(def.Executor.Cwd) {
			cwd = def.Executor.Cwd
		} else {
			cwd = filepath.Join(s.WorkingDir, def.Executor.Cwd)
		}
	}

	cmd := osexec.CommandContext(ctx, def.Executor.Command[0], def.Executor.Command[1:]...)
	cmd.Dir = cwd
	if len(def.Executor.Env) > 0 {
		cmd.Env = append(os.Environ(), flattenEnv(def.Executor.Env)...)
	}

	start := time.Now().UTC()
	stdout, stderr, runErr := captureCommand(cmd)
	finished := time.Now().UTC()
	duration := finished.Sub(start)

	result := RunResult{
		Stdout: strings.TrimSpace(stdout),
		Stderr: strings.TrimSpace(stderr),
	}
	value, unit := normalizeMeasurement(stdout)
	if unit == "" && def.Result.Metric != nil {
		unit = def.Result.Metric.Unit
	}
	if unit != "" || value != 0 {
		result.Parsed = true
		result.Value = value
		result.Unit = unit
		result.Metric = &MetricObservation{
			ArtifactID:   doc.ID,
			DefinitionID: def.ID,
			ObservedAt:   start,
			Value:        value,
			Unit:         unit,
			Samples:      []float64{value},
		}
	}

	status := StatusSuccess
	exitCode := 0
	if runErr != nil {
		switch {
		case errors.Is(runErr, context.DeadlineExceeded):
			status = StatusTimedOut
		default:
			status = StatusFailed
		}
		var exitErr *osexec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
	if result.Metric != nil {
		result.Metric.Status = metricStatusForRun(status)
		if status == StatusSuccess {
			result.Metric.Comparison = ComparisonResult{
				Baseline:  value,
				Delta:     0,
				Direction: def.Evaluation.Comparison,
			}
		}
	}

	runID := fmt.Sprintf("%s@%s-%d", def.ID, start.Format(time.RFC3339Nano), atomic.AddUint64(&s.runCounter, 1))
	record := RunRecord{
		RunManifest: RunManifest{
			RunID:        runID,
			DefinitionID: def.ID,
			ArtifactIDs:  def.ArtifactIDs,
			StartedAt:    start,
			FinishedAt:   finished,
			Status:       status,
			ExitCode:     exitCode,
			Attachments: map[string]string{
				"stdout": "stdout.log",
				"stderr": "stderr.log",
				"result": "result.json",
			},
			Provenance: provenance(),
		},
		Result: result,
	}
	if writeErr := s.writeRunBundle(record); writeErr != nil {
		return RunRecord{}, writeErr
	}
	_ = duration
	return record, nil
}

func (s *Store) History(artifactID, definitionID string) ([]RunRecord, error) {
	entries := []RunRecord{}
	err := filepath.WalkDir(s.RunsDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".tmp-") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "manifest.json" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var manifest RunManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			return fmt.Errorf("parse exec manifest %q: %w", path, err)
		}
		if artifactID != "" && !containsString(manifest.ArtifactIDs, artifactID) {
			return nil
		}
		if definitionID != "" && manifest.DefinitionID != definitionID {
			return nil
		}
		rec, err := s.readRunBundle(filepath.Dir(path))
		if err != nil {
			return err
		}
		entries = append(entries, rec)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].StartedAt.Equal(entries[j].StartedAt) {
			return entries[i].RunID < entries[j].RunID
		}
		return entries[i].StartedAt.Before(entries[j].StartedAt)
	})
	return entries, nil
}

func (s *Store) Log(runID string) (string, string, error) {
	bundleDir := filepath.Join(s.RunsDir, runID)
	stdout, err := os.ReadFile(filepath.Join(bundleDir, "stdout.log"))
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	stderr, err := os.ReadFile(filepath.Join(bundleDir, "stderr.log"))
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	return string(stdout), string(stderr), nil
}

func (s *Store) Result(runID string) (RunResult, error) {
	raw, err := os.ReadFile(filepath.Join(s.RunsDir, runID, "result.json"))
	if err != nil {
		return RunResult{}, err
	}
	var result RunResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return RunResult{}, err
	}
	return result, nil
}

func (s *Store) SaveDefinition(def Definition) error {
	if def.ID == "" {
		return fmt.Errorf("id is required")
	}
	if len(def.ArtifactIDs) == 0 {
		return fmt.Errorf("artifact_ids is required")
	}
	if err := os.MkdirAll(s.DefinitionsDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(s.DefinitionsDir, def.ID+".json")
	return withPathLock(path+".lock", func() error {
		raw, err := json.MarshalIndent(def, "", "  ")
		if err != nil {
			return err
		}
		return atomicWriteFile(path, raw, 0o644)
	})
}

func (s *Store) writeRunBundle(rec RunRecord) error {
	if err := os.MkdirAll(s.RunsDir, 0o755); err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp(s.RunsDir, ".tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	manifestRaw, err := json.MarshalIndent(rec.RunManifest, "", "  ")
	if err != nil {
		return err
	}
	resultRaw, err := json.MarshalIndent(rec.Result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tempDir, "manifest.json"), manifestRaw, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tempDir, "result.json"), resultRaw, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tempDir, "stdout.log"), []byte(rec.Result.Stdout), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tempDir, "stderr.log"), []byte(rec.Result.Stderr), 0o644); err != nil {
		return err
	}
	if err := syncPath(tempDir); err != nil {
		return err
	}
	finalDir := filepath.Join(s.RunsDir, rec.RunID)
	if err := os.Rename(tempDir, finalDir); err != nil {
		return err
	}
	return syncPath(s.RunsDir)
}

func (s *Store) readRunBundle(dir string) (RunRecord, error) {
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return RunRecord{}, err
	}
	resultRaw, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		return RunRecord{}, err
	}
	var manifest RunManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return RunRecord{}, err
	}
	var result RunResult
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		return RunRecord{}, err
	}
	return RunRecord{RunManifest: manifest, Result: result}, nil
}

func withPathLock(path string, fn func() error) error {
	lockDir := path
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := os.Mkdir(lockDir, 0o755)
		if err == nil {
			defer os.RemoveAll(lockDir)
			return fn()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("exec lock timeout for %s", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
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

func syncPath(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func provenance() Provenance {
	host, _ := os.Hostname()
	return Provenance{Host: host}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func flattenEnv(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, fmt.Sprintf("%s=%s", key, value))
	}
	sort.Strings(out)
	return out
}

func captureCommand(cmd *osexec.Cmd) (string, string, error) {
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
	stdout, err := io.ReadAll(stdoutPipe)
	if err != nil {
		_ = cmd.Wait()
		return "", "", err
	}
	stderr, err := io.ReadAll(stderrPipe)
	if err != nil {
		_ = cmd.Wait()
		return "", "", err
	}
	waitErr := cmd.Wait()
	return string(stdout), string(stderr), waitErr
}

var measurementPattern = regexp.MustCompile(`(?i)(-?\d+(?:\.\d+)?)(?:\s*)(ms|s|sec|seconds?)?`)

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

func metricStatusForRun(status string) string {
	if status == StatusSuccess {
		return "pass"
	}
	return "fail"
}

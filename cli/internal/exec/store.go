package exec

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
)

var errNotExecArtifact = errors.New("not an exec artifact")

const (
	execDefinitionCollection = "exec-definitions"
	execRunCollection        = "exec-runs"
	execRunAttachmentDir     = "exec-runs.d"
)

type Store struct {
	WorkingDir           string
	ExecDir              string
	DefinitionsDir       string
	RunsDir              string
	DefinitionCollection *bead.Store
	RunCollection        *bead.Store
	AgentRunner          AgentRunner
	runCounter           uint64
}

func NewStore(workingDir string) *Store {
	base := filepath.Join(workingDir, ".ddx", "exec")
	beadRoot := filepath.Join(workingDir, ".ddx")
	return &Store{
		WorkingDir:           workingDir,
		ExecDir:              base,
		DefinitionsDir:       filepath.Join(base, "definitions"),
		RunsDir:              filepath.Join(base, "runs"),
		DefinitionCollection: bead.NewStoreWithCollection(beadRoot, execDefinitionCollection),
		RunCollection:        bead.NewStoreWithCollection(beadRoot, execRunCollection),
	}
}

func (s *Store) Init() error {
	if s.DefinitionCollection != nil {
		if err := s.DefinitionCollection.Init(); err != nil {
			return err
		}
	}
	if s.RunCollection != nil {
		if err := s.RunCollection.Init(); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(s.WorkingDir, ".ddx", execRunAttachmentDir), 0o755); err != nil {
		return err
	}
	return os.MkdirAll(s.RunsDir, 0o755)
}

func (s *Store) ListDefinitions(artifactID string) ([]Definition, error) {
	defs, err := s.loadDefinitions()
	if err != nil {
		return nil, err
	}
	if artifactID == "" {
		return defs, nil
	}
	filtered := make([]Definition, 0, len(defs))
	for _, def := range defs {
		if containsString(def.ArtifactIDs, artifactID) {
			filtered = append(filtered, def)
		}
	}
	return filtered, nil
}

func (s *Store) ShowDefinition(definitionID string) (Definition, error) {
	defs, err := s.loadDefinitions()
	if err != nil {
		return Definition{}, err
	}
	var (
		best  Definition
		found bool
	)
	for _, def := range defs {
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
		if s.AgentRunner == nil {
			return RunRecord{}, fmt.Errorf("agent runner not configured for exec definition %q", def.ID)
		}

		var promptFile, promptText string
		if len(def.Executor.Command) > 0 {
			promptFile = def.Executor.Command[0]
		} else {
			promptText = def.Executor.Env["DDX_AGENT_PROMPT"]
		}

		workDir := def.Executor.Cwd
		if workDir == "" {
			workDir = s.WorkingDir
		} else if !filepath.IsAbs(workDir) {
			workDir = filepath.Join(s.WorkingDir, workDir)
		}

		var timeout time.Duration
		if def.Executor.TimeoutMS > 0 {
			timeout = time.Duration(def.Executor.TimeoutMS) * time.Millisecond
		}

		sessionID := genAgentSessionID()
		opts := agent.RunOptions{
			Harness:    def.Executor.Env["DDX_AGENT_HARNESS"],
			Prompt:     promptText,
			PromptFile: promptFile,
			Correlation: map[string]string{
				"definition_id": def.ID,
				"artifact_ids":  strings.Join(def.ArtifactIDs, ","),
				"session_id":    sessionID,
			},
			Model:   def.Executor.Env["DDX_AGENT_MODEL"],
			Effort:  def.Executor.Env["DDX_AGENT_EFFORT"],
			Timeout: timeout,
			WorkDir: workDir,
		}

		start := time.Now().UTC()
		agentResult, agentErr := s.AgentRunner.Run(opts)
		finished := time.Now().UTC()

		status := StatusSuccess
		exitCode := 0
		var stdout, stderr string
		if agentErr != nil {
			status = StatusErrored
			exitCode = 1
			stderr = agentErr.Error()
		} else {
			exitCode = agentResult.ExitCode
			stdout = strings.TrimSpace(agentResult.Output)
			stderr = strings.TrimSpace(agentResult.Stderr)
			if exitCode != 0 {
				if exitCode == -1 && strings.Contains(agentResult.Error, "timeout") {
					status = StatusTimedOut
				} else {
					status = StatusFailed
				}
			}
		}

		runID := fmt.Sprintf("%s@%s-%d", def.ID, start.Format(time.RFC3339Nano), atomic.AddUint64(&s.runCounter, 1))
		record := RunRecord{
			RunManifest: RunManifest{
				RunID:          runID,
				DefinitionID:   def.ID,
				ArtifactIDs:    def.ArtifactIDs,
				StartedAt:      start,
				FinishedAt:     finished,
				Status:         status,
				ExitCode:       exitCode,
				MergeBlocking:  def.Required && status != StatusSuccess,
				AgentSessionID: sessionID,
				Attachments:    runAttachmentRefs(s.WorkingDir, runID),
				Provenance:     provenance(),
			},
			Result: RunResult{
				Stdout: stdout,
				Stderr: stderr,
			},
		}
		if saveErr := s.saveRunRecord(record); saveErr != nil {
			return RunRecord{}, saveErr
		}
		return record, nil
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
			RunID:         runID,
			DefinitionID:  def.ID,
			ArtifactIDs:   def.ArtifactIDs,
			StartedAt:     start,
			FinishedAt:    finished,
			Status:        status,
			ExitCode:      exitCode,
			MergeBlocking: def.Required && status != StatusSuccess,
			Attachments:   runAttachmentRefs(s.WorkingDir, runID),
			Provenance:    provenance(),
		},
		Result: result,
	}
	if writeErr := s.saveRunRecord(record); writeErr != nil {
		return RunRecord{}, writeErr
	}
	_ = duration
	return record, nil
}

// SaveRunRecord persists a run record without executing the underlying command.
func (s *Store) SaveRunRecord(rec RunRecord) error {
	return s.saveRunRecord(rec)
}

func (s *Store) History(artifactID, definitionID string) ([]RunRecord, error) {
	entries, err := s.loadRuns()
	if err != nil {
		return nil, err
	}
	filtered := make([]RunRecord, 0, len(entries))
	for _, rec := range entries {
		if artifactID != "" && !containsString(rec.ArtifactIDs, artifactID) {
			continue
		}
		if definitionID != "" && rec.DefinitionID != definitionID {
			continue
		}
		filtered = append(filtered, rec)
	}
	return filtered, nil
}

func (s *Store) Log(runID string) (string, string, error) {
	rec, err := s.loadRunByID(runID)
	if err != nil {
		return "", "", err
	}
	return rec.Result.Stdout, rec.Result.Stderr, nil
}

func (s *Store) Result(runID string) (RunResult, error) {
	rec, err := s.loadRunByID(runID)
	if err != nil {
		return RunResult{}, err
	}
	return rec.Result, nil
}

func (s *Store) SaveDefinition(def Definition) error {
	if def.ID == "" {
		return fmt.Errorf("id is required")
	}
	if len(def.ArtifactIDs) == 0 {
		return fmt.Errorf("artifact_ids is required")
	}
	return s.saveDefinitionBead(def)
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
	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	return stdoutBuf.String(), stderrBuf.String(), err
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

func genAgentSessionID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "as-" + hex.EncodeToString(b)
}

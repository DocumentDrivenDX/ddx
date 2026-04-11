package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type ExecuteLoopWorkerSpec struct {
	Harness      string        `json:"harness,omitempty"`
	Model        string        `json:"model,omitempty"`
	Effort       string        `json:"effort,omitempty"`
	Once         bool          `json:"once,omitempty"`
	PollInterval time.Duration `json:"poll_interval,omitempty"`
}

type WorkerRecord struct {
	ID             string                 `json:"id"`
	Kind           string                 `json:"kind"`
	State          string                 `json:"state"`
	Status         string                 `json:"status,omitempty"`
	ProjectRoot    string                 `json:"project_root"`
	Harness        string                 `json:"harness,omitempty"`
	Provider       string                 `json:"provider,omitempty"`
	Model          string                 `json:"model,omitempty"`
	Effort         string                 `json:"effort,omitempty"`
	Once           bool                   `json:"once,omitempty"`
	PollInterval   string                 `json:"poll_interval,omitempty"`
	PID            int                    `json:"pid,omitempty"`
	StartedAt      time.Time              `json:"started_at,omitempty"`
	FinishedAt     time.Time              `json:"finished_at,omitempty"`
	ExitCode       *int                   `json:"exit_code,omitempty"`
	Error          string                 `json:"error,omitempty"`
	StdoutPath     string                 `json:"stdout_path,omitempty"`
	StderrPath     string                 `json:"stderr_path,omitempty"`
	SpecPath       string                 `json:"spec_path,omitempty"`
	Command        []string               `json:"command,omitempty"`
	Attempts       int                    `json:"attempts,omitempty"`
	Successes      int                    `json:"successes,omitempty"`
	Failures       int                    `json:"failures,omitempty"`
	CurrentBead    string                 `json:"current_bead,omitempty"`
	CurrentAttempt string                 `json:"current_attempt,omitempty"`
	LastError      string                 `json:"last_error,omitempty"`
	LastResult     *WorkerExecutionResult `json:"last_result,omitempty"`
}

type WorkerExecutionResult struct {
	BeadID     string `json:"bead_id,omitempty"`
	AttemptID  string `json:"attempt_id,omitempty"`
	WorkerID   string `json:"worker_id,omitempty"`
	Harness    string `json:"harness,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Status     string `json:"status,omitempty"`
	Detail     string `json:"detail,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	BaseRev    string `json:"base_rev,omitempty"`
	ResultRev  string `json:"result_rev,omitempty"`
	RetryAfter string `json:"retry_after,omitempty"`
}

type executeLoopSummary struct {
	Attempts          int                     `json:"attempts"`
	Successes         int                     `json:"successes"`
	Failures          int                     `json:"failures"`
	LastFailureStatus string                  `json:"last_failure_status,omitempty"`
	Results           []WorkerExecutionResult `json:"results"`
}

type workerExecuteBeadManifest struct {
	AttemptID string `json:"attempt_id"`
	WorkerID  string `json:"worker_id,omitempty"`
	BeadID    string `json:"bead_id"`
	BaseRev   string `json:"base_rev,omitempty"`
}

type workerHandle struct {
	record WorkerRecord
	cmd    *exec.Cmd
}

type workerCommandBuilder func(spec ExecuteLoopWorkerSpec) (*exec.Cmd, error)

type WorkerManager struct {
	projectRoot string
	rootDir     string

	mu      sync.Mutex
	workers map[string]*workerHandle
	build   workerCommandBuilder
}

func NewWorkerManager(projectRoot string) *WorkerManager {
	m := &WorkerManager{
		projectRoot: projectRoot,
		rootDir:     filepath.Join(projectRoot, ".ddx", "workers"),
		workers:     map[string]*workerHandle{},
	}
	m.build = m.defaultBuildExecuteLoop
	return m
}

func (m *WorkerManager) StartExecuteLoop(spec ExecuteLoopWorkerSpec) (WorkerRecord, error) {
	if err := os.MkdirAll(m.rootDir, 0o755); err != nil {
		return WorkerRecord{}, err
	}

	id := "worker-" + time.Now().UTC().Format("20060102T150405") + "-" + randomSuffix(4)
	dir := filepath.Join(m.rootDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return WorkerRecord{}, err
	}

	cmd, err := m.build(spec)
	if err != nil {
		return WorkerRecord{}, err
	}
	cmd.Dir = m.projectRoot

	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")
	specPath := filepath.Join(dir, "spec.json")
	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return WorkerRecord{}, err
	}
	defer stdoutFile.Close()
	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return WorkerRecord{}, err
	}
	defer stderrFile.Close()

	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	baseEnv := cmd.Env
	if len(baseEnv) == 0 {
		baseEnv = os.Environ()
	}
	cmd.Env = append(baseEnv,
		"DDX_WORKER_ID="+id,
		"DDX_WORKER_KIND=execute-loop",
	)

	specData, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return WorkerRecord{}, err
	}
	if err := os.WriteFile(specPath, append(specData, '\n'), 0o644); err != nil {
		return WorkerRecord{}, err
	}

	record := WorkerRecord{
		ID:           id,
		Kind:         "execute-loop",
		State:        "starting",
		Status:       "starting",
		ProjectRoot:  m.projectRoot,
		Harness:      spec.Harness,
		Model:        spec.Model,
		Effort:       spec.Effort,
		Once:         spec.Once,
		PollInterval: spec.PollInterval.String(),
		StdoutPath:   relToProject(m.projectRoot, stdoutPath),
		StderrPath:   relToProject(m.projectRoot, stderrPath),
		SpecPath:     relToProject(m.projectRoot, specPath),
		Command:      commandVector(cmd),
		StartedAt:    time.Now().UTC(),
	}
	if err := m.writeRecord(dir, record); err != nil {
		return WorkerRecord{}, err
	}

	if err := cmd.Start(); err != nil {
		record.State = "failed_to_start"
		record.Error = err.Error()
		record.FinishedAt = time.Now().UTC()
		_ = m.writeRecord(dir, record)
		return WorkerRecord{}, err
	}

	record.State = "running"
	record.Status = "running"
	record.PID = cmd.Process.Pid
	if err := m.writeRecord(dir, record); err != nil {
		return WorkerRecord{}, err
	}

	m.mu.Lock()
	m.workers[id] = &workerHandle{record: record, cmd: cmd}
	m.mu.Unlock()

	go m.waitForExit(id, dir)

	return record, nil
}

func (m *WorkerManager) List() ([]WorkerRecord, error) {
	if err := os.MkdirAll(m.rootDir, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(m.rootDir)
	if err != nil {
		return nil, err
	}
	var out []WorkerRecord
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		rec, err := m.readRecord(filepath.Join(m.rootDir, entry.Name()))
		if err != nil {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out, nil
}

func (m *WorkerManager) Show(id string) (WorkerRecord, error) {
	return m.readRecord(filepath.Join(m.rootDir, id))
}

func (m *WorkerManager) Stop(id string) error {
	m.mu.Lock()
	handle := m.workers[id]
	m.mu.Unlock()
	if handle == nil || handle.cmd == nil || handle.cmd.Process == nil {
		return fmt.Errorf("worker not running")
	}
	if err := handle.cmd.Process.Kill(); err != nil {
		return err
	}
	return nil
}

func (m *WorkerManager) Logs(id string) (string, string, error) {
	rec, err := m.Show(id)
	if err != nil {
		return "", "", err
	}
	stdout, err := os.ReadFile(filepath.Join(m.projectRoot, rec.StdoutPath))
	if err != nil {
		return "", "", err
	}
	stderr, err := os.ReadFile(filepath.Join(m.projectRoot, rec.StderrPath))
	if err != nil {
		return "", "", err
	}
	return string(stdout), string(stderr), nil
}

func (m *WorkerManager) waitForExit(id, dir string) {
	m.mu.Lock()
	handle := m.workers[id]
	m.mu.Unlock()
	if handle == nil || handle.cmd == nil {
		return
	}

	err := handle.cmd.Wait()

	m.mu.Lock()
	record := handle.record
	delete(m.workers, id)
	m.mu.Unlock()

	record.FinishedAt = time.Now().UTC()
	if err == nil {
		record.State = "exited"
		record.Status = "exited"
		code := 0
		record.ExitCode = &code
	} else if exitErr, ok := err.(*exec.ExitError); ok {
		record.State = "exited"
		record.Status = "exited"
		code := exitErr.ExitCode()
		record.ExitCode = &code
		record.Error = strings.TrimSpace(exitErr.Error())
	} else {
		record.State = "failed"
		record.Status = "failed"
		record.Error = err.Error()
	}
	m.enrichFromExecuteLoopOutput(dir, &record)
	_ = m.writeRecord(dir, record)
}

func (m *WorkerManager) defaultBuildExecuteLoop(spec ExecuteLoopWorkerSpec) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := []string{"agent", "execute-loop", "--json"}
	if spec.Once {
		args = append(args, "--once")
	}
	if spec.PollInterval > 0 {
		args = append(args, "--poll-interval", spec.PollInterval.String())
	}
	if spec.Harness != "" {
		args = append(args, "--harness", spec.Harness)
	}
	if spec.Model != "" {
		args = append(args, "--model", spec.Model)
	}
	if spec.Effort != "" {
		args = append(args, "--effort", spec.Effort)
	}
	return exec.Command(exe, args...), nil
}

func (m *WorkerManager) writeRecord(dir string, record WorkerRecord) error {
	if record.Status == "" {
		record.Status = record.State
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "status.json"), append(data, '\n'), 0o644)
}

func (m *WorkerManager) readRecord(dir string) (WorkerRecord, error) {
	data, err := os.ReadFile(filepath.Join(dir, "status.json"))
	if err != nil {
		return WorkerRecord{}, err
	}
	var record WorkerRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return WorkerRecord{}, err
	}
	if record.Status == "" {
		record.Status = record.State
	}
	m.enrichFromLiveArtifacts(&record)
	m.enrichFromExecuteLoopOutput(dir, &record)
	return record, nil
}

func (m *WorkerManager) enrichFromLiveArtifacts(record *WorkerRecord) {
	if record == nil || record.ID == "" {
		return
	}
	execRoot := filepath.Join(m.projectRoot, ".ddx", "executions")
	entries, err := os.ReadDir(execRoot)
	if err != nil {
		return
	}
	var newest time.Time
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(execRoot, entry.Name(), "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest workerExecuteBeadManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}
		if manifest.WorkerID != record.ID {
			continue
		}
		info, err := os.Stat(manifestPath)
		if err != nil {
			continue
		}
		if !info.ModTime().After(newest) {
			continue
		}
		newest = info.ModTime()
		record.CurrentBead = manifest.BeadID
		record.CurrentAttempt = manifest.AttemptID
	}
}

func (m *WorkerManager) enrichFromExecuteLoopOutput(dir string, record *WorkerRecord) {
	if record == nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(dir, "stdout.log"))
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		if record.LastError == "" {
			record.LastError = record.Error
		}
		return
	}
	var summary executeLoopSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		if record.LastError == "" {
			record.LastError = record.Error
		}
		return
	}
	record.Attempts = summary.Attempts
	record.Successes = summary.Successes
	record.Failures = summary.Failures
	if len(summary.Results) > 0 {
		last := summary.Results[len(summary.Results)-1]
		record.CurrentBead = last.BeadID
		record.CurrentAttempt = last.AttemptID
		record.LastResult = &last
		if record.Harness == "" && last.Harness != "" {
			record.Harness = last.Harness
		}
		if record.Provider == "" && last.Provider != "" {
			record.Provider = last.Provider
		}
		if record.Model == "" && last.Model != "" {
			record.Model = last.Model
		}
		if last.Status != "" {
			record.Status = last.Status
		}
		if last.Detail != "" {
			record.LastError = last.Detail
		}
	} else if summary.LastFailureStatus != "" {
		record.Status = summary.LastFailureStatus
	}
	if record.LastError == "" {
		record.LastError = record.Error
	}
}

func commandVector(cmd *exec.Cmd) []string {
	if cmd == nil {
		return nil
	}
	return append([]string{cmd.Path}, cmd.Args[1:]...)
}

func relToProject(projectRoot, path string) string {
	rel, err := filepath.Rel(projectRoot, path)
	if err != nil {
		return path
	}
	return rel
}

func randomSuffix(n int) string {
	if n <= 0 {
		n = 4
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())[:n]
	}
	return hex.EncodeToString(buf)[:n]
}

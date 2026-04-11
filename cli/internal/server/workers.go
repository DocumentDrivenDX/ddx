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
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	State        string    `json:"state"`
	ProjectRoot  string    `json:"project_root"`
	Harness      string    `json:"harness,omitempty"`
	Model        string    `json:"model,omitempty"`
	Effort       string    `json:"effort,omitempty"`
	Once         bool      `json:"once,omitempty"`
	PollInterval string    `json:"poll_interval,omitempty"`
	PID          int       `json:"pid,omitempty"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	FinishedAt   time.Time `json:"finished_at,omitempty"`
	ExitCode     *int      `json:"exit_code,omitempty"`
	Error        string    `json:"error,omitempty"`
	StdoutPath   string    `json:"stdout_path,omitempty"`
	StderrPath   string    `json:"stderr_path,omitempty"`
	Command      []string  `json:"command,omitempty"`
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

	record := WorkerRecord{
		ID:           id,
		Kind:         "execute-loop",
		State:        "starting",
		ProjectRoot:  m.projectRoot,
		Harness:      spec.Harness,
		Model:        spec.Model,
		Effort:       spec.Effort,
		Once:         spec.Once,
		PollInterval: spec.PollInterval.String(),
		StdoutPath:   relToProject(m.projectRoot, stdoutPath),
		StderrPath:   relToProject(m.projectRoot, stderrPath),
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
		code := 0
		record.ExitCode = &code
	} else if exitErr, ok := err.(*exec.ExitError); ok {
		record.State = "exited"
		code := exitErr.ExitCode()
		record.ExitCode = &code
		record.Error = strings.TrimSpace(exitErr.Error())
	} else {
		record.State = "failed"
		record.Error = err.Error()
	}
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
	return record, nil
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

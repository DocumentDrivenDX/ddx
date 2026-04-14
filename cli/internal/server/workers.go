package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
)

type ExecuteLoopWorkerSpec struct {
	Harness      string        `json:"harness,omitempty"`
	Model        string        `json:"model,omitempty"`
	Effort       string        `json:"effort,omitempty"`
	Once         bool          `json:"once,omitempty"`
	PollInterval time.Duration `json:"poll_interval,omitempty"`
}

type WorkerRecord struct {
	ID           string                 `json:"id"`
	Kind         string                 `json:"kind"`
	State        string                 `json:"state"`
	Status       string                 `json:"status,omitempty"`
	ProjectRoot  string                 `json:"project_root"`
	Harness      string                 `json:"harness,omitempty"`
	Provider     string                 `json:"provider,omitempty"`
	Model        string                 `json:"model,omitempty"`
	Effort       string                 `json:"effort,omitempty"`
	Once         bool                   `json:"once,omitempty"`
	PollInterval string                 `json:"poll_interval,omitempty"`
	StartedAt    time.Time              `json:"started_at,omitempty"`
	FinishedAt   time.Time              `json:"finished_at,omitempty"`
	Error        string                 `json:"error,omitempty"`
	StdoutPath   string                 `json:"stdout_path,omitempty"`
	SpecPath     string                 `json:"spec_path,omitempty"`
	Attempts     int                    `json:"attempts,omitempty"`
	Successes    int                    `json:"successes,omitempty"`
	Failures     int                    `json:"failures,omitempty"`
	CurrentBead  string                 `json:"current_bead,omitempty"`
	LastError    string                 `json:"last_error,omitempty"`
	LastResult   *WorkerExecutionResult `json:"last_result,omitempty"`
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

type workerHandle struct {
	record  WorkerRecord
	cancel  context.CancelFunc
	logBuf  *bytes.Buffer
	logFile *os.File
}

// WorkerManager manages in-process execute-loop workers as goroutines.
type WorkerManager struct {
	projectRoot        string
	rootDir            string
	AgentRunnerFactory AgentRunnerFactory

	mu      sync.Mutex
	workers map[string]*workerHandle
}

// AgentRunnerFactory creates an agent.Runner for a project. Override for testing.
type AgentRunnerFactory func(projectRoot string) *agent.Runner

func NewWorkerManager(projectRoot string) *WorkerManager {
	return &WorkerManager{
		projectRoot: projectRoot,
		rootDir:     filepath.Join(projectRoot, ".ddx", "workers"),
		workers:     map[string]*workerHandle{},
	}
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

	// Write spec
	specData, _ := json.MarshalIndent(spec, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "spec.json"), append(specData, '\n'), 0o644)

	// Open log file
	logPath := filepath.Join(dir, "worker.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return WorkerRecord{}, err
	}

	// Open structured event sink — loop milestones (bead.claimed, bead.result,
	// loop.start/end) land here as JSONL so log aggregators and future server
	// endpoints can parse them independently of the human-readable worker.log.
	eventsPath := filepath.Join(dir, "worker-events.jsonl")
	eventsFile, eventsErr := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if eventsErr != nil {
		eventsFile = nil // non-fatal; structured events silently disabled
	}

	record := WorkerRecord{
		ID:           id,
		Kind:         "execute-loop",
		State:        "running",
		Status:       "running",
		ProjectRoot:  m.projectRoot,
		Harness:      spec.Harness,
		Model:        spec.Model,
		Effort:       spec.Effort,
		Once:         spec.Once,
		PollInterval: spec.PollInterval.String(),
		StdoutPath:   relToProject(m.projectRoot, logPath),
		SpecPath:     relToProject(m.projectRoot, filepath.Join(dir, "spec.json")),
		StartedAt:    time.Now().UTC(),
	}
	_ = m.writeRecord(dir, record)

	ctx, cancel := context.WithCancel(context.Background())
	logBuf := &bytes.Buffer{}
	multiLog := io.MultiWriter(logBuf, logFile)

	handle := &workerHandle{
		record:  record,
		cancel:  cancel,
		logBuf:  logBuf,
		logFile: logFile,
	}

	m.mu.Lock()
	m.workers[id] = handle
	m.mu.Unlock()

	go m.runWorker(ctx, id, dir, spec, handle, multiLog, eventsFile)

	return record, nil
}

func (m *WorkerManager) runWorker(ctx context.Context, id, dir string, spec ExecuteLoopWorkerSpec, handle *workerHandle, log io.Writer, eventSink io.WriteCloser) {
	if eventSink != nil {
		defer eventSink.Close() //nolint:errcheck
	}
	store := bead.NewStore(filepath.Join(m.projectRoot, ".ddx"))

	// Build an executor that calls agent.ExecuteBead directly (in-process, no subprocess)
	executor := agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		runner := m.buildAgentRunner(m.projectRoot)
		gitOps := &agent.RealGitOps{}

		res, err := agent.ExecuteBead(m.projectRoot, beadID, agent.ExecuteBeadOptions{
			Harness: spec.Harness,
			Model:   spec.Model,
			Effort:  spec.Effort,
		}, gitOps, runner)
		if err != nil {
			return agent.ExecuteBeadReport{}, err
		}
		return agent.ExecuteBeadReport{
			BeadID:      res.BeadID,
			AttemptID:   res.AttemptID,
			WorkerID:    res.WorkerID,
			Harness:     res.Harness,
			Provider:    res.Provider,
			Model:       res.Model,
			Status:      res.Status,
			Detail:      res.Detail,
			SessionID:   res.SessionID,
			BaseRev:     res.BaseRev,
			ResultRev:   res.ResultRev,
			PreserveRef: res.PreserveRef,
		}, nil
	})

	worker := &agent.ExecuteBeadWorker{
		Store:    store,
		Executor: executor,
	}

	loopResult, err := worker.Run(ctx, agent.ExecuteBeadLoopOptions{
		Assignee:     "ddx",
		Once:         spec.Once,
		PollInterval: spec.PollInterval,
		Log:          log,
		EventSink:    eventSink,
		WorkerID:     id,
		ProjectRoot:  m.projectRoot,
		Harness:      spec.Harness,
		Model:        spec.Model,
	})

	m.mu.Lock()
	record := handle.record
	record.FinishedAt = time.Now().UTC()
	_ = handle.logFile.Close()

	if err != nil {
		record.State = "failed"
		record.Status = "failed"
		record.Error = err.Error()
		record.LastError = err.Error()
	} else {
		record.State = "exited"
		record.Attempts = loopResult.Attempts
		record.Successes = loopResult.Successes
		record.Failures = loopResult.Failures

		if loopResult.NoReadyWork {
			record.Status = "no_ready_work"
		} else if loopResult.Failures > 0 && loopResult.Successes == 0 {
			record.Status = "execution_failed"
			if loopResult.LastFailureStatus != "" {
				record.Status = loopResult.LastFailureStatus
			}
		} else if loopResult.Successes > 0 {
			record.Status = "success"
		} else {
			record.Status = "exited"
		}

		if len(loopResult.Results) > 0 {
			last := loopResult.Results[len(loopResult.Results)-1]
			r := WorkerExecutionResult{
				BeadID:     last.BeadID,
				AttemptID:  last.AttemptID,
				WorkerID:   last.WorkerID,
				Harness:    last.Harness,
				Provider:   last.Provider,
				Model:      last.Model,
				Status:     last.Status,
				Detail:     last.Detail,
				SessionID:  last.SessionID,
				BaseRev:    last.BaseRev,
				ResultRev:  last.ResultRev,
				RetryAfter: last.RetryAfter,
			}
			record.CurrentBead = last.BeadID
			record.LastResult = &r
			if last.Detail != "" {
				record.LastError = last.Detail
			}
			if last.Harness != "" && record.Harness == "" {
				record.Harness = last.Harness
			}
			if last.Model != "" && record.Model == "" {
				record.Model = last.Model
			}
			if last.Provider != "" && record.Provider == "" {
				record.Provider = last.Provider
			}
		}
	}
	_ = m.writeRecord(dir, record)
	handle.record = record
	m.mu.Unlock()
}

func (m *WorkerManager) buildAgentRunner(projectRoot string) *agent.Runner {
	if m.AgentRunnerFactory != nil {
		return m.AgentRunnerFactory(projectRoot)
	}

	// Load project config so server workers respect the same settings as CLI commands.
	cfg, err := config.LoadWithWorkingDir(projectRoot)
	agentCfg := agent.Config{}
	if err == nil && cfg.Agent != nil {
		agentCfg.Harness = cfg.Agent.Harness
		agentCfg.Model = cfg.Agent.Model
		agentCfg.Models = cfg.Agent.Models
		agentCfg.ReasoningLevels = cfg.Agent.ReasoningLevels
		agentCfg.TimeoutMS = cfg.Agent.TimeoutMS
		agentCfg.SessionLogDir = cfg.Agent.SessionLogDir
		agentCfg.Permissions = cfg.Agent.Permissions
	}

	r := agent.NewRunner(agentCfg)

	// Wire agent config loader — reads from .ddx/config.yaml on each invocation.
	// This ensures server workers use the same configuration as CLI commands.
	r.AgentConfigLoader = func() *agent.AgentYAMLConfig {
		c, err := config.LoadWithWorkingDir(projectRoot)
		if err != nil || c.Agent == nil || c.Agent.AgentRunner == nil {
			return nil
		}
		fc := c.Agent.AgentRunner
		yaml := &agent.AgentYAMLConfig{
			Provider:      fc.Provider,
			BaseURL:       fc.BaseURL,
			APIKey:        fc.APIKey,
			Model:         fc.Model,
			Preset:        fc.Preset,
			MaxIterations: fc.MaxIterations,
		}
		if fc.Models != nil {
			yaml.Models = make(map[string]*agent.LLMPresetYAML, len(fc.Models))
			for name, p := range fc.Models {
				yaml.Models[name] = &agent.LLMPresetYAML{
					Model:     p.Model,
					Provider:  p.Provider,
					Endpoints: p.Endpoints,
					APIKey:    p.APIKey,
					Strategy:  p.Strategy,
				}
			}
		}
		return yaml
	}

	return r
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

	// Merge in-memory state for active workers
	m.mu.Lock()
	for i := range out {
		if handle, ok := m.workers[out[i].ID]; ok {
			out[i] = handle.record
		}
	}
	m.mu.Unlock()

	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out, nil
}

func (m *WorkerManager) Show(id string) (WorkerRecord, error) {
	m.mu.Lock()
	if handle, ok := m.workers[id]; ok {
		rec := handle.record
		m.mu.Unlock()
		return rec, nil
	}
	m.mu.Unlock()
	return m.readRecord(filepath.Join(m.rootDir, id))
}

func (m *WorkerManager) Stop(id string) error {
	m.mu.Lock()
	handle := m.workers[id]
	m.mu.Unlock()
	if handle == nil || handle.cancel == nil {
		return fmt.Errorf("worker not running")
	}
	handle.cancel()
	return nil
}

func (m *WorkerManager) Logs(id string) (string, string, error) {
	m.mu.Lock()
	if handle, ok := m.workers[id]; ok {
		log := handle.logBuf.String()
		sessionLog := m.readActiveSessionLog(handle)
		m.mu.Unlock()
		if sessionLog != "" {
			return log + "\n" + sessionLog, "", nil
		}
		return log, "", nil
	}
	m.mu.Unlock()

	// Fall back to reading from disk for completed workers
	rec, err := m.Show(id)
	if err != nil {
		return "", "", err
	}
	if rec.StdoutPath == "" {
		return "", "", nil
	}
	data, err := os.ReadFile(filepath.Join(m.projectRoot, rec.StdoutPath))
	if err != nil {
		return "", "", err
	}
	return string(data), "", nil
}

// readActiveSessionLog reads the latest session log entries for an active worker.
// The ddx-agent library writes per-iteration entries to .ddx/agent-logs/agent-*.jsonl
// in real-time, so this gives live visibility into what the model provider is doing.
func (m *WorkerManager) readActiveSessionLog(handle *workerHandle) string {
	logDir := filepath.Join(m.projectRoot, ".ddx", "agent-logs")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return ""
	}

	// Find the most recent agent-*.jsonl file that was modified in the last 30 minutes
	var newest string
	var newestMod time.Time
	cutoff := time.Now().Add(-30 * time.Minute)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "agent-") || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		// Skip loop event files — they contain loop milestones, not agent session
		// entries. Loop milestone progress is already captured in worker.log.
		if strings.HasPrefix(entry.Name(), "agent-loop-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newestMod) && info.ModTime().After(cutoff) {
			newest = filepath.Join(logDir, entry.Name())
			newestMod = info.ModTime()
		}
	}
	if newest == "" {
		return ""
	}

	// Read the last N lines of the session log and format them as readable progress
	data, err := os.ReadFile(newest)
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// Show the last 50 entries
	start := 0
	if len(lines) > 50 {
		start = len(lines) - 50
	}

	return agent.FormatSessionLogLines(lines[start:])
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
	return record, nil
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

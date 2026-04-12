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

	go m.runWorker(ctx, id, dir, spec, handle, multiLog)

	return record, nil
}

func (m *WorkerManager) runWorker(ctx context.Context, id, dir string, spec ExecuteLoopWorkerSpec, handle *workerHandle, log io.Writer) {
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
	return agent.NewRunner(agent.Config{})
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

	return formatSessionLogLines(lines[start:])
}

// formatSessionLogLines formats ddx-agent JSONL log entries into readable progress.
func formatSessionLogLines(lines []string) string {
	var sb strings.Builder
	sb.WriteString("--- session log (latest entries) ---\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entryType, _ := entry["type"].(string)
		switch entryType {
		case "session.start":
			model, _ := entry["data"].(map[string]any)["model"].(string)
			fmt.Fprintf(&sb, "▶ session started (model: %s)\n", model)
		case "llm.request":
			data, _ := entry["data"].(map[string]any)
			attemptIdx, _ := data["attempt_index"].(float64)
			fmt.Fprintf(&sb, "  → llm request (attempt %.0f)\n", attemptIdx)
		case "llm.response":
			data, _ := entry["data"].(map[string]any)
			model, _ := data["model"].(string)
			latency, _ := data["latency_ms"].(float64)
			// Tokens: data.attempt.cost.raw.total_tokens
			var tokens float64
			if attempt, ok := data["attempt"].(map[string]any); ok {
				if cost, ok := attempt["cost"].(map[string]any); ok {
					if raw, ok := cost["raw"].(map[string]any); ok {
						tokens, _ = raw["total_tokens"].(float64)
					}
				}
			}
			// Tool calls from response
			toolCalls, _ := data["tool_calls"].([]any)
			finishReason, _ := data["finish_reason"].(string)
			suffix := ""
			if len(toolCalls) > 0 {
				var names []string
				for _, tc := range toolCalls {
					if tcm, ok := tc.(map[string]any); ok {
						n, _ := tcm["name"].(string)
						if n != "" {
							names = append(names, n)
						}
					}
				}
				suffix = fmt.Sprintf(" → %s", strings.Join(names, ", "))
			} else if finishReason != "" {
				suffix = fmt.Sprintf(" (%s)", finishReason)
			}
			fmt.Fprintf(&sb, "  ← llm response (%.0f tokens, %.1fs) %s%s\n", tokens, latency/1000, model, suffix)
		case "llm.delta":
			// Skip deltas — too verbose for summary
		case "tool.call":
			data, _ := entry["data"].(map[string]any)
			name, _ := data["tool"].(string)
			inp, _ := data["input"].(map[string]any)
			dur, _ := data["duration_ms"].(float64)
			argHint := ""
			if len(inp) > 0 {
				// Prefer path/command/query keys for display
				for _, key := range []string{"path", "command", "query", "file"} {
					if v, ok := inp[key]; ok {
						argHint = truncateStr(fmt.Sprintf("%v", v), 60)
						break
					}
				}
				if argHint == "" {
					for _, v := range inp {
						argHint = truncateStr(fmt.Sprintf("%v", v), 60)
						break
					}
				}
			}
			errMsg, _ := data["error"].(string)
			errSuffix := ""
			if errMsg != "" {
				errSuffix = fmt.Sprintf(" ❌ %s", truncateStr(errMsg, 40))
			}
			durSuffix := ""
			if dur > 0 {
				durSuffix = fmt.Sprintf(" (%.1fs)", dur/1000)
			}
			fmt.Fprintf(&sb, "  🔧 %s %s%s%s\n", name, argHint, durSuffix, errSuffix)
		case "compaction.start":
			fmt.Fprintf(&sb, "  ⚡ compacting context...\n")
		case "compaction.end":
			fmt.Fprintf(&sb, "  ⚡ compaction done\n")
		}
	}
	return sb.String()
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

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

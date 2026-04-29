package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// claudeStreamEvent is a minimal, lenient view of a claude CLI stream-json
// event. Only the fields we need for progress and final result are decoded.
type claudeStreamEvent struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype"`
	Message json.RawMessage `json:"message"`
	Result  string          `json:"result"`

	// result-event fields
	Usage struct {
		InputTokens         int `json:"input_tokens"`
		OutputTokens        int `json:"output_tokens"`
		CacheCreationTokens int `json:"cache_creation_input_tokens"`
		CacheReadTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
	TotalCostUSD    float64 `json:"total_cost_usd"`
	DurationMsField int64   `json:"duration_ms"`
	SessionID       string  `json:"session_id"`
	IsError         bool    `json:"is_error"`

	// system/init fields
	Model string `json:"model"`
}

// claudeAssistantMessage is the shape of the "message" field in an
// {"type":"assistant",...} stream-json event. It is Claude's native
// Messages API payload.
type claudeAssistantMessage struct {
	ID      string               `json:"id"`
	Model   string               `json:"model"`
	Content []claudeMessageBlock `json:"content"`
	Usage   claudeAssistantUsage `json:"usage"`
	Stop    string               `json:"stop_reason,omitempty"`
}

type claudeMessageBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type claudeAssistantUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// claudeUserMessage decodes the "message" field of a {"type":"user",...}
// stream event so we can recover tool_result output, error flags, and
// match them back to the originating tool_use_id. Used by verbose mode to
// populate tool.call output/duration_ms/error fields.
type claudeUserMessage struct {
	Content []claudeUserBlock `json:"content"`
}

type claudeUserBlock struct {
	Type      string          `json:"type"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

// pendingToolCall tracks an in-flight tool_use awaiting its tool_result so
// the parser can emit a single tool.call event with output, duration_ms,
// and error populated. Only used in verbose mode.
type pendingToolCall struct {
	name      string
	input     map[string]any
	startedAt time.Time
	turn      int
	elapsedMS int64
}

// claudeStreamResult is the aggregated final state from parsing a
// claude stream-json output.
type claudeStreamResult struct {
	FinalText    string
	SessionID    string
	Model        string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	ToolCalls    int
	TurnCount    int
	IsError      bool
}

// parseClaudeStream reads newline-delimited claude stream-json events from r,
// writes translated progress entries to progressLog (one JSONL line per tool
// call and a session.start event on init), and returns an aggregated
// claudeStreamResult covering token usage, final text, and counts.
//
// The progress entries use the same event type names the ddx-agent harness
// emits (session.start, tool.call, llm.response) so the existing
// TailSessionLogs / FormatSessionLogLines pipeline can render them as
// real-time progress without any changes.
//
// beadID and startTime are optional: when provided, each progress entry
// receives a data.bead_id and data.elapsed_ms field for cross-referencing.
//
// progressLog may be nil; if nil, progress entries are discarded but the
// aggregate result is still returned.
func parseClaudeStream(r io.Reader, progressLog io.Writer, sessionID, beadID string, startTime time.Time) (*claudeStreamResult, error) {
	if startTime.IsZero() {
		startTime = time.Now()
	}
	verbose := os.Getenv("DDX_AGENT_LOG_VERBOSE") == "1"
	pendingTools := map[string]*pendingToolCall{}
	res := &claudeStreamResult{SessionID: sessionID}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 256*1024), 16*1024*1024) // 16MB max line — claude can dump big tool results

	seq := 0
	emit := func(evType, dataJSON string) {
		if progressLog == nil {
			return
		}
		entry := map[string]any{
			"session_id": sessionID,
			"seq":        seq,
			"type":       evType,
			"ts":         time.Now().UTC().Format(time.RFC3339Nano),
			"data":       json.RawMessage(dataJSON),
		}
		seq++
		line, err := json.Marshal(entry)
		if err != nil {
			return
		}
		_, _ = progressLog.Write(line)
		_, _ = progressLog.Write([]byte("\n"))
	}

	wroteStart := false
	// lastRequestSentAt tracks when the most recent LLM request was sent.
	// For the first turn it equals startTime (the session began with a
	// request). After every "user" event (tool_result returned to the model)
	// we reset it: the next assistant event represents a new round-trip.
	// Per-call latency is then `now - lastRequestSentAt`, distinct from
	// cumulative elapsed_ms.
	lastRequestSentAt := startTime
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev claudeStreamEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			// Not a JSON event — skip silently to be lenient.
			continue
		}

		now := time.Now()
		elapsedMS := now.Sub(startTime).Milliseconds()
		switch ev.Type {
		case "system":
			if ev.Subtype == "init" && !wroteStart {
				wroteStart = true
				if ev.Model != "" {
					res.Model = ev.Model
				}
				if ev.SessionID != "" {
					res.SessionID = ev.SessionID
				}
				data, _ := json.Marshal(map[string]any{
					"model":      res.Model,
					"session_id": res.SessionID,
					"bead_id":    beadID,
					"elapsed_ms": elapsedMS,
					"harness":    "claude",
				})
				emit("session.start", string(data))
			}

		case "assistant":
			var msg claudeAssistantMessage
			if len(ev.Message) > 0 {
				_ = json.Unmarshal(ev.Message, &msg)
			}
			if msg.Model != "" && res.Model == "" {
				res.Model = msg.Model
			}
			// Running usage totals so TailSessionLogs has the latest view.
			if msg.Usage.InputTokens > 0 {
				res.InputTokens = msg.Usage.InputTokens
			}
			if msg.Usage.OutputTokens > 0 {
				res.OutputTokens = msg.Usage.OutputTokens
			}

			// Count each assistant message as one turn for operator visibility.
			res.TurnCount++

			// Collect tool-use names emitted in this turn, so we can translate
			// the assistant event into an llm.response-like progress entry.
			var toolNames []map[string]string
			var textOut strings.Builder
			for _, block := range msg.Content {
				switch block.Type {
				case "text":
					if block.Text != "" {
						if textOut.Len() > 0 {
							textOut.WriteString("\n")
						}
						textOut.WriteString(block.Text)
					}
				case "tool_use":
					name := block.Name
					if name == "" {
						name = "tool"
					}
					toolNames = append(toolNames, map[string]string{"name": name})
				}
			}

			latencyMS := now.Sub(lastRequestSentAt).Milliseconds()
			if latencyMS < 0 {
				latencyMS = 0
			}
			llmData := map[string]any{
				"model":         res.Model,
				"latency_ms":    latencyMS,
				"tool_calls":    toolNames,
				"turn":          res.TurnCount,
				"bead_id":       beadID,
				"elapsed_ms":    elapsedMS,
				"input_tokens":  res.InputTokens,
				"output_tokens": res.OutputTokens,
				"attempt": map[string]any{
					"cost": map[string]any{
						"raw": map[string]any{
							"total_tokens": res.InputTokens + res.OutputTokens,
						},
					},
				},
			}
			if verbose {
				llmData["content"] = textOut.String()
				llmData["finish_reason"] = msg.Stop
				llmData["usage"] = map[string]any{
					"input_tokens":                msg.Usage.InputTokens,
					"output_tokens":               msg.Usage.OutputTokens,
					"cache_creation_input_tokens": msg.Usage.CacheCreationInputTokens,
					"cache_read_input_tokens":     msg.Usage.CacheReadInputTokens,
					"total_tokens":                msg.Usage.InputTokens + msg.Usage.OutputTokens,
				}
			}
			data, _ := json.Marshal(llmData)
			emit("llm.response", string(data))

			// Emit (lean) or defer (verbose) a tool.call entry per tool_use
			// block. Verbose mode buffers each call until the matching
			// tool_result arrives so the emitted event includes output,
			// duration_ms, and error.
			for _, block := range msg.Content {
				if block.Type != "tool_use" {
					continue
				}
				res.ToolCalls++
				input := map[string]any{}
				if len(block.Input) > 0 {
					_ = json.Unmarshal(block.Input, &input)
				}
				if verbose {
					pendingTools[block.ID] = &pendingToolCall{
						name:      block.Name,
						input:     input,
						startedAt: now,
						turn:      res.TurnCount,
						elapsedMS: elapsedMS,
					}
					continue
				}
				toolData, _ := json.Marshal(map[string]any{
					"tool":       block.Name,
					"input":      input,
					"bead_id":    beadID,
					"elapsed_ms": elapsedMS,
					"turn":       res.TurnCount,
				})
				emit("tool.call", string(toolData))
			}

			// Save the most recent text content so, if the final result
			// event is missing or truncated, we can still return the
			// agent's last spoken text.
			if t := textOut.String(); t != "" {
				res.FinalText = t
			}

		case "user":
			// user events carry tool_result content. They mark the moment
			// the next LLM request is sent, so we reset the per-call
			// latency baseline. In verbose mode they also flush deferred
			// tool.call events with output, duration_ms, and error attached.
			if verbose && len(ev.Message) > 0 && len(pendingTools) > 0 {
				var um claudeUserMessage
				if json.Unmarshal(ev.Message, &um) == nil {
					for _, blk := range um.Content {
						if blk.Type != "tool_result" || blk.ToolUseID == "" {
							continue
						}
						pending, ok := pendingTools[blk.ToolUseID]
						if !ok {
							continue
						}
						delete(pendingTools, blk.ToolUseID)
						output := decodeToolResultContent(blk.Content)
						errMsg := ""
						if blk.IsError {
							errMsg = output
						}
						durationMS := now.Sub(pending.startedAt).Milliseconds()
						if durationMS < 0 {
							durationMS = 0
						}
						toolData, _ := json.Marshal(map[string]any{
							"tool":        pending.name,
							"input":       pending.input,
							"output":      output,
							"duration_ms": durationMS,
							"error":       errMsg,
							"bead_id":     beadID,
							"elapsed_ms":  pending.elapsedMS,
							"turn":        pending.turn,
						})
						emit("tool.call", string(toolData))
					}
				}
			}
			lastRequestSentAt = now

		case "result":
			// Final event. Extract authoritative usage/cost/result text.
			if ev.Result != "" {
				res.FinalText = ev.Result
			}
			if ev.Usage.InputTokens > 0 {
				res.InputTokens = ev.Usage.InputTokens
			}
			if ev.Usage.OutputTokens > 0 {
				res.OutputTokens = ev.Usage.OutputTokens
			}
			if ev.TotalCostUSD > 0 {
				res.CostUSD = ev.TotalCostUSD
			}
			if ev.SessionID != "" {
				res.SessionID = ev.SessionID
			}
			if ev.IsError {
				res.IsError = true
			}
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return res, err
	}
	// Flush any deferred tool.call events whose tool_result never arrived
	// (stream truncated or the model made a tool call as its final action).
	if verbose && len(pendingTools) > 0 {
		flushAt := time.Now()
		for _, pending := range pendingTools {
			toolData, _ := json.Marshal(map[string]any{
				"tool":        pending.name,
				"input":       pending.input,
				"output":      "",
				"duration_ms": flushAt.Sub(pending.startedAt).Milliseconds(),
				"error":       "no tool_result received",
				"bead_id":     beadID,
				"elapsed_ms":  pending.elapsedMS,
				"turn":        pending.turn,
			})
			emit("tool.call", string(toolData))
		}
	}
	return res, nil
}

// decodeToolResultContent normalizes the polymorphic tool_result.content
// field (either a string or an array of {type:"text",text:"..."} blocks)
// into a single string suitable for the verbose tool.call output field.
func decodeToolResultContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var sb strings.Builder
		for i, b := range blocks {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(b.Text)
		}
		return sb.String()
	}
	return string(raw)
}

// resolveClaudeProgressLogDir picks the directory for the per-run claude
// progress JSONL trace. The per-run override (opts.SessionLogDir) takes
// precedence over the runner-wide Config.SessionLogDir so managed
// execute-bead invocations can redirect the trace into the DDx-owned
// execution bundle's embedded/ dir instead of the runner's default
// .ddx/agent-logs. Matches the agent_runner.go precedence pattern so the
// two harnesses behave identically under execute-bead.
func resolveClaudeProgressLogDir(opts RunArgs, cfg Config) string {
	if opts.SessionLogDir != "" {
		return opts.SessionLogDir
	}
	return cfg.SessionLogDir
}

// runClaudeStreaming is the streaming execution path for the claude harness.
// It launches the claude binary directly (bypassing the standard Executor
// abstraction so it can pipe stdout line-by-line), parses stream-json events
// as they arrive, and writes per-tool progress entries to a ddx-agent-style
// JSONL file in opts.SessionLogDir (when set) or r.Config.SessionLogDir so
// `ddx server workers log` sees real-time activity.
//
// On success it returns a *Result shaped exactly like the non-streaming
// path, so callers downstream (processResult, session logging) are unchanged.
//
// If the claude CLI rejects the stream-json flags (older build, unsupported
// option), the caller falls back to the non-streaming path via
// runClaudeWithFallbackFn.
func runClaudeStreamingFn(r *Runner, ctx context.Context, harness harnessConfig, harnessName, model string, resolvedOpts RunArgs, prompt, execDir string, timeout time.Duration) (*Result, error) {
	args := BuildArgs(harness, buildArgsInputFromRunArgs(resolvedOpts), model)

	start := time.Now()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := osexec.CommandContext(runCtx, harness.Binary, args...)
	if execDir != "" {
		cmd.Dir = execDir
	}
	if harness.PromptMode == "stdin" {
		cmd.Stdin = strings.NewReader(prompt)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Open the progress log file. Using agent-<sessionID>.jsonl so
	// TailSessionLogs (which only reads files matching "agent-*.jsonl")
	// picks the stream up automatically.
	sessionID := fmt.Sprintf("claude-%d", time.Now().UnixNano())
	beadID := ""
	if resolvedOpts.Correlation != nil {
		beadID = resolvedOpts.Correlation["bead_id"]
	}
	var progressLog *os.File
	progressLogDir := resolveClaudeProgressLogDir(resolvedOpts, r.Config)
	if progressLogDir != "" {
		if err := os.MkdirAll(progressLogDir, 0o755); err == nil {
			logPath := filepath.Join(progressLogDir, "agent-"+sessionID+".jsonl")
			if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
				progressLog = f
				defer progressLog.Close()
			}
		}
	}

	// Stream stdout: parse events as they arrive and mirror raw bytes into
	// a buffer so we can return the full output for session logging.
	var rawOut strings.Builder
	stdoutReader, stdoutWriter := io.Pipe()
	var parseResult *claudeStreamResult
	var parseErr error
	var parseWG sync.WaitGroup
	parseWG.Add(1)
	go func() {
		defer parseWG.Done()
		parseResult, parseErr = parseClaudeStream(stdoutReader, progressLog, sessionID, beadID, start)
	}()

	// Tee stdout into rawOut (for the final Result.Output) and the parser.
	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		// Pipe bytes through a tee so both the raw buffer and the parser see them.
		tee := io.TeeReader(stdoutPipe, &stringWriter{&rawOut})
		_, _ = io.Copy(stdoutWriter, tee)
		_ = stdoutWriter.Close()
	}()

	var stderrBuf strings.Builder
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		_, _ = io.Copy(&stringWriter{&stderrBuf}, stderrPipe)
	}()

	// Idle-timeout watchdog: if no stdout progress for `timeout` duration,
	// cancel the context. The tee+parse goroutines handle activity naturally
	// via the pipe; for simplicity we use an overall deadline here. The
	// existing stream-json output is frequent enough that a long per-bead
	// deadline is the right behavior for operators.
	var timedOut bool
	if timeout > 0 {
		stop := make(chan struct{})
		go func() {
			select {
			case <-stop:
			case <-time.After(timeout):
				timedOut = true
				cancel()
			}
		}()
		defer close(stop)
	}

	<-stdoutDone
	<-stderrDone
	parseWG.Wait()
	runErr := cmd.Wait()
	elapsed := time.Since(start)

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*osexec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if errors.Is(runCtx.Err(), context.DeadlineExceeded) || timedOut {
			exitCode = -1
		} else {
			exitCode = -1
		}
	}

	execResult := &ExecResult{
		Stdout:   rawOut.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
	}

	var execErr error
	if timedOut {
		execErr = context.DeadlineExceeded
	} else if runErr != nil && exitCode == 0 {
		execErr = runErr
	}

	result := r.processResult(harnessName, model, harness, execResult, execErr, elapsed, runCtx)

	// If the parser succeeded, prefer its aggregated values — they come from
	// the authoritative "result" event and may be more accurate than the
	// legacy extractor path.
	if parseErr == nil && parseResult != nil {
		if parseResult.InputTokens > 0 || parseResult.OutputTokens > 0 {
			result.InputTokens = parseResult.InputTokens
			result.OutputTokens = parseResult.OutputTokens
			result.Tokens = parseResult.InputTokens + parseResult.OutputTokens
		}
		if parseResult.CostUSD > 0 {
			result.CostUSD = parseResult.CostUSD
		}
		if parseResult.SessionID != "" {
			result.AgentSessionID = parseResult.SessionID
		}
	}

	return result, nil
}

// stringWriter adapts *strings.Builder to io.Writer without allocating.
type stringWriter struct {
	sb *strings.Builder
}

func (w *stringWriter) Write(p []byte) (int, error) {
	return w.sb.Write(p)
}

// claudeStreamArgsUnsupported reports whether the claude CLI rejected the
// stream-json flags. Used by the fallback path to retry with legacy args.
func claudeStreamArgsUnsupported(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "unknown option") ||
		strings.Contains(lower, "unrecognized option") ||
		strings.Contains(lower, "invalid value for --output-format") ||
		strings.Contains(lower, "unknown argument") ||
		strings.Contains(lower, "unknown flag")
}

// runClaudeWithFallback attempts the streaming path first and, if the claude
// CLI rejects the stream-json flags, retries with the legacy buffered
// --print/-p/--output-format=json invocation so existing non-streaming
// contracts remain intact.
func runClaudeWithFallbackFn(r *Runner, ctx context.Context, harness harnessConfig, harnessName, model string, resolvedOpts RunArgs, prompt, execDir string, timeout time.Duration) (*Result, error) {
	result, err := runClaudeStreamingFn(r, ctx, harness, harnessName, model, resolvedOpts, prompt, execDir, timeout)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	// Streaming path succeeded (even if the bead itself failed).
	if result.ExitCode != 2 || !claudeStreamArgsUnsupported(result.Stderr) {
		return result, nil
	}

	// claude CLI rejected stream-json (older build). Retry with the legacy
	// non-streaming args so the run still completes.
	legacyHarness := harness
	legacyHarness.BaseArgs = []string{"--print", "-p", "--output-format", "json"}
	args := BuildArgs(legacyHarness, buildArgsInputFromRunArgs(resolvedOpts), model)
	stdin := ""
	if harness.PromptMode == "stdin" {
		stdin = prompt
	}
	start := time.Now()
	execResult, execErr := r.Executor.ExecuteInDir(ctx, harness.Binary, args, stdin, execDir)
	elapsed := time.Since(start)
	return r.processResult(harnessName, model, harness, execResult, execErr, elapsed, ctx), nil
}

// finalizeClaudeResult writes the session log entry and records the routing
// outcome, mirroring the tail end of Runner.Run for the non-streaming path.
func finalizeClaudeResult(r *Runner, result *Result, opts RunArgs, prompt string, elapsed time.Duration) {
	promptSource := opts.PromptSource
	if promptSource == "" {
		if opts.PromptFile != "" {
			promptSource = opts.PromptFile
		} else {
			promptSource = "inline"
		}
	}
	r.logSession(result, len(prompt), prompt, promptSource, opts.Correlation)
	r.recordRoutingOutcome(result, elapsed, opts)
}

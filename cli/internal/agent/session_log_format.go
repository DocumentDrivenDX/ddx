package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Session log JSONL schema reference.
//
// The agent harness writes one event per line to .ddx/agent-logs/agent-*.jsonl
// (or to the per-run SessionLogDir override). Every line has the envelope:
//
//	{"session_id": string, "seq": int, "type": string, "ts": RFC3339Nano, "data": {...}}
//
// The standard schema is always emitted and is what existing tooling consumes.
// Diagnostic logging is opt-in via DDX_LOG_VERBOSE=1; when set, the writer adds
// raw body fields so a session can be replayed and root-caused without loss of
// inputs and outputs. Diagnostic fields are strictly additive: event ordering
// and event types stay the same across harnesses.
//
// Per-event-type fields:
//
//	session.start:
//	  data.model       string  — model id reported by the harness
//	  data.session_id  string  — session id from the harness CLI
//	  data.bead_id     string  — bead under which the session ran (may be "")
//	  data.elapsed_ms  int     — ms since session start (always 0 here)
//	  data.harness     string  — "claude" | "fiz" | etc.
//
//	llm.request:
//	  data.attempt_index int        — request attempt counter (0-based)
//	  data.messages      []object   — conversation messages (last user message hint used by the renderer)
//
//	llm.response:
//	  data.model         string  — resolved model id
//	  data.latency_ms    int     — per-call wall time (request → response)
//	  data.tool_calls    []{name} — tool_use names emitted in this turn
//	  data.turn          int     — 1-based assistant turn count
//	  data.bead_id       string
//	  data.elapsed_ms    int     — ms since session start
//	  data.input_tokens  int     — running input tokens (latest seen)
//	  data.output_tokens int     — running output tokens (latest seen)
//	  data.attempt.cost.raw.total_tokens int — running total
//
//	llm.response (diagnostic adds):
//	  data.content       string  — concatenated assistant text blocks for this turn
//	  data.finish_reason string  — claude stop_reason ("end_turn", "tool_use", "max_tokens", ...)
//	  data.usage object:
//	    input_tokens                int
//	    output_tokens               int
//	    cache_creation_input_tokens int
//	    cache_read_input_tokens     int
//	    total_tokens                int
//
//	tool.call:
//	  data.tool       string         — tool name
//	  data.input      object         — tool arguments as decoded JSON
//	  data.bead_id    string
//	  data.elapsed_ms int            — ms since session start
//	  data.turn       int            — assistant turn that issued the call
//
//
//	tool.result:
//	  data.tool         string
//	  data.output_bytes int
//	  data.output_excerpt string — compact first-line excerpt safe for progress logs
//	  data.duration_ms  int     — wall time from tool_use → tool_result
//	  data.error        string  — non-empty when tool_result is_error=true; empty otherwise
//	  data.bead_id      string
//	  data.elapsed_ms   int
//	  data.turn         int
//
//	tool.result (diagnostic adds):
//	  data.output string — tool_result content (joined if structured)
//
// Diagnostic logging is off by default; existing consumers of the standard
// schema (TailSessionLogs, FormatSessionLogLines, the server worker log
// endpoint) keep working unchanged because they tolerate extra fields.

// FormatSessionLogLines formats Fizeau-style JSONL log entries into readable progress.
// It is used by both the CLI (local execute-loop) and the server worker log endpoint.
func FormatSessionLogLines(lines []string) string {
	var sb strings.Builder
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
			requestBytes := encodedPayloadSize(data["messages"])
			// Extract a hint from the last user message in the conversation.
			promptHint := ""
			if msgs, ok := data["messages"].([]any); ok {
				for i := len(msgs) - 1; i >= 0; i-- {
					if msg, ok := msgs[i].(map[string]any); ok {
						if role, _ := msg["role"].(string); role == "user" {
							if content, _ := msg["content"].(string); content != "" {
								promptHint = " [" + truncateStr(strings.TrimSpace(content), 60) + "]"
							}
							break
						}
					}
				}
			}
			sizeHint := formatSizeSuffix("req", requestBytes)
			fmt.Fprintf(&sb, "  → llm request (attempt %.0f%s)%s\n", attemptIdx, sizeHint, promptHint)
		case "llm.response":
			data, _ := entry["data"].(map[string]any)
			model, _ := data["model"].(string)
			latency, _ := data["latency_ms"].(float64)
			responseBytes := progressInt(data, "response_bytes")
			if responseBytes <= 0 {
				responseBytes = encodedPayloadSize(data["content"])
			}
			thinkingBytes := progressInt(data, "thinking_bytes")
			toolInputBytes := progressInt(data, "tool_input_bytes")
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
			byteHints := formatPayloadHints([]payloadHint{
				{label: "text", bytes: responseBytes},
				{label: "think", bytes: thinkingBytes},
				{label: "tool_in", bytes: toolInputBytes},
			})
			if byteHints != "" {
				byteHints = ", " + byteHints
			}
			throughput := formatTokenThroughput(int(tokens), int(latency))
			if throughput != "" {
				throughput = ", " + throughput
			}
			fmt.Fprintf(&sb, "  ← llm response (%.0f tokens, %.1fs%s%s) %s%s\n", tokens, latency/1000, throughput, byteHints, model, suffix)
		case "llm.delta":
			// Skip deltas — too verbose for summary
		case "progress":
			if line := formatProgressLogEntry(entry); line != "" {
				fmt.Fprint(&sb, line)
			}
		case "tool.call":
			data, _ := entry["data"].(map[string]any)
			name, _ := data["tool"].(string)
			inp, _ := data["input"].(map[string]any)
			dur, _ := data["duration_ms"].(float64)
			inputBytes := progressInt(data, "input_bytes")
			if inputBytes <= 0 {
				inputBytes = encodedPayloadSize(inp)
			}
			output, _ := data["output"].(string)
			outputBytes := progressInt(data, "output_bytes")
			if outputBytes <= 0 {
				outputBytes = len([]byte(output))
			}
			argHint := ""
			if len(inp) > 0 {
				// Prefer path/command/query keys for display
				for _, key := range []string{"path", "command", "query", "file"} {
					if v, ok := inp[key]; ok {
						argHint = fmt.Sprintf("%v", v)
						break
					}
				}
				if argHint == "" {
					for _, v := range inp {
						argHint = fmt.Sprintf("%v", v)
						break
					}
				}
				argHint = compactToolDisplay("", argHint)
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
			sizeSuffix := formatPayloadHints([]payloadHint{
				{label: "in", bytes: inputBytes},
				{label: "out", bytes: outputBytes},
			})
			if sizeSuffix != "" {
				sizeSuffix = " " + sizeSuffix
			}
			fmt.Fprintf(&sb, "  🔧 %s %s%s%s%s\n", name, argHint, sizeSuffix, durSuffix, errSuffix)
		case "tool.result", "tool_result":
			data, _ := entry["data"].(map[string]any)
			name, _ := data["tool"].(string)
			if name == "" {
				name, _ = data["name"].(string)
			}
			output, _ := data["output"].(string)
			outputBytes := progressInt(data, "output_bytes")
			if outputBytes <= 0 {
				outputBytes = len([]byte(output))
			}
			dur, _ := data["duration_ms"].(float64)
			errMsg, _ := data["error"].(string)
			durSuffix := ""
			if dur > 0 {
				durSuffix = " " + (time.Duration(int64(dur)) * time.Millisecond).String()
			}
			errSuffix := ""
			if errMsg != "" {
				errSuffix = " failed"
			}
			name = compactToolDisplay(name, "")
			excerpt := compactOutputExcerpt(data)
			if excerpt != "" {
				excerpt = " " + excerpt
			}
			fmt.Fprintf(&sb, "  < %s out=%s%s%s%s\n",
				name, formatByteSize(outputBytes), excerpt, durSuffix, errSuffix)
		case "bead.claimed":
			data, _ := entry["data"].(map[string]any)
			beadID, _ := data["bead_id"].(string)
			title, _ := data["title"].(string)
			if title != "" {
				fmt.Fprintf(&sb, "\n▶ %s: %s\n", beadID, title)
			} else {
				fmt.Fprintf(&sb, "\n▶ %s\n", beadID)
			}
		case "bead.result":
			data, _ := entry["data"].(map[string]any)
			beadID, _ := data["bead_id"].(string)
			status, _ := data["status"].(string)
			detail, _ := data["detail"].(string)
			resultRev, _ := data["result_rev"].(string)
			preserveRef, _ := data["preserve_ref"].(string)
			rationale, _ := data["no_changes_rationale"].(string)
			durationMs, _ := data["duration_ms"].(float64)
			var outcome string
			switch status {
			case "success":
				shortRev := resultRev
				if len(shortRev) > 8 {
					shortRev = shortRev[:8]
				}
				if shortRev != "" {
					outcome = fmt.Sprintf("merged (%s)", shortRev)
				} else {
					outcome = "merged"
				}
			case "already_satisfied", "no_changes":
				outcome = status
			default:
				if detail == "" {
					detail = status
				}
				if preserveRef != "" {
					outcome = fmt.Sprintf("preserved: %s", detail)
				} else {
					outcome = fmt.Sprintf("error: %s", detail)
				}
			}
			durStr := ""
			if durationMs > 0 {
				durStr = fmt.Sprintf(" (%.1fs)", durationMs/1000)
			}
			fmt.Fprintf(&sb, "✓ %s → %s%s\n", beadID, outcome, durStr)
			if rationale != "" {
				fmt.Fprintf(&sb, "  rationale: %s\n", rationale)
			}
		case "loop.end":
			data, _ := entry["data"].(map[string]any)
			attempts, _ := data["attempts"].(float64)
			successes, _ := data["successes"].(float64)
			failures, _ := data["failures"].(float64)
			if attempts > 0 {
				fmt.Fprintf(&sb, "\nloop done: %.0f attempted, %.0f succeeded, %.0f failed\n", attempts, successes, failures)
			}
		}
	}
	return sb.String()
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

type payloadHint struct {
	label string
	bytes int
}

func formatPayloadHints(hints []payloadHint) string {
	parts := make([]string, 0, len(hints))
	for _, hint := range hints {
		if hint.bytes <= 0 {
			continue
		}
		parts = append(parts, hint.label+"="+formatByteSize(hint.bytes))
	}
	return strings.Join(parts, " ")
}

func formatSizeSuffix(label string, bytes int) string {
	if bytes <= 0 {
		return ""
	}
	return ", " + label + "=" + formatByteSize(bytes)
}

func formatByteSize(bytes int) string {
	switch {
	case bytes <= 0:
		return "0B"
	case bytes < 1024:
		return fmt.Sprintf("%dB", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
}

func encodedPayloadSize(v any) int {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case string:
		return len([]byte(x))
	case json.RawMessage:
		return len(x)
	case []byte:
		return len(x)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return 0
		}
		return len(raw)
	}
}

func formatProgressLogEntry(entry map[string]any) string {
	data, _ := entry["data"].(map[string]any)
	phase, _ := data["phase"].(string)
	state, _ := data["state"].(string)
	if line := formatStructuredProgressLine(phase, state, data); line != "" {
		return "  " + line + "\n"
	}
	message, _ := data["message"].(string)
	if message != "" {
		return fmt.Sprintf("  %s\n", truncateStr(message, 120))
	}
	return ""
}

func formatStructuredProgressLine(phase, state string, data map[string]any) string {
	switch phase {
	case "thinking":
		return formatThinkingProgressLine(state, data)
	case "tool":
		return formatToolProgressLine(state, data)
	case "response":
		return formatResponseProgressLine(state, data)
	case "context":
		return formatContextProgressLine(state, data)
	case "compaction":
		return formatCompactionProgressLine(state, data)
	default:
		return ""
	}
}

func formatThinkingProgressLine(state string, data map[string]any) string {
	switch state {
	case "start":
		return "thinking ..."
	case "update":
		msg := progressSummaryFromData(data)
		if msg == "" {
			msg = "thinking update ..."
		} else {
			msg = "thinking update: " + msg
		}
		return truncateStr(msg, 120)
	case "complete":
		tokens := progressTokenCount(data, "output_tokens", "total_tokens", "input_tokens")
		duration := progressDurationString(data)
		if tokens > 0 && duration != "" {
			return fmt.Sprintf("thinking complete %d tok in %s%s", tokens, duration, progressThroughputSuffix(data, tokens))
		}
		if tokens > 0 {
			return fmt.Sprintf("thinking complete %d tok", tokens)
		}
		if duration != "" {
			return fmt.Sprintf("thinking complete in %s", duration)
		}
		return "thinking complete"
	default:
		return ""
	}
}

func formatToolProgressLine(state string, data map[string]any) string {
	toolName, _ := data["tool_name"].(string)
	command, _ := data["command"].(string)
	display := compactProgressToolDisplay(data, toolName, command, 96)
	switch state {
	case "start":
		if display != "" {
			return truncateStr("> "+display, 120)
		}
		return "> tool"
	case "complete":
		tokens := progressTokenCount(data, "total_tokens", "output_tokens", "input_tokens")
		duration := progressDurationString(data)
		outputSummary, _ := data["output_summary"].(string)
		if outputSummary != "" {
			display = compactProgressToolDisplay(data, toolName, command, 80)
		}
		line := "ok"
		if display != "" {
			line += " " + display
		}
		if outputSummary != "" {
			line += " < " + compactOutputSummaryLimit(outputSummary, 56)
		}
		suffix := ""
		switch {
		case duration != "" && tokens > 0:
			suffix = fmt.Sprintf(" %s %dtok", duration, tokens)
		case duration != "":
			suffix = " " + duration
		case tokens > 0:
			suffix = fmt.Sprintf(" %dtok", tokens)
		}
		return truncateStr(line, maxInt(1, 120-len(suffix))) + suffix
	default:
		return ""
	}
}

func formatTokenThroughput(tokens int, durationMS int) string {
	if tokens <= 0 || durationMS <= 0 {
		return ""
	}
	return fmt.Sprintf("%s tok/s", strconv.FormatFloat(float64(tokens)*1000/float64(durationMS), 'f', 1, 64))
}

func progressThroughputSuffix(data map[string]any, tokens int) string {
	throughput := formatTokenThroughput(tokens, progressInt(data, "duration_ms"))
	if throughput == "" {
		return ""
	}
	return ", " + throughput
}

func compactProgressToolDisplay(data map[string]any, toolName, command string, limit int) string {
	action := progressActionFromData(data)
	if action == "" {
		action = describeToolCommand(toolName, command)
	}
	if action == "" {
		action = compactToolDisplayLimit(toolName, command, limit)
	}
	prefix := compactProgressEventIdentity(data)
	if prefix != "" {
		action = prefix + " " + action
	}
	return truncateStr(action, limit)
}

func progressActionFromData(data map[string]any) string {
	if data == nil {
		return ""
	}
	for _, key := range []string{"description", "action"} {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return compactTargetedAction(value, data)
		}
	}
	return ""
}

func compactTargetedAction(action string, data map[string]any) string {
	action = strings.Join(strings.Fields(strings.TrimSpace(action)), " ")
	target := progressTargetFromData(data)
	compactedTarget := compactPathToken(target, 36)
	if target == "" || strings.Contains(action, target) || strings.Contains(action, compactedTarget) {
		return action
	}
	return action + " to " + compactedTarget
}

func progressTargetFromData(data map[string]any) string {
	for _, key := range []string{"target", "path", "file"} {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func compactProgressEventIdentity(data map[string]any) string {
	if data == nil {
		return ""
	}
	taskID, _ := data["task_id"].(string)
	taskID = strings.TrimSpace(taskID)
	turn := progressTurnNumber(data)
	switch {
	case taskID != "" && turn > 0:
		return fmt.Sprintf("%s %d", taskID, turn)
	case taskID != "":
		return taskID
	case turn > 0:
		return fmt.Sprintf("%d", turn)
	default:
		return ""
	}
}

func progressTurnNumber(data map[string]any) int {
	if turn := progressInt(data, "turn_index"); turn > 0 {
		return turn
	}
	return progressInt(data, "round")
}

func describeToolCommand(toolName, command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return strings.TrimSpace(toolName)
	}
	command = unwrapToolCommandSummary(command)
	command = stripShellLoginWrapper(command)
	command = firstShellCommandSegment(command)
	command = strings.Join(strings.Fields(command), " ")
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	switch fields[0] {
	case "sed":
		if len(fields) >= 4 && fields[1] == "-n" {
			return "inspect " + strings.Trim(fields[2], "'\"") + " in " + compactPathToken(fields[3], 36)
		}
	case "cat":
		if len(fields) >= 2 {
			return "inspect " + compactPathToken(fields[len(fields)-1], 36)
		}
	case "rg":
		return describeRipgrepCommand(fields)
	case "go":
		if len(fields) >= 2 && fields[1] == "test" {
			return "test " + strings.Join(compactPathTokens(fields[2:], 64), " ")
		}
	case "git":
		return describeGitCommand(fields)
	}
	return compactToolDisplayLimit(toolName, command, 96)
}

func describeRipgrepCommand(fields []string) string {
	target := ""
	pattern := ""
	for _, field := range fields[1:] {
		if strings.HasPrefix(field, "-") {
			continue
		}
		if pattern == "" {
			pattern = strings.Trim(field, "'\"")
			continue
		}
		target = field
	}
	if target != "" && pattern != "" {
		return "search " + strconv.Quote(truncateStr(pattern, 32)) + " in " + compactPathToken(target, 36)
	}
	if pattern != "" {
		return "search " + strconv.Quote(truncateStr(pattern, 32))
	}
	return "search"
}

func describeGitCommand(fields []string) string {
	if len(fields) < 2 {
		return "git"
	}
	switch fields[1] {
	case "add":
		if len(fields) > 2 {
			return "stage " + strings.Join(compactPathTokens(fields[2:], 36), " ")
		}
		return "stage changes"
	case "commit":
		return "commit changes"
	case "diff":
		return "inspect diff"
	case "status":
		return "inspect git status"
	case "log":
		return "inspect git log"
	default:
		return "git " + fields[1]
	}
}

func compactPathTokens(tokens []string, limit int) []string {
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		out = append(out, compactPathToken(token, limit))
	}
	return out
}

func compactOutputExcerpt(data map[string]any) string {
	if data == nil {
		return ""
	}
	for _, key := range []string{"output_excerpt", "output_summary"} {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return compactOutputSummary(value)
		}
	}
	if output, ok := data["output"].(string); ok {
		return outputSummaryFromRaw(output)
	}
	return ""
}

func compactOutputSummary(summary string) string {
	return compactOutputSummaryLimit(summary, 80)
}

func compactOutputSummaryLimit(summary string, limit int) string {
	summary = strings.Join(strings.Fields(strings.TrimSpace(summary)), " ")
	return truncateStr(summary, limit)
}

func outputSummaryFromRaw(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	byteCount := len([]byte(output))
	lineCount := strings.Count(output, "\n") + 1
	parts := []string{}
	if lineCount == 1 {
		parts = append(parts, "1 line")
	} else {
		parts = append(parts, fmt.Sprintf("%d lines", lineCount))
	}
	if byteCount > 40 {
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				parts = append(parts, strconv.Quote(truncateStr(line, 48)))
				break
			}
		}
	}
	return compactOutputSummary(strings.Join(parts, " "))
}

func compactToolDisplay(toolName, command string) string {
	return compactToolDisplayLimit(toolName, command, 96)
}

func compactToolDisplayLimit(toolName, command string, limit int) string {
	display := strings.TrimSpace(command)
	if display == "" {
		display = strings.TrimSpace(toolName)
	}
	display = unwrapToolCommandSummary(display)
	display = stripShellLoginWrapper(display)
	display = firstShellCommandSegment(display)
	fields := strings.Fields(display)
	for i := range fields {
		fields[i] = compactPathToken(fields[i], 36)
	}
	display = strings.Join(fields, " ")
	return truncateStr(display, limit)
}

func compactPathToken(token string, limit int) string {
	if limit <= 0 || len(token) <= limit || !strings.Contains(token, "/") {
		return token
	}
	prefix, suffix := splitTokenAffixes(token)
	core := strings.TrimPrefix(strings.TrimSuffix(token, suffix), prefix)
	if len(core) <= limit || !strings.Contains(core, "/") {
		return token
	}
	parts := strings.Split(core, "/")
	last := parts[len(parts)-1]
	if last == "" && len(parts) > 1 {
		last = parts[len(parts)-2] + "/"
	}
	if last == "" {
		return truncateStr(token, limit)
	}
	compacted := "…/" + last
	if len(prefix)+len(compacted)+len(suffix) <= limit {
		return prefix + compacted + suffix
	}
	return prefix + truncateStr(compacted, maxInt(1, limit-len(prefix)-len(suffix))) + suffix
}

func splitTokenAffixes(token string) (string, string) {
	prefix := ""
	suffix := ""
	for len(token) > len(prefix) {
		switch token[len(prefix)] {
		case '\'', '"', '`', '(', '[', '{':
			prefix += string(token[len(prefix)])
		default:
			goto suffixLoop
		}
	}
suffixLoop:
	for len(token) > len(prefix)+len(suffix) {
		switch token[len(token)-len(suffix)-1] {
		case '\'', '"', '`', ')', ']', '}', ',', ';':
			suffix = string(token[len(token)-len(suffix)-1]) + suffix
		default:
			return prefix, suffix
		}
	}
	return prefix, suffix
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstShellCommandSegment(command string) string {
	command = strings.TrimSpace(command)
	for _, sep := range []string{" && ", " || ", " ; "} {
		if idx := strings.Index(command, sep); idx >= 0 {
			return strings.TrimSpace(command[:idx])
		}
	}
	return command
}

func unwrapToolCommandSummary(display string) string {
	display = strings.TrimSpace(display)
	for _, prefix := range []string{`{command=`, `{"command":`} {
		if !strings.HasPrefix(display, prefix) {
			continue
		}
		value := strings.TrimPrefix(display, prefix)
		value = strings.TrimSuffix(value, "}")
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
		return strings.Trim(value, `"`)
	}
	return display
}

func stripShellLoginWrapper(command string) string {
	command = strings.TrimSpace(command)
	for _, prefix := range []string{"/bin/zsh -lc ", "zsh -lc ", "/bin/bash -lc ", "bash -lc "} {
		if !strings.HasPrefix(command, prefix) {
			continue
		}
		inner := strings.TrimSpace(strings.TrimPrefix(command, prefix))
		if unquoted, err := strconv.Unquote(inner); err == nil {
			return unquoted
		}
		return strings.Trim(inner, `"`)
	}
	return command
}

func formatResponseProgressLine(state string, data map[string]any) string {
	switch state {
	case "complete":
		tokens := progressTokenCount(data, "output_tokens", "total_tokens", "input_tokens")
		duration := progressDurationString(data)
		if tokens > 0 && duration != "" {
			return fmt.Sprintf("response complete %d tok in %s%s", tokens, duration, progressThroughputSuffix(data, tokens))
		}
		if tokens > 0 {
			return fmt.Sprintf("response complete %d tok", tokens)
		}
		if duration != "" {
			return fmt.Sprintf("response complete in %s", duration)
		}
		return "response complete"
	default:
		return ""
	}
}

func formatContextProgressLine(state string, data map[string]any) string {
	switch state {
	case "update", "complete":
		summary := progressSummaryFromData(data)
		if summary != "" {
			return truncateStr("context summary: "+summary, 120)
		}
		msg := "context summary updated"
		if state == "complete" {
			msg = "context summary complete"
		}
		return msg
	default:
		return ""
	}
}

func formatCompactionProgressLine(state string, data map[string]any) string {
	switch state {
	case "start":
		return "compaction started"
	case "update":
		summary := progressSummaryFromData(data)
		if summary != "" {
			return truncateStr("compaction update: "+summary, 120)
		}
		return "compaction update"
	case "complete":
		before := progressInt(data, "tokens_before")
		after := progressInt(data, "tokens_after")
		summary := progressSummaryFromData(data)
		line := "compaction complete"
		if before > 0 && after > 0 {
			line = fmt.Sprintf("compaction complete %d -> %d tokens", before, after)
		}
		if summary != "" {
			line += ": " + summary
		}
		return truncateStr(line, 120)
	default:
		return ""
	}
}

func progressSummaryFromData(data map[string]any) string {
	if data == nil {
		return ""
	}
	for _, key := range []string{"session_summary", "summary", "message"} {
		if v, ok := data[key].(string); ok && strings.TrimSpace(v) != "" {
			return truncateStr(strings.TrimSpace(v), 120)
		}
	}
	return ""
}

func progressTokenCount(data map[string]any, keys ...string) int {
	for _, key := range keys {
		if v, ok := data[key]; ok {
			switch n := v.(type) {
			case float64:
				if n > 0 {
					return int(n)
				}
			case int:
				if n > 0 {
					return n
				}
			}
		}
	}
	return 0
}

func progressInt(data map[string]any, key string) int {
	if data == nil {
		return 0
	}
	switch v := data[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func progressDurationString(data map[string]any) string {
	ms := progressInt(data, "duration_ms")
	if ms <= 0 {
		return ""
	}
	return (time.Duration(ms) * time.Millisecond).String()
}

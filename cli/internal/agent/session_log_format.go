package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	agentlib "github.com/easel/fizeau"
)

// FormatServiceProgressEntries formats canonical Fizeau progress payloads
// directly. It is used by callers that already have structured ServiceEvent
// data and should not round-trip through session-log JSONL line parsing.
func FormatServiceProgressEntries(entries []agentlib.ServiceProgressData) string {
	var sb strings.Builder
	for _, entry := range entries {
		raw, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal(raw, &data); err != nil {
			continue
		}
		if line := formatProgressLogEntry(map[string]any{
			"type": "progress",
			"data": data,
		}); line != "" {
			sb.WriteString(line)
		}
	}
	return sb.String()
}

func FormatServiceRoutingDecision(routing *agentlib.ServiceRoutingDecisionData) string {
	if routing == nil || (routing.Model == "" && routing.Harness == "" && routing.Provider == "") {
		return ""
	}
	parts := []string{}
	if routing.Harness != "" {
		parts = append(parts, "harness="+routing.Harness)
	}
	if routing.Provider != "" {
		parts = append(parts, "provider="+routing.Provider)
	}
	if routing.Model != "" {
		parts = append(parts, "model="+routing.Model)
	}
	if routing.Reason != "" {
		parts = append(parts, "reason="+routing.Reason)
	}
	if len(parts) == 0 {
		return ""
	}
	return "route: " + strings.Join(parts, " ")
}

// FormatSessionLogLines formats newline-delimited session log JSON records
// using the same progress renderer as structured ServiceEvent progress.
// Malformed lines are skipped.
func FormatSessionLogLines(lines string) string {
	var entries []agentlib.ServiceProgressData
	_ = scanJSONLReader[agentlib.ServiceProgressData](strings.NewReader(lines), func(entry agentlib.ServiceProgressData) error {
		entries = append(entries, entry)
		return nil
	})
	return FormatServiceProgressEntries(entries)
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
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
		outputSummary := progressOutputSummary(data)
		if outputSummary != "" {
			display = compactProgressToolDisplay(data, toolName, command, 80)
		}
		line := "ok"
		if display != "" {
			line += " " + display
		}
		if outputSummary != "" {
			line += " < " + outputSummary
		}
		suffix := ""
		if duration != "" {
			suffix = " " + duration
		}
		if tokens > 0 {
			suffix += fmt.Sprintf(" %dtok", tokens)
		}
		if throughput := progressThroughputText(data, tokens); throughput != "" {
			if suffix != "" {
				suffix += ", " + throughput
			} else {
				suffix = " " + throughput
			}
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
	throughput := progressThroughputText(data, tokens)
	if throughput == "" {
		return ""
	}
	return ", " + throughput
}

func progressThroughputText(data map[string]any, tokens int) string {
	if throughput := progressFloat(data, "tok_per_sec"); throughput > 0 {
		return fmt.Sprintf("%s tok/s", strconv.FormatFloat(throughput, 'f', 1, 64))
	}
	return formatTokenThroughput(tokens, progressInt(data, "duration_ms"))
}

func progressOutputSummary(data map[string]any) string {
	if data == nil {
		return ""
	}
	if summary, _ := data["output_summary"].(string); strings.TrimSpace(summary) != "" {
		return compactOutputSummaryLimit(summary, 56)
	}
	parts := make([]string, 0, 3)
	if bytes := progressInt(data, "output_bytes"); bytes > 0 {
		parts = append(parts, "out="+formatByteSize(bytes))
	}
	if lines := progressInt(data, "output_lines"); lines > 0 {
		parts = append(parts, fmt.Sprintf("%d lines", lines))
	}
	if excerpt, _ := data["output_excerpt"].(string); strings.TrimSpace(excerpt) != "" {
		parts = append(parts, compactOutputSummaryLimit(excerpt, 56))
	}
	if len(parts) == 0 {
		return ""
	}
	return compactOutputSummary(strings.Join(parts, " "))
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
	for _, key := range []string{"subject", "target", "path", "file"} {
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
	phase, _ := data["phase"].(string)
	turn := progressVisibleCounter(phase, data)
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

func progressVisibleCounter(phase string, data map[string]any) int {
	if phase == "tool" {
		if toolCallIndex := progressInt(data, "tool_call_index"); toolCallIndex > 0 {
			return toolCallIndex
		}
	}
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

func compactOutputSummary(summary string) string {
	return compactOutputSummaryLimit(summary, 80)
}

func compactOutputSummaryLimit(summary string, limit int) string {
	summary = strings.Join(strings.Fields(strings.TrimSpace(summary)), " ")
	return truncateStr(summary, limit)
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

func progressFloat(data map[string]any, key string) float64 {
	if data == nil {
		return 0
	}
	switch v := data[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
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

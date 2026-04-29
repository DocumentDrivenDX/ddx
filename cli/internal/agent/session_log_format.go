package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Session log JSONL schema reference.
//
// The agent harness writes one event per line to .ddx/agent-logs/agent-*.jsonl
// (or to the per-run SessionLogDir override). Every line has the envelope:
//
//	{"session_id": string, "seq": int, "type": string, "ts": RFC3339Nano, "data": {...}}
//
// Two schema modes coexist. Lean mode is always on by default and is what
// existing tooling consumes. Verbose mode is opt-in via the env var
// DDX_AGENT_LOG_VERBOSE=1; when set, the writer adds extra fields to a
// few event types so a session can be replayed and root-caused without
// loss of inputs and outputs. Lean fields are never removed in verbose
// mode — verbose is strictly additive (with the single exception that
// tool.call events are deferred until the matching tool_result arrives,
// so they can include output/duration_ms/error in one line).
//
// Per-event-type fields:
//
//	session.start (lean and verbose):
//	  data.model       string  — model id reported by the harness
//	  data.session_id  string  — session id from the harness CLI
//	  data.bead_id     string  — bead under which the session ran (may be "")
//	  data.elapsed_ms  int     — ms since session start (always 0 here)
//	  data.harness     string  — "claude" | "ddx-agent" | etc.
//
//	llm.request (lean and verbose):
//	  data.attempt_index int        — request attempt counter (0-based)
//	  data.messages      []object   — conversation messages (last user message hint used by the renderer)
//
//	llm.response (lean):
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
//	llm.response (verbose adds):
//	  data.content       string  — concatenated assistant text blocks for this turn
//	  data.finish_reason string  — claude stop_reason ("end_turn", "tool_use", "max_tokens", ...)
//	  data.usage object:
//	    input_tokens                int
//	    output_tokens               int
//	    cache_creation_input_tokens int
//	    cache_read_input_tokens     int
//	    total_tokens                int
//
//	tool.call (lean):
//	  data.tool       string         — tool name
//	  data.input      object         — tool arguments as decoded JSON
//	  data.bead_id    string
//	  data.elapsed_ms int            — ms since session start
//	  data.turn       int            — assistant turn that issued the call
//
//	tool.call (verbose) — emitted when the matching tool_result arrives so output is populated:
//	  data.tool        string
//	  data.input       object
//	  data.output      string  — tool_result content (joined if structured)
//	  data.duration_ms int     — wall time from tool_use → tool_result
//	  data.error       string  — non-empty when tool_result is_error=true; empty otherwise
//	  data.bead_id     string
//	  data.elapsed_ms  int     — ms-from-session-start at the time of the originating tool_use
//	  data.turn        int     — assistant turn that issued the call
//
// Verbose mode is off by default; existing consumers of the lean schema
// (TailSessionLogs, FormatSessionLogLines, the server worker log endpoint)
// keep working unchanged because they tolerate extra fields.

// FormatSessionLogLines formats ddx-agent JSONL log entries into readable progress.
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
			fmt.Fprintf(&sb, "  → llm request (attempt %.0f)%s\n", attemptIdx, promptHint)
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
		case "compaction.start":
			// Suppress: we'll show a single line on compaction.end only if it succeeded.
		case "compaction.end":
			data, _ := entry["data"].(map[string]any)
			success, _ := data["success"].(bool)
			if success {
				tokensBefore, _ := data["tokens_before"].(float64)
				tokensAfter, _ := data["tokens_after"].(float64)
				if tokensBefore > 0 && tokensAfter > 0 {
					fmt.Fprintf(&sb, "  ⚡ compacted context (%.0f → %.0f tokens)\n", tokensBefore, tokensAfter)
				} else {
					fmt.Fprintf(&sb, "  ⚡ compacted context\n")
				}
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

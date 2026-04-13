package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

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

package agent

// Provider envelope parsing is retained only for legacy fixtures. Fizeau
// normalizes provider output into ServiceFinalData.FinalText and usage before
// DDx receives it.

import (
	"encoding/json"
	"strings"
)

type UsageData struct {
	InputTokens  int
	CachedTokens int
	OutputTokens int
	CostUSD      float64
}

func ExtractUsage(harnessName, output string) UsageData {
	switch harnessName {
	case "codex":
		for _, line := range strings.Split(output, "\n") {
			var event struct {
				Type  string `json:"type"`
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal([]byte(strings.TrimSpace(line)), &event) == nil && event.Type == "turn.completed" {
				return UsageData{InputTokens: event.Usage.InputTokens, OutputTokens: event.Usage.OutputTokens}
			}
		}
	case "claude":
		return extractUsageClaude(output)
	case "opencode":
		var envelope struct {
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			TotalCostUSD float64 `json:"total_cost_usd"`
		}
		candidates := append([]string{output}, nonEmptyLinesReverse(output)...)
		for _, candidate := range candidates {
			if json.Unmarshal([]byte(candidate), &envelope) == nil &&
				(envelope.Usage.InputTokens != 0 || envelope.Usage.OutputTokens != 0 || envelope.TotalCostUSD != 0) {
				return UsageData{InputTokens: envelope.Usage.InputTokens, OutputTokens: envelope.Usage.OutputTokens, CostUSD: envelope.TotalCostUSD}
			}
		}
	case "pi":
		for _, line := range nonEmptyLinesReverse(output) {
			var event struct {
				Message struct {
					Usage struct {
						Input  int `json:"input"`
						Output int `json:"output"`
						Cost   struct {
							Total float64 `json:"total"`
						} `json:"cost"`
					} `json:"usage"`
				} `json:"message"`
				Partial struct {
					Usage struct {
						Input  int `json:"input"`
						Output int `json:"output"`
						Cost   struct {
							Total float64 `json:"total"`
						} `json:"cost"`
					} `json:"usage"`
				} `json:"partial"`
			}
			if json.Unmarshal([]byte(line), &event) != nil {
				continue
			}
			if event.Message.Usage.Input != 0 || event.Message.Usage.Output != 0 {
				return UsageData{InputTokens: event.Message.Usage.Input, OutputTokens: event.Message.Usage.Output, CostUSD: event.Message.Usage.Cost.Total}
			}
			if event.Partial.Usage.Input != 0 || event.Partial.Usage.Output != 0 {
				return UsageData{InputTokens: event.Partial.Usage.Input, OutputTokens: event.Partial.Usage.Output, CostUSD: event.Partial.Usage.Cost.Total}
			}
		}
	case "gemini":
		lines := nonEmptyLinesReverse(output)
		if len(lines) == 0 {
			return UsageData{}
		}
		var envelope struct {
			Stats struct {
				Models map[string]struct {
					Tokens struct {
						Input int `json:"input"`
						Total int `json:"total"`
					} `json:"tokens"`
				} `json:"models"`
			} `json:"stats"`
		}
		if json.Unmarshal([]byte(lines[0]), &envelope) != nil {
			return UsageData{}
		}
		var usage UsageData
		for _, model := range envelope.Stats.Models {
			usage.InputTokens += model.Tokens.Input
			usage.OutputTokens += model.Tokens.Total - model.Tokens.Input
		}
		return usage
	}
	return UsageData{}
}

func ExtractOutput(harnessName, rawOutput string) string {
	switch harnessName {
	case "codex":
		return extractOutputCodex(rawOutput)
	case "claude":
		return extractOutputClaude(rawOutput)
	case "pi", "gemini":
		return extractOutputPiGemini(rawOutput)
	default:
		return rawOutput
	}
}

func extractOutputPiGemini(rawOutput string) string {
	lines := nonEmptyLinesReverse(rawOutput)
	if len(lines) == 0 {
		return rawOutput
	}
	var envelope struct {
		Response string `json:"response"`
	}
	if json.Unmarshal([]byte(lines[0]), &envelope) != nil {
		return rawOutput
	}
	return envelope.Response
}

func extractOutputCodex(rawOutput string) string {
	for _, line := range strings.Split(rawOutput, "\n") {
		var item struct {
			Type string `json:"type"`
			Item struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &item) == nil && item.Type == "output" && item.Item.Type == "agent_message" {
			return item.Item.Text
		}
	}
	return rawOutput
}

func extractOutputClaude(rawOutput string) string {
	var envelope struct {
		Type   string `json:"type"`
		Result string `json:"result"`
	}
	if json.Unmarshal([]byte(rawOutput), &envelope) == nil && envelope.Result != "" {
		return envelope.Result
	}
	for _, line := range nonEmptyLinesReverse(rawOutput) {
		if json.Unmarshal([]byte(line), &envelope) == nil && envelope.Result != "" && (envelope.Type == "" || envelope.Type == "result") {
			return envelope.Result
		}
	}
	return rawOutput
}

func extractUsageClaude(output string) UsageData {
	parse := func(candidate string) (UsageData, bool) {
		var envelope struct {
			Usage struct {
				InputTokens          int `json:"input_tokens"`
				OutputTokens         int `json:"output_tokens"`
				CacheReadInputTokens int `json:"cache_read_input_tokens"`
			} `json:"usage"`
			TotalCostUSD float64 `json:"total_cost_usd"`
		}
		if json.Unmarshal([]byte(candidate), &envelope) != nil ||
			(envelope.Usage.InputTokens == 0 && envelope.Usage.OutputTokens == 0 && envelope.TotalCostUSD == 0) {
			return UsageData{}, false
		}
		return UsageData{
			InputTokens: envelope.Usage.InputTokens, CachedTokens: envelope.Usage.CacheReadInputTokens,
			OutputTokens: envelope.Usage.OutputTokens, CostUSD: envelope.TotalCostUSD,
		}, true
	}
	if usage, ok := parse(output); ok {
		return usage
	}
	for _, line := range nonEmptyLinesReverse(output) {
		if usage, ok := parse(line); ok {
			return usage
		}
	}
	return UsageData{}
}

func nonEmptyLinesReverse(value string) []string {
	lines := strings.Split(strings.TrimRight(value, "\n"), "\n")
	out := make([]string, 0, len(lines))
	for index := len(lines) - 1; index >= 0; index-- {
		if line := strings.TrimSpace(lines[index]); line != "" {
			out = append(out, line)
		}
	}
	return out
}

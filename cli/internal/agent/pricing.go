package agent

// ModelPricing holds per-million-token pricing for a model.
type ModelPricing struct {
	InputPer1M  float64
	OutputPer1M float64
}

// Pricing is the built-in model pricing table.
var Pricing = map[string]ModelPricing{
	// OpenAI (current generation)
	"gpt-5.4":      {InputPer1M: 2.00, OutputPer1M: 8.00},
	"gpt-5.4-mini": {InputPer1M: 0.30, OutputPer1M: 1.20},

	// Anthropic (current generation)
	"claude-opus-4-6":           {InputPer1M: 15.00, OutputPer1M: 75.00},
	"claude-sonnet-4-6":         {InputPer1M: 3.00, OutputPer1M: 15.00},
	"claude-haiku-4-5-20251001": {InputPer1M: 0.80, OutputPer1M: 4.00},

	// Local models (free)
	"qwen/qwen3-coder-next": {InputPer1M: 0, OutputPer1M: 0},
	"qwen/qwen3-coder-30b":  {InputPer1M: 0, OutputPer1M: 0},
	"minimax/minimax-m2.5":  {InputPer1M: 0, OutputPer1M: 0},
	"google/gemma-4-26b-a4b": {InputPer1M: 0, OutputPer1M: 0},
}

// EstimateCost returns the estimated cost in USD for the given model and token counts.
// Returns -1 if the model is not in the pricing table.
func EstimateCost(model string, inputTokens, outputTokens int) float64 {
	p, ok := Pricing[model]
	if !ok {
		return -1
	}
	return (float64(inputTokens)/1_000_000)*p.InputPer1M + (float64(outputTokens)/1_000_000)*p.OutputPer1M
}

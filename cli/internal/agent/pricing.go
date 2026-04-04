package agent

// ModelPricing holds per-million-token pricing for a model.
type ModelPricing struct {
	InputPer1M  float64
	OutputPer1M float64
}

// Pricing is the built-in model pricing table.
var Pricing = map[string]ModelPricing{
	"o3-mini":                  {InputPer1M: 1.10, OutputPer1M: 4.40},
	"gpt-4o":                   {InputPer1M: 2.50, OutputPer1M: 10.00},
	"gpt-5.4":                  {InputPer1M: 2.00, OutputPer1M: 8.00},
	"claude-sonnet-4-20250514": {InputPer1M: 3.00, OutputPer1M: 15.00},
	"claude-opus-4-20250514":   {InputPer1M: 15.00, OutputPer1M: 75.00},
	"claude-haiku-4-20250514":  {InputPer1M: 0.80, OutputPer1M: 4.00},
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

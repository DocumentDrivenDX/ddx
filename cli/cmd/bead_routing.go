package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// routingEvent is the parsed body of a kind:routing bead event.
type routingEvent struct {
	ResolvedProvider string   `json:"resolved_provider"`
	ResolvedModel    string   `json:"resolved_model,omitempty"`
	RouteReason      string   `json:"route_reason,omitempty"`
	FallbackChain    []string `json:"fallback_chain"`
	BaseURL          string   `json:"base_url,omitempty"`
	// Timestamp from the outer event, added for JSON output.
	CreatedAt time.Time `json:"created_at"`
	// Summary line from the outer event.
	Summary string `json:"summary,omitempty"`
}

func (f *CommandFactory) newBeadRoutingCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "routing <id>",
		Short: "Show routing decisions recorded for a bead",
		Long: `Show the kind:routing evidence entries written when agents executed this bead.

Each entry records which provider and model was selected, why, and whether any
fallback chain was applied. Use --json to get machine-readable output suitable
for feeding into the cost-tier analysis pipeline.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beadID := args[0]
			last, _ := cmd.Flags().GetInt("last")
			asJSON, _ := cmd.Flags().GetBool("json")

			events, err := f.beadStore().EventsByKind(beadID, "routing")
			if err != nil {
				return err
			}

			// Trim to last N if requested.
			if last > 0 && len(events) > last {
				events = events[len(events)-last:]
			}

			// Parse the JSON body of each routing event.
			parsed := make([]routingEvent, 0, len(events))
			for _, e := range events {
				re := routingEvent{
					CreatedAt: e.CreatedAt,
					Summary:   e.Summary,
				}
				if e.Body != "" {
					_ = json.Unmarshal([]byte(e.Body), &re)
				}
				parsed = append(parsed, re)
			}

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(parsed)
			}

			// Text output: per-event lines followed by a summary.
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "bead %s  routing decisions: %d\n\n", beadID, len(parsed))
			for _, re := range parsed {
				fmt.Fprintf(w, "  %s  %s\n", re.CreatedAt.Format(time.RFC3339), re.Summary)
			}

			if len(parsed) == 0 {
				return nil
			}

			// Aggregate counts.
			providerCounts := map[string]int{}
			modelCounts := map[string]int{}
			reasonCounts := map[string]int{}
			for _, re := range parsed {
				if re.ResolvedProvider != "" {
					providerCounts[re.ResolvedProvider]++
				}
				if re.ResolvedModel != "" {
					modelCounts[re.ResolvedModel]++
				}
				if re.RouteReason != "" {
					reasonCounts[re.RouteReason]++
				}
			}

			fmt.Fprintln(w, "\nSummary:")
			fmt.Fprintf(w, "  providers:  %s\n", formatCountMap(providerCounts))
			fmt.Fprintf(w, "  models:     %s\n", formatCountMap(modelCounts))
			fmt.Fprintf(w, "  reasons:    %s\n", formatCountMap(reasonCounts))
			return nil
		},
	}
	cmd.Flags().Int("last", 0, "Show only the last N routing decisions (0 = all)")
	cmd.Flags().Bool("json", false, "Output as JSON array")
	return cmd
}

// formatCountMap renders a map[string]int as "key1(n) key2(n) ..." sorted by count desc.
func formatCountMap(m map[string]int) string {
	if len(m) == 0 {
		return "(none)"
	}
	// Build sorted slice: highest count first, then lexicographic.
	type kv struct {
		k string
		v int
	}
	pairs := make([]kv, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, kv{k, v})
	}
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[j].v > pairs[i].v || (pairs[j].v == pairs[i].v && pairs[j].k < pairs[i].k) {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}
	out := ""
	for _, p := range pairs {
		if out != "" {
			out += " "
		}
		out += fmt.Sprintf("%s(%d)", p.k, p.v)
	}
	return out
}

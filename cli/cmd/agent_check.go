package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	agentconfig "github.com/DocumentDrivenDX/agent/config"
	"github.com/DocumentDrivenDX/ddx/internal/agent/providerstatus"
	"github.com/spf13/cobra"
)

const checkProbeTimeout = 5 * time.Second

type checkResultEntry struct {
	Provider  string `json:"provider"`
	Harness   string `json:"harness"`
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error"`
}

func (f *CommandFactory) newAgentCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Probe provider runtime availability (liveness, model inventory)",
		Long: `Probes each configured provider's /v1/models endpoint to report runtime availability.

Semantic distinction from 'doctor': doctor answers "is my config valid?" (config
validation, missing API keys); check answers "what providers can I use right now?"
(runtime liveness, which providers respond, which models are available).

Exits 0 if at least one provider is reachable and has at least one usable model.
Exits 1 otherwise.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := agentconfig.Load(f.WorkingDir)
			if err != nil {
				return fmt.Errorf("loading agent config: %w", err)
			}

			providerName, _ := cmd.Flags().GetString("provider")
			asJSON, _ := cmd.Flags().GetBool("json")

			var names []string
			if providerName != "" {
				if _, ok := cfg.GetProvider(providerName); !ok {
					return fmt.Errorf("unknown provider %q", providerName)
				}
				names = []string{providerName}
			} else {
				names = cfg.ProviderNames()
			}

			type namedResult struct {
				name    string
				harness string
				r       providerstatus.Result
				latency time.Duration
			}
			results := make([]namedResult, 0, len(names))
			for _, name := range names {
				pc := cfg.Providers[name]
				ctx, cancel := context.WithTimeout(context.Background(), checkProbeTimeout)
				start := time.Now()
				r := providerstatus.Probe(ctx, pc)
				elapsed := time.Since(start)
				cancel()
				results = append(results, namedResult{name: name, harness: pc.Type, r: r, latency: elapsed})
			}

			anyReachable := false
			for _, nr := range results {
				if nr.r.Reachable {
					if nr.r.Models == nil || len(nr.r.Models) > 0 {
						anyReachable = true
					}
				}
			}

			if asJSON {
				entries := make([]checkResultEntry, 0, len(results))
				for _, nr := range results {
					status := "unreachable"
					if nr.r.Reachable {
						status = "ok"
					}
					errMsg := ""
					if !nr.r.Reachable {
						errMsg = nr.r.Message
					}
					entries = append(entries, checkResultEntry{
						Provider:  nr.name,
						Harness:   nr.harness,
						Status:    status,
						LatencyMs: nr.latency.Milliseconds(),
						Error:     errMsg,
					})
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(entries); err != nil {
					return err
				}
			} else {
				for _, nr := range results {
					status := "UNREACHABLE"
					if nr.r.Reachable {
						status = "OK"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%-12s  %-12s  %s\n", nr.name, status, nr.r.Message)
				}
			}

			if !anyReachable {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().String("provider", "", "Check only this provider")
	cmd.Flags().Bool("json", false, "Output as JSON array")
	return cmd
}

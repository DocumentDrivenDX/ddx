package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	agentconfig "github.com/DocumentDrivenDX/agent/config"
	"github.com/DocumentDrivenDX/ddx/internal/agent/providerstatus"
	"github.com/spf13/cobra"
)

const checkProbeTimeout = 5 * time.Second

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
				name string
				r    providerstatus.Result
			}
			results := make([]namedResult, 0, len(names))
			for _, name := range names {
				pc := cfg.Providers[name]
				ctx, cancel := context.WithTimeout(context.Background(), checkProbeTimeout)
				r := providerstatus.Probe(ctx, pc)
				cancel()
				results = append(results, namedResult{name: name, r: r})
			}

			anyReachable := false
			for _, nr := range results {
				status := "UNREACHABLE"
				if nr.r.Reachable {
					status = "OK"
					// Anthropic: Models==nil means key-based (no listing endpoint),
					// treat as usable. OAI: require at least one discovered model.
					if nr.r.Models == nil || len(nr.r.Models) > 0 {
						anyReachable = true
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-12s  %-12s  %s\n", nr.name, status, nr.r.Message)
			}

			if !anyReachable {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().String("provider", "", "Check only this provider")
	return cmd
}

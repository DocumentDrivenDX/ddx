package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	agentconfig "github.com/DocumentDrivenDX/agent/config"
	oai "github.com/DocumentDrivenDX/agent/provider/openai"
	"github.com/spf13/cobra"
)

const checkProbeTimeout = 5 * time.Second

type checkResult struct {
	name      string
	reachable bool
	models    int
	msg       string
}

func checkProvider(name string, pc agentconfig.ProviderConfig) checkResult {
	if pc.Type == "anthropic" {
		if pc.APIKey == "" {
			return checkResult{name: name, reachable: false, msg: "missing API key"}
		}
		// Anthropic doesn't expose /v1/models — treat as reachable when key is present.
		return checkResult{name: name, reachable: true, models: -1, msg: "api key configured (model listing not supported)"}
	}

	if strings.TrimSpace(pc.BaseURL) == "" {
		return checkResult{name: name, reachable: false, msg: "no URL configured"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), checkProbeTimeout)
	defer cancel()

	ids, err := oai.DiscoverModels(ctx, pc.BaseURL, pc.APIKey)
	if err != nil {
		return checkResult{name: name, reachable: false, msg: err.Error()}
	}
	return checkResult{name: name, reachable: true, models: len(ids), msg: fmt.Sprintf("connected (%d models)", len(ids))}
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

			var names []string
			if providerName != "" {
				if _, ok := cfg.GetProvider(providerName); !ok {
					return fmt.Errorf("unknown provider %q", providerName)
				}
				names = []string{providerName}
			} else {
				names = cfg.ProviderNames()
			}

			results := make([]checkResult, 0, len(names))
			for _, name := range names {
				pc := cfg.Providers[name]
				r := checkProvider(name, pc)
				results = append(results, r)
			}

			anyReachable := false
			for _, r := range results {
				status := "UNREACHABLE"
				if r.reachable {
					status = "OK"
					if r.models > 0 {
						anyReachable = true
					} else if r.models < 0 {
						// Anthropic: key configured, treat as usable.
						anyReachable = true
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-12s  %-12s  %s\n", r.name, status, r.msg)
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

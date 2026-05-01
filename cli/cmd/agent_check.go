package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	agentlib "github.com/DocumentDrivenDX/fizeau"
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
			svc, err := agent.NewServiceFromWorkDir(f.WorkingDir)
			if err != nil {
				return fmt.Errorf("loading agent config: %w", err)
			}

			providerName, _ := cmd.Flags().GetString("provider")
			asJSON, _ := cmd.Flags().GetBool("json")

			ctx, cancel := context.WithTimeout(context.Background(), checkProbeTimeout*2)
			defer cancel()

			providers, err := svc.ListProviders(ctx)
			if err != nil {
				return fmt.Errorf("loading agent config: %w", err)
			}

			// Filter to named provider if requested.
			if providerName != "" {
				found := false
				for _, p := range providers {
					if p.Name == providerName {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("unknown provider %q", providerName)
				}
				filtered := make([]agentlib.ProviderInfo, 0, 1)
				for _, p := range providers {
					if p.Name == providerName {
						filtered = append(filtered, p)
					}
				}
				providers = filtered
			}

			anyReachable := false
			for _, p := range providers {
				if p.Status == "connected" {
					anyReachable = true
					break
				}
			}

			if asJSON {
				entries := make([]checkResultEntry, 0, len(providers))
				for _, p := range providers {
					status := "unreachable"
					errMsg := ""
					if p.Status == "connected" {
						status = "ok"
					} else {
						errMsg = p.Status
						// Strip "error: " prefix for cleaner error messages.
						errMsg = strings.TrimPrefix(errMsg, "error: ")
					}
					entries = append(entries, checkResultEntry{
						Provider:  p.Name,
						Harness:   p.Type,
						Status:    status,
						LatencyMs: 0, // latency not tracked by service layer
						Error:     errMsg,
					})
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(entries); err != nil {
					return err
				}
			} else {
				for _, p := range providers {
					status := "UNREACHABLE"
					if p.Status == "connected" {
						status = "OK"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%-12s  %-12s  %s\n", p.Name, status, p.Status)
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

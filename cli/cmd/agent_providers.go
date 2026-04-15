package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agentconfig "github.com/DocumentDrivenDX/agent/config"
	oai "github.com/DocumentDrivenDX/agent/provider/openai"
	"github.com/spf13/cobra"
)

const providerProbeTimeout = 3 * time.Second

type providerStatusEntry struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	Default bool   `json:"default,omitempty"`
	Status  string `json:"status"`
}

func probeProvider(pc agentconfig.ProviderConfig) string {
	if pc.Type == "anthropic" {
		if pc.APIKey == "" {
			return "missing API key"
		}
		return "api key configured"
	}
	if strings.TrimSpace(pc.BaseURL) == "" {
		return "no URL configured"
	}
	ctx, cancel := context.WithTimeout(context.Background(), providerProbeTimeout)
	defer cancel()
	models, err := oai.DiscoverModels(ctx, pc.BaseURL, pc.APIKey)
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("connected (%d models)", len(models))
}

func (f *CommandFactory) newAgentProvidersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "providers",
		Short: "List configured providers with live status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := agentconfig.Load(f.WorkingDir)
			if err != nil {
				return fmt.Errorf("loading agent config: %w", err)
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			defName := cfg.DefaultName()

			if asJSON {
				entries := make([]providerStatusEntry, 0, len(cfg.Providers))
				for _, name := range cfg.ProviderNames() {
					pc := cfg.Providers[name]
					url := pc.BaseURL
					if url == "" {
						url = "(api)"
					}
					entries = append(entries, providerStatusEntry{
						Name:    name,
						Type:    pc.Type,
						BaseURL: url,
						Model:   pc.Model,
						Default: name == defName,
						Status:  probeProvider(pc),
					})
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%-12s %-15s %-40s %-30s %s\n", "NAME", "TYPE", "URL", "MODEL", "STATUS")
			for _, name := range cfg.ProviderNames() {
				pc := cfg.Providers[name]
				status := probeProvider(pc)
				marker := " "
				if name == defName {
					marker = "*"
				}
				url := pc.BaseURL
				if url == "" {
					url = "(api)"
				}
				if len(url) > 38 {
					url = url[:38] + ".."
				}
				modelStr := pc.Model
				if len(modelStr) > 28 {
					modelStr = modelStr[:28] + ".."
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%-11s %-15s %-40s %-30s %s\n", marker, name, pc.Type, url, modelStr, status)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON array")
	return cmd
}

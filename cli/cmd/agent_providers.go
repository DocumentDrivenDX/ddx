package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	agentconfig "github.com/DocumentDrivenDX/agent/config"
	oai "github.com/DocumentDrivenDX/agent/provider/openai"
	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/spf13/cobra"
)

const providerProbeTimeout = 3 * time.Second

type providerStatusEntry struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	BaseURL       string `json:"base_url"`
	Model         string `json:"model"`
	Default       bool   `json:"default,omitempty"`
	Status        string `json:"status"`
	CooldownUntil string `json:"cooldown_until,omitempty"`
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
		Long: `List configured providers with live connectivity status.

Also shows process-local provider health cooldowns set by tier-based
auto-escalation (ddx agent execute-loop). A provider on cooldown was
recently found to be unreachable and will be skipped by the escalation
loop until the cooldown expires.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := agentconfig.Load(f.WorkingDir)
			if err != nil {
				return fmt.Errorf("loading agent config: %w", err)
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			defName := cfg.DefaultName()
			healthSnap := agent.GlobalProviderHealth.Snapshot()

			if asJSON {
				entries := make([]providerStatusEntry, 0, len(cfg.Providers))
				for _, name := range cfg.ProviderNames() {
					pc := cfg.Providers[name]
					url := pc.BaseURL
					if url == "" {
						url = "(api)"
					}
					entry := providerStatusEntry{
						Name:    name,
						Type:    pc.Type,
						BaseURL: url,
						Model:   pc.Model,
						Default: name == defName,
						Status:  probeProvider(pc),
					}
					if until, ok := healthSnap[name]; ok {
						entry.CooldownUntil = until.UTC().Format(time.RFC3339)
					}
					entries = append(entries, entry)
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
				if until, ok := healthSnap[name]; ok {
					fmt.Fprintf(cmd.OutOrStdout(), "  ⚠ cooldown active until %s\n", until.UTC().Format(time.RFC3339))
				}
			}

			// Show any harnesses on cooldown that aren't in the provider config
			// (e.g. binary harnesses like "claude" or "codex").
			if len(healthSnap) > 0 {
				names := cfg.ProviderNames()
				providerSet := make(map[string]bool, len(names))
				for _, n := range names {
					providerSet[n] = true
				}
				var extra []string
				for name := range healthSnap {
					if !providerSet[name] {
						extra = append(extra, name)
					}
				}
				if len(extra) > 0 {
					sort.Strings(extra)
					fmt.Fprintln(cmd.OutOrStdout(), "\nHarness cooldowns (set by execute-loop escalation):")
					for _, name := range extra {
						until := healthSnap[name]
						fmt.Fprintf(cmd.OutOrStdout(), "  %-20s cooldown until %s\n", name, until.UTC().Format(time.RFC3339))
					}
				}
			}

			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON array")
	return cmd
}

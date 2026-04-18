package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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

func (f *CommandFactory) newAgentProvidersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "providers",
		Short: "List configured providers with live status",
		Long: `List configured providers with live connectivity status.

Also shows routing-cooldown state for providers recently demoted by
the routing engine. A provider on cooldown was found unreachable and
will be skipped by escalation/failover until the cooldown expires.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := agent.NewServiceFromWorkDir(f.WorkingDir)
			if err != nil {
				return fmt.Errorf("constructing agent service: %w", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), providerProbeTimeout*time.Duration(2))
			defer cancel()

			providers, err := svc.ListProviders(ctx)
			if err != nil {
				return fmt.Errorf("listing providers: %w", err)
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				entries := make([]providerStatusEntry, 0, len(providers))
				for _, p := range providers {
					url := p.BaseURL
					if url == "" {
						url = "(api)"
					}
					entry := providerStatusEntry{
						Name:    p.Name,
						Type:    p.Type,
						BaseURL: url,
						Model:   p.DefaultModel,
						Default: p.IsDefault,
						Status:  p.Status,
					}
					if p.CooldownState != nil && !p.CooldownState.Until.IsZero() {
						entry.CooldownUntil = p.CooldownState.Until.UTC().Format(time.RFC3339)
					}
					entries = append(entries, entry)
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%-12s %-15s %-40s %-30s %s\n", "NAME", "TYPE", "URL", "MODEL", "STATUS")
			for _, p := range providers {
				marker := " "
				if p.IsDefault {
					marker = "*"
				}
				url := p.BaseURL
				if url == "" {
					url = "(api)"
				}
				if len(url) > 38 {
					url = url[:38] + ".."
				}
				modelStr := p.DefaultModel
				if len(modelStr) > 28 {
					modelStr = modelStr[:28] + ".."
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%-11s %-15s %-40s %-30s %s\n", marker, p.Name, p.Type, url, modelStr, p.Status)
				if p.CooldownState != nil && !p.CooldownState.Until.IsZero() {
					fmt.Fprintf(cmd.OutOrStdout(), "  ⚠ cooldown active until %s\n", p.CooldownState.Until.UTC().Format(time.RFC3339))
				}
			}

			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON array")
	return cmd
}

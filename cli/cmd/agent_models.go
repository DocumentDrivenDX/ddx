package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	agentlib "github.com/DocumentDrivenDX/fizeau"
	"github.com/spf13/cobra"
)

const modelsProbeTimeout = 3 * time.Second

func (f *CommandFactory) newAgentModelsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "models",
		Short: "List models for a configured provider",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := agent.NewServiceFromWorkDir(f.WorkingDir)
			if err != nil {
				return fmt.Errorf("loading agent config: %w", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), modelsProbeTimeout*2)
			defer cancel()

			showAll, _ := cmd.Flags().GetBool("all")
			providerName, _ := cmd.Flags().GetString("provider")

			providers, err := svc.ListProviders(ctx)
			if err != nil {
				return fmt.Errorf("listing providers: %w", err)
			}

			// Build a provider-type lookup for the anthropic special case.
			providerType := make(map[string]string, len(providers))
			providerDefault := make(map[string]string, len(providers))
			for _, p := range providers {
				providerType[p.Name] = p.Type
				providerDefault[p.Name] = p.DefaultModel
			}

			if showAll {
				for _, p := range providers {
					fmt.Fprintf(cmd.OutOrStdout(), "[%s]\n", p.Name)
					models, _ := svc.ListModels(ctx, agentlib.ModelFilter{Provider: p.Name})
					printModels(cmd, p.Name, p.Type, p.DefaultModel, models)
					fmt.Fprintln(cmd.OutOrStdout())
				}
				return nil
			}

			name := providerName
			if name == "" {
				// Find the default provider.
				for _, p := range providers {
					if p.IsDefault {
						name = p.Name
						break
					}
				}
			}
			if name == "" && len(providers) > 0 {
				name = providers[0].Name
			}

			pType, ok := providerType[name]
			if !ok {
				return fmt.Errorf("unknown provider %q", name)
			}

			if pType == "anthropic" {
				fmt.Fprintln(cmd.OutOrStdout(), "Anthropic does not support model listing.")
				if m := providerDefault[name]; m != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Configured model: %s\n", m)
				}
				return nil
			}

			models, err := svc.ListModels(ctx, agentlib.ModelFilter{Provider: name})
			if err != nil {
				return fmt.Errorf("listing models: %w", err)
			}
			printModels(cmd, name, pType, providerDefault[name], models)
			return nil
		},
	}
	cmd.Flags().String("provider", "", "Provider name (default: configured default)")
	cmd.Flags().Bool("all", false, "List models for every configured provider")
	return cmd
}

// printModels renders the model list for one provider.
// The configured model is marked with "*". The auto-selected model (when no
// static model is set and rank 0 is available) is marked with ">".
// Catalog-recognized models show their catalog reference in brackets.
func printModels(cmd *cobra.Command, providerName, providerTyp, configuredModel string, models []agentlib.ModelInfo) {
	out := cmd.OutOrStdout()

	if providerTyp == "anthropic" {
		fmt.Fprintln(out, "  (anthropic — no model listing endpoint)")
		return
	}

	if len(models) == 0 {
		fmt.Fprintln(out, "  (unavailable)")
		return
	}

	// Determine auto-selected model: first by rank when no model is configured.
	autoSelected := ""
	if configuredModel == "" {
		// Find the model with the lowest RankPosition.
		best := -1
		for _, m := range models {
			if best < 0 || m.RankPosition < best {
				best = m.RankPosition
				autoSelected = m.ID
			}
		}
	}

	for _, m := range models {
		marker := "  "
		if m.ID == configuredModel {
			marker = "* "
		} else if m.ID == autoSelected {
			marker = "> "
		}
		annotation := ""
		if m.Power > 0 {
			annotation = fmt.Sprintf("  [power: %d]", m.Power)
		}
		if m.CatalogRef != "" {
			annotation += "  [catalog: " + m.CatalogRef + "]"
		}
		fmt.Fprintf(out, "%s%s%s\n", marker, m.ID, annotation)
	}
	if configuredModel == "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "  * = configured  > = would auto-select")
	}
}

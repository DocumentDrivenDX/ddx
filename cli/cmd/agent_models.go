package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentconfig "github.com/DocumentDrivenDX/agent/config"
	"github.com/DocumentDrivenDX/agent/modelcatalog"
	oai "github.com/DocumentDrivenDX/agent/provider/openai"
	"github.com/spf13/cobra"
)

const modelsProbeTimeout = 3 * time.Second

func (f *CommandFactory) newAgentModelsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "models",
		Short: "List models for a configured provider",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := agentconfig.Load(f.WorkingDir)
			if err != nil {
				return fmt.Errorf("loading agent config: %w", err)
			}

			cat, _ := modelcatalog.Default()

			showAll, _ := cmd.Flags().GetBool("all")
			providerName, _ := cmd.Flags().GetString("provider")

			if showAll {
				for _, name := range cfg.ProviderNames() {
					pc := cfg.Providers[name]
					fmt.Fprintf(cmd.OutOrStdout(), "[%s]\n", name)
					printModelsForProvider(cmd, pc, cat)
					fmt.Fprintln(cmd.OutOrStdout())
				}
				return nil
			}

			name := providerName
			if name == "" {
				name = cfg.DefaultName()
			}
			pc, ok := cfg.GetProvider(name)
			if !ok {
				return fmt.Errorf("unknown provider %q", name)
			}

			if pc.Type == "anthropic" {
				fmt.Fprintln(cmd.OutOrStdout(), "Anthropic does not support model listing.")
				if pc.Model != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Configured model: %s\n", pc.Model)
				}
				return nil
			}

			printModelsForProvider(cmd, pc, cat)
			return nil
		},
	}
	cmd.Flags().String("provider", "", "Provider name (default: configured default)")
	cmd.Flags().Bool("all", false, "List models for every configured provider")
	return cmd
}

// printModelsForProvider probes a provider's /v1/models endpoint and prints the
// full ranked list. The configured model is marked with "*". The auto-selected
// model (when no static model is set) is marked with ">". Catalog-recognized
// models show their catalog target ID in brackets; pattern-matched models show
// [pattern].
func printModelsForProvider(cmd *cobra.Command, pc agentconfig.ProviderConfig, cat *modelcatalog.Catalog) {
	out := cmd.OutOrStdout()

	if pc.Type == "anthropic" {
		fmt.Fprintln(out, "  (anthropic — no model listing endpoint)")
		return
	}

	var knownModels map[string]string
	if cat != nil {
		knownModels = cat.AllConcreteModels(modelcatalog.SurfaceAgentOpenAI)
	}

	if strings.TrimSpace(pc.BaseURL) == "" {
		fmt.Fprintln(out, "  (unavailable)")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), modelsProbeTimeout)
	defer cancel()

	ids, err := oai.DiscoverModels(ctx, pc.BaseURL, pc.APIKey)
	if err != nil || len(ids) == 0 {
		fmt.Fprintln(out, "  (unavailable)")
		return
	}

	ranked, err := oai.RankModels(ids, knownModels, pc.ModelPattern)
	if err != nil {
		// Pattern compile error — fall back to plain list.
		for _, id := range ids {
			fmt.Fprintf(out, "  %s\n", id)
		}
		return
	}

	autoSelected := ""
	if pc.Model == "" && len(ranked) > 0 {
		autoSelected = ranked[0].ID
	}

	for _, sm := range ranked {
		marker := "  "
		if sm.ID == pc.Model {
			marker = "* "
		} else if sm.ID == autoSelected {
			marker = "> "
		}
		annotation := ""
		if sm.CatalogRef != "" {
			annotation = "  [catalog: " + sm.CatalogRef + "]"
		} else if sm.PatternMatch {
			annotation = "  [pattern]"
		}
		fmt.Fprintf(out, "%s%s%s\n", marker, sm.ID, annotation)
	}
	if pc.Model == "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "  * = configured  > = would auto-select")
	}
}

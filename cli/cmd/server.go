package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/spf13/cobra"
)

// resolveTsnetAuthKey applies auth key precedence:
// TS_AUTHKEY env var > --tsnet-auth-key CLI flag > config file auth_key.
// The env var is preferred because CLI flags are visible in ps/history.
func resolveTsnetAuthKey(envKey, flagKey, configKey string) string {
	if envKey != "" {
		return envKey
	}
	if flagKey != "" {
		return flagKey
	}
	return configKey
}

// defaultTsnetHostname returns "ddx-<short-hostname>" for the current host,
// falling back to "ddx" when the hostname is unavailable or empty.
func defaultTsnetHostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "ddx"
	}
	// Trim any trailing domain so the ts-net name stays short and stable.
	if i := strings.Index(h, "."); i >= 0 {
		h = h[:i]
	}
	return "ddx-" + h
}

func (f *CommandFactory) newServerCommand() *cobra.Command {
	var port int
	var addr string
	var tlsCert string
	var tlsKey string
	var tsnetEnabled bool
	var tsnetHostname string
	var tsnetAuthKey string
	var hubMode bool
	var federationAllowPlainHTTP bool
	var hubURL string
	var federationSelfURL string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start the DDx HTTP and MCP server",
		Long: `Start the DDx server exposing documents, beads, document graph,
and agent session logs over HTTP REST and MCP endpoints.

HTTP API:
  GET /api/health              Liveness check
  GET /api/ready               Readiness check with dependency status
  GET /api/documents           List library documents (?type=)
  GET /api/documents/:path     Read a document
  GET /api/search?q=           Full-text search across documents
  GET /api/personas/:role      Resolve persona for a role
  GET /api/beads               List beads (?status=&label=)
  GET /api/beads/:id           Show a specific bead
  GET /api/beads/ready         List ready beads
  GET /api/beads/blocked       List blocked beads
  GET /api/beads/status        Bead summary counts
  GET /api/beads/dep/tree/:id  Dependency tree for a bead
  GET /api/docs/graph          Document dependency graph
  GET /api/docs/stale          Stale documents
  GET /api/docs/:id            Document metadata and staleness
  GET /api/docs/:id/deps       Upstream dependencies
  GET /api/docs/:id/dependents Downstream dependents
  GET /api/agent/sessions      List agent sessions (?harness=&since=)
  GET /api/agent/sessions/:id  Session detail
  GET /api/metrics/summary     Process metrics summary (?since=)
  GET /api/metrics/cost        Process metrics cost (?bead=&feature=&since=)
  GET /api/metrics/cycle-time  Process metrics cycle time (?since=)
  GET /api/metrics/rework      Process metrics rework (?since=)

MCP (POST /mcp):
  ddx_list_documents           List library documents
  ddx_read_document            Read a document
  ddx_search                   Full-text search
  ddx_resolve_persona          Resolve persona for a role
  ddx_list_beads               List beads
  ddx_show_bead                Show a specific bead
  ddx_bead_ready               List ready beads
  ddx_bead_status              Bead summary counts
  ddx_doc_graph                Document dependency graph
  ddx_doc_stale                Stale documents
  ddx_doc_show                 Document metadata
  ddx_doc_deps                 Upstream dependencies
  ddx_agent_sessions           List agent sessions`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if federationAllowPlainHTTP && !hubMode {
				return fmt.Errorf("--federation-allow-plain-http requires --hub-mode")
			}
			listenAddr := fmt.Sprintf("%s:%d", addr, port)
			fmt.Fprintf(cmd.OutOrStdout(), "DDx server listening on https://%s\n", listenAddr)
			srv := server.New(listenAddr, f.WorkingDir)
			if hubMode {
				srv.EnableHubMode(federationAllowPlainHTTP)
				fmt.Fprintf(cmd.OutOrStdout(), "DDx hub-mode enabled (federation routes mounted)\n")
				if federationAllowPlainHTTP {
					fmt.Fprintf(cmd.OutOrStdout(), "WARNING: --federation-allow-plain-http is ON; non-ts-net peers may register\n")
				}
			}

			if strings.TrimSpace(hubURL) != "" {
				selfURL := federationSelfURL
				if selfURL == "" {
					selfURL = "https://" + listenAddr
				}
				if err := srv.EnableSpokeMode(cmd.Context(), hubURL, selfURL); err != nil {
					return fmt.Errorf("federation spoke registration failed: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "DDx spoke-mode enabled (registered with hub %s as %s)\n", hubURL, selfURL)
			}

			// Build tsnet config. tsnet is on by default — flags override config,
			// and --tsnet=false disables it. Hostname defaults to ddx-<hostname>.
			tc := &config.TsnetConfig{Enabled: true}
			if cfg, err := config.LoadWithWorkingDir(f.WorkingDir); err == nil && cfg.Server != nil && cfg.Server.Tsnet != nil {
				*tc = *cfg.Server.Tsnet
			}
			if cmd.Flags().Changed("tsnet") {
				tc.Enabled = tsnetEnabled
			}
			if tsnetHostname != "" {
				tc.Hostname = tsnetHostname
			}
			if tc.Hostname == "" {
				tc.Hostname = defaultTsnetHostname()
			}
			// Prefer TS_AUTHKEY env var; CLI flag is a fallback (secrets on CLI are visible in ps/history)
			tc.AuthKey = resolveTsnetAuthKey(os.Getenv("TS_AUTHKEY"), tsnetAuthKey, tc.AuthKey)
			if tc.Enabled {
				srv.TsnetConfig = tc
				fmt.Fprintf(cmd.OutOrStdout(), "DDx ts-net enabled (hostname: %s)\n", tc.Hostname)
			}

			return srv.ListenAndServeTLS(tlsCert, tlsKey)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 7743, "Port to listen on")
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1", "Address to bind to")
	cmd.Flags().StringVar(&tlsCert, "tls-cert", "", "TLS certificate file (PEM); auto-generates self-signed cert if omitted")
	cmd.Flags().StringVar(&tlsKey, "tls-key", "", "TLS private key file (PEM); auto-generates self-signed key if omitted")
	cmd.Flags().BoolVar(&tsnetEnabled, "tsnet", true, "Enable Tailscale ts-net listener (use --tsnet=false to disable; see ADR-006)")
	cmd.Flags().StringVar(&tsnetHostname, "tsnet-hostname", "", "Tailscale hostname (default: ddx-<hostname>)")
	cmd.Flags().StringVar(&tsnetAuthKey, "tsnet-auth-key", "", "Tailscale auth key for headless/CI use (SECURITY: visible in ps/history; prefer TS_AUTHKEY env var)")
	cmd.Flags().BoolVar(&hubMode, "hub-mode", false, "Run as a federation hub: mount /api/federation/* routes (FEAT-026)")
	cmd.Flags().BoolVar(&federationAllowPlainHTTP, "federation-allow-plain-http", false, "Accept federation registrations from non-loopback non-ts-net peers (hub-mode only; logs WARN on each accepted plain-HTTP registration)")
	cmd.Flags().StringVar(&hubURL, "hub-address", "", "Federation hub base URL to register with on startup (e.g. https://hub:7743). Enables spoke-mode (FEAT-026). Combine with --hub-mode for hub_spoke role.")
	cmd.Flags().StringVar(&federationSelfURL, "federation-self-url", "", "Self-URL advertised to the hub when --hub-address is set; defaults to https://<listen-addr>")

	// Worker management
	cmd.AddCommand(f.newServerWorkersCommand())

	// State file utilities (prune, …)
	cmd.AddCommand(f.newServerStateCommand())

	// Service management (systemd on Linux, launchd on macOS)
	cmd.AddCommand(f.newServerInstallCommand())
	cmd.AddCommand(f.newServerUninstallCommand())
	cmd.AddCommand(f.newServerStartCommand())
	cmd.AddCommand(f.newServerStopCommand())
	cmd.AddCommand(f.newServerStatusCommand())

	return cmd
}

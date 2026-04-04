package cmd

import (
	"fmt"

	"github.com/easel/ddx/internal/server"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newServerCommand() *cobra.Command {
	var port int
	var addr string

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
		Aliases: []string{"serve"},
		RunE: func(cmd *cobra.Command, args []string) error {
			listenAddr := fmt.Sprintf("%s:%d", addr, port)
			fmt.Fprintf(cmd.OutOrStdout(), "DDx server listening on %s\n", listenAddr)
			srv := server.New(listenAddr, f.WorkingDir)
			return srv.ListenAndServe()
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to listen on")
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1", "Address to bind to")

	return cmd
}

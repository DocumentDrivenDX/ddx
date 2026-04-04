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
		Long: `Start the DDx server exposing documents, beads, and document graph
over HTTP REST and MCP endpoints.

HTTP API:
  GET /api/documents          List library documents
  GET /api/documents/:path    Read a document
  GET /api/beads              List beads (optional ?status=&label=)
  GET /api/beads/ready        List ready beads
  GET /api/beads/status       Bead summary counts
  GET /api/docs/graph         Document dependency graph
  GET /api/docs/stale         Stale documents

MCP (POST /mcp):
  ddx_list_documents          List library documents
  ddx_read_document           Read a document
  ddx_list_beads              List beads
  ddx_bead_ready              List ready beads`,
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

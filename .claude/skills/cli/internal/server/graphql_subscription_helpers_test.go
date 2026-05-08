package server

// newGraphQLSubscriptionTestServer — shared helper for GraphQL-over-WebSocket
// subscription tests. Extracted from the per-feature test files to avoid
// duplicating the ~8-line gqlgen + httptest setup.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
	"github.com/gorilla/websocket"
)

// newGraphQLSubscriptionTestServer creates a GraphQL-over-WebSocket test server
// from a pre-populated resolver. Returns the httptest.Server and a dial func
// that opens a *websocket.Conn using the graphql-transport-ws subprotocol.
// The server is closed via t.Cleanup; the caller is responsible for closing
// the returned connection.
func newGraphQLSubscriptionTestServer(t *testing.T, resolver *ddxgraphql.Resolver) (*httptest.Server, func() *websocket.Conn) {
	t.Helper()
	gqlSrv := handler.New(ddxgraphql.NewExecutableSchema(ddxgraphql.Config{
		Resolvers:  resolver,
		Directives: ddxgraphql.DirectiveRoot{},
	}))
	gqlSrv.AddTransport(transport.POST{})
	gqlSrv.AddTransport(transport.GET{})
	gqlSrv.AddTransport(transport.Websocket{
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	})
	ts := httptest.NewServer(gqlSrv)
	t.Cleanup(ts.Close)

	dial := func() *websocket.Conn {
		t.Helper()
		wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/graphql"
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, map[string][]string{
			"Sec-WebSocket-Protocol": {"graphql-transport-ws"},
		})
		if err != nil {
			t.Fatalf("WebSocket dial: %v", err)
		}
		return conn
	}
	return ts, dial
}

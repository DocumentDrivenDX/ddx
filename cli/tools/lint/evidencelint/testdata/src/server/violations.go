package server

import (
	"library"
)

// Pattern 2: composite literal mcpContent{Type:"text"}.
func pattern2(payload string) mcpToolResult {
	return mcpToolResult{
		Content: []mcpContent{
			{Type: "text", Text: payload}, // want `evidencelint: mcpContent.*Type.*text`
		},
	}
}

// Pattern 4: response writer fed by a source-package call.
func pattern4(w ResponseWriter, name string) {
	_, _ = w.Write(library.ReadDoc(name)) // want `evidencelint: server response writer.*library/exec/diff/session/persona`
}

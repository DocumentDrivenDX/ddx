// Package server provides a stub mcpContent type for analyzer tests.
package server

type mcpContent struct {
	Type string
	Text string
}

type mcpToolResult struct {
	Content []mcpContent
}

type ResponseWriter interface {
	Write(p []byte) (int, error)
}

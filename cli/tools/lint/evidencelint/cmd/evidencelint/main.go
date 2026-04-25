// Command evidencelint runs the FEAT-022 no-unbounded-prompts analyzer.
//
// Usage:
//
//	go run ./tools/lint/evidencelint/cmd/evidencelint ./...
package main

import (
	"github.com/DocumentDrivenDX/ddx/tools/lint/evidencelint"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(evidencelint.Analyzer)
}

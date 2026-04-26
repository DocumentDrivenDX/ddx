// Command runtimelint runs the SD-024 Stage 4 structural analyzer.
//
// Usage:
//
//	go run ./tools/lint/runtimelint/cmd/runtimelint ./...
package main

import (
	"github.com/DocumentDrivenDX/ddx/tools/lint/runtimelint"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(runtimelint.Analyzer)
}

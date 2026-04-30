// Command routinglint runs the FEAT-006 routing-cleanup analyzer
// (ddx-653f6ac9). It fails if any of the compensating-routing
// helpers retired by ddx-3bd7396a have been reintroduced.
//
// Usage:
//
//	go run ./tools/lint/routinglint/cmd/routinglint ./...
package main

import (
	"github.com/DocumentDrivenDX/ddx/tools/lint/routinglint"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(routinglint.Analyzer)
}

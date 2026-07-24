// Command lockreentrylint runs the collection-lock re-entry analyzer.
//
// Usage:
//
//	go run ./tools/lint/lockreentrylint/cmd/lockreentrylint ./...
package main

import (
	"github.com/DocumentDrivenDX/ddx/tools/lint/lockreentrylint"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(lockreentrylint.Analyzer)
}

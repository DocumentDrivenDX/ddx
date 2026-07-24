package lockreentrylint

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestViolations(t *testing.T) {
	dir := analysistest.TestData()
	analysistest.Run(t, dir, Analyzer, "violations")
}

func TestClean(t *testing.T) {
	dir := analysistest.TestData()
	analysistest.Run(t, dir, Analyzer, "clean")
}

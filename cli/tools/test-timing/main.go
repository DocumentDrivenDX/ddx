// test-timing reads go test -json output from stdin and writes a Markdown
// table of the top N slowest tests to stdout.
//
// Usage:
//
//	go test -json ./... | go run ./tools/test-timing
//	go run ./tools/test-timing -n 30 < test-output.json
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
)

type testEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
}

func main() {
	n := flag.Int("n", 20, "number of slowest tests to show")
	flag.Parse()

	var results []testEvent
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var ev testEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Test == "" {
			continue
		}
		if ev.Action != "pass" && ev.Action != "fail" {
			continue
		}
		results = append(results, ev)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "read error: %v\n", err)
		os.Exit(1)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Elapsed > results[j].Elapsed
	})

	top := results
	if len(top) > *n {
		top = top[:*n]
	}

	if len(top) == 0 {
		fmt.Println("# Test Timing Summary\n\nNo test results found.")
		return
	}

	fmt.Printf("# Top %d Slowest Tests\n\n", len(top))
	fmt.Printf("| Rank | Test | Package | Elapsed (s) | Result |\n")
	fmt.Printf("|------|------|---------|-------------|--------|\n")
	for i, ev := range top {
		status := "PASS"
		if ev.Action == "fail" {
			status = "FAIL"
		}
		fmt.Printf("| %d | `%s` | `%s` | %.3f | %s |\n", i+1, ev.Test, ev.Package, ev.Elapsed, status)
	}
}

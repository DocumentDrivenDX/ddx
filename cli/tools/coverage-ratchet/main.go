// coverage-ratchet reads an aggregate coverage percentage from a Go
// coverage profile and fails when it regresses below the baseline
// recorded in a checked-in config file.
//
// Usage:
//
//	go test -coverprofile=coverage.out ./...
//	go run ./tools/coverage-ratchet -profile coverage.out -baseline coverage-baseline.yml
//
// The baseline value is loaded from the YAML file; there is no compiled
// fallback. A missing or unreadable config file is an error so the ratchet
// cannot silently regress.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type config struct {
	Baseline float64 `yaml:"baseline"`
}

// LoadBaseline reads the baseline percentage from the given YAML config
// file. It returns an error if the file cannot be read or parsed; there
// is no compiled default so callers must always provide a valid config.
func LoadBaseline(path string) (float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read baseline config %q: %w", path, err)
	}
	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return 0, fmt.Errorf("parse baseline config %q: %w", path, err)
	}
	return cfg.Baseline, nil
}

// CoverageFromProfile parses a Go coverage profile and returns the
// aggregate covered-statement percentage across every block.
func CoverageFromProfile(path string) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open profile %q: %w", path, err)
	}
	defer f.Close()

	var total, covered int64
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		stmts, err := strconv.ParseInt(parts[len(parts)-2], 10, 64)
		if err != nil {
			continue
		}
		count, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
		if err != nil {
			continue
		}
		total += stmts
		if count > 0 {
			covered += stmts
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read profile %q: %w", path, err)
	}
	if total == 0 {
		return 0, fmt.Errorf("no statements found in profile %q", path)
	}
	return 100.0 * float64(covered) / float64(total), nil
}

// Enforce reports an error when measured aggregate coverage falls
// below the configured baseline. Equal or improved coverage passes.
func Enforce(measured, baseline float64) error {
	const eps = 1e-9
	if measured+eps < baseline {
		return fmt.Errorf("coverage regression: measured %.2f%% < baseline %.2f%%", measured, baseline)
	}
	return nil
}

func main() {
	profile := flag.String("profile", "coverage.out", "path to go test coverage profile")
	baselinePath := flag.String("baseline", "coverage-baseline.yml", "path to baseline config file")
	flag.Parse()

	baseline, err := LoadBaseline(*baselinePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "coverage-ratchet: %v\n", err)
		os.Exit(2)
	}

	measured, err := CoverageFromProfile(*profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "coverage-ratchet: %v\n", err)
		os.Exit(2)
	}

	fmt.Printf("coverage: measured=%.2f%% baseline=%.2f%%\n", measured, baseline)
	if err := Enforce(measured, baseline); err != nil {
		fmt.Fprintf(os.Stderr, "coverage-ratchet: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("coverage-ratchet: OK")
}

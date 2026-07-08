package agent

import (
	"path/filepath"
	"strings"
)

func isLockMetricsPath(path string) bool {
	clean := strings.TrimSpace(filepath.ToSlash(path))
	return clean == ".ddx/metrics/locks.jsonl" || strings.HasPrefix(clean, ".ddx/metrics/locks.jsonl.")
}

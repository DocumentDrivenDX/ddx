//go:build linux

package workerstatus

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func systemLoad5() (float64, bool, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, true, fmt.Errorf("read /proc/loadavg: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, true, fmt.Errorf("parse /proc/loadavg: expected at least two fields")
	}
	load5, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, true, fmt.Errorf("parse /proc/loadavg five-minute load: %w", err)
	}
	return load5, true, nil
}

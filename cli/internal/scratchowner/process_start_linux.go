//go:build linux

package scratchowner

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// processStartIdentity returns a durable process-start token for pid from
// /proc/<pid>/stat field 22 (starttime, clock ticks since boot). Empty when
// unavailable. Format: "linux-startticks:<n>".
func processStartIdentity(pid int) string {
	if pid <= 0 {
		return ""
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	// Field 2 (comm) is parenthesized and may contain spaces; scan past the
	// final ')' then take field index 20 of the remainder (starttime is the
	// 22nd whitespace field overall; after comm the zero-based index is 19
	// if counting state as 0… actually: after ')' the fields are:
	// state ppid pgrp session tty_nr tpgid flags minflt cminflt majflt
	// cmajflt utime stime cutime cstime priority nice num_threads itrealvalue
	// starttime  — starttime is the 20th field in the tail (0-based index 19).
	// workerstatus uses const starttimeIndex = 22 - 3 = 19.
	close := strings.LastIndex(string(data), ")")
	if close < 0 {
		return ""
	}
	tail := strings.Fields(string(data)[close+1:])
	const starttimeIndex = 22 - 3 // 19
	if starttimeIndex >= len(tail) {
		return ""
	}
	v, err := strconv.ParseInt(tail[starttimeIndex], 10, 64)
	if err != nil || v < 0 {
		return ""
	}
	return fmt.Sprintf("linux-startticks:%d", v)
}

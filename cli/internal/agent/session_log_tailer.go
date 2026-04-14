package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type fileTrackState struct {
	offset int64
}

// TailSessionLogs watches the agent log directory for new/modified session log
// files and writes formatted progress to dst. It stops when ctx is cancelled.
// Callers should run this in a goroutine while ExecuteBead runs synchronously.
func TailSessionLogs(ctx context.Context, projectRoot string, dst io.Writer) {
	logDir := filepath.Join(projectRoot, DefaultLogDir)
	states := make(map[string]*fileTrackState)

	// Record existing files so we don't replay old logs
	if entries, err := os.ReadDir(logDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "agent-") && strings.HasSuffix(entry.Name(), ".jsonl") && !strings.HasPrefix(entry.Name(), "agent-loop-") {
				if info, err := entry.Info(); err == nil {
					states[filepath.Join(logDir, entry.Name())] = &fileTrackState{offset: info.Size()}
				}
			}
		}
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			readNewLogLines(logDir, states, dst)
			return
		case <-ticker.C:
			readNewLogLines(logDir, states, dst)
		}
	}
}

// readNewLogLines reads any new lines from agent session logs and formats them.
func readNewLogLines(logDir string, states map[string]*fileTrackState, dst io.Writer) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "agent-") || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		// Skip loop event files — their milestones are emitted directly via
		// opts.Log in ExecuteBeadWorker; processing them here would produce
		// duplicate output for human operators.
		if strings.HasPrefix(entry.Name(), "agent-loop-") {
			continue
		}

		path := filepath.Join(logDir, entry.Name())
		st, err := os.Stat(path)
		if err != nil {
			continue
		}

		fs, ok := states[path]
		if !ok {
			fs = &fileTrackState{}
			states[path] = fs
		}

		if st.Size() <= fs.offset {
			continue
		}

		f, err := os.Open(path)
		if err != nil {
			continue
		}

		_, _ = f.Seek(fs.offset, io.SeekStart)
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // 4MB max line
		var newLines []string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) != "" {
				newLines = append(newLines, line)
			}
		}
		fs.offset, _ = f.Seek(0, io.SeekCurrent)
		f.Close()

		if len(newLines) > 0 {
			formatted := FormatSessionLogLines(newLines)
			if formatted != "" {
				fmt.Fprint(dst, formatted)
			}
		}
	}
}

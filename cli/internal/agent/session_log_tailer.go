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

type sessionLogFile struct {
	path string
}

// TailSessionLogs watches the agent log directory for new/modified session log
// files and writes formatted progress to dst. It stops when ctx is cancelled.
// Callers should run this in a goroutine while ExecuteBead runs synchronously.
func TailSessionLogs(ctx context.Context, projectRoot string, dst io.Writer) {
	logDir := filepath.Join(projectRoot, DefaultLogDir)
	states := make(map[string]*fileTrackState)

	// Record existing files so we don't replay old logs
	for _, file := range listSessionLogFiles(projectRoot, logDir) {
		if info, err := os.Stat(file.path); err == nil {
			states[file.path] = &fileTrackState{offset: info.Size()}
		}
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			readNewLogLines(projectRoot, logDir, states, dst)
			return
		case <-ticker.C:
			readNewLogLines(projectRoot, logDir, states, dst)
		}
	}
}

// readNewLogLines reads any new lines from agent session logs and formats them.
func readNewLogLines(projectRoot, logDir string, states map[string]*fileTrackState, dst io.Writer) {
	for _, file := range listSessionLogFiles(projectRoot, logDir) {
		path := file.path
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
		// 16MB max line. Session-log `response` fields observed up to ~7MB in
		// real incidents (2026-04-20 reviewer-malfunction report). Matches bead
		// store and claude stream for consistency.
		scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
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

func listSessionLogFiles(projectRoot, logDir string) []sessionLogFile {
	var files []sessionLogFile
	appendDirLogs := func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}
			name := entry.Name()
			if strings.HasPrefix(name, "agent-loop-") {
				continue
			}
			if strings.HasPrefix(name, "agent-") || strings.HasPrefix(name, "svc-") {
				files = append(files, sessionLogFile{path: filepath.Join(dir, name)})
			}
		}
	}

	appendDirLogs(logDir)
	embeddedDirs, _ := filepath.Glob(filepath.Join(projectRoot, ".ddx", "executions", "*", "embedded"))
	for _, dir := range embeddedDirs {
		appendDirLogs(dir)
	}
	return files
}

package jsonl

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testRow struct {
	Writer  int    `json:"writer"`
	Seq     int    `json:"seq"`
	Payload string `json:"payload"`
}

func TestLockedJSONLAppend_CrossProcessWritersDoNotInterleaveRows(t *testing.T) {
	if os.Getenv("JSONL_APPEND_HELPER") == "1" {
		runAppendHelper(t)
		return
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "stream.jsonl")
	startFile := filepath.Join(dir, "start")
	readyDir := filepath.Join(dir, "ready")
	require.NoError(t, os.MkdirAll(readyDir, 0o755))

	const workers = 6
	const rowsPerWorker = 40

	cmds := make([]*exec.Cmd, 0, workers)
	for i := 0; i < workers; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=TestLockedJSONLAppend_CrossProcessWritersDoNotInterleaveRows", "-test.count=1")
		cmd.Env = append(os.Environ(),
			"JSONL_APPEND_HELPER=1",
			"JSONL_APPEND_PATH="+path,
			"JSONL_APPEND_START_FILE="+startFile,
			"JSONL_APPEND_READY_DIR="+readyDir,
			"JSONL_APPEND_WRITER="+strconv.Itoa(i),
			"JSONL_APPEND_ROWS="+strconv.Itoa(rowsPerWorker),
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmds = append(cmds, cmd)
	}

	for _, cmd := range cmds {
		require.NoError(t, cmd.Start())
	}

	require.NoError(t, waitForReadyFiles(readyDir, workers, 5*time.Second))
	require.NoError(t, os.WriteFile(startFile, []byte("go"), 0o644))

	for _, cmd := range cmds {
		require.NoError(t, cmd.Wait())
	}

	lines := readLines(t, path)
	require.Len(t, lines, workers*rowsPerWorker)

	seen := make(map[string]int, len(lines))
	for _, line := range lines {
		var row testRow
		require.NoError(t, json.Unmarshal([]byte(line), &row))
		key := fmt.Sprintf("%d:%d", row.Writer, row.Seq)
		seen[key]++
		assert.True(t, strings.HasPrefix(row.Payload, "payload-"))
	}
	for writer := 0; writer < workers; writer++ {
		for seq := 0; seq < rowsPerWorker; seq++ {
			assert.Equal(t, 1, seen[fmt.Sprintf("%d:%d", writer, seq)], "missing or duplicated row %d:%d", writer, seq)
		}
	}
}

func TestLockedJSONLAppend_TimeoutReturnsTypedError(t *testing.T) {
	if os.Getenv("JSONL_LOCK_HOLDER") == "1" {
		runLockHolder(t)
		return
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "stream.jsonl")
	readyFile := filepath.Join(dir, "held")

	cmd := exec.Command(os.Args[0], "-test.run=TestLockedJSONLAppend_TimeoutReturnsTypedError", "-test.count=1")
	cmd.Env = append(os.Environ(),
		"JSONL_LOCK_HOLDER=1",
		"JSONL_LOCK_PATH="+path,
		"JSONL_LOCK_READY_FILE="+readyFile,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	defer func() { _ = cmd.Process.Kill() }()

	require.NoError(t, waitForFile(readyFile, 5*time.Second))

	err := AppendJSONL(context.Background(), path, testRow{Writer: 1, Seq: 1, Payload: "payload"}, WithLockTimeout(50*time.Millisecond))
	var timeoutErr *TimeoutError
	require.Error(t, err)
	require.True(t, errors.As(err, &timeoutErr), "append must return a typed timeout error")
	assert.Equal(t, path, timeoutErr.Path)
	assert.Equal(t, path+".lock", timeoutErr.LockPath)
	assert.Equal(t, 50*time.Millisecond, timeoutErr.Timeout)

	require.NoError(t, cmd.Wait())
}

func runAppendHelper(t *testing.T) {
	t.Helper()
	path := os.Getenv("JSONL_APPEND_PATH")
	startFile := os.Getenv("JSONL_APPEND_START_FILE")
	readyDir := os.Getenv("JSONL_APPEND_READY_DIR")
	writer, err := strconv.Atoi(os.Getenv("JSONL_APPEND_WRITER"))
	require.NoError(t, err)
	rows, err := strconv.Atoi(os.Getenv("JSONL_APPEND_ROWS"))
	require.NoError(t, err)

	readyFile := filepath.Join(readyDir, fmt.Sprintf("writer-%d.ready", writer))
	require.NoError(t, os.WriteFile(readyFile, []byte("ready"), 0o644))
	require.NoError(t, waitForFile(startFile, 5*time.Second))

	for i := 0; i < rows; i++ {
		row := testRow{
			Writer:  writer,
			Seq:     i,
			Payload: strings.Repeat(fmt.Sprintf("payload-%d-", writer), 8),
		}
		require.NoError(t, AppendJSONL(context.Background(), path, row))
	}
}

func runLockHolder(t *testing.T) {
	t.Helper()
	path := os.Getenv("JSONL_LOCK_PATH")
	readyFile := os.Getenv("JSONL_LOCK_READY_FILE")

	lockPath := path + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	require.NoError(t, err)
	defer lockFile.Close() //nolint:errcheck

	require.NoError(t, tryLockFile(lockFile))
	require.NoError(t, os.WriteFile(readyFile, []byte("held"), 0o644))
	time.Sleep(2 * time.Second)
}

func waitForReadyFiles(dir string, want int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		count := 0
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".ready") {
				count++
			}
		}
		if count >= want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %d ready files in %s", want, dir)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	require.NoError(t, sc.Err())
	return lines
}

package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
)

// ForEachJSONL opens path and calls fn for each valid JSON record.
// Malformed lines are skipped. Missing files are treated as empty.
// Handles lines of any length.
func ForEachJSONL[T any](path string, fn func(T) error) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	return scanJSONLReader[T](f, fn)
}

// ReadAllJSONL loads all records from a JSONL file into a slice.
func ReadAllJSONL[T any](path string) ([]T, error) {
	var records []T
	err := ForEachJSONL[T](path, func(rec T) error {
		records = append(records, rec)
		return nil
	})
	return records, err
}

// scanJSONLReader reads newline-delimited JSON records from r, calling fn for each valid record.
// It uses bufio.Reader.ReadBytes so there is no line-length limit.
func scanJSONLReader[T any](r io.Reader, fn func(T) error) error {
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadBytes('\n')
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			var rec T
			if jsonErr := json.Unmarshal(line, &rec); jsonErr == nil {
				if fnErr := fn(rec); fnErr != nil {
					return fnErr
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

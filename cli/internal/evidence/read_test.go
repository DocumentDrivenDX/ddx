package evidence

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadFileClampedUnderCap(t *testing.T) {
	p := writeTemp(t, "f.txt", "hello world")
	content, truncated, orig, err := ReadFileClamped(p, 100)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Errorf("unexpectedly truncated")
	}
	if string(content) != "hello world" {
		t.Errorf("got %q", content)
	}
	if orig != 11 {
		t.Errorf("orig = %d", orig)
	}
}

func TestReadFileClampedOverCap(t *testing.T) {
	body := strings.Repeat("x", 5000)
	p := writeTemp(t, "f.txt", body)
	content, truncated, orig, err := ReadFileClamped(p, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("expected truncated")
	}
	if len(content) != 100 {
		t.Errorf("len(content) = %d, want 100", len(content))
	}
	if orig != 5000 {
		t.Errorf("orig = %d, want 5000", orig)
	}
}

func TestReadFileHardFailUnderCap(t *testing.T) {
	p := writeTemp(t, "f.txt", "ok")
	got, err := ReadFileHardFail(p, 100, "test")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ok" {
		t.Errorf("got %q", got)
	}
}

func TestReadFileHardFailOverCap(t *testing.T) {
	body := strings.Repeat("x", 5000)
	p := writeTemp(t, "f.txt", body)
	_, err := ReadFileHardFail(p, 100, "user-prompt")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrOversize) {
		t.Errorf("error does not wrap ErrOversize: %v", err)
	}
	var oe *OversizeError
	if !errors.As(err, &oe) {
		t.Fatalf("expected *OversizeError, got %T", err)
	}
	if oe.CapBytes != 100 || oe.ObservedBytes != 5000 || oe.Source != "user-prompt" {
		t.Errorf("OversizeError fields wrong: %+v", oe)
	}
}

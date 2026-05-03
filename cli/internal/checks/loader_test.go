package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDir_MissingDirReturnsNil(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	checks, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checks != nil {
		t.Fatalf("expected nil, got %v", checks)
	}
}

func TestLoadDir_Valid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.yaml"), `
name: alpha
command: "true"
when: pre_merge
applies_to:
  paths: ["**/*.go"]
`)
	writeFile(t, filepath.Join(dir, "b.yml"), `
name: beta
command: "echo hi"
when: pre_merge
applies_to:
  labels: [area:foo]
`)
	writeFile(t, filepath.Join(dir, "ignore.txt"), "not yaml")

	checks, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(checks) != 2 {
		t.Fatalf("want 2 checks, got %d", len(checks))
	}
	if checks[0].Name != "alpha" || checks[1].Name != "beta" {
		t.Fatalf("unexpected order/names: %+v", checks)
	}
	if checks[0].SourceFile == "" {
		t.Fatalf("expected SourceFile to be populated")
	}
}

func TestLoadFile_BadYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.yaml")
	writeFile(t, p, "name: oops\n  command: : :\n")
	_, err := LoadFile(p)
	if err == nil {
		t.Fatalf("expected parse error")
	}
	if !strings.Contains(err.Error(), p) {
		t.Fatalf("error must include source path %q: %v", p, err)
	}
	if !strings.Contains(err.Error(), "line") {
		t.Fatalf("error must include line info: %v", err)
	}
}

func TestLoadFile_UnknownField(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "unknown.yaml")
	writeFile(t, p, `
name: x
command: "true"
when: pre_merge
extra_unknown: 1
`)
	_, err := LoadFile(p)
	if err == nil {
		t.Fatalf("expected unknown-field error")
	}
	if !strings.Contains(err.Error(), p) || !strings.Contains(err.Error(), "extra_unknown") {
		t.Fatalf("error should mention path and unknown field: %v", err)
	}
}

func TestLoadFile_MissingRequired(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"no-name":    "command: x\nwhen: pre_merge\n",
		"no-command": "name: x\nwhen: pre_merge\n",
		"no-when":    "name: x\ncommand: y\n",
		"bad-when":   "name: x\ncommand: y\nwhen: post_close\n",
	}
	for label, body := range cases {
		t.Run(label, func(t *testing.T) {
			p := filepath.Join(dir, label+".yaml")
			writeFile(t, p, body)
			if _, err := LoadFile(p); err == nil {
				t.Fatalf("%s: expected error", label)
			}
		})
	}
}

func TestLoadDir_DuplicateNames(t *testing.T) {
	dir := t.TempDir()
	body := "name: dup\ncommand: 'true'\nwhen: pre_merge\n"
	writeFile(t, filepath.Join(dir, "1.yaml"), body)
	writeFile(t, filepath.Join(dir, "2.yaml"), body)
	_, err := LoadDir(dir)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate-name error, got %v", err)
	}
}

func TestApplies(t *testing.T) {
	cases := []struct {
		name string
		c    Check
		ctx  InvocationContext
		want bool
	}{
		{
			name: "no filter applies always",
			c:    Check{Name: "x"},
			ctx:  InvocationContext{},
			want: true,
		},
		{
			name: "label match",
			c:    Check{AppliesTo: AppliesTo{Labels: []string{"area:foo"}}},
			ctx:  InvocationContext{BeadLabels: []string{"area:foo", "kind:platform"}},
			want: true,
		},
		{
			name: "label no match",
			c:    Check{AppliesTo: AppliesTo{Labels: []string{"area:foo"}}},
			ctx:  InvocationContext{BeadLabels: []string{"area:bar"}},
			want: false,
		},
		{
			name: "path glob basename match",
			c:    Check{AppliesTo: AppliesTo{Paths: []string{"*.go"}}},
			ctx:  InvocationContext{ChangedPaths: []string{"cli/cmd/foo.go"}},
			want: true,
		},
		{
			name: "path glob no match",
			c:    Check{AppliesTo: AppliesTo{Paths: []string{"*.py"}}},
			ctx:  InvocationContext{ChangedPaths: []string{"cli/cmd/foo.go"}},
			want: false,
		},
		{
			name: "OR semantics — label matches even if paths don't",
			c: Check{AppliesTo: AppliesTo{
				Paths:  []string{"*.py"},
				Labels: []string{"area:foo"},
			}},
			ctx:  InvocationContext{ChangedPaths: []string{"a.go"}, BeadLabels: []string{"area:foo"}},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.c.Applies(tc.ctx)
			if got != tc.want {
				t.Fatalf("Applies = %v, want %v", got, tc.want)
			}
		})
	}
}

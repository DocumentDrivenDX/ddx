package checks

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadDir reads all *.yaml and *.yml files in dir as Check definitions.
// If dir does not exist, returns (nil, nil) — projects without a checks
// directory have no checks. Any unreadable file, malformed YAML, or
// unknown field is reported with the source file and (when available)
// line:column.
func LoadDir(dir string) ([]Check, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("checks: read dir %s: %w", dir, err)
	}

	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	sort.Strings(paths)

	out := make([]Check, 0, len(paths))
	seen := make(map[string]string, len(paths))
	for _, p := range paths {
		c, err := LoadFile(p)
		if err != nil {
			return nil, err
		}
		if prior, dup := seen[c.Name]; dup {
			return nil, fmt.Errorf("checks: duplicate check name %q in %s (also defined in %s)", c.Name, p, prior)
		}
		seen[c.Name] = p
		out = append(out, c)
	}
	return out, nil
}

// LoadFile parses a single check definition file. Unknown fields and
// malformed YAML are reported with file:line context.
func LoadFile(path string) (Check, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Check{}, fmt.Errorf("checks: read %s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var c Check
	if err := dec.Decode(&c); err != nil {
		return Check{}, fmt.Errorf("checks: parse %s: %w", path, err)
	}
	c.SourceFile = path
	if err := c.Validate(); err != nil {
		return Check{}, fmt.Errorf("checks: %s: %w", path, err)
	}
	return c, nil
}

// Validate enforces required-field rules on a parsed check.
func (c *Check) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("missing required field: name")
	}
	if c.Command == "" {
		return fmt.Errorf("missing required field: command")
	}
	if c.When == "" {
		return fmt.Errorf("missing required field: when")
	}
	if c.When != HookPreMerge {
		return fmt.Errorf("unsupported hook %q (only %q is supported)", c.When, HookPreMerge)
	}
	return nil
}

// Applies reports whether the check should run for the given context.
func (c *Check) Applies(ctx InvocationContext) bool {
	a := c.AppliesTo
	if len(a.Paths) == 0 && len(a.Labels) == 0 {
		return true
	}
	for _, lbl := range a.Labels {
		for _, b := range ctx.BeadLabels {
			if lbl == b {
				return true
			}
		}
	}
	for _, glob := range a.Paths {
		for _, p := range ctx.ChangedPaths {
			if matchPathGlob(glob, p) {
				return true
			}
		}
	}
	return false
}

func matchPathGlob(pattern, name string) bool {
	pattern = filepath.ToSlash(pattern)
	name = filepath.ToSlash(name)
	if matched, _ := path.Match(pattern, name); matched {
		return true
	}
	if matched, _ := path.Match(pattern, path.Base(name)); matched {
		return true
	}
	if !strings.Contains(pattern, "**") {
		return false
	}
	return matchDoubleStar(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchDoubleStar(patternParts, nameParts []string) bool {
	if len(patternParts) == 0 {
		return len(nameParts) == 0
	}
	if patternParts[0] == "**" {
		for i := 0; i <= len(nameParts); i++ {
			if matchDoubleStar(patternParts[1:], nameParts[i:]) {
				return true
			}
		}
		return false
	}
	if len(nameParts) == 0 {
		return false
	}
	matched, err := path.Match(patternParts[0], nameParts[0])
	if err != nil || !matched {
		return false
	}
	return matchDoubleStar(patternParts[1:], nameParts[1:])
}

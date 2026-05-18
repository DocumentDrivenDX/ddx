package cmd

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveProjectRootPrefersDDXProjectRootEnv(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "project")
	t.Setenv("DDX_PROJECT_ROOT", projectRoot)

	got := resolveProjectRoot("", filepath.Join(t.TempDir(), "runtime"))
	assert.Equal(t, projectRoot, got)
}

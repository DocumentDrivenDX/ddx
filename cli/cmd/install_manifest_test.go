package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallLocalRejectsUnsupportedAPIVersion(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	localPlugin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, "package.yaml"), []byte(`name: sample-plugin
version: 1.0.0
description: Sample plugin
type: plugin
source: https://example.com/sample-plugin
api_version: 2
`), 0o644))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "sample-plugin", "--local", localPlugin)
	require.Error(t, err)
	assert.True(t, strings.Contains(output, "validating package manifest") || strings.Contains(err.Error(), "api_version"))
}

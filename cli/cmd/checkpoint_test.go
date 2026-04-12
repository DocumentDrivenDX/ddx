package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initGitRepo creates a minimal git repo with an initial commit in dir.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		require.NoError(t, c.Run(), "git setup %v", args)
	}
	// create initial commit
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("init\n"), 0644))
	for _, args := range [][]string{
		{"git", "add", "README.md"},
		{"git", "commit", "-m", "initial"},
	} {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		require.NoError(t, c.Run(), "git %v", args)
	}
}

func runCheckpointCmd(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	f := NewCommandFactory(dir)
	root := f.NewRootCommand()

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	fullArgs := append([]string{"checkpoint"}, args...)
	root.SetArgs(fullArgs)

	err := root.Execute()
	return buf.String(), err
}

func TestCheckpointCreate(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	// change to dir so git commands run there
	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(orig) }()

	out, err := runCheckpointCmd(t, dir, "pre-build")
	require.NoError(t, err)
	assert.Contains(t, out, "checkpoint created: ddx/pre-build")

	// verify tag exists
	c := exec.Command("git", "tag", "-l", "ddx/pre-build")
	c.Dir = dir
	tagOut, _ := c.Output()
	assert.Equal(t, "ddx/pre-build\n", string(tagOut))
}

func TestCheckpointList(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(orig) }()

	// Create a couple of tags manually
	for _, tag := range []string{"ddx/alpha", "ddx/beta"} {
		c := exec.Command("git", "tag", tag)
		c.Dir = dir
		require.NoError(t, c.Run())
	}

	out, err := runCheckpointCmd(t, dir, "--list")
	require.NoError(t, err)
	assert.Contains(t, out, "ddx/alpha")
	assert.Contains(t, out, "ddx/beta")
}

func TestCheckpointListEmpty(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(orig) }()

	out, err := runCheckpointCmd(t, dir, "--list")
	require.NoError(t, err)
	assert.Contains(t, out, "No checkpoints found.")
}

func TestCheckpointInvalidName(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(orig) }()

	_, err := runCheckpointCmd(t, dir, "bad name!")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid characters"), err.Error())
}

func TestCheckpointNoArgs(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(orig) }()

	_, err := runCheckpointCmd(t, dir)
	require.Error(t, err)
}

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDDXVersionCommit(t *testing.T) {
	output := "DDx v0.6.2-alpha48\nCommit: 8274fc381\nBuilt: 2026-05-12T18:37:34Z\n"

	assert.Equal(t, "8274fc381", parseDDXVersionCommit(output))
}

func TestShouldRefreshDDXBinary(t *testing.T) {
	assert.True(t, shouldRefreshDDXBinary("528f77644", "8274fc381"))
	assert.False(t, shouldRefreshDDXBinary("8274fc381", "8274fc381"))
	assert.False(t, shouldRefreshDDXBinary("8274fc381abcdef", "8274fc381"))
	assert.False(t, shouldRefreshDDXBinary("dev", "8274fc381"))
	assert.False(t, shouldRefreshDDXBinary("8274fc381", "unknown"))
}

func TestWorkBinaryRefreshEnabledOnlyForInstalledDDX(t *testing.T) {
	assert.True(t, workBinaryRefreshEnabled([]string{"/home/erik/.local/bin/ddx", "work"}))
	assert.False(t, workBinaryRefreshEnabled([]string{"/tmp/go-build123/cmd.test", "-test.run=TestWork"}))
	assert.False(t, workBinaryRefreshEnabled(nil))
}

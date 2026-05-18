package service

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServerRuntimeDirUsesXDGStateHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)

	assert.Equal(t, filepath.Join(xdg, "ddx", "server"), ServerRuntimeDir())
}

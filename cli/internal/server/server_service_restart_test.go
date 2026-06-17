//go:build !windows

package server

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServerServiceRestart_RewritesServerAddrBeforeStatus(t *testing.T) {
	root := setupTestDir(t)
	addr := "127.0.0.1:18443"
	stalePID := 999999
	if processAlive(stalePID) {
		t.Skipf("pid %d is alive on this host; cannot use it as stale test data", stalePID)
	}

	addrDir := serverAddrDir()
	require.NoError(t, os.MkdirAll(addrDir, 0o755))
	staleAddrPath := filepath.Join(addrDir, "server.addr")
	staleAddr := map[string]any{
		"url": "https://127.0.0.1:1",
		"pid": stalePID,
	}
	staleData, err := json.Marshal(staleAddr)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(staleAddrPath, staleData, 0o600))

	srv := New(addr, root)

	certEntered := make(chan struct{})
	releaseCert := make(chan struct{})
	prevHook := ensureSelfSignedCertHook
	ensureSelfSignedCertHook = func(dir string) (string, string, error) {
		close(certEntered)
		<-releaseCert
		return "", "", errors.New("blocked certificate generation")
	}
	t.Cleanup(func() { ensureSelfSignedCertHook = prevHook })

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServeTLS("", "")
	}()

	select {
	case <-certEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("ListenAndServeTLS did not reach certificate generation")
	}

	data, err := os.ReadFile(staleAddrPath)
	require.NoError(t, err, "server.addr must be rewritten before certificate generation blocks startup")
	var fresh struct {
		URL string `json:"url"`
		PID int    `json:"pid"`
	}
	require.NoError(t, json.Unmarshal(data, &fresh))
	require.Equal(t, "https://"+addr, fresh.URL)
	require.Equal(t, os.Getpid(), fresh.PID)

	close(releaseCert)
	require.Error(t, <-errCh)
}

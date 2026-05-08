// Package serverreg provides fire-and-forget project registration with the
// ddx-server. CLI commands call TryRegisterAsync so the server always has an
// up-to-date project list. If no server is reachable the call is silently
// discarded — the CLI never depends on the server being available.
package serverreg

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const defaultServerURL = "https://localhost:7743"

// TryRegisterAsync fires off a project registration in a background goroutine
// and returns immediately. Errors are silently discarded.
func TryRegisterAsync(projectPath string) {
	url := resolveServerURL()
	if url == "" {
		return
	}
	go register(url, projectPath)
}

func register(serverURL, projectPath string) {
	body, err := json.Marshal(map[string]string{"path": projectPath})
	if err != nil {
		return
	}

	client := &http.Client{
		Timeout: 500 * time.Millisecond,
		// Accept self-signed certs — the server uses an auto-generated cert.
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}

	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/projects/register", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// resolveServerURL returns, in order of preference:
//  1. DDX_SERVER_URL environment variable
//  2. URL from ~/.local/share/ddx/server.addr
//  3. https://localhost:7743 (default)
func resolveServerURL() string {
	if u := os.Getenv("DDX_SERVER_URL"); u != "" {
		return u
	}
	if u := readAddrFile(); u != "" {
		return u
	}
	return defaultServerURL
}

func readAddrFile() string {
	type addrFile struct {
		URL string `json:"url"`
	}
	dir := addrDir()
	data, err := os.ReadFile(filepath.Join(dir, "server.addr"))
	if err != nil {
		return ""
	}
	var af addrFile
	if err := json.Unmarshal(data, &af); err != nil {
		return ""
	}
	return af.URL
}

func addrDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "ddx")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", "ddx")
	}
	return filepath.Join(home, ".local", "share", "ddx")
}

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
	"regexp"
	"strings"
	"time"
)

const defaultServerURL = "https://localhost:7743"

var goTestSegmentRegexp = regexp.MustCompile(`/Test[A-Z][^/]*\d+/`)

// TryRegisterAsync fires off a project registration in a background goroutine
// and returns immediately. Errors are silently discarded.
func TryRegisterAsync(projectPath string) {
	if isTransientProjectPath(projectPath) {
		return
	}
	url := resolveServerURL()
	if url == "" {
		return
	}
	go register(url, projectPath)
}

func isTransientProjectPath(path string) bool {
	if path == "" {
		return false
	}
	if hasPathPrefix(path, "/tmp") ||
		hasPathPrefix(path, "/private/tmp") ||
		hasPathPrefix(path, "/var/tmp") ||
		hasPathPrefix(path, "/var/folders") ||
		hasPathPrefix(path, os.TempDir()) {
		return true
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if hasPathPrefix(path, filepath.Join(home, "tmp")) ||
			hasPathPrefix(path, filepath.Join(home, ".cache", "fleet-tmp")) ||
			filepath.Clean(path) == filepath.Join(home, "Projects") {
			return true
		}
	}
	if strings.Contains(path, "/.cache/fleet-tmp/") ||
		strings.Contains(path, "/ddx-cmd-tests-") ||
		strings.Contains(path, "/.ddx-exec-wt/") ||
		strings.Contains(path, "/.ddx-external-workers/") ||
		strings.Contains(path, "/.claude/worktrees/") ||
		strings.Contains(path, "/.agents/worktrees/") ||
		(strings.Contains(path, "/runs/") && filepath.Base(path) == "workspace") ||
		strings.HasPrefix(filepath.Base(path), ".execute-bead-wt-") {
		return true
	}
	probe := path
	if !strings.HasPrefix(probe, "/") {
		probe = "/" + probe
	}
	if !strings.HasSuffix(probe, "/") {
		probe += "/"
	}
	return goTestSegmentRegexp.MatchString(probe)
}

func hasPathPrefix(path, prefix string) bool {
	if path == "" || prefix == "" {
		return false
	}
	cleanPath := filepath.Clean(path)
	cleanPrefix := filepath.Clean(prefix)
	if cleanPath == cleanPrefix {
		return true
	}
	return strings.HasPrefix(cleanPath, cleanPrefix+string(filepath.Separator))
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

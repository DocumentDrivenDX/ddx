package ddxroot

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
)

// Path returns the DDx state root for projectRoot.
//
// When the project already has an in-tree `.ddx/`, that remains authoritative.
// Otherwise DDx falls back to an XDG-scoped convention root derived from the
// project's canonical git remote, or a deterministic local identity.
func Path(ctx context.Context, projectRoot string) string {
	inTree := filepath.Join(projectRoot, ".ddx")
	if info, err := os.Stat(inTree); err == nil && info.IsDir() {
		return inTree
	}
	return filepath.Join(projectsRoot(), projectIdentity(ctx, projectRoot))
}

func projectsRoot() string {
	return filepath.Join(xdgDataHome(), "ddx", "projects")
}

func xdgDataHome() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return xdg
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "ddx-xdg")
	}
	return filepath.Join(home, ".local", "share")
}

func projectIdentity(ctx context.Context, projectRoot string) string {
	return projectIdentityFromRemote(projectRoot, resolveOriginURL(ctx, projectRoot))
}

func projectIdentityFromRemote(projectRoot, remoteURL string) string {
	if remoteProject, ok := parseRemoteProject(remoteURL); ok {
		return remoteProject
	}
	return localProjectIdentity(projectRoot)
}

func resolveOriginURL(ctx context.Context, projectRoot string) string {
	if ctx == nil {
		ctx = context.Background()
	}
	out, err := gitpkg.Command(ctx, projectRoot, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func parseRemoteProject(remoteURL string) (string, bool) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return "", false
	}
	if host, repoPath, ok := parseSCPLikeRemote(remoteURL); ok {
		return filepath.Join(host, repoPath), true
	}
	u, err := url.Parse(remoteURL)
	if err != nil || u.Host == "" {
		return "", false
	}
	repoPath := normalizeRepoPath(u.Path)
	if repoPath == "" {
		return "", false
	}
	return filepath.Join(u.Host, repoPath), true
}

func parseSCPLikeRemote(remoteURL string) (host, repoPath string, ok bool) {
	if strings.Contains(remoteURL, "://") {
		return "", "", false
	}
	at := strings.IndexByte(remoteURL, '@')
	colon := strings.IndexByte(remoteURL, ':')
	if at <= 0 || colon <= at+1 {
		return "", "", false
	}
	host = strings.TrimSpace(remoteURL[at+1 : colon])
	repoPath = normalizeRepoPath(remoteURL[colon+1:])
	if host == "" || repoPath == "" {
		return "", "", false
	}
	return host, repoPath, true
}

func normalizeRepoPath(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "/")
	raw = strings.TrimSuffix(raw, ".git")
	if raw == "" {
		return ""
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	if len(parts) == 0 {
		return ""
	}
	return filepath.Join(parts...)
}

func localProjectIdentity(projectRoot string) string {
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		absRoot = filepath.Clean(projectRoot)
	}
	base := filepath.Base(absRoot)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "project"
	}
	return filepath.Join("local", base+"-"+shortPathHash(absRoot))
}

func shortPathHash(path string) string {
	sum := sha1.Sum([]byte(path))
	return hex.EncodeToString(sum[:])[:8]
}

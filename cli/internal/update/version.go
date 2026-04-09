package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

const (
	githubAPIURL = "https://api.github.com/repos/DocumentDrivenDX/ddx/releases/latest"
)

// FetchLatestRelease fetches the latest release information from GitHub
func FetchLatestRelease() (*GitHubRelease, error) {
	return fetchLatestRelease(githubAPIURL)
}

// FetchLatestReleaseForRepo fetches the latest release for a GitHub repo URL
// e.g. "https://github.com/DocumentDrivenDX/helix"
func FetchLatestReleaseForRepo(repoURL string) (*GitHubRelease, error) {
	// Convert https://github.com/owner/repo → https://api.github.com/repos/owner/repo/releases/latest
	repoURL = strings.TrimRight(repoURL, "/")
	const githubBase = "https://github.com/"
	if !strings.HasPrefix(repoURL, githubBase) {
		return nil, fmt.Errorf("unsupported repo URL: %s", repoURL)
	}
	path := strings.TrimPrefix(repoURL, githubBase)
	apiURL := "https://api.github.com/repos/" + path + "/releases/latest"
	return fetchLatestRelease(apiURL)
}

func fetchLatestRelease(url string) (*GitHubRelease, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("checking for DDx updates: failed to fetch latest release from %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		details := strings.TrimSpace(string(body))
		if details != "" {
			return nil, fmt.Errorf(
				"checking for DDx updates: fetching latest release from %s failed: GitHub API returned %s: %s",
				url,
				resp.Status,
				details,
			)
		}
		return nil, fmt.Errorf(
			"checking for DDx updates: fetching latest release from %s failed: GitHub API returned %s",
			url,
			resp.Status,
		)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("checking for DDx updates: failed to parse release info from %s: %w", url, err)
	}

	return &release, nil
}

// NeedsUpgrade compares two version strings and returns true if an upgrade is needed
func NeedsUpgrade(current, latest string) (bool, error) {
	// Normalize versions (remove 'v' prefix)
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	// Dev builds bypass update checks
	if strings.Contains(current, "dev") {
		return false, nil
	}

	// Parse semantic versions
	currentParts, err := ParseVersion(current)
	if err != nil {
		return false, err
	}

	latestParts, err := ParseVersion(latest)
	if err != nil {
		return false, err
	}

	// Compare major.minor.patch
	for i := 0; i < 3; i++ {
		if latestParts[i] > currentParts[i] {
			return true, nil
		}
		if latestParts[i] < currentParts[i] {
			return false, nil
		}
	}

	// Versions are equal
	return false, nil
}

// ParseVersion parses a semantic version string into [major, minor, patch]
func ParseVersion(version string) ([3]int, error) {
	var parts [3]int

	// Remove any suffixes like -dev, -beta, etc.
	version = regexp.MustCompile(`[+-].*`).ReplaceAllString(version, "")

	// Split by dots
	components := strings.Split(version, ".")
	if len(components) < 1 || len(components) > 3 {
		return parts, fmt.Errorf("invalid version format: %s", version)
	}

	// Parse each component
	for i := 0; i < len(components) && i < 3; i++ {
		num, err := strconv.Atoi(components[i])
		if err != nil {
			return parts, fmt.Errorf("invalid version number: %s", components[i])
		}
		parts[i] = num
	}

	return parts, nil
}

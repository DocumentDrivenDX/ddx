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

	// Parse semantic versions including prerelease precedence.
	currentVersion, err := parseDetailedVersion(current)
	if err != nil {
		return false, err
	}

	latestVersion, err := parseDetailedVersion(latest)
	if err != nil {
		return false, err
	}

	return compareDetailedVersions(currentVersion, latestVersion) < 0, nil
}

// ParseVersion parses a semantic version string into [major, minor, patch]
func ParseVersion(version string) ([3]int, error) {
	var parts [3]int

	detailed, err := parseDetailedVersion(version)
	if err != nil {
		return parts, err
	}
	return detailed.Core, nil
}

type parsedVersion struct {
	Core       [3]int
	PreRelease []versionIdentifier
}

type versionIdentifier struct {
	Kind  string
	Text  string
	Value int
}

func parseDetailedVersion(version string) (parsedVersion, error) {
	var parsed parsedVersion

	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	version = strings.SplitN(version, "+", 2)[0]

	mainVersion := version
	prerelease := ""
	if idx := strings.Index(mainVersion, "-"); idx >= 0 {
		prerelease = mainVersion[idx+1:]
		mainVersion = mainVersion[:idx]
	}

	components := strings.Split(mainVersion, ".")
	if len(components) < 1 || len(components) > 3 {
		return parsed, fmt.Errorf("invalid version format: %s", version)
	}

	// Parse each component
	for i := 0; i < len(components) && i < 3; i++ {
		num, err := strconv.Atoi(components[i])
		if err != nil {
			return parsed, fmt.Errorf("invalid version number: %s", components[i])
		}
		parsed.Core[i] = num
	}

	parsed.PreRelease = parsePrerelease(prerelease)
	return parsed, nil
}

func parsePrerelease(prerelease string) []versionIdentifier {
	if prerelease == "" {
		return nil
	}

	rawSegments := strings.FieldsFunc(strings.ToLower(prerelease), func(r rune) bool {
		return r == '.' || r == '-'
	})
	if len(rawSegments) == 0 {
		return nil
	}

	tokens := make([]versionIdentifier, 0, len(rawSegments))
	re := regexp.MustCompile(`^([a-z]+)(\d+)$`)
	for _, segment := range rawSegments {
		if segment == "" {
			continue
		}
		if n, err := strconv.Atoi(segment); err == nil {
			tokens = append(tokens, versionIdentifier{Kind: "num", Value: n})
			continue
		}
		if matches := re.FindStringSubmatch(segment); matches != nil {
			tokens = append(tokens, versionIdentifier{Kind: "str", Text: matches[1]})
			n, _ := strconv.Atoi(matches[2])
			tokens = append(tokens, versionIdentifier{Kind: "num", Value: n})
			continue
		}
		tokens = append(tokens, versionIdentifier{Kind: "str", Text: segment})
	}
	return tokens
}

func compareDetailedVersions(current, latest parsedVersion) int {
	for i := 0; i < 3; i++ {
		if current.Core[i] < latest.Core[i] {
			return -1
		}
		if current.Core[i] > latest.Core[i] {
			return 1
		}
	}

	switch {
	case len(current.PreRelease) == 0 && len(latest.PreRelease) == 0:
		return 0
	case len(current.PreRelease) == 0:
		return 1
	case len(latest.PreRelease) == 0:
		return -1
	}

	for i := 0; i < len(current.PreRelease) && i < len(latest.PreRelease); i++ {
		if cmp := compareVersionIdentifier(current.PreRelease[i], latest.PreRelease[i]); cmp != 0 {
			return cmp
		}
	}

	switch {
	case len(current.PreRelease) < len(latest.PreRelease):
		return -1
	case len(current.PreRelease) > len(latest.PreRelease):
		return 1
	default:
		return 0
	}
}

func compareVersionIdentifier(current, latest versionIdentifier) int {
	if current.Kind == "num" && latest.Kind == "num" {
		switch {
		case current.Value < latest.Value:
			return -1
		case current.Value > latest.Value:
			return 1
		default:
			return 0
		}
	}
	if current.Kind == "num" {
		return -1
	}
	if latest.Kind == "num" {
		return 1
	}

	currentRank := prereleaseRank(current.Text)
	latestRank := prereleaseRank(latest.Text)
	if currentRank != latestRank {
		switch {
		case currentRank < latestRank:
			return -1
		case currentRank > latestRank:
			return 1
		}
	}
	return strings.Compare(current.Text, latest.Text)
}

func prereleaseRank(id string) int {
	switch strings.ToLower(id) {
	case "alpha":
		return 10
	case "beta":
		return 20
	case "rc":
		return 30
	default:
		return 100
	}
}

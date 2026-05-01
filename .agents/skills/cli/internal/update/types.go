package update

import "time"

// CacheData represents cached update information
type CacheData struct {
	LastCheck       time.Time `json:"last_check"`
	CurrentVersion  string    `json:"current_version"`
	LatestVersion   string    `json:"latest_version"`
	UpdateAvailable bool      `json:"update_available"`
	CheckError      string    `json:"check_error,omitempty"`
}

// UpdateCheckResult represents the result of an update check
type UpdateCheckResult struct {
	UpdateAvailable bool
	LatestVersion   string
	Error           error
}

// GitHubRelease represents a GitHub release (will be moved from upgrade.go)
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
}

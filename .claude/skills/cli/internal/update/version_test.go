package update

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchLatestRelease_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/DocumentDrivenDX/ddx/releases/latest", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","name":"v1.2.3","body":"notes","html_url":"https://example.com"}`))
	}))
	t.Cleanup(server.Close)

	release, err := fetchLatestRelease(server.URL + "/repos/DocumentDrivenDX/ddx/releases/latest")
	require.NoError(t, err)
	require.NotNil(t, release)
	assert.Equal(t, "v1.2.3", release.TagName)
	assert.Equal(t, "v1.2.3", release.Name)
}

func TestFetchLatestRelease_StatusErrorIncludesContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	t.Cleanup(server.Close)

	release, err := fetchLatestRelease(server.URL + "/repos/DocumentDrivenDX/ddx/releases/latest")
	require.Error(t, err)
	assert.Nil(t, release)
	msg := err.Error()
	assert.Contains(t, msg, "checking for DDx updates")
	assert.Contains(t, msg, "fetching latest release from")
	assert.Contains(t, msg, "403 Forbidden")
	assert.Contains(t, msg, "API rate limit exceeded")
	assert.True(t, strings.Contains(msg, server.URL))
}

func TestNeedsUpgradeStableAndPrereleaseOrdering(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{
			name:    "stable upgrades from older stable",
			current: "v0.2.0",
			latest:  "v0.2.1",
			want:    true,
		},
		{
			name:    "stable final is newer than release candidate",
			current: "v0.3.0-rc1",
			latest:  "v0.3.0",
			want:    true,
		},
		{
			name:    "stable final does not downgrade to same-base prerelease",
			current: "v0.3.0",
			latest:  "v0.3.0-rc2",
			want:    false,
		},
		{
			name:    "prerelease does not outrank installed final",
			current: "v0.3.0",
			latest:  "v0.3.1-rc1",
			want:    true,
		},
		{
			name:    "alpha upgrades to beta",
			current: "v0.3.0-alpha1",
			latest:  "v0.3.0-beta1",
			want:    true,
		},
		{
			name:    "beta upgrades to rc",
			current: "v0.3.0-beta2",
			latest:  "v0.3.0-rc1",
			want:    true,
		},
		{
			name:    "rc1 upgrades to rc2",
			current: "v0.3.0-rc1",
			latest:  "v0.3.0-rc2",
			want:    true,
		},
		{
			name:    "rc10 upgrades correctly over rc2 when current older",
			current: "v0.3.0-rc2",
			latest:  "v0.3.0-rc10",
			want:    true,
		},
		{
			name:    "newer prerelease does not downgrade to older prerelease",
			current: "v0.3.0-rc10",
			latest:  "v0.3.0-rc2",
			want:    false,
		},
		{
			name:    "unknown hyphenated suffix is still older than final",
			current: "v0.3.0-preview1",
			latest:  "v0.3.0",
			want:    true,
		},
		{
			name:    "same prerelease is not an upgrade",
			current: "v0.3.0-rc2",
			latest:  "v0.3.0-rc2",
			want:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NeedsUpgrade(tc.current, tc.latest)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseVersionIgnoresPrereleaseForCoreVersion(t *testing.T) {
	got, err := ParseVersion("v1.2.3-rc4+build7")
	require.NoError(t, err)
	assert.Equal(t, [3]int{1, 2, 3}, got)
}

func TestParseDetailedVersionPreReleaseTokens(t *testing.T) {
	got, err := parseDetailedVersion("1.2.3-rc10")
	require.NoError(t, err)
	require.Len(t, got.PreRelease, 2)
	assert.Equal(t, versionIdentifier{Kind: "str", Text: "rc"}, got.PreRelease[0])
	assert.Equal(t, versionIdentifier{Kind: "num", Value: 10}, got.PreRelease[1])
}

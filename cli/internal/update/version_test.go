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
		assert.Equal(t, "/repos/easel/ddx/releases/latest", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","name":"v1.2.3","body":"notes","html_url":"https://example.com"}`))
	}))
	t.Cleanup(server.Close)

	release, err := fetchLatestRelease(server.URL + "/repos/easel/ddx/releases/latest")
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

	release, err := fetchLatestRelease(server.URL + "/repos/easel/ddx/releases/latest")
	require.Error(t, err)
	assert.Nil(t, release)
	msg := err.Error()
	assert.Contains(t, msg, "checking for DDx updates")
	assert.Contains(t, msg, "fetching latest release from")
	assert.Contains(t, msg, "403 Forbidden")
	assert.Contains(t, msg, "API rate limit exceeded")
	assert.True(t, strings.Contains(msg, server.URL))
}

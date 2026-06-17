package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillsHelpDoesNotRecommendBuiltinProjectPayloadPath(t *testing.T) {
	factory := NewCommandFactory(t.TempDir())

	output, err := executeCommand(factory.NewRootCommand(), "skills", "--help")
	require.NoError(t, err, output)

	assert.NotContains(t, output, "ddx skills check .ddx/plugins/ddx")
	assert.Contains(t, output, ".agents/skills")
	assert.Contains(t, output, ".claude/skills")
	assert.Contains(t, output, "local development overlay")
	assert.Contains(t, output, "legacy compatibility")
}

func TestFrontendReadmeDescribesCacheBackedFixtureTopology(t *testing.T) {
	readme := readRepoFile(t, filepath.Join("..", "internal", "server", "frontend", "README.md"))

	assert.Contains(t, readme, "cache/defaultplugin resolution")
	assert.Contains(t, readme, "generated adapters")
	assert.Contains(t, readme, ".agents/skills/")
	assert.Contains(t, readme, ".claude/skills/")
}

func TestWebsitePluginsDocsUseMaterializeForMarketplaceExample(t *testing.T) {
	docs := readRepoFile(t, filepath.Join("..", "..", "website", "content", "docs", "plugins.md"))

	assert.Contains(t, docs, "materialize:")
	assert.Contains(t, docs, "For registry installs, DDx caches the complete package payload outside the\nproject and applies `materialize.skills`")
	assert.NotContains(t, docs, "install:\n  root:\n    source: \".\"\n    target: \".ddx/plugins/my-plugin\"")
}

func TestDocsMentionProjectPayloadOnlyForLocalOverlayOrLegacyCompatibility(t *testing.T) {
	paths := []string{
		filepath.Join("skills.go"),
		filepath.Join("..", "internal", "server", "frontend", "README.md"),
	}

	for _, rel := range paths {
		assertPayloadMentionsAreContextual(t, rel, readRepoFile(t, rel))
	}
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()

	data, err := os.ReadFile(rel)
	require.NoError(t, err)
	return string(data)
}

func assertPayloadMentionsAreContextual(t *testing.T, file string, text string) {
	t.Helper()

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.Contains(line, ".ddx/plugins/ddx") {
			continue
		}

		lower := strings.ToLower(line)
		if strings.Contains(lower, "local development overlay") ||
			strings.Contains(lower, "local overlay") ||
			strings.Contains(lower, "legacy compatibility") {
			continue
		}

		t.Fatalf("%s:%d mentions .ddx/plugins/ddx outside explicit local-overlay or legacy-compatibility context: %q", file, i+1, line)
	}
}

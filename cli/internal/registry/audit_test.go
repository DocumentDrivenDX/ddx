package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditInstalledEntryReportsMissingManifest(t *testing.T) {
	root := t.TempDir()
	entry := InstalledEntry{
		Name:    "sample-plugin",
		Version: "1.0.0",
		Type:    PackageTypePlugin,
		Source:  root,
		Files:   []string{root},
	}

	issues := AuditInstalledEntry(entry, nil)
	require.NotEmpty(t, issues)
	assert.Contains(t, issues[0].Error(), "missing package.yaml")
}

func TestAuditInstalledEntryReportsManifestValidationWithoutMissingManifest(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "package.yaml"), []byte(`name: sample-plugin
version: 1.0.0
description: Sample plugin
type: plugin
source: https://example.com/sample-plugin
api_version: [not-a-scalar]
`), 0o644))

	entry := InstalledEntry{
		Name:    "sample-plugin",
		Version: "1.0.0",
		Type:    PackageTypePlugin,
		Source:  root,
		Files:   []string{root},
	}

	issues := AuditInstalledEntry(entry, nil)
	require.NotEmpty(t, issues)

	var sawValidationError, sawMissingManifest bool
	for _, issue := range issues {
		if strings.Contains(issue.Error(), "unsupported `api_version`") {
			sawValidationError = true
		}
		if strings.Contains(issue.Error(), "missing package.yaml") {
			sawMissingManifest = true
		}
	}

	assert.True(t, sawValidationError, "expected manifest validation issue, got: %+v", issues)
	assert.False(t, sawMissingManifest, "did not expect missing package.yaml, got: %+v", issues)
}

func TestAuditInstalledEntryReportsMissingRequiredManifestFields(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "package.yaml"), []byte(`name: sample-plugin
version: 1.0.0
type: plugin
source: https://example.com/sample-plugin
api_version: 1
`), 0o644))

	entry := InstalledEntry{
		Name:    "sample-plugin",
		Version: "1.0.0",
		Type:    PackageTypePlugin,
		Source:  root,
		Files:   []string{root},
	}

	issues := AuditInstalledEntry(entry, nil)
	require.NotEmpty(t, issues)

	var sawMissingDescription bool
	for _, issue := range issues {
		if strings.Contains(issue.Error(), "missing required field `description`") {
			sawMissingDescription = true
		}
	}

	assert.True(t, sawMissingDescription, "expected missing description issue, got: %+v", issues)
}

func TestAuditInstalledEntryReportsBrokenSymlinkAndMissingSkillMD(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills", "broken-skill"), 0o755))
	require.NoError(t, os.Symlink("does-not-exist", filepath.Join(root, "broken-link")))
	require.NoError(t, os.WriteFile(filepath.Join(root, "package.yaml"), []byte(`name: sample-plugin
version: 1.0.0
description: Sample plugin
type: plugin
source: https://example.com/sample-plugin
api_version: 1
install:
  root:
    source: .
    target: .ddx/plugins/sample-plugin
`), 0o644))

	entry := InstalledEntry{
		Name:    "sample-plugin",
		Version: "1.0.0",
		Type:    PackageTypePlugin,
		Source:  root,
		Files:   []string{root},
	}

	issues := AuditInstalledEntry(entry, nil)
	var sawBrokenLink, sawMissingSkill bool
	for _, issue := range issues {
		if strings.Contains(issue.Error(), "broken symlink") {
			sawBrokenLink = true
		}
		if strings.Contains(issue.Error(), "missing SKILL.md") {
			sawMissingSkill = true
		}
	}

	assert.True(t, sawBrokenLink, "expected broken symlink issue, got: %+v", issues)
	assert.True(t, sawMissingSkill, "expected missing SKILL.md issue, got: %+v", issues)
}

func TestAuditInstalledEntryReportsBrokenPluginRootSymlinkAndManifestSchemaIssues(t *testing.T) {
	workDir := t.TempDir()
	target := filepath.Join(workDir, "missing-plugin-root")
	root := filepath.Join(workDir, "sample-plugin")
	require.NoError(t, os.Symlink(target, root))

	entry := InstalledEntry{
		Name:   "sample-plugin",
		Source: root,
	}

	issues := AuditInstalledEntry(entry, nil)
	require.NotEmpty(t, issues)

	var sawBrokenRoot, sawMissingManifest bool
	for _, issue := range issues {
		if strings.Contains(issue.Error(), "broken symlink target") && strings.Contains(issue.Error(), "sample-plugin") {
			sawBrokenRoot = true
		}
		if strings.Contains(issue.Error(), "missing package.yaml") {
			sawMissingManifest = true
		}
	}

	assert.True(t, sawBrokenRoot, "expected broken root symlink issue, got: %+v", issues)
	assert.True(t, sawMissingManifest, "expected missing manifest issue, got: %+v", issues)
}

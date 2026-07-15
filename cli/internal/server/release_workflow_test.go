package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const (
	releaseFrontendJob      = "frontend"
	releaseFrontendArtifact = "verified-frontend-bundle"
	releaseFrontendPath     = "cli/internal/server/frontend/build"
	resolvedReleaseTagExpr  = "${{ github.event_name == 'workflow_dispatch' && inputs.tag || github.ref_name }}"
	resolvedReleaseCommit   = "${{ needs.prepare.outputs.commit }}"
)

var goBuildCommand = regexp.MustCompile(`(?m)^\s*go\s+build(?:\s|$)`)

type releaseWorkflow struct {
	Env  map[string]string             `yaml:"env"`
	Jobs map[string]releaseWorkflowJob `yaml:"jobs"`
}

type releaseWorkflowJob struct {
	Needs    releaseWorkflowNeeds    `yaml:"needs"`
	Outputs  map[string]string       `yaml:"outputs"`
	Strategy releaseWorkflowStrategy `yaml:"strategy"`
	Steps    []releaseWorkflowStep   `yaml:"steps"`
}

type releaseWorkflowStrategy struct {
	Matrix releaseWorkflowMatrix `yaml:"matrix"`
}

type releaseWorkflowMatrix struct {
	Include []releaseWorkflowMatrixEntry `yaml:"include"`
}

type releaseWorkflowMatrixEntry struct {
	GOOS   string `yaml:"goos"`
	GOARCH string `yaml:"goarch"`
	Runner string `yaml:"runner"`
}

type releaseWorkflowStep struct {
	ID               string            `yaml:"id"`
	Name             string            `yaml:"name"`
	Uses             string            `yaml:"uses"`
	WorkingDirectory string            `yaml:"working-directory"`
	Run              string            `yaml:"run"`
	Env              map[string]string `yaml:"env"`
	With             map[string]any    `yaml:"with"`
}

type releaseWorkflowNeeds []string

func (n *releaseWorkflowNeeds) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case 0:
		return nil
	case yaml.ScalarNode:
		*n = releaseWorkflowNeeds{strings.TrimSpace(node.Value)}
		return nil
	case yaml.SequenceNode:
		for _, child := range node.Content {
			*n = append(*n, strings.TrimSpace(child.Value))
		}
		return nil
	default:
		return fmt.Errorf("release workflow needs must be a string or sequence")
	}
}

func TestReleaseWorkflowResolvesManualAndTagEventToSameImmutableTag(t *testing.T) {
	wf := loadReleaseWorkflow(t)
	require.Equal(t, resolvedReleaseTagExpr, wf.Env["RELEASE_TAG"],
		"the sole event resolver must select inputs.tag for manual dispatch and github.ref_name for tag pushes")

	prepare, ok := wf.Jobs["prepare"]
	require.True(t, ok)
	assert.Equal(t, "${{ steps.identity.outputs.tag }}", prepare.Outputs["tag"])
	assert.Equal(t, "${{ steps.identity.outputs.commit }}", prepare.Outputs["commit"])
	assert.Equal(t, "${{ steps.version.outputs.version }}", prepare.Outputs["version"])
	assert.Equal(t, "${{ steps.previous.outputs.tag }}", prepare.Outputs["previous_tag"])
	assert.Equal(t, "${{ steps.previous.outputs.commit }}", prepare.Outputs["previous_commit"])

	identityAt := stepIndex(prepare, func(step releaseWorkflowStep) bool { return step.ID == "identity" })
	require.NotEqual(t, -1, identityAt, "prepare must resolve the requested tag to an immutable commit")
	identity := prepare.Steps[identityAt]
	assert.Contains(t, identity.Run, `TAG="${RELEASE_TAG}"`)
	assert.Contains(t, identity.Run, `git rev-parse "${TAG}^{commit}"`)
	assert.Contains(t, identity.Run, `git rev-parse HEAD`)
	assert.Contains(t, identity.Run, `echo "tag=${TAG}" >> "${GITHUB_OUTPUT}"`)
	assert.Contains(t, identity.Run, `echo "commit=${TAG_COMMIT}" >> "${GITHUB_OUTPUT}"`)

	for jobName, job := range wf.Jobs {
		if jobName != "prepare" {
			require.Containsf(t, []string(job.Needs), "prepare", "%s must consume the resolved release identity", jobName)
		}
		checkoutAt := stepIndex(job, func(step releaseWorkflowStep) bool {
			return strings.HasPrefix(step.Uses, "actions/checkout@")
		})
		require.NotEqualf(t, -1, checkoutAt, "%s must check out release source", jobName)
		wantRef := resolvedReleaseCommit
		if jobName == "prepare" {
			wantRef = "${{ env.RELEASE_TAG }}"
		}
		assert.Equalf(t, wantRef, stepWithString(job.Steps[checkoutAt], "ref"),
			"%s must check out the single resolved release identity", jobName)
	}

	buildJob := wf.Jobs["build-matrix"]
	buildAt := stepIndex(buildJob, func(step releaseWorkflowStep) bool {
		return strings.Contains(step.Run, "LDFLAGS=")
	})
	require.NotEqual(t, -1, buildAt)
	assert.Contains(t, buildJob.Steps[buildAt].Run, `-X main.commit=${{ needs.prepare.outputs.commit }}`)
	archiveAt := stepIndex(buildJob, func(step releaseWorkflowStep) bool {
		return strings.Contains(step.Run, "Commit:")
	})
	require.NotEqual(t, -1, archiveAt)
	assert.Contains(t, buildJob.Steps[archiveAt].Run, `Commit: ${{ needs.prepare.outputs.commit }}`)

	releaseJob := wf.Jobs["release"]
	releaseAt := stepIndex(releaseJob, func(step releaseWorkflowStep) bool {
		return strings.HasPrefix(step.Uses, "softprops/action-gh-release@")
	})
	require.NotEqual(t, -1, releaseAt)
	releaseStep := releaseJob.Steps[releaseAt]
	assert.Equal(t, "${{ needs.prepare.outputs.tag }}", stepWithString(releaseStep, "tag_name"))
	assert.Equal(t, "${{ needs.prepare.outputs.commit }}", stepWithString(releaseStep, "target_commitish"))
}

func TestReleaseWorkflowDoesNotUseAmbientRefForReleaseIdentity(t *testing.T) {
	source := loadReleaseWorkflowSource(t)
	assert.Equal(t, 1, strings.Count(source, "github.ref_name"),
		"github.ref_name is allowed only in the tag-push/manual-dispatch resolver")
	assert.Contains(t, source, "RELEASE_TAG: \""+resolvedReleaseTagExpr+"\"")
	assert.NotContains(t, source, "github.sha", "release identity must use the commit peeled from the resolved tag")
	assert.NotContains(t, source, "..HEAD", "changelog ranges must terminate at the resolved tag commit")

	wf := loadReleaseWorkflow(t)
	prepare := wf.Jobs["prepare"]
	versionAt := stepIndex(prepare, func(step releaseWorkflowStep) bool { return step.ID == "version" })
	require.NotEqual(t, -1, versionAt)
	assert.Contains(t, prepare.Steps[versionAt].Run, `VERSION="${{ steps.identity.outputs.tag }}"`)

	releaseJob := wf.Jobs["release"]
	changelogAt := stepIndex(releaseJob, func(step releaseWorkflowStep) bool { return step.ID == "changelog" })
	require.NotEqual(t, -1, changelogAt)
	changelogStep := releaseJob.Steps[changelogAt]
	assert.Equal(t, "${{ needs.prepare.outputs.previous_tag }}", changelogStep.Env["PREVIOUS_TAG"])
	assert.Equal(t, "${{ needs.prepare.outputs.previous_commit }}", changelogStep.Env["BASELINE_COMMIT"])
	assert.Equal(t, "${{ needs.prepare.outputs.commit }}", changelogStep.Env["RELEASE_COMMIT"])
	assert.NotContains(t, changelogStep.Run, "${{", "job outputs must enter shell through env, not generated shell source")
	assert.Contains(t, changelogStep.Run, `git log "${BASELINE_COMMIT}..${RELEASE_COMMIT}"`)

	releaseAt := stepIndex(releaseJob, func(step releaseWorkflowStep) bool {
		return strings.HasPrefix(step.Uses, "softprops/action-gh-release@")
	})
	require.NotEqual(t, -1, releaseAt)
	releaseStep := releaseJob.Steps[releaseAt]
	assert.Equal(t, "DDx ${{ needs.prepare.outputs.tag }}", stepWithString(releaseStep, "name"))
	assert.Equal(t, "${{ needs.prepare.outputs.tag }}", stepWithString(releaseStep, "tag_name"))
	assert.Equal(t, "${{ contains(needs.prepare.outputs.tag, '-') }}", stepWithString(releaseStep, "prerelease"))

	finalAt := stepIndex(releaseJob, func(step releaseWorkflowStep) bool {
		return strings.Contains(step.Run, "/releases/tag/")
	})
	require.NotEqual(t, -1, finalAt)
	assert.Contains(t, releaseJob.Steps[finalAt].Run, `/releases/tag/${{ needs.prepare.outputs.tag }}`)
}

func TestReleaseBaselineScriptPrefersPreviousPublishedRelease(t *testing.T) {
	fixture := newReleaseBaselineFixture(t)
	metadata := `[
  [
    {"tag_name":"v0.6.2-alpha105","draft":false,"published_at":"2026-07-14T12:00:00Z"},
    {"tag_name":"v9.9.9","draft":false,"published_at":"2026-07-13T12:00:00Z"},
    {"tag_name":"v0.1.0\"$(id)\"","draft":false,"published_at":"2026-07-12T12:00:00Z"},
    {"tag_name":"--format=x","draft":false,"published_at":"2026-07-11T12:00:00Z"}
  ],
  [
    {"tag_name":"v0.6.2-alpha104","draft":true,"published_at":null},
    {"tag_name":"v0.6.2-alpha103","draft":true,"published_at":null},
    {"tag_name":"v0.6.2-alpha102","draft":false,"published_at":"2026-07-10T12:00:00Z"}
  ]
]`

	baseline, _, err := runReleaseBaselineScript(
		t, fixture.repo, "v0.6.2-alpha105", fixture.commits["alpha105"], metadata,
	)
	require.NoError(t, err)
	assert.Equal(t, "v0.6.2-alpha102", baseline.Tag)
	assert.Equal(t, fixture.commits["alpha102"], baseline.Commit)
	assert.NotEqual(t, "v0.6.2-alpha104", baseline.Tag, "nearest unpublished tag must not become the changelog baseline")

	t.Run("distinct prior release at the candidate commit remains eligible", func(t *testing.T) {
		metadata := `[
  {"tag_name":"v0.6.2-alpha105","draft":false,"published_at":"2026-07-14T12:00:00Z"},
  {"tag_name":"v0.6.2-alpha100","draft":false,"published_at":"2026-07-09T12:00:00Z"}
]`
		baseline, _, err := runReleaseBaselineScript(
			t, fixture.repo, "v0.6.2-alpha105", fixture.commits["alpha105"], metadata,
		)
		require.NoError(t, err)
		assert.Equal(t, "v0.6.2-alpha100", baseline.Tag)
		assert.Equal(t, fixture.commits["alpha105"], baseline.Commit)
	})
}

func TestReleaseChangelogIncludesCommitsBehindUnpublishedTags(t *testing.T) {
	fixture := newReleaseBaselineFixture(t)
	metadata := `[{"tag_name":"v0.6.2-alpha102","draft":false,"published_at":"2026-07-10T12:00:00Z"}]`
	baseline, _, err := runReleaseBaselineScript(
		t, fixture.repo, "v0.6.2-alpha105", fixture.commits["alpha105"], metadata,
	)
	require.NoError(t, err)
	originalBaseline := baseline.Commit
	runGitFixture(t, fixture.repo, "update-ref", "refs/tags/v0.6.2-alpha102", fixture.commits["alpha104"])
	assert.Equal(t, fixture.commits["alpha104"], strings.TrimSpace(runGitFixture(
		t, fixture.repo, "rev-parse", "v0.6.2-alpha102^{commit}",
	)))

	cmd := exec.Command("git", "-C", fixture.repo, "log", originalBaseline+".."+fixture.commits["alpha105"], "--format=%s", "--no-merges")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	changelog := string(out)
	assert.Contains(t, changelog, "feat: unpublished alpha103")
	assert.Contains(t, changelog, "fix: unpublished alpha104")
	assert.Contains(t, changelog, "docs: change after alpha104 without its own tag")
	assert.Contains(t, changelog, "chore: alpha105 release head")
	assert.NotContains(t, changelog, "fix: published alpha102")
	assert.NotContains(t, changelog, "chore: history before alpha102")
}

func TestReleaseWorkflowUsesPublishedReleaseBaseline(t *testing.T) {
	source := loadReleaseWorkflowSource(t)
	assert.NotContains(t, source, "git describe", "tag proximity is not publication evidence")

	wf := loadReleaseWorkflow(t)
	prepare := wf.Jobs["prepare"]
	metadataAt := stepIndex(prepare, func(step releaseWorkflowStep) bool {
		return strings.Contains(step.Run, "gh api --paginate --slurp") &&
			strings.Contains(step.Run, "/releases?per_page=100")
	})
	previousAt := stepIndex(prepare, func(step releaseWorkflowStep) bool { return step.ID == "previous" })
	require.NotEqual(t, -1, metadataAt, "prepare must fetch GitHub Release metadata")
	require.NotEqual(t, -1, previousAt, "prepare must select the previous published release")
	assert.Less(t, metadataAt, previousAt)
	previous := prepare.Steps[previousAt]
	assert.Contains(t, previous.Run, "python3 scripts/release-baseline.py")
	assert.Equal(t, "${{ steps.identity.outputs.tag }}", previous.Env["RELEASE_TAG"])
	assert.Equal(t, "${{ steps.identity.outputs.commit }}", previous.Env["RELEASE_COMMIT"])
	assert.NotContains(t, previous.Run, "${{", "step outputs must enter shell through env, not generated shell source")
	assert.Contains(t, previous.Run, `--release-tag "${RELEASE_TAG}"`)
	assert.Contains(t, previous.Run, `--release-commit "${RELEASE_COMMIT}"`)
	assert.Contains(t, previous.Run, `--releases-json "${RELEASES_JSON}"`)
	assert.Contains(t, previous.Run, `printf 'commit=%s\n' "${BASELINE_COMMIT}"`)

	releaseJob := wf.Jobs["release"]
	changelogAt := stepIndex(releaseJob, func(step releaseWorkflowStep) bool { return step.ID == "changelog" })
	require.NotEqual(t, -1, changelogAt)
	changelogStep := releaseJob.Steps[changelogAt]
	changelog := changelogStep.Run
	assert.Contains(t, changelog, "No previous published GitHub Release baseline was proven")
	assert.Equal(t, "${{ needs.prepare.outputs.previous_commit }}", changelogStep.Env["BASELINE_COMMIT"])
	assert.NotContains(t, changelog, "${{", "published tag text must not be interpolated into shell source")
	assert.Contains(t, changelog, `git log "${BASELINE_COMMIT}..${RELEASE_COMMIT}"`)

	fixture := newReleaseBaselineFixture(t)
	_, output, err := runReleaseBaselineScript(
		t, fixture.repo, "v0.6.2-alpha105", fixture.commits["alpha105"], `[]`,
	)
	require.Error(t, err)
	assert.Contains(t, output, "no previous published GitHub Release tag is reachable")
	assert.Contains(t, output, "refusing nearest-tag fallback")

	doc, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "releasing.md"))
	require.NoError(t, err)
	assert.Contains(t, string(doc), "newest non-draft GitHub Release")
	assert.Contains(t, string(doc), "Do not substitute the")
	assert.Contains(t, string(doc), "nearest local tag")
}

func TestReleaseChecklistNamesNineAssetAndSmokeProofs(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "releasing.md"))
	require.NoError(t, err)
	doc := string(data)

	archives := []string{
		"ddx-linux-amd64.tar.gz",
		"ddx-linux-arm64.tar.gz",
		"ddx-darwin-amd64.tar.gz",
		"ddx-darwin-arm64.tar.gz",
	}
	for _, archive := range archives {
		assert.Contains(t, doc, archive)
		assert.Contains(t, doc, archive+".sha256")
	}
	for _, proof := range []string{
		"checksums.sha256",
		"Release workflow conclusion: success",
		"sha256sum -c checksums.sha256",
		"./ddx version",
		`DDX_VERSION="$TAG"`,
		"Tag commit SHA",
		"Workflow URL",
		"Go/No-Go",
	} {
		assert.Contains(t, doc, proof)
	}
	assert.Contains(t, doc, "not bit-for-bit reproducibility")
}

func TestContributingReleaseGuidanceUsesDurableChecklist(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "CONTRIBUTING.md"))
	require.NoError(t, err)
	doc := string(data)
	assert.Contains(t, doc, "[release checklist](docs/releasing.md)")
	assert.NotContains(t, doc, ".ddx/skills/ddx-release")
	assert.NotContains(t, doc, "/ddx-release")
}

func TestReleaseWorkflowBuildsAndVerifiesFrontendArtifact(t *testing.T) {
	wf := loadReleaseWorkflow(t)
	producer, ok := wf.Jobs[releaseFrontendJob]
	require.True(t, ok, "release workflow must define one %q producer job", releaseFrontendJob)
	require.NoError(t, validateReleaseFrontendProducer(producer))
	require.Contains(t, []string(producer.Needs), "prepare")

	setupAt := stepIndex(producer, func(step releaseWorkflowStep) bool {
		return strings.HasPrefix(step.Uses, "oven-sh/setup-bun@")
	})
	buildAt := stepIndex(producer, func(step releaseWorkflowStep) bool {
		return cleanWorkflowPath(step.WorkingDirectory) == "cli/internal/server/frontend" &&
			strings.Contains(step.Run, "bun install --frozen-lockfile") &&
			strings.Contains(step.Run, "bun run build")
	})
	verifyAt := stepIndex(producer, func(step releaseWorkflowStep) bool {
		return cleanWorkflowPath(step.WorkingDirectory) == "cli" &&
			strings.Contains(step.Run, "make frontend-verify")
	})
	uploadAt := stepIndex(producer, func(step releaseWorkflowStep) bool {
		return strings.HasPrefix(step.Uses, "actions/upload-artifact@") &&
			stepWithString(step, "name") == releaseFrontendArtifact &&
			cleanWorkflowPath(stepWithString(step, "path")) == releaseFrontendPath &&
			stepWithString(step, "if-no-files-found") == "error"
	})

	require.NotEqual(t, -1, setupAt, "frontend producer must install Bun")
	require.NotEqual(t, -1, buildAt, "frontend producer must build from the frozen Bun lockfile")
	require.NotEqual(t, -1, verifyAt, "frontend producer must run frontend-verify")
	require.NotEqual(t, -1, uploadAt, "frontend producer must upload the verified build directory")
	assert.True(t, setupAt < buildAt && buildAt < verifyAt && verifyAt < uploadAt,
		"producer order must be setup Bun -> locked build -> verify -> upload")
	for _, step := range producer.Steps {
		assert.False(t, goBuildCommand.MatchString(step.Run), "frontend producer must not build a Go binary")
	}
	uploadCount := 0
	for _, job := range wf.Jobs {
		for _, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "actions/upload-artifact@") && stepWithString(step, "name") == releaseFrontendArtifact {
				uploadCount++
			}
		}
	}
	assert.Equal(t, 1, uploadCount, "the workflow must have exactly one verified frontend artifact producer")
}

func TestReleaseWorkflowConsumersEmbedVerifiedFrontendArtifact(t *testing.T) {
	wf := loadReleaseWorkflow(t)
	var goBuildJobs []string
	for jobName, job := range wf.Jobs {
		if stepIndex(job, func(step releaseWorkflowStep) bool { return goBuildCommand.MatchString(step.Run) }) >= 0 {
			goBuildJobs = append(goBuildJobs, jobName)
		}
	}
	sort.Strings(goBuildJobs)
	require.Equal(t, []string{"build-matrix", "test"}, goBuildJobs,
		"every release Go binary builder must be covered as a verified frontend consumer")

	for _, jobName := range []string{"test", "build-matrix"} {
		job, ok := wf.Jobs[jobName]
		require.Truef(t, ok, "release workflow must define %q", jobName)
		require.NoError(t, validateReleaseFrontendConsumer(jobName, job))
		require.Containsf(t, []string(job.Needs), releaseFrontendJob,
			"%s must directly depend on the verified frontend producer", jobName)

		downloadAt := stepIndex(job, func(step releaseWorkflowStep) bool {
			return strings.HasPrefix(step.Uses, "actions/download-artifact@") &&
				stepWithString(step, "name") == releaseFrontendArtifact &&
				cleanWorkflowPath(stepWithString(step, "path")) == releaseFrontendPath
		})
		verifyAt := stepIndex(job, func(step releaseWorkflowStep) bool {
			return cleanWorkflowPath(step.WorkingDirectory) == "cli" &&
				strings.Contains(step.Run, "make frontend-verify")
		})
		buildAt := stepIndex(job, func(step releaseWorkflowStep) bool {
			return goBuildCommand.MatchString(step.Run)
		})
		require.NotEqualf(t, -1, downloadAt, "%s must download the verified frontend artifact", jobName)
		require.NotEqualf(t, -1, verifyAt, "%s must verify the downloaded frontend artifact", jobName)
		require.NotEqualf(t, -1, buildAt, "%s must build a Go binary", jobName)
		assert.Lessf(t, downloadAt, verifyAt, "%s must download the frontend before verifying it", jobName)
		assert.Lessf(t, verifyAt, buildAt, "%s must verify the downloaded frontend before go build", jobName)
	}

	matrix := wf.Jobs["build-matrix"].Strategy.Matrix.Include
	require.Len(t, matrix, 4, "release matrix must retain all Linux/Darwin binaries")
	got := make([]string, 0, len(matrix))
	for _, entry := range matrix {
		require.Contains(t, []string{"linux", "darwin"}, entry.GOOS)
		require.Contains(t, []string{"amd64", "arm64"}, entry.GOARCH)
		require.NotEmpty(t, entry.Runner)
		got = append(got, entry.GOOS+"/"+entry.GOARCH)
	}
	sort.Strings(got)
	assert.Equal(t, []string{"darwin/amd64", "darwin/arm64", "linux/amd64", "linux/arm64"}, got)
}

func TestReleaseWorkflowFreshCheckoutCannotShipGitkeepOnlyBundle(t *testing.T) {
	wf := loadReleaseWorkflow(t)

	producer := wf.Jobs[releaseFrontendJob]
	for i := range producer.Steps {
		producer.Steps[i].Run = strings.Replace(producer.Steps[i].Run, "bun run build", "", 1)
	}
	wf.Jobs[releaseFrontendJob] = producer
	require.ErrorContains(t, validateReleaseFrontendProducer(producer), "locked frontend build")

	wf = loadReleaseWorkflow(t)
	testJob := wf.Jobs["test"]
	for i := range testJob.Steps {
		if strings.HasPrefix(testJob.Steps[i].Uses, "actions/download-artifact@") {
			testJob.Steps = append(testJob.Steps[:i], testJob.Steps[i+1:]...)
			break
		}
	}
	require.ErrorContains(t, validateReleaseFrontendConsumer("test", testJob), "download")

	wf = loadReleaseWorkflow(t)
	testJob = wf.Jobs["test"]
	for i := range testJob.Steps {
		testJob.Steps[i].Run = strings.Replace(testJob.Steps[i].Run, "make frontend-verify", "", 1)
	}
	require.ErrorContains(t, validateReleaseFrontendConsumer("test", testJob), "verify")

	gitkeepOnly := t.TempDir()
	gitkeepCmd := exec.Command("git", "-C", repoRoot(t), "show", "HEAD:"+releaseFrontendPath+"/.gitkeep")
	gitkeep, err := gitkeepCmd.Output()
	require.NoError(t, err, "fresh checkout must contain the tracked build/.gitkeep fallback")
	require.NoError(t, os.WriteFile(filepath.Join(gitkeepOnly, ".gitkeep"), gitkeep, 0o644))
	verifyFrontendFixture(t, gitkeepOnly, "index.html is missing or empty")

	emptyIndex := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(emptyIndex, "index.html"), nil, 0o644))
	verifyFrontendFixture(t, emptyIndex, "index.html is missing or empty")

	noAppReferences := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(noAppReferences, "index.html"), []byte("<html><body>no application assets</body></html>"), 0o644))
	verifyFrontendFixture(t, noAppReferences, "references no /_app assets")

	missingAsset := t.TempDir()
	missingIndex := `<html><script src="/_app/immutable/entry/missing.js"></script></html>`
	require.NoError(t, os.WriteFile(filepath.Join(missingAsset, "index.html"), []byte(missingIndex), 0o644))
	verifyFrontendFixture(t, missingAsset, "referenced asset(s) missing")

	complete := t.TempDir()
	index := `<html data-sveltekit-preload-data="hover"><head><link href="/_app/immutable/assets/app.css" rel="stylesheet"></head><body><script src="/_app/immutable/entry/start.js"></script></body></html>`
	require.NoError(t, os.WriteFile(filepath.Join(complete, "index.html"), []byte(index), 0o644))
	for _, asset := range []string{
		"_app/immutable/assets/app.css",
		"_app/immutable/entry/start.js",
	} {
		path := filepath.Join(complete, filepath.FromSlash(asset))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("fixture"), 0o644))
	}
	verifyFrontendFixture(t, complete, "")
}

func loadReleaseWorkflow(t *testing.T) releaseWorkflow {
	t.Helper()
	data := []byte(loadReleaseWorkflowSource(t))
	var wf releaseWorkflow
	require.NoError(t, yaml.Unmarshal(data, &wf))
	require.NotEmpty(t, wf.Jobs)
	return wf
}

func loadReleaseWorkflowSource(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "release.yml"))
	require.NoError(t, err)
	return string(data)
}

func validateReleaseFrontendProducer(job releaseWorkflowJob) error {
	setupAt := stepIndex(job, func(step releaseWorkflowStep) bool {
		return strings.HasPrefix(step.Uses, "oven-sh/setup-bun@")
	})
	buildAt := stepIndex(job, func(step releaseWorkflowStep) bool {
		return cleanWorkflowPath(step.WorkingDirectory) == "cli/internal/server/frontend" &&
			strings.Contains(step.Run, "bun install --frozen-lockfile") &&
			strings.Contains(step.Run, "bun run build")
	})
	verifyAt := stepIndex(job, func(step releaseWorkflowStep) bool {
		return cleanWorkflowPath(step.WorkingDirectory) == "cli" && strings.Contains(step.Run, "make frontend-verify")
	})
	uploadAt := stepIndex(job, func(step releaseWorkflowStep) bool {
		return strings.HasPrefix(step.Uses, "actions/upload-artifact@") &&
			stepWithString(step, "name") == releaseFrontendArtifact &&
			cleanWorkflowPath(stepWithString(step, "path")) == releaseFrontendPath &&
			stepWithString(step, "if-no-files-found") == "error"
	})
	for _, required := range []struct {
		name  string
		index int
	}{
		{name: "Bun setup", index: setupAt},
		{name: "locked frontend build", index: buildAt},
		{name: "frontend verification", index: verifyAt},
		{name: "verified artifact upload", index: uploadAt},
	} {
		if required.index < 0 {
			return fmt.Errorf("frontend producer is missing %s", required.name)
		}
	}
	if setupAt >= buildAt || buildAt >= verifyAt || verifyAt >= uploadAt {
		return fmt.Errorf("frontend producer order must be setup, locked build, verify, upload")
	}
	return nil
}

func validateReleaseFrontendConsumer(name string, job releaseWorkflowJob) error {
	if !releaseJobNeeds(job, releaseFrontendJob) {
		return fmt.Errorf("%s must depend on the frontend producer", name)
	}
	downloadAt := stepIndex(job, func(step releaseWorkflowStep) bool {
		return strings.HasPrefix(step.Uses, "actions/download-artifact@") &&
			stepWithString(step, "name") == releaseFrontendArtifact &&
			cleanWorkflowPath(stepWithString(step, "path")) == releaseFrontendPath
	})
	if downloadAt < 0 {
		return fmt.Errorf("%s must download the verified frontend artifact", name)
	}
	verifyAt := stepIndex(job, func(step releaseWorkflowStep) bool {
		return cleanWorkflowPath(step.WorkingDirectory) == "cli" &&
			strings.Contains(step.Run, "make frontend-verify")
	})
	if verifyAt < 0 {
		return fmt.Errorf("%s must verify the downloaded frontend artifact", name)
	}
	buildAt := stepIndex(job, func(step releaseWorkflowStep) bool {
		return goBuildCommand.MatchString(step.Run)
	})
	if buildAt < 0 {
		return fmt.Errorf("%s has no Go build step", name)
	}
	if downloadAt >= verifyAt || verifyAt >= buildAt {
		return fmt.Errorf("%s order must be download, verify, go build", name)
	}
	return nil
}

func releaseJobNeeds(job releaseWorkflowJob, dependency string) bool {
	for _, need := range job.Needs {
		if need == dependency {
			return true
		}
	}
	return false
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func stepIndex(job releaseWorkflowJob, match func(releaseWorkflowStep) bool) int {
	for i, step := range job.Steps {
		if match(step) {
			return i
		}
	}
	return -1
}

func stepWithString(step releaseWorkflowStep, key string) string {
	value, ok := step.With[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func cleanWorkflowPath(path string) string {
	return strings.TrimSuffix(filepath.ToSlash(strings.TrimSpace(path)), "/")
}

func verifyFrontendFixture(t *testing.T, dir, wantError string) {
	t.Helper()
	cmd := exec.Command("make", "--no-print-directory", "frontend-verify", "FRONTEND_VERIFY_DIR="+dir)
	cmd.Dir = filepath.Join(repoRoot(t), "cli")
	out, err := cmd.CombinedOutput()
	if wantError == "" {
		require.NoError(t, err, "%s", out)
		assert.Contains(t, string(out), "ok")
		return
	}
	require.Error(t, err, "%s", out)
	assert.Contains(t, string(out), wantError)
}

type releaseBaselineFixture struct {
	repo    string
	commits map[string]string
}

type releaseBaselineSelection struct {
	Tag    string `json:"tag"`
	Commit string `json:"commit"`
}

func newReleaseBaselineFixture(t *testing.T) releaseBaselineFixture {
	t.Helper()
	repo := t.TempDir()
	runGitFixture(t, repo, "init", "--quiet")
	runGitFixture(t, repo, "config", "user.name", "Release Test")
	runGitFixture(t, repo, "config", "user.email", "release-test@example.com")

	commits := map[string]string{}
	commit := func(key, subject string) {
		t.Helper()
		path := filepath.Join(repo, key+".txt")
		require.NoError(t, os.WriteFile(path, []byte(subject+"\n"), 0o644))
		runGitFixture(t, repo, "add", filepath.Base(path))
		runGitFixture(t, repo, "commit", "--quiet", "-m", subject)
		commits[key] = strings.TrimSpace(runGitFixture(t, repo, "rev-parse", "HEAD"))
	}
	tag := func(key, name string) {
		t.Helper()
		runGitFixture(t, repo, "tag", name, commits[key])
	}

	commit("history", "chore: history before alpha102")
	commit("alpha102", "fix: published alpha102")
	tag("alpha102", "v0.6.2-alpha102")
	commit("alpha103", "feat: unpublished alpha103")
	tag("alpha103", "v0.6.2-alpha103")
	commit("alpha104", "fix: unpublished alpha104")
	tag("alpha104", "v0.6.2-alpha104")
	commit("after-alpha104", "docs: change after alpha104 without its own tag")
	commit("alpha105", "chore: alpha105 release head")
	tag("alpha105", "v0.6.2-alpha105")
	tag("alpha105", "v0.6.2-alpha100")

	unreachableTree := strings.TrimSpace(runGitFixture(t, repo, "rev-parse", commits["history"]+"^{tree}"))
	commits["unreachable"] = strings.TrimSpace(runGitFixture(
		t, repo, "commit-tree", unreachableTree, "-m", "chore: unrelated published release",
	))
	runGitFixture(t, repo, "update-ref", "refs/tags/v9.9.9", commits["unreachable"])
	runGitFixture(t, repo, "update-ref", `refs/tags/v0.1.0"$(id)"`, commits["alpha104"])
	runGitFixture(t, repo, "update-ref", "refs/tags/--format=x", commits["alpha104"])

	return releaseBaselineFixture{repo: repo, commits: commits}
}

func runReleaseBaselineScript(
	t *testing.T, repo, releaseTag, releaseCommit, metadata string,
) (releaseBaselineSelection, string, error) {
	t.Helper()
	metadataPath := filepath.Join(t.TempDir(), "github-releases.json")
	require.NoError(t, os.WriteFile(metadataPath, []byte(metadata), 0o644))
	cmd := exec.Command(
		"python3",
		filepath.Join(repoRoot(t), "scripts", "release-baseline.py"),
		"--repo", repo,
		"--release-tag", releaseTag,
		"--release-commit", releaseCommit,
		"--releases-json", metadataPath,
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	output := strings.TrimSpace(stderr.String() + stdout.String())
	if err != nil {
		return releaseBaselineSelection{}, output, err
	}
	var selection releaseBaselineSelection
	if decodeErr := json.Unmarshal(stdout.Bytes(), &selection); decodeErr != nil {
		return releaseBaselineSelection{}, output, decodeErr
	}
	return selection, output, nil
}

func runGitFixture(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	return string(out)
}

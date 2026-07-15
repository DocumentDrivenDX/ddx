package server

import (
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
)

var goBuildCommand = regexp.MustCompile(`(?m)^\s*go\s+build(?:\s|$)`)

type releaseWorkflow struct {
	Jobs map[string]releaseWorkflowJob `yaml:"jobs"`
}

type releaseWorkflowJob struct {
	Needs    releaseWorkflowNeeds    `yaml:"needs"`
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
	Name             string         `yaml:"name"`
	Uses             string         `yaml:"uses"`
	WorkingDirectory string         `yaml:"working-directory"`
	Run              string         `yaml:"run"`
	With             map[string]any `yaml:"with"`
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
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "release.yml"))
	require.NoError(t, err)
	var wf releaseWorkflow
	require.NoError(t, yaml.Unmarshal(data, &wf))
	require.NotEmpty(t, wf.Jobs)
	return wf
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

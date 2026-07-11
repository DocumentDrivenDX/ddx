package cmd

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildServerServiceConfigUsesXDGRuntimeDir(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-secret")

	projectRoot := filepath.Join(t.TempDir(), "project")
	cfg := buildServerServiceConfig("/usr/local/bin/ddx", projectRoot)

	require.Equal(t, projectRoot, cfg.ProjectRoot)
	assert.Equal(t, filepath.Join(xdg, "ddx", "server"), cfg.WorkDir)
	assert.Equal(t, filepath.Join(xdg, "ddx", "server", "ddx-server.log"), cfg.LogPath)
	assert.Equal(t, projectRoot, cfg.Env["DDX_PROJECT_ROOT"])
	assert.Equal(t, "anthropic-secret", cfg.Env["ANTHROPIC_API_KEY"])
}

type fakeServiceBackend struct {
	installedCfg service.Config
	installCalls int
}

func (f *fakeServiceBackend) Install(cfg service.Config) error {
	f.installedCfg = cfg
	f.installCalls++
	return nil
}
func (f *fakeServiceBackend) Uninstall() error { return nil }
func (f *fakeServiceBackend) Start() error     { return nil }
func (f *fakeServiceBackend) Stop() error      { return nil }
func (f *fakeServiceBackend) Status() error    { return nil }

func withFakeServiceBackend(t *testing.T) *fakeServiceBackend {
	t.Helper()
	fake := &fakeServiceBackend{}
	prev := serviceNew
	serviceNew = func() (service.Backend, error) { return fake, nil }
	t.Cleanup(func() { serviceNew = prev })
	return fake
}

func TestServerInstall_ResourceBudgetFlagsReachServiceConfig(t *testing.T) {
	fake := withFakeServiceBackend(t)

	f := &CommandFactory{WorkingDir: t.TempDir()}
	cmd := f.newServerInstallCommand()
	cmd.SetArgs([]string{
		"--exec", "/usr/local/bin/ddx",
		"--cpu-quota-percent", "250",
		"--cpu-weight", "42",
		"--nice", "15",
	})
	require.NoError(t, cmd.Execute())

	require.Equal(t, 1, fake.installCalls)
	assert.Equal(t, 250, fake.installedCfg.ResourcePolicy.CPUQuotaPercent)
	assert.Equal(t, 42, fake.installedCfg.ResourcePolicy.CPUWeight)
	assert.Equal(t, 15, fake.installedCfg.ResourcePolicy.Nice)
}

func TestServerInstall_DefaultResourceBudgetIsHostBounded(t *testing.T) {
	fake := withFakeServiceBackend(t)

	f := &CommandFactory{WorkingDir: t.TempDir()}
	cmd := f.newServerInstallCommand()
	cmd.SetArgs([]string{"--exec", "/usr/local/bin/ddx"})
	require.NoError(t, cmd.Execute())

	require.Equal(t, 1, fake.installCalls)
	got := fake.installedCfg.ResourcePolicy.CPUQuotaPercent
	maxAllowed := runtime.NumCPU() * 100
	if maxAllowed > 400 {
		maxAllowed = 400
	}
	assert.LessOrEqual(t, got, maxAllowed)
	assert.LessOrEqual(t, got, runtime.NumCPU()*100)
	assert.Greater(t, got, 0)
}

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/service"
	"github.com/spf13/cobra"
)

// serviceNew resolves the platform service backend. Overridable for testing.
var serviceNew = service.New

// envKeysForService are forwarded from the current environment into the
// installed service's environment so agent harnesses work out of the box.
var envKeysForService = []string{
	"ANTHROPIC_API_KEY",
	"OPENAI_API_KEY",
	"OPENROUTER_API_KEY",
	"GEMINI_API_KEY",
	"DDX_AGENT_HARNESS",
	"DDX_AGENT_MODEL",
	"DDX_AGENT_EFFORT",
}

func buildServerServiceConfig(execPath, projectRoot string) service.Config {
	runtimeDir := service.ServerRuntimeDir()
	env := map[string]string{
		"DDX_PROJECT_ROOT": projectRoot,
	}
	for _, k := range envKeysForService {
		if v := os.Getenv(k); v != "" {
			env[k] = v
		}
	}
	return service.Config{
		ExecPath:    execPath,
		ProjectRoot: projectRoot,
		WorkDir:     runtimeDir,
		LogPath:     filepath.Join(runtimeDir, "ddx-server.log"),
		Env:         env,
	}
}

func (f *CommandFactory) newServerInstallCommand() *cobra.Command {
	var workDir string
	var execPath string
	defaultPolicy := service.DefaultResourcePolicy()
	var cpuQuotaPercent int
	var cpuWeight int
	var nice int

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install ddx server as a user service (systemd on Linux, launchd on macOS)",
		RunE: func(cmd *cobra.Command, args []string) error {
			resourcePolicy := service.ResourcePolicy{
				CPUQuotaPercent: cpuQuotaPercent,
				CPUWeight:       cpuWeight,
				Nice:            nice,
			}
			if err := resourcePolicy.Validate(); err != nil {
				return fmt.Errorf("invalid resource budget: %w", err)
			}

			backend, err := serviceNew()
			if err != nil {
				return err
			}

			resolvedExec, err := os.Executable()
			if err != nil {
				resolvedExec, err = exec.LookPath("ddx")
				if err != nil {
					return fmt.Errorf("cannot locate ddx binary; specify --exec")
				}
			}
			if execPath != "" {
				resolvedExec = execPath
			}

			resolvedWork := f.WorkingDir
			if workDir != "" {
				resolvedWork = workDir
			}
			if resolvedWork == "" {
				return fmt.Errorf("cannot determine project root; specify --workdir")
			}
			resolvedWork = gitpkg.FindProjectRoot(resolvedWork)
			cfg := buildServerServiceConfig(resolvedExec, resolvedWork)
			cfg.ResourcePolicy = resourcePolicy
			return backend.Install(cfg)
		},
	}
	cmd.Flags().StringVar(&workDir, "workdir", "", "Project root registered on server startup (default: current directory); service cwd is XDG-scoped")
	cmd.Flags().StringVar(&execPath, "exec", "", "Path to ddx binary (default: auto-detected)")
	cmd.Flags().IntVar(&cpuQuotaPercent, "cpu-quota-percent", defaultPolicy.CPUQuotaPercent, "Aggregate CPU quota for the server and its managed workers/providers/builds, as a percentage of one core (0 = unbounded)")
	cmd.Flags().IntVar(&cpuWeight, "cpu-weight", defaultPolicy.CPUWeight, "CPU scheduling weight for the server-owned execution tree (1-10000, 0 = systemd default)")
	cmd.Flags().IntVar(&nice, "nice", defaultPolicy.Nice, "Scheduling priority (nice value, -20 to 19) for the server-owned execution tree")
	return cmd
}

func (f *CommandFactory) newServerUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the ddx server user service",
		RunE: func(cmd *cobra.Command, args []string) error {
			backend, err := serviceNew()
			if err != nil {
				return err
			}
			return backend.Uninstall()
		},
	}
}

func (f *CommandFactory) newServerStartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the ddx server service",
		RunE: func(cmd *cobra.Command, args []string) error {
			backend, err := serviceNew()
			if err != nil {
				return err
			}
			return backend.Start()
		},
	}
}

func (f *CommandFactory) newServerStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the ddx server service",
		RunE: func(cmd *cobra.Command, args []string) error {
			backend, err := serviceNew()
			if err != nil {
				return err
			}
			return backend.Stop()
		},
	}
}

func (f *CommandFactory) newServerStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show ddx server service status",
		RunE: func(cmd *cobra.Command, args []string) error {
			backend, err := serviceNew()
			if err != nil {
				return err
			}
			return backend.Status()
		},
	}
}

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/service"
	"github.com/spf13/cobra"
)

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
		"PATH":             serverServicePath(execPath),
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

func serverServicePath(execPath string) string {
	parts := make([]string, 0, 16)
	seen := map[string]struct{}{}
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		parts = append(parts, path)
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		add(filepath.Join(home, ".local", "bin"))
		add(filepath.Join(home, "bin"))
	}
	if dir := filepath.Dir(execPath); dir != "." && dir != "" {
		add(dir)
	}
	for _, part := range filepath.SplitList(os.Getenv("PATH")) {
		add(part)
	}
	if len(parts) == 0 {
		add("/usr/local/bin")
		add("/usr/bin")
		add("/bin")
	}
	return strings.Join(parts, string(os.PathListSeparator))
}

func (f *CommandFactory) newServerInstallCommand() *cobra.Command {
	var workDir string
	var execPath string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install ddx server as a user service (systemd on Linux, launchd on macOS)",
		RunE: func(cmd *cobra.Command, args []string) error {
			backend, err := service.New()
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
			return backend.Install(buildServerServiceConfig(resolvedExec, resolvedWork))
		},
	}
	cmd.Flags().StringVar(&workDir, "workdir", "", "Project root registered on server startup (default: current directory); service cwd is XDG-scoped")
	cmd.Flags().StringVar(&execPath, "exec", "", "Path to ddx binary (default: auto-detected)")
	return cmd
}

func (f *CommandFactory) newServerUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the ddx server user service",
		RunE: func(cmd *cobra.Command, args []string) error {
			backend, err := service.New()
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
			backend, err := service.New()
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
			backend, err := service.New()
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
			backend, err := service.New()
			if err != nil {
				return err
			}
			return backend.Status()
		},
	}
}

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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

			env := map[string]string{}
			for _, k := range envKeysForService {
				if v := os.Getenv(k); v != "" {
					env[k] = v
				}
			}

			return backend.Install(service.Config{
				ExecPath: resolvedExec,
				WorkDir:  resolvedWork,
				LogPath:  filepath.Join(resolvedWork, ".ddx", "logs", "ddx-server.log"),
				Env:      env,
			})
		},
	}
	cmd.Flags().StringVar(&workDir, "workdir", "", "Project root for the server (default: current directory)")
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

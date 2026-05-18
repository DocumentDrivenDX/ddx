package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
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

			cfg, err := f.serverServiceConfig(resolvedExec, workDir)
			if err != nil {
				return err
			}

			env := map[string]string{}
			for _, k := range envKeysForService {
				if v := os.Getenv(k); v != "" {
					env[k] = v
				}
			}
			cfg.Env = env

			return backend.Install(cfg)
		},
	}
	cmd.Flags().StringVar(&workDir, "workdir", "", "Override service working directory (default: user home)")
	cmd.Flags().StringVar(&execPath, "exec", "", "Path to ddx binary (default: auto-detected)")
	return cmd
}

func (f *CommandFactory) serverServiceConfig(execPath, workDir string) (service.Config, error) {
	if workDir != "" {
		return service.Config{
			ExecPath: execPath,
			WorkDir:  workDir,
			LogPath:  ddxroot.JoinProject(workDir, "logs", "ddx-server.log"),
		}, nil
	}

	defaultWorkDir, err := service.DefaultWorkDir()
	if err != nil {
		return service.Config{}, fmt.Errorf("resolve service working directory: %w", err)
	}
	defaultLogPath, err := service.DefaultLogPath()
	if err != nil {
		return service.Config{}, fmt.Errorf("resolve service log path: %w", err)
	}
	return service.Config{
		ExecPath: execPath,
		WorkDir:  defaultWorkDir,
		LogPath:  defaultLogPath,
	}, nil
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

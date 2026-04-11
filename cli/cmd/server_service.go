package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/DocumentDrivenDX/ddx/internal/systemd"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newServerInstallServiceCommand() *cobra.Command {
	var workDir string
	var execPath string

	cmd := &cobra.Command{
		Use:   "install-service",
		Short: "Install ddx server as a systemd user service",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "linux" {
				return fmt.Errorf("install-service is only supported on Linux (systemd user service)")
			}
			resolvedExec, err := os.Executable()
			if err != nil {
				resolvedExec, err = exec.LookPath("ddx")
				if err != nil {
					return fmt.Errorf("cannot locate ddx binary; specify --exec-path")
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

			cfg := systemd.UnitConfig{
				ExecPath: resolvedExec,
				WorkDir:  resolvedWork,
				LogPath:  resolvedWork + "/.ddx/logs/ddx-server.log",
				Env:      collectEnv(),
			}
			return systemd.Install(cfg)
		},
	}
	cmd.Flags().StringVar(&workDir, "workdir", "", "Project root for the server (default: current directory)")
	cmd.Flags().StringVar(&execPath, "exec", "", "Path to ddx binary (default: auto-detected)")
	return cmd
}

func (f *CommandFactory) newServerServiceStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "service-status",
		Short: "Show ddx server systemd service status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "linux" {
				return fmt.Errorf("service-status is only supported on Linux")
			}
			return systemd.Status()
		},
	}
}

func (f *CommandFactory) newServerUninstallServiceCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall-service",
		Short: "Remove ddx server systemd service",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "linux" {
				return fmt.Errorf("uninstall-service is only supported on Linux")
			}
			return systemd.Uninstall()
		},
	}
}

// collectEnv captures API key env vars the service needs.
func collectEnv() []string {
	keep := []string{
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"OPENROUTER_API_KEY",
		"GEMINI_API_KEY",
		"DDX_AGENT_HARNESS",
		"DDX_AGENT_MODEL",
		"DDX_AGENT_EFFORT",
	}
	var env []string
	for _, k := range keep {
		v := os.Getenv(k)
		if v != "" {
			env = append(env, k+"="+v)
		}
	}
	return env
}

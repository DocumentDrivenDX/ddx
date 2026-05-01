package service

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"text/template"
)

const unitName = "ddx-server.service"

const unitTemplate = `[Unit]
Description=DDx Server — AI agent execution and document management
After=network.target

[Service]
Type=simple
ExecStart={{.ExecPath}} server
WorkingDirectory={{.WorkDir}}
Restart=on-failure
RestartSec=5
StandardOutput=append:{{.LogPath}}
StandardError=append:{{.LogPath}}
{{if .EnvFile}}EnvironmentFile={{.EnvFile}}
{{end}}
[Install]
WantedBy=default.target
`

type systemdBackend struct{}

type systemdUnitParams struct {
	ExecPath string
	WorkDir  string
	LogPath  string
	EnvFile  string
}

func systemdServiceDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}

func (systemdBackend) Install(cfg Config) error {
	serviceDir, err := systemdServiceDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	envFile := filepath.Join(home, ".config", "ddx", "server.env")
	if err := os.MkdirAll(filepath.Dir(envFile), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.LogPath), 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	if err := writeEnvFile(envFile, cfg.Env); err != nil {
		return fmt.Errorf("write env file: %w", err)
	}

	unitPath := filepath.Join(serviceDir, unitName)
	tmpl, err := template.New("unit").Parse(unitTemplate)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, systemdUnitParams{
		ExecPath: cfg.ExecPath,
		WorkDir:  cfg.WorkDir,
		LogPath:  cfg.LogPath,
		EnvFile:  envFile,
	}); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write unit file %s: %w", unitPath, err)
	}

	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	if err := systemctl("enable", "--now", unitName); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Installed %s → %s\n", unitName, unitPath)
	fmt.Fprintf(os.Stdout, "  journalctl --user -u ddx-server -f  # follow logs\n")
	fmt.Fprintf(os.Stdout, "  ddx server status                    # status\n")
	fmt.Fprintf(os.Stdout, "  ddx server stop|start                # lifecycle\n")
	return nil
}

func (systemdBackend) Uninstall() error {
	_ = systemctl("disable", "--now", unitName)
	serviceDir, err := systemdServiceDir()
	if err != nil {
		return nil
	}
	_ = os.Remove(filepath.Join(serviceDir, unitName))
	_ = systemctl("daemon-reload")
	fmt.Fprintln(os.Stdout, "Uninstalled ddx-server.service")
	return nil
}

func (systemdBackend) Start() error  { return systemctl("start", unitName) }
func (systemdBackend) Stop() error   { return systemctl("stop", unitName) }
func (systemdBackend) Status() error { return systemctl("status", unitName) }

func systemctl(args ...string) error {
	full := append([]string{"--user"}, args...)
	cmd := exec.Command("systemctl", full...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func writeEnvFile(path string, env map[string]string) error {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var body bytes.Buffer
	body.WriteString("# DDx server environment (edit and run: ddx server stop && ddx server start)\n")
	for _, k := range keys {
		fmt.Fprintf(&body, "%s=%s\n", k, env[k])
	}
	return os.WriteFile(path, body.Bytes(), 0o600)
}

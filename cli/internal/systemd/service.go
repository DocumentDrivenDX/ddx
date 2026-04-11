// Package systemd manages the DDx server as a systemd user service.
package systemd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

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

{{range .Env}}
Environment="{{.}}"
{{end}}

[Install]
WantedBy=default.target
`

// UnitConfig holds the parameters for the ddx-server unit.
type UnitConfig struct {
	ExecPath string
	WorkDir  string
	LogPath  string
	Env      []string
}

// ServiceDir returns the systemd user unit directory.
func ServiceDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}

// Install writes the unit file and enables/starts the service.
func Install(cfg UnitConfig) error {
	serviceDir, err := ServiceDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		return err
	}
	unitPath := filepath.Join(serviceDir, "ddx-server.service")

	tmpl, err := template.New("unit").Parse(unitTemplate)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write unit file %s: %w", unitPath, err)
	}

	if err := doSystemctl("--user", "daemon-reload"); err != nil {
		return err
	}
	if err := doSystemctl("--user", "enable", "--now", "ddx-server.service"); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Installed ddx-server.service → %s\n", unitPath)
	fmt.Fprintf(os.Stdout, "  journalctl --user -u ddx-server -f  # follow logs\n")
	fmt.Fprintf(os.Stdout, "  systemctl --user status ddx-server   # status\n")
	fmt.Fprintf(os.Stdout, "  systemctl --user restart ddx-server   # restart\n")
	return nil
}

// Uninstall disables and stops the service, removing the unit file.
func Uninstall() error {
	_ = doSystemctl("--user", "disable", "--now", "ddx-server.service")
	serviceDir, err := ServiceDir()
	if err != nil {
		return nil
	}
	_ = os.Remove(filepath.Join(serviceDir, "ddx-server.service"))
	_ = doSystemctl("--user", "daemon-reload")
	fmt.Fprintln(os.Stdout, "Uninstalled ddx-server.service")
	return nil
}

// Status prints the status of the ddx-server unit.
func Status() error {
	return doSystemctl("--user", "status", "ddx-server.service")
}

func doSystemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

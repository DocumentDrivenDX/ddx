package service

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

const launchdLabel = "com.documentdrivendx.ddx-server"

type launchdBackend struct{}

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist"), nil
}

func (launchdBackend) Install(cfg Config) error {
	plistPath, err := launchAgentPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.LogPath), 0o755); err != nil {
		return err
	}

	body, err := renderLaunchdPlist(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(plistPath, body, 0o644); err != nil {
		return fmt.Errorf("write plist %s: %w", plistPath, err)
	}

	// If already loaded, unload first so the new plist takes effect.
	_ = launchctl("unload", plistPath)
	if err := launchctl("load", "-w", plistPath); err != nil {
		return fmt.Errorf("launchctl load: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Installed %s → %s\n", launchdLabel, plistPath)
	fmt.Fprintf(os.Stdout, "  tail -f %s  # follow logs\n", cfg.LogPath)
	fmt.Fprintf(os.Stdout, "  ddx server status              # status\n")
	fmt.Fprintf(os.Stdout, "  ddx server stop|start          # lifecycle\n")
	return nil
}

func (launchdBackend) Uninstall() error {
	plistPath, err := launchAgentPath()
	if err != nil {
		return nil
	}
	_ = launchctl("unload", plistPath)
	_ = os.Remove(plistPath)
	fmt.Fprintln(os.Stdout, "Uninstalled "+launchdLabel)
	return nil
}

func (launchdBackend) Start() error { return launchctl("start", launchdLabel) }
func (launchdBackend) Stop() error  { return launchctl("stop", launchdLabel) }

func (launchdBackend) Status() error {
	cmd := exec.Command("launchctl", "list", launchdLabel)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func launchctl(args ...string) error {
	cmd := exec.Command("launchctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// renderLaunchdPlist produces the LaunchAgent XML plist for ddx-server.
// KeepAlive with SuccessfulExit=false mirrors systemd's Restart=on-failure.
func renderLaunchdPlist(cfg Config) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	buf.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	buf.WriteString(`<plist version="1.0">` + "\n")
	buf.WriteString("<dict>\n")
	writePlistString(&buf, "Label", launchdLabel)
	buf.WriteString("  <key>ProgramArguments</key>\n  <array>\n")
	writePlistArrayString(&buf, cfg.ExecPath)
	writePlistArrayString(&buf, "server")
	buf.WriteString("  </array>\n")
	writePlistString(&buf, "WorkingDirectory", cfg.WorkDir)
	writePlistString(&buf, "StandardOutPath", cfg.LogPath)
	writePlistString(&buf, "StandardErrorPath", cfg.LogPath)
	buf.WriteString("  <key>RunAtLoad</key>\n  <true/>\n")
	buf.WriteString("  <key>KeepAlive</key>\n  <dict>\n    <key>SuccessfulExit</key>\n    <false/>\n  </dict>\n")

	if len(cfg.Env) > 0 {
		keys := make([]string, 0, len(cfg.Env))
		for k := range cfg.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteString("  <key>EnvironmentVariables</key>\n  <dict>\n")
		for _, k := range keys {
			fmt.Fprintf(&buf, "    <key>%s</key>\n    <string>%s</string>\n", xmlEscape(k), xmlEscape(cfg.Env[k]))
		}
		buf.WriteString("  </dict>\n")
	}

	buf.WriteString("</dict>\n</plist>\n")
	return buf.Bytes(), nil
}

func writePlistString(buf *bytes.Buffer, key, value string) {
	fmt.Fprintf(buf, "  <key>%s</key>\n  <string>%s</string>\n", xmlEscape(key), xmlEscape(value))
}

func writePlistArrayString(buf *bytes.Buffer, value string) {
	fmt.Fprintf(buf, "    <string>%s</string>\n", xmlEscape(value))
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

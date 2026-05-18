package service

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSystemdInstall_RestartsActiveServiceAfterUnitRewrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var calls [][]string
	prev := runSystemctl
	runSystemctl = func(args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		if reflect.DeepEqual(args, []string{"is-active", "--quiet", unitName}) {
			return nil
		}
		return nil
	}
	t.Cleanup(func() { runSystemctl = prev })

	cfg := Config{
		ExecPath: "/usr/local/bin/ddx",
		WorkDir:  home,
		LogPath:  filepath.Join(home, ".local", "share", "ddx", "logs", "ddx-server.log"),
	}
	if err := (systemdBackend{}).Install(cfg); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	want := [][]string{
		{"is-active", "--quiet", unitName},
		{"daemon-reload"},
		{"enable", unitName},
		{"restart", unitName},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("systemctl calls:\nwant %#v\ngot  %#v", want, calls)
	}

	unitPath := filepath.Join(home, ".config", "systemd", "user", unitName)
	body, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	unit := string(body)
	if !strings.Contains(unit, "WorkingDirectory="+home) {
		t.Fatalf("unit missing neutral working dir %q:\n%s", home, unit)
	}
	if !strings.Contains(unit, "StandardOutput=append:"+cfg.LogPath) {
		t.Fatalf("unit missing user-scoped log path %q:\n%s", cfg.LogPath, unit)
	}
}

func TestSystemdInstall_StartsInactiveServiceWithoutDoubleRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var calls [][]string
	prev := runSystemctl
	runSystemctl = func(args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		if reflect.DeepEqual(args, []string{"is-active", "--quiet", unitName}) {
			return errors.New("inactive")
		}
		return nil
	}
	t.Cleanup(func() { runSystemctl = prev })

	cfg := Config{
		ExecPath: "/usr/local/bin/ddx",
		WorkDir:  home,
		LogPath:  filepath.Join(home, ".local", "share", "ddx", "logs", "ddx-server.log"),
	}
	if err := (systemdBackend{}).Install(cfg); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	want := [][]string{
		{"is-active", "--quiet", unitName},
		{"daemon-reload"},
		{"enable", unitName},
		{"start", unitName},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("systemctl calls:\nwant %#v\ngot  %#v", want, calls)
	}
}

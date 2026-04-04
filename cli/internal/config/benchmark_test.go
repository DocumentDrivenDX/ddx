package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func BenchmarkNewValidator(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		validator, err := NewValidator()
		if err != nil {
			b.Fatalf("NewValidator failed: %v", err)
		}
		_ = validator
	}
}

func BenchmarkLoadWithWorkingDir_NoConfig(b *testing.B) {
	b.ReportAllocs()
	dir := b.TempDir()
	originalHome := os.Getenv("HOME")
	defer func() { _ = os.Setenv("HOME", originalHome) }()
	_ = os.Setenv("HOME", dir)

	for i := 0; i < b.N; i++ {
		cfg, err := LoadWithWorkingDir(dir)
		if err != nil {
			b.Fatalf("LoadWithWorkingDir failed: %v", err)
		}
		if cfg == nil {
			b.Fatal("LoadWithWorkingDir returned nil config")
		}
	}
}

func BenchmarkLoadWithWorkingDir_ConfigPresent(b *testing.B) {
	b.ReportAllocs()
	dir := b.TempDir()

	configData, err := yaml.Marshal(&Config{
		Version: "1.0",
		Library: &LibraryConfig{
			Path: "./library",
			Repository: &RepositoryConfig{
				URL:    "https://github.com/easel/ddx-library",
				Branch: "main",
			},
		},
	})
	if err != nil {
		b.Fatalf("yaml.Marshal failed: %v", err)
	}

	ddxDir := filepath.Join(dir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		b.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), configData, 0o644); err != nil {
		b.Fatalf("WriteFile failed: %v", err)
	}

	originalHome := os.Getenv("HOME")
	defer func() { _ = os.Setenv("HOME", originalHome) }()
	_ = os.Setenv("HOME", dir)

	for i := 0; i < b.N; i++ {
		cfg, err := LoadWithWorkingDir(dir)
		if err != nil {
			b.Fatalf("LoadWithWorkingDir failed: %v", err)
		}
		if cfg == nil {
			b.Fatal("LoadWithWorkingDir returned nil config")
		}
	}
}

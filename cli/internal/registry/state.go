package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// InstalledEntry records an installed package.
type InstalledEntry struct {
	Name        string      `yaml:"name"`
	Version     string      `yaml:"version"`
	Type        PackageType `yaml:"type"`
	Source      string      `yaml:"source"`
	InstalledAt time.Time   `yaml:"installed_at"`
	Files       []string    `yaml:"files"`
}

// InstalledState is the top-level structure of installed.yaml.
type InstalledState struct {
	Installed []InstalledEntry `yaml:"installed"`
}

// installedStatePath returns the path to ~/.ddx/installed.yaml.
func installedStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".ddx", "installed.yaml"), nil
}

// LoadState reads installed.yaml, returning an empty state if the file doesn't exist.
func LoadState() (*InstalledState, error) {
	path, err := installedStatePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &InstalledState{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading installed state: %w", err)
	}

	var state InstalledState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing installed state: %w", err)
	}
	return &state, nil
}

// SaveState writes installed.yaml.
func SaveState(state *InstalledState) error {
	path, err := installedStatePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating ~/.ddx directory: %w", err)
	}

	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshaling installed state: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing installed state: %w", err)
	}
	return nil
}

// VerifyFiles checks whether any recorded files for the entry exist on disk.
// Returns true if at least one file exists, false if all are missing.
func (e *InstalledEntry) VerifyFiles() bool {
	if len(e.Files) == 0 {
		return false
	}
	for _, f := range e.Files {
		expanded := ExpandHome(f)
		if _, err := os.Stat(expanded); err == nil {
			return true
		}
		// Check if it's a symlink that resolves
		if info, err := os.Lstat(expanded); err == nil && info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(expanded)
			if err == nil {
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(expanded), target)
				}
				if _, err := os.Stat(target); err == nil {
					return true
				}
			}
		}
	}
	return false
}

// FindInstalled returns the entry for name, or nil if not installed.
func (s *InstalledState) FindInstalled(name string) *InstalledEntry {
	for i := range s.Installed {
		if s.Installed[i].Name == name {
			return &s.Installed[i]
		}
	}
	return nil
}

// AddOrUpdate adds a new entry or replaces an existing one.
func (s *InstalledState) AddOrUpdate(entry InstalledEntry) {
	for i := range s.Installed {
		if s.Installed[i].Name == entry.Name {
			s.Installed[i] = entry
			return
		}
	}
	s.Installed = append(s.Installed, entry)
}

// Remove removes an entry by name. Returns false if not found.
func (s *InstalledState) Remove(name string) bool {
	for i := range s.Installed {
		if s.Installed[i].Name == name {
			s.Installed = append(s.Installed[:i], s.Installed[i+1:]...)
			return true
		}
	}
	return false
}

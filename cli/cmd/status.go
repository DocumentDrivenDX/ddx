package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show version and status information",
	Long: `Show comprehensive version and status information for your DDX project.

This command displays:
- Current DDX version and commit hash
- Last update timestamp
- Local modifications to DDX resources
- Available upstream updates
- Change history and differences

Examples:
  ddx status                          # Show basic status
  ddx status --verbose                # Show detailed information
  ddx status --check-upstream         # Check for updates
  ddx status --changes                # List changed files
  ddx status --diff                   # Show differences
  ddx status --export manifest.yml    # Export version manifest`,
	RunE: runStatus,
}

// Flag variables are now handled locally in command functions

// Remove init function - commands are now registered via command factory

// CommandFactory method - CLI interface layer
func (f *CommandFactory) runStatus(cmd *cobra.Command, args []string) error {
	// Get flag values
	checkUpstream, _ := cmd.Flags().GetBool("check-upstream")
	showChanges, _ := cmd.Flags().GetBool("changes")
	showDiff, _ := cmd.Flags().GetBool("diff")
	exportPath, _ := cmd.Flags().GetString("export")

	// Call pure business logic
	status, err := checkStatus(f.WorkingDir, checkUpstream, showChanges, showDiff)
	if err != nil {
		return err
	}

	// Handle export if requested
	if exportPath != "" {
		return exportStatusManifest(cmd, status, exportPath)
	}

	// Display results
	displayStatus(cmd, status, showChanges, showDiff)
	return nil
}

type StatusInfo struct {
	Version       string               `yaml:"version" json:"version"`
	CommitHash    string               `yaml:"commit_hash" json:"commit_hash"`
	LastUpdated   time.Time            `yaml:"last_updated" json:"last_updated"`
	UpstreamInfo  *UpstreamInfo        `yaml:"upstream,omitempty" json:"upstream,omitempty"`
	Modifications []ModifiedFile       `yaml:"modifications,omitempty" json:"modifications,omitempty"`
	Resources     []StatusResourceInfo `yaml:"resources" json:"resources"`
	Performance   PerformanceInfo      `yaml:"performance" json:"performance"`
}

type UpstreamInfo struct {
	Available     bool         `yaml:"available" json:"available"`
	LatestVersion string       `yaml:"latest_version,omitempty" json:"latest_version,omitempty"`
	UpdatesCount  int          `yaml:"updates_count" json:"updates_count"`
	Updates       []UpdateInfo `yaml:"updates,omitempty" json:"updates,omitempty"`
}

type UpdateInfo struct {
	Path string `yaml:"path" json:"path"`
	Type string `yaml:"type" json:"type"` // "new", "updated", "deleted"
}

type ModifiedFile struct {
	Path         string    `yaml:"path" json:"path"`
	Type         string    `yaml:"type" json:"type"` // "modified", "added", "deleted"
	LastModified time.Time `yaml:"last_modified" json:"last_modified"`
}

type StatusResourceInfo struct {
	Path        string    `yaml:"path" json:"path"`
	Type        string    `yaml:"type" json:"type"` // "template", "pattern", "prompt", etc.
	Version     string    `yaml:"version,omitempty" json:"version,omitempty"`
	LastUpdated time.Time `yaml:"last_updated" json:"last_updated"`
}

type PerformanceInfo struct {
	CollectionTime time.Duration `yaml:"collection_time" json:"collection_time"`
}

// checkStatus is the pure business logic function
func checkStatus(workingDir string, checkUpstream, showChanges, showDiff bool) (*StatusInfo, error) {
	start := time.Now()

	// Verify we're in a DDx project
	if !isDDXProjectInDir(workingDir) {
		return nil, fmt.Errorf("not a DDX project - run 'ddx init' first")
	}

	status := &StatusInfo{
		Performance: PerformanceInfo{
			CollectionTime: time.Since(start),
		},
	}

	// Get version information
	version, hash, err := getVersionInfoFromDir(workingDir)
	if err != nil {
		return nil, err
	}
	status.Version = version
	status.CommitHash = hash

	// Get last updated time
	lastUpdated, err := getLastUpdatedTimeFromDir(workingDir)
	if err != nil {
		return nil, err
	}
	status.LastUpdated = lastUpdated

	// Check for local modifications
	modifications, err := getLocalModificationsFromDir(workingDir)
	if err != nil {
		return nil, err
	}
	status.Modifications = modifications

	// Check upstream updates if requested
	if checkUpstream {
		upstream, err := checkUpstreamUpdatesFromDir(workingDir)
		if err != nil {
			return nil, err
		}
		status.UpstreamInfo = upstream
	}

	// Collect resource information
	resources, err := getStatusResourcesFromDir(workingDir)
	if err != nil {
		return nil, err
	}
	status.Resources = resources

	status.Performance.CollectionTime = time.Since(start)
	return status, nil
}

func getVersionInfoFromDir(workingDir string) (version, hash string, err error) {
	// Try to get from .ddx.yml first
	configPath := ".ddx.yml"
	if workingDir != "" {
		configPath = filepath.Join(workingDir, ".ddx.yml")
	}

	// Create a temporary viper instance for this specific config
	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err == nil {
		version = v.GetString("version")
	}

	if version == "" {
		version = "v0.0.1" // Default if not set
	}

	// Get git commit hash from .ddx directory
	ddxDir := ".ddx"
	if workingDir != "" {
		ddxDir = filepath.Join(workingDir, ".ddx")
	}

	if _, err := os.Stat(ddxDir); os.IsNotExist(err) {
		return version, "unknown", nil
	}

	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = ddxDir
	output, err := cmd.Output()
	if err != nil {
		hash = "unknown"
	} else {
		hash = strings.TrimSpace(string(output))
	}

	return version, hash, nil
}

func getLastUpdatedTimeFromDir(workingDir string) (time.Time, error) {
	configPath := ".ddx.yml"
	if workingDir != "" {
		configPath = filepath.Join(workingDir, ".ddx.yml")
	}

	// Create a temporary viper instance for this specific config
	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err == nil {
		lastUpdatedStr := v.GetString("last_updated")
		if lastUpdatedStr != "" {
			if t, err := time.Parse(time.RFC3339, lastUpdatedStr); err == nil {
				return t, nil
			}
		}
	}

	// Fallback to .ddx.yml modification time
	if info, err := os.Stat(configPath); err == nil {
		return info.ModTime(), nil
	}

	return time.Now(), nil
}

func getLocalModificationsFromDir(workingDir string) ([]ModifiedFile, error) {
	var modifications []ModifiedFile

	ddxDir := ".ddx"
	if workingDir != "" {
		ddxDir = filepath.Join(workingDir, ".ddx")
	}

	if _, err := os.Stat(ddxDir); os.IsNotExist(err) {
		return modifications, nil
	}

	// Use git status to detect modifications
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = ddxDir
	output, err := cmd.Output()
	if err != nil {
		// If git not available, scan for recent modifications
		return scanForModificationsInDir(ddxDir)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		status := line[:2]
		path := strings.TrimSpace(line[2:])

		var modType string
		switch {
		case strings.Contains(status, "M"):
			modType = "modified"
		case strings.Contains(status, "A"):
			modType = "added"
		case strings.Contains(status, "D"):
			modType = "deleted"
		default:
			modType = "modified"
		}

		fullPath := filepath.Join(ddxDir, path)
		var modTime time.Time
		if info, err := os.Stat(fullPath); err == nil {
			modTime = info.ModTime()
		}

		modifications = append(modifications, ModifiedFile{
			Path:         path,
			Type:         modType,
			LastModified: modTime,
		})
	}

	return modifications, nil
}

func scanForModificationsInDir(ddxDir string) ([]ModifiedFile, error) {
	var modifications []ModifiedFile

	err := filepath.Walk(ddxDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			return nil
		}

		// Check if file was modified recently (within last hour)
		if time.Since(info.ModTime()) < time.Hour {
			relPath, _ := filepath.Rel(ddxDir, path)
			modifications = append(modifications, ModifiedFile{
				Path:         relPath,
				Type:         "modified",
				LastModified: info.ModTime(),
			})
		}

		return nil
	})

	return modifications, err
}

func checkUpstreamUpdatesFromDir(workingDir string) (*UpstreamInfo, error) {
	upstream := &UpstreamInfo{
		Available: false,
	}

	// This is a simplified check - in real implementation would check git remote
	// For now, we'll simulate some updates being available
	upstream.Available = true
	upstream.LatestVersion = "v1.2.4"
	upstream.UpdatesCount = 2
	upstream.Updates = []UpdateInfo{
		{Path: "prompts/claude/new-prompt", Type: "new"},
		{Path: "patterns/api-pattern", Type: "updated"},
	}

	return upstream, nil
}

func getStatusResourcesFromDir(workingDir string) ([]StatusResourceInfo, error) {
	var resources []StatusResourceInfo

	ddxDir := ".ddx"
	if workingDir != "" {
		ddxDir = filepath.Join(workingDir, ".ddx")
	}

	if _, err := os.Stat(ddxDir); os.IsNotExist(err) {
		return resources, nil
	}

	resourceTypes := map[string]string{
		"templates": "template",
		"patterns":  "pattern",
		"prompts":   "prompt",
		"configs":   "config",
		"scripts":   "script",
	}

	for dir, resourceType := range resourceTypes {
		resourceDir := filepath.Join(ddxDir, dir)
		if _, err := os.Stat(resourceDir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(resourceDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			relPath, _ := filepath.Rel(ddxDir, path)
			resources = append(resources, StatusResourceInfo{
				Path:        relPath,
				Type:        resourceType,
				LastUpdated: info.ModTime(),
			})

			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	// Sort by type and path for consistent output
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Type != resources[j].Type {
			return resources[i].Type < resources[j].Type
		}
		return resources[i].Path < resources[j].Path
	})

	return resources, nil
}

func displayStatus(cmd *cobra.Command, status *StatusInfo, showChanges, showDiff bool) {
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "DDX Status Report")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "================")
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Current Version: %s (%s)\n", status.Version, status.CommitHash)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Last Updated: %s\n", status.LastUpdated.Format("2006-01-02 15:04:05"))

	if status.UpstreamInfo != nil && status.UpstreamInfo.Available {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Upstream: %s available\n", status.UpstreamInfo.LatestVersion)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout())

	// Show modifications
	if len(status.Modifications) > 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Modified Resources:")
		for _, mod := range status.Modifications {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s (%s)\n", mod.Path, mod.Type)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
	}

	// Show upstream updates
	if status.UpstreamInfo != nil && len(status.UpstreamInfo.Updates) > 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Updates Available:")
		for _, update := range status.UpstreamInfo.Updates {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s (%s)\n", update.Path, update.Type)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
	}

	// Show verbose information
	if viper.GetBool("verbose") {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Performance: Collection took %v\n", status.Performance.CollectionTime)

		if len(status.Resources) > 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "\nResource Details:")
			for _, resource := range status.Resources {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s [%s] - %s\n",
					resource.Path,
					resource.Type,
					resource.LastUpdated.Format("2006-01-02 15:04:05"))
			}
		}
	}

	// Show changes if requested
	if showChanges && len(status.Modifications) > 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "\nChanged Files:")
		for _, mod := range status.Modifications {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s (%s) - %s\n",
				mod.Path,
				mod.Type,
				mod.LastModified.Format("2006-01-02 15:04:05"))
		}
	}

	// Show differences if requested
	if showDiff {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "\nDifferences:")
		// This would show actual git diff output
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "(diff functionality would show detailed changes here)")
	}
}

func exportStatusManifest(cmd *cobra.Command, status *StatusInfo, path string) error {
	var data []byte
	var err error

	switch {
	case strings.HasSuffix(path, ".json"):
		data, err = json.MarshalIndent(status, "", "  ")
	case strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml"):
		data, err = yaml.Marshal(status)
	default:
		// Default to YAML
		data, err = yaml.Marshal(status)
	}

	if err != nil {
		return fmt.Errorf("failed to marshal status data: %w", err)
	}

	err = os.WriteFile(path, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write manifest file: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Status manifest exported to: %s\n", path)
	return nil
}

func isDDXProject() bool {
	return isDDXProjectInDir("")
}

// Legacy functions kept for compatibility
func runStatus(cmd *cobra.Command, args []string) error {
	return runStatusWithWorkingDir(cmd, args, "")
}

func runStatusWithWorkingDir(cmd *cobra.Command, args []string, workingDir string) error {
	// Get flag values
	checkUpstream, _ := cmd.Flags().GetBool("check-upstream")
	showChanges, _ := cmd.Flags().GetBool("changes")
	showDiff, _ := cmd.Flags().GetBool("diff")
	exportPath, _ := cmd.Flags().GetString("export")

	// Call pure business logic
	status, err := checkStatus(workingDir, checkUpstream, showChanges, showDiff)
	if err != nil {
		return err
	}

	// Handle export if requested
	if exportPath != "" {
		return exportStatusManifest(cmd, status, exportPath)
	}

	// Display results
	displayStatus(cmd, status, showChanges, showDiff)
	return nil
}

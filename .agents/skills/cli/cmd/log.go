package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// LogEntry represents a single log entry
type LogEntry struct {
	Hash    string   `json:"hash"`
	Date    string   `json:"date"`
	Author  string   `json:"author"`
	Message string   `json:"message"`
	Files   []string `json:"files,omitempty"`
	Changes string   `json:"changes,omitempty"`
}

// LogOptions contains options for retrieving log history
type LogOptions struct {
	Limit      int
	Oneline    bool
	Diff       bool
	Since      string
	Author     string
	Grep       string
	PathFilter string
	Export     string
}

// Flag variables are now handled locally in command functions

// Remove init function - commands are now registered via command factory

// CLI Interface Layer - handles UI concerns only
func runLog(cmd *cobra.Command, args []string) error {
	return runLogWithWorkingDir(cmd, args, "")
}

func (f *CommandFactory) runLog(cmd *cobra.Command, args []string) error {
	return runLogWithWorkingDir(cmd, args, f.WorkingDir)
}

func runLogWithWorkingDir(cmd *cobra.Command, args []string, workingDir string) error {
	// Extract flags - CLI interface layer responsibility
	logLimit, _ := cmd.Flags().GetInt("number")
	logLimitAlt, _ := cmd.Flags().GetInt("limit")
	logOneline, _ := cmd.Flags().GetBool("oneline")
	logDiff, _ := cmd.Flags().GetBool("diff")
	logExport, _ := cmd.Flags().GetString("export")
	logSince, _ := cmd.Flags().GetString("since")
	logAuthor, _ := cmd.Flags().GetString("author")
	logGrep, _ := cmd.Flags().GetString("grep")

	// Use --limit if specified, otherwise use --number
	if logLimitAlt != 20 { // 20 is the default, so if it's different, --limit was used
		logLimit = logLimitAlt
	}

	// Get path filter from args
	var pathFilter string
	if len(args) > 0 {
		pathFilter = args[0]
	}

	// Build options
	opts := LogOptions{
		Limit:      logLimit,
		Oneline:    logOneline,
		Diff:       logDiff,
		Since:      logSince,
		Author:     logAuthor,
		Grep:       logGrep,
		PathFilter: pathFilter,
		Export:     logExport,
	}

	// Handle export functionality
	if logExport != "" {
		return handleLogExport(workingDir, opts)
	}

	return handleLogDisplay(cmd.OutOrStdout(), cmd.ErrOrStderr(), workingDir, opts)
}

// CLI handlers - handle presentation and user interaction
func handleLogDisplay(stdout, stderr io.Writer, workingDir string, opts LogOptions) error {
	entries, err := logHistory(workingDir, opts)
	if err != nil {
		return err
	}

	// Present log entries to user
	_, _ = fmt.Fprintln(stdout, "DDX Asset History")
	_, _ = fmt.Fprintln(stdout, "================")
	_, _ = fmt.Fprintln(stdout)

	if len(entries) == 0 {
		_, _ = fmt.Fprintln(stdout, "No log entries found.")
		return nil
	}

	if opts.Oneline {
		// Compact format
		for _, entry := range entries {
			_, _ = fmt.Fprintf(stdout, "%s %s %s\n", entry.Hash[:8], entry.Date, entry.Message)
		}
	} else {
		// Detailed format
		for i, entry := range entries {
			if i > 0 {
				_, _ = fmt.Fprintln(stdout)
			}
			_, _ = fmt.Fprintf(stdout, "commit %s\n", entry.Hash)
			_, _ = fmt.Fprintf(stdout, "Date: %s\n", entry.Date)
			_, _ = fmt.Fprintf(stdout, "Author: %s\n", entry.Author)
			_, _ = fmt.Fprintf(stdout, "\n    %s\n", entry.Message)
			if opts.Diff && entry.Changes != "" {
				_, _ = fmt.Fprintf(stdout, "\n%s\n", entry.Changes)
			}
		}
	}

	return nil
}

func handleLogExport(workingDir string, opts LogOptions) error {
	entries, err := logHistory(workingDir, opts)
	if err != nil {
		return err
	}

	return exportLogEntries(entries, opts.Export)
}

// Business Logic Layer - pure functions that return data
// logHistory returns log entries from the DDX directory
func logHistory(workingDir string, opts LogOptions) ([]LogEntry, error) {
	// Determine DDX directory path
	ddxDir := ".ddx"
	if workingDir != "" {
		ddxDir = filepath.Join(workingDir, ".ddx")
	}

	// Verify DDX project exists
	if !isDDXProjectInDir(workingDir) {
		return nil, fmt.Errorf("not a DDX project - run 'ddx init' first")
	}

	if _, err := os.Stat(ddxDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("DDX directory not found - project may not be properly initialized")
	}

	// Try to get git log data first
	entries, err := getGitLogData(ddxDir, opts)
	if err != nil {
		// Fallback to file-based data
		return getFileBasedLogData(ddxDir, opts)
	}

	return entries, nil
}

// isDDXProjectInDir checks if a directory contains a DDX project
func isDDXProjectInDir(workingDir string) bool {
	if workingDir == "" {
		return isDDXProject()
	}

	ddxFile := filepath.Join(workingDir, ".ddx.yml")
	if _, err := os.Stat(ddxFile); err == nil {
		return true
	}

	ddxDir := filepath.Join(workingDir, ".ddx")
	if _, err := os.Stat(ddxDir); err == nil {
		return true
	}

	return false
}

// getGitLogData retrieves log data from git in the given directory
func getGitLogData(ddxDir string, opts LogOptions) ([]LogEntry, error) {
	// Build git log command
	args := []string{"log"}

	// Add limit
	if opts.Limit > 0 {
		args = append(args, "-n", strconv.Itoa(opts.Limit))
	}

	// Add format for structured parsing
	args = append(args, "--pretty=format:%H|%ad|%an|%s", "--date=short")

	// Add filters
	if opts.Since != "" {
		args = append(args, "--since", opts.Since)
	}

	if opts.Author != "" {
		args = append(args, "--author", opts.Author)
	}

	if opts.Grep != "" {
		args = append(args, "--grep", opts.Grep)
	}

	// Add path filter
	if opts.PathFilter != "" {
		args = append(args, "--", opts.PathFilter)
	}

	// Execute git log command
	gitCmd := exec.Command("git", args...)
	gitCmd.Dir = ddxDir
	output, err := gitCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	// Parse git log output
	entries := []LogEntry{}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) == 4 {
			entry := LogEntry{
				Hash:    parts[0],
				Date:    parts[1],
				Author:  parts[2],
				Message: parts[3],
			}

			// Add diff if requested
			if opts.Diff {
				changes, _ := getGitDiffForCommit(ddxDir, entry.Hash)
				entry.Changes = changes
			}

			entries = append(entries, entry)
		}
	}

	return entries, nil
}

// getGitDiffForCommit gets the diff for a specific commit
func getGitDiffForCommit(ddxDir, hash string) (string, error) {
	cmd := exec.Command("git", "show", "--stat", hash)
	cmd.Dir = ddxDir
	output, err := cmd.Output()
	return string(output), err
}

// getFileBasedLogData creates log entries from file modification times
func getFileBasedLogData(ddxDir string, opts LogOptions) ([]LogEntry, error) {
	entries := []LogEntry{}

	// Create a simple entry based on DDX directory
	entry := LogEntry{
		Hash:    "file-based",
		Date:    time.Now().Format("2006-01-02"),
		Author:  "DDX System",
		Message: "File-based history (git not available)",
		Files:   []string{ddxDir},
	}

	if opts.PathFilter != "" {
		entry.Message = fmt.Sprintf("File-based history for %s (git not available)", opts.PathFilter)
		entry.Files = []string{opts.PathFilter}
	}

	entries = append(entries, entry)
	return entries, nil
}

// exportLogEntries exports log entries to a file
func exportLogEntries(entries []LogEntry, exportPath string) error {
	// Determine export format from file extension
	format := "markdown" // default
	if strings.HasSuffix(exportPath, ".json") {
		format = "json"
	} else if strings.HasSuffix(exportPath, ".csv") {
		format = "csv"
	} else if strings.HasSuffix(exportPath, ".html") {
		format = "html"
	}

	// Create export file
	file, err := os.Create(exportPath)
	if err != nil {
		return fmt.Errorf("failed to create export file: %v", err)
	}
	defer func() { _ = file.Close() }()

	// Export in requested format
	switch format {
	case "json":
		return exportJSON(file, entries)
	case "csv":
		return exportCSV(file, entries)
	case "html":
		return exportHTML(file, entries)
	default:
		return exportMarkdown(file, entries)
	}
}

// Export functions - handle file output formatting
func exportMarkdown(file *os.File, entries []LogEntry) error {
	_, err := file.WriteString("# DDX Asset History\n\n")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		_, err := file.WriteString(fmt.Sprintf("## %s - %s\n\n", entry.Date, entry.Author))
		if err != nil {
			return err
		}

		_, err = file.WriteString(fmt.Sprintf("**Message:** %s\n\n", entry.Message))
		if err != nil {
			return err
		}

		if len(entry.Files) > 0 {
			_, err = file.WriteString("**Files:**\n")
			if err != nil {
				return err
			}
			for _, f := range entry.Files {
				_, err = file.WriteString(fmt.Sprintf("- %s\n", f))
				if err != nil {
					return err
				}
			}
			_, err = file.WriteString("\n")
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func exportJSON(file *os.File, entries []LogEntry) error {
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(map[string]interface{}{
		"ddx_history": entries,
		"exported_at": time.Now().Format(time.RFC3339),
	})
}

func exportCSV(file *os.File, entries []LogEntry) error {
	_, err := file.WriteString("Hash,Date,Author,Message,Files\n")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		files := strings.Join(entry.Files, ";")
		_, err := file.WriteString(fmt.Sprintf("%s,%s,%s,\"%s\",\"%s\"\n",
			entry.Hash, entry.Date, entry.Author, entry.Message, files))
		if err != nil {
			return err
		}
	}

	return nil
}

func exportHTML(file *os.File, entries []LogEntry) error {
	_, err := file.WriteString(`<!DOCTYPE html>
<html>
<head>
    <title>DDX Asset History</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .entry { border-bottom: 1px solid #ccc; margin-bottom: 20px; padding-bottom: 20px; }
        .date { color: #666; font-size: 0.9em; }
        .message { font-weight: bold; margin: 10px 0; }
        .files { background: #f5f5f5; padding: 10px; border-radius: 4px; }
    </style>
</head>
<body>
    <h1>DDX Asset History</h1>
`)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		_, err := file.WriteString(fmt.Sprintf(`
    <div class="entry">
        <div class="date">%s - %s</div>
        <div class="message">%s</div>
        <div class="files">Files: %s</div>
    </div>
`, entry.Date, entry.Author, entry.Message, strings.Join(entry.Files, ", ")))
		if err != nil {
			return err
		}
	}

	_, err = file.WriteString(`
</body>
</html>`)
	return err
}

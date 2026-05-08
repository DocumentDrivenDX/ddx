package metaprompt

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MetaPromptInjector manages meta-prompt injection into CLAUDE.md
type MetaPromptInjector interface {
	// InjectMetaPrompt injects a meta-prompt from library into CLAUDE.md
	// promptPath is relative to the library prompts/ dir (e.g., "claude/system-prompts/focused.md")
	InjectMetaPrompt(promptPath string) error

	// RemoveMetaPrompt removes the meta-prompt section from CLAUDE.md
	RemoveMetaPrompt() error

	// IsInSync checks if CLAUDE.md prompt matches library version
	// Returns true if in sync, false if out of sync or not found
	IsInSync() (bool, error)

	// GetCurrentMetaPrompt returns the currently injected prompt source path
	// Returns source path or error if not found
	GetCurrentMetaPrompt() (string, error)
}

// MetaPromptInjectorImpl implements MetaPromptInjector
type MetaPromptInjectorImpl struct {
	claudeFilePath string // Path to CLAUDE.md (typically "CLAUDE.md")
	libraryPath    string // Path to library root (typically ".ddx/plugins/ddx")
	workingDir     string // Working directory for relative path resolution
}

// Constants for marker handling (same pattern as persona system)
const (
	MetaPromptStartMarker = "<!-- DDX-META-PROMPT:START -->"
	MetaPromptEndMarker   = "<!-- DDX-META-PROMPT:END -->"
	MaxMetaPromptSize     = 1024 * 512 // 512KB max
)

// NewMetaPromptInjector creates a new injector with default paths
func NewMetaPromptInjector() MetaPromptInjector {
	return &MetaPromptInjectorImpl{
		claudeFilePath: "CLAUDE.md",
		libraryPath:    ".ddx/plugins/ddx",
		workingDir:     ".",
	}
}

// NewMetaPromptInjectorWithPaths creates an injector with custom paths
func NewMetaPromptInjectorWithPaths(claudeFile, libraryPath, workingDir string) MetaPromptInjector {
	return &MetaPromptInjectorImpl{
		claudeFilePath: claudeFile,
		libraryPath:    libraryPath,
		workingDir:     workingDir,
	}
}

// InjectMetaPrompt injects a meta-prompt from library into CLAUDE.md
func (m *MetaPromptInjectorImpl) InjectMetaPrompt(promptPath string) error {
	// 1. Validate prompt path
	if strings.TrimSpace(promptPath) == "" {
		return fmt.Errorf("prompt path cannot be empty")
	}

	// 2. Read prompt content from library
	promptFullPath := filepath.Join(m.workingDir, m.libraryPath, "prompts", promptPath)
	promptContent, err := os.ReadFile(promptFullPath)
	if err != nil {
		return fmt.Errorf("failed to read meta-prompt from %s: %w", promptFullPath, err)
	}

	// 3. Validate size
	if len(promptContent) > MaxMetaPromptSize {
		return fmt.Errorf("meta-prompt too large: %d bytes (max %d)", len(promptContent), MaxMetaPromptSize)
	}

	// 4. Read or create CLAUDE.md
	claudeFullPath := filepath.Join(m.workingDir, m.claudeFilePath)
	var claudeContent string
	if fileExists(claudeFullPath) {
		existing, err := os.ReadFile(claudeFullPath)
		if err != nil {
			return fmt.Errorf("failed to read CLAUDE.md: %w", err)
		}
		claudeContent = string(existing)
	} else {
		// Create default CLAUDE.md if doesn't exist
		claudeContent = "# CLAUDE.md\n\nThis file provides guidance to Claude when working with code in this repository."
	}

	// 5. Remove existing meta-prompt section (if any)
	claudeContent = m.removeMetaPromptSection(claudeContent)

	// 6. Build new meta-prompt section
	metaPromptSection := m.buildMetaPromptSection(string(promptContent), promptPath)

	// 7. Append meta-prompt section to CLAUDE.md
	claudeContent = strings.TrimSpace(claudeContent) + "\n\n" + metaPromptSection

	// 8. Write updated CLAUDE.md
	if err := m.saveCLAUDEFile(claudeContent); err != nil {
		return fmt.Errorf("failed to save CLAUDE.md: %w", err)
	}

	return nil
}

// RemoveMetaPrompt removes the meta-prompt section from CLAUDE.md
func (m *MetaPromptInjectorImpl) RemoveMetaPrompt() error {
	claudeFullPath := filepath.Join(m.workingDir, m.claudeFilePath)
	if !fileExists(claudeFullPath) {
		return nil // Nothing to remove
	}

	content, err := os.ReadFile(claudeFullPath)
	if err != nil {
		return fmt.Errorf("failed to read CLAUDE.md: %w", err)
	}

	// Remove meta-prompt section
	cleanContent := m.removeMetaPromptSection(string(content))

	return m.saveCLAUDEFile(cleanContent)
}

// IsInSync checks if CLAUDE.md prompt matches library version
func (m *MetaPromptInjectorImpl) IsInSync() (bool, error) {
	// 1. Read CLAUDE.md
	claudeFullPath := filepath.Join(m.workingDir, m.claudeFilePath)
	if !fileExists(claudeFullPath) {
		return false, fmt.Errorf("CLAUDE.md not found")
	}

	claudeContent, err := os.ReadFile(claudeFullPath)
	if err != nil {
		return false, fmt.Errorf("failed to read CLAUDE.md: %w", err)
	}

	// 2. Extract current meta-prompt section
	currentContent, sourcePath, err := m.extractCurrentMetaPrompt(string(claudeContent))
	if err != nil {
		return false, err
	}

	// 3. Read library prompt
	promptFullPath := filepath.Join(m.workingDir, m.libraryPath, "prompts", sourcePath)
	if !fileExists(promptFullPath) {
		// Library file missing - definitely out of sync
		return false, nil
	}

	libraryContent, err := os.ReadFile(promptFullPath)
	if err != nil {
		return false, nil
	}

	// 4. Normalize and compare
	currentNorm := normalizeWhitespace(currentContent)
	libraryNorm := normalizeWhitespace(string(libraryContent))

	return currentNorm == libraryNorm, nil
}

// GetCurrentMetaPrompt returns the currently injected prompt source path
func (m *MetaPromptInjectorImpl) GetCurrentMetaPrompt() (string, error) {
	claudeFullPath := filepath.Join(m.workingDir, m.claudeFilePath)
	if !fileExists(claudeFullPath) {
		return "", fmt.Errorf("CLAUDE.md not found")
	}

	claudeContent, err := os.ReadFile(claudeFullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read CLAUDE.md: %w", err)
	}

	_, sourcePath, err := m.extractCurrentMetaPrompt(string(claudeContent))
	if err != nil {
		return "", err
	}

	return sourcePath, nil
}

// removeMetaPromptSection removes the meta-prompt section from content
func (m *MetaPromptInjectorImpl) removeMetaPromptSection(content string) string {
	// Same algorithm as persona system (proven reliable)
	startIdx := strings.Index(content, MetaPromptStartMarker)
	if startIdx == -1 {
		return content // No section found
	}

	endIdx := strings.Index(content[startIdx:], MetaPromptEndMarker)
	if endIdx == -1 {
		// Malformed section - remove from start marker to end
		return strings.TrimSpace(content[:startIdx])
	}

	// Calculate absolute end index
	endIdx = startIdx + endIdx + len(MetaPromptEndMarker)

	// Remove the section
	before := strings.TrimRight(content[:startIdx], " \t\n")
	after := strings.TrimLeft(content[endIdx:], " \t\n")

	if before != "" && after != "" {
		return before + "\n\n" + after
	} else if before != "" {
		return before
	} else if after != "" {
		return after
	}
	return ""
}

// buildMetaPromptSection creates the meta-prompt section content
func (m *MetaPromptInjectorImpl) buildMetaPromptSection(promptContent, sourcePath string) string {
	var sections []string

	sections = append(sections, MetaPromptStartMarker)
	sections = append(sections, fmt.Sprintf("<!-- Source: %s -->", sourcePath))
	sections = append(sections, promptContent)
	sections = append(sections, MetaPromptEndMarker)

	return strings.Join(sections, "\n")
}

// extractCurrentMetaPrompt extracts the meta-prompt content and source path from CLAUDE.md
func (m *MetaPromptInjectorImpl) extractCurrentMetaPrompt(content string) (string, string, error) {
	// Find markers
	startIdx := strings.Index(content, MetaPromptStartMarker)
	if startIdx == -1 {
		return "", "", fmt.Errorf("meta-prompt section not found")
	}

	endIdx := strings.Index(content[startIdx:], MetaPromptEndMarker)
	if endIdx == -1 {
		return "", "", fmt.Errorf("malformed meta-prompt section (missing end marker)")
	}

	// Extract section
	endIdx = startIdx + endIdx
	section := content[startIdx:endIdx]

	// Extract source path from comment
	sourcePattern := `<!-- Source: (.+) -->`
	re := regexp.MustCompile(sourcePattern)
	matches := re.FindStringSubmatch(section)
	if len(matches) < 2 {
		return "", "", fmt.Errorf("source path not found in meta-prompt section")
	}
	sourcePath := strings.TrimSpace(matches[1])

	// Extract content (between source comment and end marker)
	// Find the source comment specifically (not the start marker's -->)
	sourceComment := fmt.Sprintf("<!-- Source: %s -->", sourcePath)
	sourceCommentIdx := strings.Index(section, sourceComment)
	if sourceCommentIdx == -1 {
		return "", "", fmt.Errorf("source comment not found in section")
	}
	contentStart := sourceCommentIdx + len(sourceComment)
	promptContent := strings.TrimSpace(section[contentStart:])

	return promptContent, sourcePath, nil
}

// saveCLAUDEFile saves content to CLAUDE.md with proper formatting
func (m *MetaPromptInjectorImpl) saveCLAUDEFile(content string) error {
	// Clean up trailing whitespace but preserve structure
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}

	// Remove trailing empty lines
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	cleanContent := strings.Join(lines, "\n")

	claudeFullPath := filepath.Join(m.workingDir, m.claudeFilePath)
	if err := os.WriteFile(claudeFullPath, []byte(cleanContent), 0644); err != nil {
		return fmt.Errorf("failed to write CLAUDE.md: %w", err)
	}

	return nil
}

// normalizeWhitespace removes all whitespace for comparison
func normalizeWhitespace(s string) string {
	// Remove all whitespace (handles formatting differences)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\t", "")
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

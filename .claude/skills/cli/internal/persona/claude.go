package persona

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// ClaudeInjectorImpl implements the ClaudeInjector interface
type ClaudeInjectorImpl struct {
	claudeFilePath string
}

// NewClaudeInjector creates a new Claude injector with the default CLAUDE.md path
func NewClaudeInjector() ClaudeInjector {
	return &ClaudeInjectorImpl{
		claudeFilePath: ClaudeFileName, // "CLAUDE.md" in current directory
	}
}

// NewClaudeInjectorWithPath creates a new Claude injector with a specific file path
func NewClaudeInjectorWithPath(claudeFilePath string) ClaudeInjector {
	return &ClaudeInjectorImpl{
		claudeFilePath: claudeFilePath,
	}
}

// InjectPersona injects a single persona into CLAUDE.md for the specified role
func (c *ClaudeInjectorImpl) InjectPersona(persona *Persona, role string) error {
	if persona == nil {
		return NewPersonaError(ErrorValidation, "persona cannot be nil", nil)
	}
	if strings.TrimSpace(role) == "" {
		return NewPersonaError(ErrorValidation, "role cannot be empty", nil)
	}
	if strings.TrimSpace(persona.Content) == "" {
		return NewPersonaError(ErrorValidation, "persona content cannot be empty", nil)
	}

	// Load existing personas and add/update the new one
	existingPersonas := c.getExistingPersonas()
	existingPersonas[role] = persona

	return c.InjectMultiple(existingPersonas)
}

// InjectMultiple injects multiple personas into CLAUDE.md
func (c *ClaudeInjectorImpl) InjectMultiple(personas map[string]*Persona) error {
	if personas == nil {
		return NewPersonaError(ErrorValidation, "personas map cannot be nil", nil)
	}

	// Validate personas
	for role, persona := range personas {
		if persona == nil {
			return NewPersonaError(ErrorValidation,
				fmt.Sprintf("persona for role '%s' cannot be nil", role), nil)
		}
		if strings.TrimSpace(persona.Content) == "" {
			return NewPersonaError(ErrorValidation,
				fmt.Sprintf("persona content for role '%s' cannot be empty", role), nil)
		}
	}

	// Read existing content or create default if file doesn't exist
	var content string
	if fileExists(c.claudeFilePath) {
		existingContent, err := os.ReadFile(c.claudeFilePath)
		if err != nil {
			return NewPersonaError(ErrorFileOperation,
				fmt.Sprintf("failed to read CLAUDE.md file %s", c.claudeFilePath), err)
		}
		content = string(existingContent)
	} else {
		content = "# CLAUDE.md\n\nThis file provides guidance to Claude when working with code in this repository."
	}

	// Remove existing personas section
	content = c.removePersonasSection(content)

	// If no personas to inject, just save and return
	if len(personas) == 0 {
		return c.saveClaudeFile(content)
	}

	// Add personas section
	personasSection := c.buildPersonasSection(personas)
	content = content + "\n\n" + personasSection

	return c.saveClaudeFile(content)
}

// RemovePersonas removes all personas from CLAUDE.md
func (c *ClaudeInjectorImpl) RemovePersonas() error {
	if !fileExists(c.claudeFilePath) {
		return nil // Nothing to remove
	}

	content, err := os.ReadFile(c.claudeFilePath)
	if err != nil {
		return NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to read CLAUDE.md file %s", c.claudeFilePath), err)
	}

	// Remove personas section
	cleanContent := c.removePersonasSection(string(content))

	return c.saveClaudeFile(cleanContent)
}

// GetLoadedPersonas returns the currently loaded personas as role->persona map
func (c *ClaudeInjectorImpl) GetLoadedPersonas() (map[string]string, error) {
	if !fileExists(c.claudeFilePath) {
		return make(map[string]string), nil
	}

	content, err := os.ReadFile(c.claudeFilePath)
	if err != nil {
		return nil, NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to read CLAUDE.md file %s", c.claudeFilePath), err)
	}

	return c.extractRolePersonaPairs(string(content)), nil
}

// removePersonasSection removes the personas section from the content
func (c *ClaudeInjectorImpl) removePersonasSection(content string) string {
	// Find personas section and remove it
	startMarker := PersonasStartMarker
	endMarker := PersonasEndMarker

	// Handle both proper sections and malformed ones
	startIdx := strings.Index(content, startMarker)
	if startIdx == -1 {
		return content // No personas section found
	}

	endIdx := strings.Index(content[startIdx:], endMarker)
	if endIdx == -1 {
		// Malformed section - remove from start marker to end of file
		return strings.TrimSpace(content[:startIdx])
	}

	// Calculate absolute end index
	endIdx = startIdx + endIdx + len(endMarker)

	// Remove the section
	before := content[:startIdx]
	after := content[endIdx:]

	// Clean up whitespace
	before = strings.TrimRight(before, " \t\n")
	after = strings.TrimLeft(after, " \t\n")

	if before != "" && after != "" {
		return before + "\n\n" + after
	} else if before != "" {
		return before
	} else if after != "" {
		return after
	} else {
		return ""
	}
}

// buildPersonasSection creates the personas section content
func (c *ClaudeInjectorImpl) buildPersonasSection(personas map[string]*Persona) string {
	var sections []string

	sections = append(sections, PersonasStartMarker)
	sections = append(sections, PersonasHeader)
	sections = append(sections, "")

	// Sort roles for consistent output (optional, but helpful for testing)
	roles := make([]string, 0, len(personas))
	for role := range personas {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	for _, role := range roles {
		persona := personas[role]
		roleDisplay := formatRoleDisplay(role)
		personaHeader := fmt.Sprintf("### %s: %s", roleDisplay, persona.Name)
		sections = append(sections, personaHeader)
		sections = append(sections, persona.Content)
		sections = append(sections, "")
	}

	sections = append(sections, PersonasFooter)
	sections = append(sections, PersonasEndMarker)

	return strings.Join(sections, "\n")
}

// extractRolePersonaPairs extracts role->persona mappings from the content
func (c *ClaudeInjectorImpl) extractRolePersonaPairs(content string) map[string]string {
	pairs := make(map[string]string)

	// Find personas section
	startIdx := strings.Index(content, PersonasStartMarker)
	if startIdx == -1 {
		return pairs
	}

	endIdx := strings.Index(content[startIdx:], PersonasEndMarker)
	if endIdx == -1 {
		return pairs
	}

	// Extract personas section
	endIdx = startIdx + endIdx
	personasSection := content[startIdx:endIdx]

	// Parse role and persona names using regex
	// Format: "### Role: persona-name" - must be at start of line
	personaPattern := regexp.MustCompile(`(?m)^###\s+([^:]+):\s+([^\n]+)`)
	matches := personaPattern.FindAllStringSubmatch(personasSection, -1)

	for _, match := range matches {
		if len(match) > 2 {
			role := strings.TrimSpace(match[1])
			personaName := strings.TrimSpace(match[2])
			if role != "" && personaName != "" {
				pairs[role] = personaName
			}
		}
	}

	return pairs
}

// formatRoleDisplay formats a role name for display (e.g., "code-reviewer" -> "Code Reviewer")
func formatRoleDisplay(role string) string {
	if role == "" {
		return ""
	}

	// Replace separators with spaces and title case each word
	words := regexp.MustCompile(`[-_]+`).Split(role, -1)
	var formattedWords []string

	for _, word := range words {
		if word != "" {
			// Title case: first letter uppercase, rest lowercase
			if len(word) == 1 {
				formattedWords = append(formattedWords, strings.ToUpper(word))
			} else {
				formatted := strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
				formattedWords = append(formattedWords, formatted)
			}
		}
	}

	return strings.Join(formattedWords, " ")
}

// saveClaudeFile saves content to the CLAUDE.md file
func (c *ClaudeInjectorImpl) saveClaudeFile(content string) error {
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

	if err := os.WriteFile(c.claudeFilePath, []byte(cleanContent), 0644); err != nil {
		return NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to write CLAUDE.md file %s", c.claudeFilePath), err)
	}

	return nil
}

// getExistingPersonas extracts existing personas from CLAUDE.md
func (c *ClaudeInjectorImpl) getExistingPersonas() map[string]*Persona {
	personas := make(map[string]*Persona)

	if !fileExists(c.claudeFilePath) {
		return personas
	}

	content, err := os.ReadFile(c.claudeFilePath)
	if err != nil {
		return personas
	}

	// Find personas section
	contentStr := string(content)
	startIdx := strings.Index(contentStr, PersonasStartMarker)
	if startIdx == -1 {
		return personas
	}

	endIdx := strings.Index(contentStr[startIdx:], PersonasEndMarker)
	if endIdx == -1 {
		return personas
	}

	// Extract personas section content
	endIdx = startIdx + endIdx
	personasSection := contentStr[startIdx:endIdx]

	// Find persona entries using regex (simpler pattern)
	lines := strings.Split(personasSection, "\n")
	var currentRole, currentPersona string
	var contentLines []string
	inPersona := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check if this is a persona header line
		if strings.HasPrefix(line, "### ") {
			// Save previous persona if we have one
			if inPersona && currentRole != "" && currentPersona != "" && len(contentLines) > 0 {
				content := strings.TrimSpace(strings.Join(contentLines, "\n"))
				if content != "" {
					role := formatRoleFromDisplay(currentRole)
					personas[role] = &Persona{
						Name:    currentPersona,
						Roles:   []string{role},
						Content: content,
					}
				}
			}

			// Parse new persona header
			headerText := strings.TrimPrefix(line, "### ")
			parts := strings.SplitN(headerText, ":", 2)
			if len(parts) == 2 {
				currentRole = strings.TrimSpace(parts[0])
				currentPersona = strings.TrimSpace(parts[1])
				contentLines = []string{}
				inPersona = true
			} else {
				inPersona = false
			}
		} else if inPersona && line != "" && !strings.Contains(line, "When responding, adopt the appropriate persona") {
			// Add content line
			contentLines = append(contentLines, line)
		}
	}

	// Save the last persona
	if inPersona && currentRole != "" && currentPersona != "" && len(contentLines) > 0 {
		content := strings.TrimSpace(strings.Join(contentLines, "\n"))
		if content != "" {
			role := formatRoleFromDisplay(currentRole)
			personas[role] = &Persona{
				Name:    currentPersona,
				Roles:   []string{role},
				Content: content,
			}
		}
	}

	return personas
}

// formatRoleFromDisplay converts display format back to role format
func formatRoleFromDisplay(display string) string {
	// Convert "Code Reviewer" back to "code-reviewer"
	words := strings.Fields(display)
	var roleWords []string

	for _, word := range words {
		roleWords = append(roleWords, strings.ToLower(word))
	}

	return strings.Join(roleWords, "-")
}

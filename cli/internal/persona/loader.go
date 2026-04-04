package persona

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"gopkg.in/yaml.v3"
)

// PersonaLoaderImpl implements the PersonaLoader interface
type PersonaLoaderImpl struct {
	personasDir string
}

// NewPersonaLoader creates a new persona loader with the default personas directory
func NewPersonaLoader(workingDir string) PersonaLoader {
	// Use the config system to get the library path, then append personas
	cfg, err := config.LoadWithWorkingDir(workingDir)
	var personasDir string
	if err != nil || cfg.Library == nil || cfg.Library.Path == "" {
		// Fallback to a reasonable default if there's an error
		homeDir, _ := os.UserHomeDir()
		personasDir = filepath.Join(homeDir, ".ddx", "library", "personas")
	} else {
		personasDir = filepath.Join(cfg.Library.Path, "personas")
	}

	return &PersonaLoaderImpl{
		personasDir: personasDir,
	}
}

// NewPersonaLoaderWithDir creates a new persona loader with a specific directory
func NewPersonaLoaderWithDir(dir string) PersonaLoader {
	return &PersonaLoaderImpl{
		personasDir: dir,
	}
}

// LoadPersona loads a persona by name from the file system
func (l *PersonaLoaderImpl) LoadPersona(name string) (*Persona, error) {
	if name == "" {
		return nil, NewPersonaError(ErrorValidation, "persona name cannot be empty", nil)
	}

	// Construct file path
	fileName := name + PersonaFileExtension
	filePath := filepath.Join(l.personasDir, fileName)

	// Check if file exists
	if !fileExists(filePath) {
		return nil, NewPersonaError(ErrorPersonaNotFound,
			fmt.Sprintf("persona '%s' not found at %s", name, filePath), nil)
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to read persona file %s", filePath), err)
	}

	// Check file size
	if len(content) > MaxPersonaFileSize {
		return nil, NewPersonaError(ErrorValidation,
			fmt.Sprintf("persona file %s exceeds maximum size of %d bytes", fileName, MaxPersonaFileSize), nil)
	}

	// Parse persona from content
	persona, err := parsePersona(content)
	if err != nil {
		return nil, NewPersonaError(ErrorInvalidPersona,
			fmt.Sprintf("failed to parse persona %s", name), err)
	}

	return persona, nil
}

// ListPersonas returns all available personas
func (l *PersonaLoaderImpl) ListPersonas() ([]*Persona, error) {
	if !dirExists(l.personasDir) {
		return []*Persona{}, nil // Return empty list if directory doesn't exist
	}

	// Read directory contents
	entries, err := os.ReadDir(l.personasDir)
	if err != nil {
		return nil, NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to read personas directory %s", l.personasDir), err)
	}

	var personas []*Persona

	// Process each markdown file
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .md files
		if !strings.HasSuffix(entry.Name(), PersonaFileExtension) {
			continue
		}

		// Skip README.md and other documentation files
		if entry.Name() == "README.md" || entry.Name() == "readme.md" {
			continue
		}

		// Extract persona name from filename
		personaName := strings.TrimSuffix(entry.Name(), PersonaFileExtension)

		// Load persona (this will handle validation)
		persona, err := l.LoadPersona(personaName)
		if err != nil {
			// Log warning but continue processing other personas
			_, _ = fmt.Fprintf(os.Stderr, "Warning: Skipping invalid persona %s: %v\n", entry.Name(), err)
			continue
		}

		personas = append(personas, persona)
	}

	return personas, nil
}

// FindByRole returns personas that can fulfill the specified role
func (l *PersonaLoaderImpl) FindByRole(role string) ([]*Persona, error) {
	if role == "" {
		return nil, NewPersonaError(ErrorValidation, "role cannot be empty", nil)
	}

	allPersonas, err := l.ListPersonas()
	if err != nil {
		return nil, err
	}

	var matchingPersonas []*Persona

	for _, persona := range allPersonas {
		for _, personaRole := range persona.Roles {
			if personaRole == role {
				matchingPersonas = append(matchingPersonas, persona)
				break // Avoid duplicate entries
			}
		}
	}

	return matchingPersonas, nil
}

// FindByTags returns personas that have all the specified tags
func (l *PersonaLoaderImpl) FindByTags(tags []string) ([]*Persona, error) {
	if len(tags) == 0 {
		return nil, NewPersonaError(ErrorValidation, "at least one tag must be specified", nil)
	}

	allPersonas, err := l.ListPersonas()
	if err != nil {
		return nil, err
	}

	var matchingPersonas []*Persona

	for _, persona := range allPersonas {
		if hasAllTags(persona.Tags, tags) {
			matchingPersonas = append(matchingPersonas, persona)
		}
	}

	return matchingPersonas, nil
}

// parsePersona parses a persona from markdown content with YAML frontmatter
func parsePersona(content []byte) (*Persona, error) {
	// Split frontmatter and content
	frontmatter, markdownContent, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	// Parse YAML frontmatter
	var persona Persona
	if err := yaml.Unmarshal(frontmatter, &persona); err != nil {
		return nil, NewPersonaError(ErrorInvalidPersona, "failed to parse YAML frontmatter", err)
	}

	// Set content
	persona.Content = string(markdownContent)

	// Validate required fields
	if err := validatePersona(&persona); err != nil {
		return nil, err
	}

	return &persona, nil
}

// splitFrontmatter splits YAML frontmatter from markdown content
func splitFrontmatter(content []byte) (frontmatter []byte, markdown []byte, err error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if len(lines) < 2 {
		return nil, nil, NewPersonaError(ErrorInvalidPersona, "file too short to contain frontmatter", nil)
	}

	// Check for frontmatter start
	if lines[0] != "---" {
		return nil, nil, NewPersonaError(ErrorInvalidPersona, "missing YAML frontmatter (must start with ---)", nil)
	}

	// Find frontmatter end
	frontmatterEnd := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			frontmatterEnd = i
			break
		}
	}

	if frontmatterEnd == -1 {
		return nil, nil, NewPersonaError(ErrorInvalidPersona, "unclosed YAML frontmatter (missing closing ---)", nil)
	}

	// Extract frontmatter (excluding delimiters)
	frontmatterLines := lines[1:frontmatterEnd]
	frontmatter = []byte(strings.Join(frontmatterLines, "\n"))

	// Extract markdown content (after frontmatter)
	var markdownLines []string
	if frontmatterEnd+1 < len(lines) {
		markdownLines = lines[frontmatterEnd+1:]
		// Remove leading empty lines
		for len(markdownLines) > 0 && strings.TrimSpace(markdownLines[0]) == "" {
			markdownLines = markdownLines[1:]
		}
	}

	markdown = []byte(strings.Join(markdownLines, "\n"))

	return frontmatter, markdown, nil
}

// validatePersona validates that a persona has all required fields
func validatePersona(persona *Persona) error {
	if persona.Name == "" {
		return NewPersonaError(ErrorValidation, "persona name is required", nil)
	}

	if len(persona.Roles) == 0 {
		return NewPersonaError(ErrorValidation, "persona must have at least one role", nil)
	}

	if persona.Description == "" {
		return NewPersonaError(ErrorValidation, "persona description is required", nil)
	}

	// Validate limits
	if len(persona.Roles) > MaxRolesPerPersona {
		return NewPersonaError(ErrorValidation,
			fmt.Sprintf("persona cannot have more than %d roles", MaxRolesPerPersona), nil)
	}

	if len(persona.Tags) > MaxTagsPerPersona {
		return NewPersonaError(ErrorValidation,
			fmt.Sprintf("persona cannot have more than %d tags", MaxTagsPerPersona), nil)
	}

	// Ensure tags is not nil (should be empty slice if not provided)
	if persona.Tags == nil {
		persona.Tags = []string{}
	}

	return nil
}

// hasAllTags checks if a persona has all the specified tags
func hasAllTags(personaTags []string, requiredTags []string) bool {
	personaTagMap := make(map[string]bool)
	for _, tag := range personaTags {
		personaTagMap[tag] = true
	}

	for _, requiredTag := range requiredTags {
		if !personaTagMap[requiredTag] {
			return false
		}
	}

	return true
}

// fileExists checks if a file exists
func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

// dirExists checks if a directory exists
func dirExists(dirPath string) bool {
	info, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

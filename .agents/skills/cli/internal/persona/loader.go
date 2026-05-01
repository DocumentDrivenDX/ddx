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

// PersonaLoaderImpl implements the PersonaLoader interface.
//
// A loader can own up to two source directories:
//   - libraryDir: read-only, shared across projects.
//   - projectDir: project-local (`.ddx/personas`). When a project persona has
//     the same name as a library persona, the project persona wins for that
//     project's bindings.
type PersonaLoaderImpl struct {
	// libraryDir is the legacy single directory and remains in use when the
	// loader is constructed via NewPersonaLoaderWithDir for test fixtures.
	personasDir string
	projectDir  string
}

// NewPersonaLoader creates a new persona loader rooted at the project's
// working directory. It discovers the library path from config and the
// project-local `.ddx/personas` directory.
func NewPersonaLoader(workingDir string) PersonaLoader {
	libraryDir := resolveLibraryPersonasDir(workingDir)
	projectDir := resolveProjectPersonasDir(workingDir)
	return &PersonaLoaderImpl{
		personasDir: libraryDir,
		projectDir:  projectDir,
	}
}

// NewPersonaLoaderWithDir creates a new persona loader with a specific
// library directory. Kept for tests and back-compat callers that only need
// the library source.
func NewPersonaLoaderWithDir(dir string) PersonaLoader {
	return &PersonaLoaderImpl{
		personasDir: dir,
	}
}

// NewPersonaLoaderWithDirs creates a loader with explicit library and project
// directories. Either may be empty to disable that source.
func NewPersonaLoaderWithDirs(libraryDir, projectDir string) PersonaLoader {
	return &PersonaLoaderImpl{
		personasDir: libraryDir,
		projectDir:  projectDir,
	}
}

// resolveLibraryPersonasDir resolves the library persona directory.
func resolveLibraryPersonasDir(workingDir string) string {
	cfg, err := config.LoadWithWorkingDir(workingDir)
	if err != nil || cfg.Library == nil || cfg.Library.Path == "" {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, ".ddx", "plugins", "ddx", "personas")
	}
	libPath := cfg.Library.Path
	if !filepath.IsAbs(libPath) && workingDir != "" {
		libPath = filepath.Join(workingDir, libPath)
	}
	return filepath.Join(libPath, "personas")
}

// resolveProjectPersonasDir returns the project-local persona directory.
func resolveProjectPersonasDir(workingDir string) string {
	if workingDir == "" {
		return ""
	}
	return filepath.Join(workingDir, ".ddx", "personas")
}

// LoadPersona loads a persona by name. Project-local personas override
// library personas with the same name.
func (l *PersonaLoaderImpl) LoadPersona(name string) (*Persona, error) {
	if name == "" {
		return nil, NewPersonaError(ErrorValidation, "persona name cannot be empty", nil)
	}

	if l.projectDir != "" {
		candidate := filepath.Join(l.projectDir, name+PersonaFileExtension)
		if fileExists(candidate) {
			return readPersonaFile(candidate, SourceProject)
		}
	}

	if l.personasDir != "" {
		candidate := filepath.Join(l.personasDir, name+PersonaFileExtension)
		if fileExists(candidate) {
			return readPersonaFile(candidate, SourceLibrary)
		}
	}

	return nil, NewPersonaError(ErrorPersonaNotFound,
		fmt.Sprintf("persona '%s' not found", name), nil)
}

// readPersonaFile reads a single persona file and tags it with its source.
func readPersonaFile(filePath, source string) (*Persona, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to read persona file %s", filePath), err)
	}
	if len(content) > MaxPersonaFileSize {
		return nil, NewPersonaError(ErrorValidation,
			fmt.Sprintf("persona file %s exceeds maximum size of %d bytes", filePath, MaxPersonaFileSize), nil)
	}
	persona, err := parsePersona(content)
	if err != nil {
		return nil, NewPersonaError(ErrorInvalidPersona,
			fmt.Sprintf("failed to parse persona %s", filePath), err)
	}
	persona.Source = source
	persona.FilePath = filePath
	return persona, nil
}

// ListPersonas returns all available personas. Project personas override
// library personas of the same name; both sets are listed, with the project
// persona taking precedence when names collide.
func (l *PersonaLoaderImpl) ListPersonas() ([]*Persona, error) {
	// Project personas first so they win on collision.
	byName := map[string]*Persona{}
	var ordered []string

	for _, dir := range []struct {
		path   string
		source string
	}{
		{l.projectDir, SourceProject},
		{l.personasDir, SourceLibrary},
	} {
		if dir.path == "" || !dirExists(dir.path) {
			continue
		}
		entries, err := os.ReadDir(dir.path)
		if err != nil {
			return nil, NewPersonaError(ErrorFileOperation,
				fmt.Sprintf("failed to read personas directory %s", dir.path), err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if !strings.HasSuffix(entry.Name(), PersonaFileExtension) {
				continue
			}
			if entry.Name() == "README.md" || entry.Name() == "readme.md" {
				continue
			}
			personaName := strings.TrimSuffix(entry.Name(), PersonaFileExtension)
			if _, seen := byName[personaName]; seen {
				continue
			}
			persona, err := readPersonaFile(filepath.Join(dir.path, entry.Name()), dir.source)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Warning: Skipping invalid persona %s: %v\n", entry.Name(), err)
				continue
			}
			byName[personaName] = persona
			ordered = append(ordered, personaName)
		}
	}

	result := make([]*Persona, 0, len(byName))
	for _, name := range ordered {
		result = append(result, byName[name])
	}
	return result, nil
}

// FindByRole returns personas that can fulfill the specified role.
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
				break
			}
		}
	}

	return matchingPersonas, nil
}

// FindByTags returns personas that have all the specified tags.
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

// parsePersona parses a persona from markdown content with YAML frontmatter.
func parsePersona(content []byte) (*Persona, error) {
	frontmatter, markdownContent, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	var persona Persona
	if err := yaml.Unmarshal(frontmatter, &persona); err != nil {
		return nil, NewPersonaError(ErrorInvalidPersona, "failed to parse YAML frontmatter", err)
	}

	persona.Content = string(markdownContent)

	if err := validatePersona(&persona); err != nil {
		return nil, err
	}

	return &persona, nil
}

// splitFrontmatter splits YAML frontmatter from markdown content.
func splitFrontmatter(content []byte) (frontmatter []byte, markdown []byte, err error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if len(lines) < 2 {
		return nil, nil, NewPersonaError(ErrorInvalidPersona, "file too short to contain frontmatter", nil)
	}

	if lines[0] != "---" {
		return nil, nil, NewPersonaError(ErrorInvalidPersona, "missing YAML frontmatter (must start with ---)", nil)
	}

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

	frontmatterLines := lines[1:frontmatterEnd]
	frontmatter = []byte(strings.Join(frontmatterLines, "\n"))

	var markdownLines []string
	if frontmatterEnd+1 < len(lines) {
		markdownLines = lines[frontmatterEnd+1:]
		for len(markdownLines) > 0 && strings.TrimSpace(markdownLines[0]) == "" {
			markdownLines = markdownLines[1:]
		}
	}

	markdown = []byte(strings.Join(markdownLines, "\n"))

	return frontmatter, markdown, nil
}

// validatePersona validates that a persona has all required fields.
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

	if len(persona.Roles) > MaxRolesPerPersona {
		return NewPersonaError(ErrorValidation,
			fmt.Sprintf("persona cannot have more than %d roles", MaxRolesPerPersona), nil)
	}

	if len(persona.Tags) > MaxTagsPerPersona {
		return NewPersonaError(ErrorValidation,
			fmt.Sprintf("persona cannot have more than %d tags", MaxTagsPerPersona), nil)
	}

	if persona.Tags == nil {
		persona.Tags = []string{}
	}

	return nil
}

// hasAllTags checks if a persona has all the specified tags.
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

// fileExists checks if a file exists.
func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

// dirExists checks if a directory exists.
func dirExists(dirPath string) bool {
	info, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return false
	}
	return info.IsDir()
}

package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProjectPersonaWriter owns create/update/delete operations for project-local
// personas (files under `.ddx/personas/*.md`). Library personas are never
// mutated by this type; attempting to do so returns an ErrorReadOnlyLibrary.
type ProjectPersonaWriter struct {
	workingDir string
	loader     *PersonaLoaderImpl
}

// NewProjectPersonaWriter returns a writer rooted at the given working
// directory. Passing an empty working dir returns a writer that errors on
// all calls because there is no project context.
func NewProjectPersonaWriter(workingDir string) *ProjectPersonaWriter {
	loader := &PersonaLoaderImpl{
		personasDir: resolveLibraryPersonasDir(workingDir),
		projectDir:  resolveProjectPersonasDir(workingDir),
	}
	return &ProjectPersonaWriter{workingDir: workingDir, loader: loader}
}

// Create writes a new project-local persona. Fails if a project persona
// with the same name already exists. A library persona with the same name
// is allowed (the project file will override it).
func (w *ProjectPersonaWriter) Create(name string, body string) (*Persona, error) {
	if w.workingDir == "" {
		return nil, NewPersonaError(ErrorNoProjectContext, "a project working directory is required to create a persona", nil)
	}
	if err := validateName(name); err != nil {
		return nil, err
	}
	dir := w.loader.projectDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to create project personas directory %s", dir), err)
	}
	filePath := filepath.Join(dir, name+PersonaFileExtension)
	if fileExists(filePath) {
		return nil, NewPersonaError(ErrorAlreadyExists,
			fmt.Sprintf("persona '%s' already exists at %s", name, filePath), nil)
	}
	normalized, err := normalizePersonaBody(name, body)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filePath, []byte(normalized), 0o644); err != nil {
		return nil, NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to write persona %s", filePath), err)
	}
	return readPersonaFile(filePath, SourceProject)
}

// Update overwrites an existing project-local persona. If the target
// persona exists only as a library persona, the update is rejected with
// ErrorReadOnlyLibrary.
func (w *ProjectPersonaWriter) Update(name string, body string) (*Persona, error) {
	if w.workingDir == "" {
		return nil, NewPersonaError(ErrorNoProjectContext, "a project working directory is required to update a persona", nil)
	}
	if err := validateName(name); err != nil {
		return nil, err
	}
	filePath := filepath.Join(w.loader.projectDir, name+PersonaFileExtension)
	if !fileExists(filePath) {
		// If a library persona exists, advise the user to fork first.
		libraryPath := filepath.Join(w.loader.personasDir, name+PersonaFileExtension)
		if fileExists(libraryPath) {
			return nil, NewPersonaError(ErrorReadOnlyLibrary,
				fmt.Sprintf("persona '%s' is a library persona and cannot be edited; fork it first", name), nil)
		}
		return nil, NewPersonaError(ErrorPersonaNotFound,
			fmt.Sprintf("project persona '%s' not found", name), nil)
	}
	normalized, err := normalizePersonaBody(name, body)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filePath, []byte(normalized), 0o644); err != nil {
		return nil, NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to write persona %s", filePath), err)
	}
	return readPersonaFile(filePath, SourceProject)
}

// Delete removes a project-local persona. Library personas are never
// deleted by this method.
func (w *ProjectPersonaWriter) Delete(name string) error {
	if w.workingDir == "" {
		return NewPersonaError(ErrorNoProjectContext, "a project working directory is required to delete a persona", nil)
	}
	if err := validateName(name); err != nil {
		return err
	}
	filePath := filepath.Join(w.loader.projectDir, name+PersonaFileExtension)
	if !fileExists(filePath) {
		libraryPath := filepath.Join(w.loader.personasDir, name+PersonaFileExtension)
		if fileExists(libraryPath) {
			return NewPersonaError(ErrorReadOnlyLibrary,
				fmt.Sprintf("persona '%s' is a library persona and cannot be deleted", name), nil)
		}
		return NewPersonaError(ErrorPersonaNotFound,
			fmt.Sprintf("project persona '%s' not found", name), nil)
	}
	if err := os.Remove(filePath); err != nil {
		return NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to delete persona %s", filePath), err)
	}
	return nil
}

// Fork copies a library persona into the project-local directory.
// `newName` may be empty to reuse the library name; if the target already
// exists, ErrorAlreadyExists is returned.
func (w *ProjectPersonaWriter) Fork(libraryName, newName string) (*Persona, error) {
	if w.workingDir == "" {
		return nil, NewPersonaError(ErrorNoProjectContext, "a project working directory is required to fork a persona", nil)
	}
	libraryPath := filepath.Join(w.loader.personasDir, libraryName+PersonaFileExtension)
	if !fileExists(libraryPath) {
		return nil, NewPersonaError(ErrorPersonaNotFound,
			fmt.Sprintf("library persona '%s' not found", libraryName), nil)
	}
	targetName := newName
	if targetName == "" {
		targetName = libraryName
	}
	if err := validateName(targetName); err != nil {
		return nil, err
	}
	content, err := os.ReadFile(libraryPath)
	if err != nil {
		return nil, NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to read library persona %s", libraryPath), err)
	}
	if targetName != libraryName {
		// Rewrite the frontmatter name so the forked file matches its new
		// filename and is discoverable under the new name.
		content = []byte(rewriteFrontmatterName(string(content), targetName))
	}
	if err := os.MkdirAll(w.loader.projectDir, 0o755); err != nil {
		return nil, NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to create project personas directory %s", w.loader.projectDir), err)
	}
	targetPath := filepath.Join(w.loader.projectDir, targetName+PersonaFileExtension)
	if fileExists(targetPath) {
		return nil, NewPersonaError(ErrorAlreadyExists,
			fmt.Sprintf("persona '%s' already exists at %s", targetName, targetPath), nil)
	}
	if err := os.WriteFile(targetPath, content, 0o644); err != nil {
		return nil, NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to write forked persona %s", targetPath), err)
	}
	return readPersonaFile(targetPath, SourceProject)
}

// ProjectDir returns the absolute path to the project-local persona dir.
func (w *ProjectPersonaWriter) ProjectDir() string { return w.loader.projectDir }

// validateName keeps names safe for filesystem use.
func validateName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return NewPersonaError(ErrorValidation, "persona name cannot be empty", nil)
	}
	if strings.ContainsAny(trimmed, "/\\") || strings.HasPrefix(trimmed, ".") {
		return NewPersonaError(ErrorValidation,
			fmt.Sprintf("invalid persona name %q", name), nil)
	}
	return nil
}

// normalizePersonaBody ensures the persona body has valid frontmatter and a
// name matching the file name. If the caller supplies a full markdown
// document with frontmatter, the frontmatter's name is forced to `name`.
// If the caller supplies raw body text without frontmatter, a minimal
// frontmatter block is synthesized.
func normalizePersonaBody(name string, body string) (string, error) {
	body = strings.TrimLeft(body, "\n")
	if body == "" {
		return "", NewPersonaError(ErrorValidation, "persona body cannot be empty", nil)
	}
	if !strings.HasPrefix(body, "---") {
		body = fmt.Sprintf("---\nname: %s\nroles: [general]\ndescription: %s\ntags: []\n---\n\n%s", name, name, body)
	}
	// Validate by parsing.
	parsed, err := parsePersona([]byte(body))
	if err != nil {
		return "", err
	}
	if parsed.Name != name {
		body = rewriteFrontmatterName(body, name)
	}
	return body, nil
}

// rewriteFrontmatterName rewrites the `name:` key in a persona document's
// YAML frontmatter. If no frontmatter is present, the content is returned
// unchanged.
func rewriteFrontmatterName(content string, newName string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return content
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return content
	}
	frontmatter := strings.Join(lines[1:end], "\n")
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(frontmatter), &node); err != nil {
		return content
	}
	// node.Content[0] is the document's mapping node.
	if len(node.Content) == 0 {
		return content
	}
	mapping := node.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return content
	}
	foundName := false
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "name" {
			mapping.Content[i+1].Value = newName
			foundName = true
			break
		}
	}
	if !foundName {
		mapping.Content = append([]*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "name"},
			{Kind: yaml.ScalarNode, Value: newName},
		}, mapping.Content...)
	}
	rewritten, err := yaml.Marshal(&node)
	if err != nil {
		return content
	}
	newFrontmatter := strings.TrimRight(string(rewritten), "\n")
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(newFrontmatter)
	b.WriteString("\n---\n")
	if end+1 < len(lines) {
		b.WriteString(strings.Join(lines[end+1:], "\n"))
	}
	return b.String()
}

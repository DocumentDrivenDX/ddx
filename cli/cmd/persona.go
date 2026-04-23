package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/persona"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

// Command registration is now handled by command_factory.go
// This file only contains the run function implementation

// PersonaInfo represents persona information
type PersonaInfo struct {
	Name        string
	Roles       []string
	Description string
	Tags        []string
	Content     string
	FilePath    string
	// Source is either "library" or "project".
	Source string
}

// deprecatedPersonas lists personas from the pre-consolidation roster
// (pre-FEAT-011 Phase 3) that have been removed. The map is kept as empty
// to preserve the function signatures for future use if needed.
var deprecatedPersonas = map[string]string{}

// deprecationNoticeFor returns a human-readable warning string (without
// a newline) for a deprecated persona, or empty if the name is current.
// Callers write it to stderr themselves so the caller controls framing.
func deprecationNoticeFor(name string) string {
	replacement, deprecated := deprecatedPersonas[name]
	if !deprecated {
		return ""
	}
	if replacement != "" {
		return fmt.Sprintf("warning: persona %q is deprecated and will be removed in a future release; use %q instead", name, replacement)
	}
	return fmt.Sprintf("warning: persona %q is deprecated and will be removed in a future release with no direct replacement; see library/personas/README.md for the current 5-persona roster", name)
}

// PersonaMetadata represents parsed persona frontmatter
type PersonaMetadata struct {
	Name        string   `yaml:"name"`
	Roles       []string `yaml:"roles"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
}

// PersonaBindings represents persona-role bindings
type PersonaBindings map[string]string

// PersonaStatus represents the status of active personas
type PersonaStatus struct {
	LoadedPersonas []string
	LoadedRoles    []string
	BindingsCount  int
	HasCLAUDEFile  bool
}

// =============================================================================
// CLI Interface Layer - Handles cobra.Command interactions and user output
// =============================================================================

// runPersona implements the persona command logic for CommandFactory
func (f *CommandFactory) runPersona(cmd *cobra.Command, args []string) error {
	return runPersonaWithWorkingDir(cmd, args, f.WorkingDir)
}

// runPersona implements the persona command logic
func runPersona(cmd *cobra.Command, args []string) error {
	return runPersonaWithWorkingDir(cmd, args, "")
}

// runPersonaWithWorkingDir implements the persona command logic with working directory support
func runPersonaWithWorkingDir(cmd *cobra.Command, args []string, workingDir string) error {
	// Extract flags from cobra.Command
	listFlag, _ := cmd.Flags().GetBool("list")
	showFlag, _ := cmd.Flags().GetString("show")
	bindFlag, _ := cmd.Flags().GetString("bind")
	roleFlag, _ := cmd.Flags().GetString("role")
	roleFilter, _ := cmd.Flags().GetString("role")
	tagFilter, _ := cmd.Flags().GetString("tag")

	// Handle subcommands
	if len(args) > 0 {
		switch args[0] {
		case "list":
			personas, err := personaList(workingDir, roleFilter, tagFilter)
			if err != nil {
				return err
			}
			return displayPersonaList(cmd, personas)
		case "show":
			if len(args) < 2 {
				return fmt.Errorf("persona name required")
			}
			persona, err := personaShow(workingDir, args[1])
			if err != nil {
				return err
			}
			return displayPersona(cmd, persona)
		case "bind":
			if len(args) < 3 {
				return fmt.Errorf("role and persona name required")
			}
			err := personaBind(workingDir, args[1], args[2])
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Bound role '%s' to persona '%s'\n", args[1], args[2])
			return nil
		case "new":
			if len(args) < 2 {
				return fmt.Errorf("persona name required")
			}
			bodyFlag, _ := cmd.Flags().GetString("body")
			created, err := personaNew(workingDir, args[1], bodyFlag)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Created project persona '%s' at %s\n", created.Name, created.FilePath)
			return nil
		case "edit":
			if len(args) < 2 {
				return fmt.Errorf("persona name required")
			}
			bodyFlag, _ := cmd.Flags().GetString("body")
			updated, err := personaEdit(cmd, workingDir, args[1], bodyFlag)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Updated project persona '%s' at %s\n", updated.Name, updated.FilePath)
			return nil
		case "fork":
			if len(args) < 2 {
				return fmt.Errorf("persona name required")
			}
			asFlag, _ := cmd.Flags().GetString("as")
			forked, err := personaFork(workingDir, args[1], asFlag)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Forked library persona '%s' to project persona '%s' at %s\n", args[1], forked.Name, forked.FilePath)
			return nil
		case "delete":
			if len(args) < 2 {
				return fmt.Errorf("persona name required")
			}
			if err := personaDelete(workingDir, args[1]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Deleted project persona '%s'\n", args[1])
			return nil
		case "load":
			loadedPersonas, err := personaLoad(workingDir, args[1:]...)
			if err != nil {
				return err
			}
			return displayLoadResult(cmd, args[1:], loadedPersonas)
		case "bindings":
			bindings, err := personaBindings(workingDir)
			if err != nil {
				return err
			}
			return displayBindings(cmd, bindings)
		case "status":
			status, err := personaStatus(workingDir)
			if err != nil {
				return err
			}
			return displayPersonaStatus(cmd, status)
		}
	}

	// Handle flags
	if listFlag {
		personas, err := personaList(workingDir, roleFilter, tagFilter)
		if err != nil {
			return err
		}
		return displayPersonaList(cmd, personas)
	}

	if showFlag != "" {
		persona, err := personaShow(workingDir, showFlag)
		if err != nil {
			return err
		}
		return displayPersona(cmd, persona)
	}

	if bindFlag != "" && roleFlag != "" {
		err := personaBind(workingDir, roleFlag, bindFlag)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Bound role '%s' to persona '%s'\n", roleFlag, bindFlag)
		return nil
	}

	// Show help when no flags or args provided
	return cmd.Help()
}

// displayPersonaList displays the list of personas to the user
func displayPersonaList(cmd *cobra.Command, personas []PersonaInfo) error {
	if len(personas) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No personas found")
		return nil
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Available Personas:")
	_, _ = fmt.Fprintln(cmd.OutOrStdout())

	// Create tabwriter for aligned output
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "PERSONA\tSOURCE\tROLE\tDESCRIPTION")
	_, _ = fmt.Fprintln(w, "-------\t------\t----\t-----------")

	var deprecatedSeen []string
	for _, p := range personas {
		roleStr := "general"
		if len(p.Roles) > 0 {
			roleStr = p.Roles[0]
		}
		displayName := p.Name
		if _, deprecated := deprecatedPersonas[p.Name]; deprecated {
			displayName = p.Name + " (deprecated)"
			deprecatedSeen = append(deprecatedSeen, p.Name)
		}
		source := p.Source
		if source == "" {
			source = "library"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", displayName, source, roleStr, p.Description)
	}

	_ = w.Flush()

	// Emit a single aggregated deprecation notice on stderr so piped stdout
	// stays clean. Individual notices go to stderr only when show/bind
	// targets one deprecated persona directly.
	for _, name := range deprecatedSeen {
		if notice := deprecationNoticeFor(name); notice != "" {
			_, _ = fmt.Fprintln(os.Stderr, notice)
		}
	}
	return nil
}

// displayPersona displays a single persona to the user
func displayPersona(cmd *cobra.Command, persona *PersonaInfo) error {
	if persona == nil {
		return fmt.Errorf("persona not found")
	}

	// Emit the deprecation notice to stderr before the persona body so the
	// warning is visible even when the caller pipes stdout elsewhere.
	if notice := deprecationNoticeFor(persona.Name); notice != "" {
		_, _ = fmt.Fprintln(os.Stderr, notice)
	}

	// Parse metadata from content
	metadata := parsePersonaMetadata(persona.Content)
	if metadata != nil {
		// Display formatted metadata
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Name: %s\n", metadata.Name)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Roles: %s\n", strings.Join(metadata.Roles, ", "))
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", metadata.Description)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Tags: %s\n", strings.Join(metadata.Tags, ", "))

		// Display content after frontmatter
		lines := strings.Split(persona.Content, "\n")
		contentStart := 0
		foundEnd := false
		for i, line := range lines {
			if i > 0 && line == "---" {
				contentStart = i + 1
				foundEnd = true
				break
			}
		}
		if foundEnd && contentStart < len(lines) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			_, _ = fmt.Fprint(cmd.OutOrStdout(), strings.Join(lines[contentStart:], "\n"))
		}
	} else {
		// No frontmatter, display raw content
		_, _ = fmt.Fprint(cmd.OutOrStdout(), persona.Content)
	}
	return nil
}

// displayBindings displays persona bindings to the user
func displayBindings(cmd *cobra.Command, bindings PersonaBindings) error {
	if len(bindings) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No persona bindings configured")
		return nil
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Current Persona Bindings:")
	_, _ = fmt.Fprintln(cmd.OutOrStdout())

	// Create tabwriter for aligned output
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ROLE\tPERSONA")
	_, _ = fmt.Fprintln(w, "----\t-------")

	for role, persona := range bindings {
		_, _ = fmt.Fprintf(w, "%s\t%s\n", role, persona)
	}

	_ = w.Flush()
	return nil
}

// displayPersonaStatus displays persona status to the user
func displayPersonaStatus(cmd *cobra.Command, status PersonaStatus) error {
	if !status.HasCLAUDEFile {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No CLAUDE.md file found - no personas loaded")
		return nil
	}

	if len(status.LoadedPersonas) > 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Loaded Personas:")
		for i, persona := range status.LoadedPersonas {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s (%s)\n", persona, status.LoadedRoles[i])
		}
	} else {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No personas currently loaded")
	}

	if status.BindingsCount > 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n%d persona binding(s) configured\n", status.BindingsCount)
	}

	return nil
}

// displayLoadResult displays the result of loading personas
func displayLoadResult(cmd *cobra.Command, requestedPersonas []string, loadedPersonas []string) error {
	if len(requestedPersonas) > 0 {
		// Specific personas loaded
		if len(loadedPersonas) == 1 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Loaded persona '%s' into CLAUDE.md\n", loadedPersonas[0])
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Loaded %d personas into CLAUDE.md\n", len(loadedPersonas))
		}
	} else {
		// All bound personas loaded
		if len(loadedPersonas) > 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✅ Loaded %d personas (%s) into CLAUDE.md\n",
				len(loadedPersonas), strings.Join(loadedPersonas, ", "))
		} else {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No bound personas to load")
		}
	}
	return nil
}

// =============================================================================
// Business Logic Layer - Pure functions that operate on working directory
// =============================================================================

// personaList returns a list of available personas, merging the library
// directory and project-local `.ddx/personas`. Project personas override
// library personas on name collision.
func personaList(workingDir string, roleFilter, tagFilter string) ([]PersonaInfo, error) {
	libPath, err := getPersonaLibraryPath(workingDir)
	libraryDir := ""
	if err == nil {
		libraryDir = filepath.Join(libPath, "personas")
	}
	projectDir := ""
	if workingDir != "" {
		projectDir = filepath.Join(workingDir, ".ddx", "personas")
	}

	seen := map[string]bool{}
	var personas []PersonaInfo

	// Project first so it wins on collision.
	for _, src := range []struct {
		dir    string
		source string
	}{
		{projectDir, persona.SourceProject},
		{libraryDir, persona.SourceLibrary},
	} {
		if src.dir == "" {
			continue
		}
		entries, err := os.ReadDir(src.dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read personas directory %s: %w", src.dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".md")
			if seen[name] {
				continue
			}
			filePath := filepath.Join(src.dir, entry.Name())
			content, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}
			metadata := parsePersonaMetadata(string(content))
			if metadata == nil {
				metadata = &PersonaMetadata{Name: name, Roles: []string{"general"}, Description: name}
			}
			if roleFilter != "" {
				hasRole := false
				for _, role := range metadata.Roles {
					if role == roleFilter {
						hasRole = true
						break
					}
				}
				if !hasRole {
					continue
				}
			}
			if tagFilter != "" {
				hasTag := false
				for _, tag := range metadata.Tags {
					if tag == tagFilter {
						hasTag = true
						break
					}
				}
				if !hasTag {
					continue
				}
			}
			personas = append(personas, PersonaInfo{
				Name:        name,
				Roles:       metadata.Roles,
				Description: metadata.Description,
				Tags:        metadata.Tags,
				Content:     string(content),
				FilePath:    filePath,
				Source:      src.source,
			})
			seen[name] = true
		}
	}

	return personas, nil
}

// personaShow returns detailed information about a specific persona,
// preferring the project-local file when a name collision exists.
func personaShow(workingDir string, personaName string) (*PersonaInfo, error) {
	candidates := []struct {
		path   string
		source string
	}{}
	if workingDir != "" {
		candidates = append(candidates, struct {
			path   string
			source string
		}{filepath.Join(workingDir, ".ddx", "personas", personaName+".md"), persona.SourceProject})
	}
	if libPath, err := getPersonaLibraryPath(workingDir); err == nil {
		candidates = append(candidates, struct {
			path   string
			source string
		}{filepath.Join(libPath, "personas", personaName+".md"), persona.SourceLibrary})
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate.path); err == nil {
			content, err := os.ReadFile(candidate.path)
			if err != nil {
				return nil, fmt.Errorf("failed to read persona: %w", err)
			}
			metadata := parsePersonaMetadata(string(content))
			if metadata == nil {
				metadata = &PersonaMetadata{Name: personaName, Roles: []string{"general"}, Description: personaName}
			}
			return &PersonaInfo{
				Name:        personaName,
				Roles:       metadata.Roles,
				Description: metadata.Description,
				Tags:        metadata.Tags,
				Content:     string(content),
				FilePath:    candidate.path,
				Source:      candidate.source,
			}, nil
		}
	}
	return nil, fmt.Errorf("persona '%s' not found", personaName)
}

// personaBind binds a role to a persona. Project-local personas satisfy
// existence checks alongside library personas.
func personaBind(workingDir string, role, personaName string) error {
	projectPath := ""
	if workingDir != "" {
		projectPath = filepath.Join(workingDir, ".ddx", "personas", personaName+".md")
	}
	libPath, err := getPersonaLibraryPath(workingDir)
	if err != nil {
		return fmt.Errorf("failed to get library path: %w", err)
	}
	libraryPath := filepath.Join(libPath, "personas", personaName+".md")

	if projectPath != "" {
		if _, perr := os.Stat(projectPath); perr == nil {
			goto foundPersona
		}
	}
	if _, perr := os.Stat(libraryPath); perr != nil {
		return fmt.Errorf("persona '%s' not found (project: %s, library: %s)", personaName, projectPath, libraryPath)
	}
foundPersona:

	// Emit a stderr deprecation warning when binding a role to a deprecated
	// persona. Non-fatal — users migrate on their own timeline during the
	// deprecation window.
	if notice := deprecationNoticeFor(personaName); notice != "" {
		_, _ = fmt.Fprintln(os.Stderr, notice)
	}

	// Load only the local config file to preserve structure
	configPath := ".ddx/config.yaml"
	if workingDir != "" {
		configPath = filepath.Join(workingDir, ".ddx/config.yaml")
	}

	// Read current config as raw YAML node to preserve structure
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var rootNode yaml.Node
	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Find or create persona_bindings section
	if err := addPersonaBindingToNode(&rootNode, role, personaName); err != nil {
		return fmt.Errorf("failed to add persona binding: %w", err)
	}

	// Write back to file
	newData, err := yaml.Marshal(&rootNode)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, newData, 0644); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", configPath, err)
	}

	return nil
}

// personaBindings returns the current persona bindings
func personaBindings(workingDir string) (PersonaBindings, error) {
	// Check if config file exists first (new format)
	configPath := ".ddx/config.yaml"
	if workingDir != "" {
		configPath = filepath.Join(workingDir, ".ddx/config.yaml")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("No .ddx/config.yaml configuration found")
	}

	// Load config
	cfg, err := loadPersonaConfig(workingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	if cfg.PersonaBindings == nil {
		return PersonaBindings{}, nil
	}

	return PersonaBindings(cfg.PersonaBindings), nil
}

// personaStatus returns the status of active personas
func personaStatus(workingDir string) (PersonaStatus, error) {
	claudePath := "CLAUDE.md"
	if workingDir != "" {
		claudePath = filepath.Join(workingDir, "CLAUDE.md")
	}

	status := PersonaStatus{
		HasCLAUDEFile: false,
	}

	// Check if CLAUDE.md exists
	if _, err := os.Stat(claudePath); os.IsNotExist(err) {
		return status, nil
	}

	status.HasCLAUDEFile = true

	// Read CLAUDE.md
	content, err := os.ReadFile(claudePath)
	if err != nil {
		return status, fmt.Errorf("failed to read CLAUDE.md: %w", err)
	}

	// Parse loaded personas
	claudeStr := string(content)
	if strings.Contains(claudeStr, "<!-- PERSONAS:START -->") &&
		strings.Contains(claudeStr, "<!-- PERSONAS:END -->") {
		// Extract persona section
		startIdx := strings.Index(claudeStr, "<!-- PERSONAS:START -->")
		endIdx := strings.Index(claudeStr, "<!-- PERSONAS:END -->")
		if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
			personaSection := claudeStr[startIdx:endIdx]

			// Parse loaded personas
			lines := strings.Split(personaSection, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "### ") {
					// Parse role and persona from header
					header := strings.TrimPrefix(line, "### ")
					parts := strings.Split(header, ": ")
					if len(parts) == 2 {
						status.LoadedRoles = append(status.LoadedRoles, parts[0])
						status.LoadedPersonas = append(status.LoadedPersonas, parts[1])
					}
				}
			}
		}
	}

	// Get bindings count
	cfg, err := loadPersonaConfig(workingDir)
	if err == nil && cfg.PersonaBindings != nil {
		status.BindingsCount = len(cfg.PersonaBindings)
	}

	return status, nil
}

// personaLoad loads personas into CLAUDE.md
func personaLoad(workingDir string, personas ...string) ([]string, error) {
	// Always check if config file exists (new format)
	configPath := ".ddx/config.yaml"
	if workingDir != "" {
		configPath = filepath.Join(workingDir, ".ddx/config.yaml")
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("No .ddx/config.yaml configuration found")
	}

	// Load config to get persona bindings
	cfg, err := loadPersonaConfig(workingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Get library path
	libPath, err := getPersonaLibraryPath(workingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get library path: %w", err)
	}

	// Read CLAUDE.md if it exists
	claudePath := "CLAUDE.md"
	if workingDir != "" {
		claudePath = filepath.Join(workingDir, "CLAUDE.md")
	}

	var claudeContent string
	if data, err := os.ReadFile(claudePath); err == nil {
		claudeContent = string(data)
	} else {
		// Create new CLAUDE.md
		claudeContent = "# CLAUDE.md\n\nProject guidance for my application."
	}

	// Remove existing persona section if present
	startMarker := "<!-- PERSONAS:START -->"
	endMarker := "<!-- PERSONAS:END -->"
	startIdx := strings.Index(claudeContent, startMarker)
	if startIdx != -1 {
		endIdx := strings.Index(claudeContent, endMarker)
		if endIdx != -1 {
			claudeContent = claudeContent[:startIdx] + claudeContent[endIdx+len(endMarker):]
		}
	}

	// Build persona content
	var personaSection strings.Builder
	personaSection.WriteString("\n" + startMarker + "\n")
	personaSection.WriteString("## Active Personas\n\n")

	// Track loaded personas
	loadedPersonas := []string{}

	// If specific personas requested, load those; otherwise load all bound personas
	if len(personas) > 0 {
		// Load specific personas
		for _, personaName := range personas {
			personaPath := filepath.Join(libPath, "personas", personaName+".md")
			if content, err := os.ReadFile(personaPath); err == nil {
				// Validate persona content if it has frontmatter
				if err := validatePersonaContent(string(content), personaName); err != nil {
					return nil, err
				}
				// Just add the content - personas have their own titles
				personaSection.WriteString(string(content) + "\n")
				loadedPersonas = append(loadedPersonas, personaName)
			} else if os.IsNotExist(err) {
				return nil, fmt.Errorf("persona '%s' not found", personaName)
			}
		}
	} else {
		// Load all bound personas from config
		if cfg.PersonaBindings != nil {
			for role, personaName := range cfg.PersonaBindings {
				personaPath := filepath.Join(libPath, "personas", personaName+".md")
				if content, err := os.ReadFile(personaPath); err == nil {
					// Validate persona content if it has frontmatter
					if err := validatePersonaContent(string(content), personaName); err != nil {
						return nil, err
					}
					// Add role header with proper capitalization
					caser := cases.Title(language.English)
					capitalizedRole := caser.String(strings.ReplaceAll(role, "-", " "))
					personaSection.WriteString(fmt.Sprintf("### %s: %s\n", capitalizedRole, personaName))
					personaSection.WriteString(string(content) + "\n")
					loadedPersonas = append(loadedPersonas, personaName)
				}
			}
		}
	}

	personaSection.WriteString(endMarker + "\n")

	// Append persona section to CLAUDE.md
	claudeContent += personaSection.String()

	// Write updated CLAUDE.md
	if err := os.WriteFile(claudePath, []byte(claudeContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write CLAUDE.md: %w", err)
	}

	return loadedPersonas, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// parsePersonaMetadata parses YAML frontmatter from persona content
func parsePersonaMetadata(content string) *PersonaMetadata {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return nil
	}

	// Find end of frontmatter
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			endIdx = i
			break
		}
	}

	if endIdx == -1 {
		return nil
	}

	// Parse YAML frontmatter
	frontmatter := strings.Join(lines[1:endIdx], "\n")
	var metadata PersonaMetadata
	if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil {
		return nil
	}

	return &metadata
}

// validatePersonaContent validates persona content structure
func validatePersonaContent(content, personaName string) error {
	if strings.HasPrefix(content, "---\n") {
		// Try to parse the frontmatter to validate it
		lines := strings.Split(content, "\n")
		endIdx := -1
		for i := 1; i < len(lines); i++ {
			if lines[i] == "---" {
				endIdx = i
				break
			}
		}
		if endIdx > 0 {
			frontmatter := strings.Join(lines[1:endIdx], "\n")
			var metadata PersonaMetadata
			if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil {
				return fmt.Errorf("failed to parse YAML frontmatter in persona '%s': %w", personaName, err)
			}
		}
	}
	return nil
}

// getPersonaLibraryPath gets library path with working directory context for persona operations
func getPersonaLibraryPath(workingDir string) (string, error) {
	// Use the config system with working directory parameter
	if workingDir == "" {
		return "", fmt.Errorf("working directory must be provided")
	}

	// Use the library path resolution from config
	cfg, err := config.LoadWithWorkingDir(workingDir)
	if err != nil {
		return "", err
	}

	if cfg.Library != nil {
		libPath := cfg.Library.Path
		// If path is relative, resolve it relative to working directory
		if !filepath.IsAbs(libPath) {
			libPath = filepath.Join(workingDir, libPath)
		}
		return libPath, nil
	}
	return "", fmt.Errorf("library path not configured")
}

// loadPersonaConfig loads config with working directory context for persona operations
func loadPersonaConfig(workingDir string) (*config.Config, error) {
	return config.LoadWithWorkingDir(workingDir)
}

// savePersonaConfig saves config to working directory for persona operations
func savePersonaConfig(workingDir string, cfg *config.Config) error {
	// Create .ddx directory if it doesn't exist
	ddxDir := ".ddx"
	if workingDir != "" {
		ddxDir = filepath.Join(workingDir, ".ddx")
	}
	if err := os.MkdirAll(ddxDir, 0755); err != nil {
		return fmt.Errorf("failed to create .ddx directory: %w", err)
	}

	configPath := filepath.Join(ddxDir, "config.yaml")

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}

// personaNew creates a new project-local persona. If bodyFile is empty, a
// scaffold is generated from the default template.
func personaNew(workingDir, name, bodyFile string) (*persona.Persona, error) {
	if workingDir == "" {
		return nil, fmt.Errorf("working directory must be provided")
	}
	body, err := readBodyInput(bodyFile, name)
	if err != nil {
		return nil, err
	}
	writer := persona.NewProjectPersonaWriter(workingDir)
	return writer.Create(name, body)
}

// personaEdit updates a project-local persona. If bodyFile is empty, it
// opens $EDITOR on the file.
func personaEdit(cmd *cobra.Command, workingDir, name, bodyFile string) (*persona.Persona, error) {
	if workingDir == "" {
		return nil, fmt.Errorf("working directory must be provided")
	}
	writer := persona.NewProjectPersonaWriter(workingDir)

	if bodyFile != "" {
		body, err := os.ReadFile(bodyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read body file %s: %w", bodyFile, err)
		}
		return writer.Update(name, string(body))
	}

	// Interactive edit: open $EDITOR on the project file in place, then
	// re-read to validate.
	projectFile := filepath.Join(writer.ProjectDir(), name+".md")
	if _, err := os.Stat(projectFile); os.IsNotExist(err) {
		// Surface the same error the writer would.
		if _, err := writer.Update(name, ""); err != nil {
			return nil, err
		}
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	editCmd := exec.Command(editor, projectFile) // #nosec G204 — intentional $EDITOR invocation
	editCmd.Stdin = cmd.InOrStdin()
	editCmd.Stdout = cmd.OutOrStdout()
	editCmd.Stderr = cmd.ErrOrStderr()
	if err := editCmd.Run(); err != nil {
		return nil, fmt.Errorf("editor exited with error: %w", err)
	}
	data, err := os.ReadFile(projectFile)
	if err != nil {
		return nil, fmt.Errorf("failed to re-read persona after edit: %w", err)
	}
	return writer.Update(name, string(data))
}

// personaFork copies a library persona into the project-local dir.
func personaFork(workingDir, libraryName, newName string) (*persona.Persona, error) {
	if workingDir == "" {
		return nil, fmt.Errorf("working directory must be provided")
	}
	writer := persona.NewProjectPersonaWriter(workingDir)
	return writer.Fork(libraryName, newName)
}

// personaDelete removes a project-local persona.
func personaDelete(workingDir, name string) error {
	if workingDir == "" {
		return fmt.Errorf("working directory must be provided")
	}
	writer := persona.NewProjectPersonaWriter(workingDir)
	return writer.Delete(name)
}

// readBodyInput resolves the body for `ddx persona new`. If the caller
// supplies a path, read it; otherwise fall back to the scaffold template.
func readBodyInput(bodyFile, name string) (string, error) {
	if bodyFile == "" {
		return scaffoldPersonaBody(name), nil
	}
	data, err := os.ReadFile(bodyFile)
	if err != nil {
		return "", fmt.Errorf("failed to read body file %s: %w", bodyFile, err)
	}
	return string(data), nil
}

// scaffoldPersonaBody returns a minimal but valid persona markdown body.
func scaffoldPersonaBody(name string) string {
	return fmt.Sprintf(`---
name: %s
roles: [general]
description: Project persona %s
tags: []
---

# %s

TODO: describe what this persona does for your team.
`, name, name, name)
}

// IsLibraryReadOnly returns true if err represents a read-only library
// persona rejection from the persona package.
func IsLibraryReadOnly(err error) bool {
	var pe *persona.PersonaError
	if errors.As(err, &pe) {
		return pe.Type == persona.ErrorReadOnlyLibrary
	}
	return false
}

// addPersonaBindingToNode adds or updates a persona binding in a YAML node tree
func addPersonaBindingToNode(rootNode *yaml.Node, role, personaName string) error {
	// Find the document node
	var docNode *yaml.Node
	if rootNode.Kind == yaml.DocumentNode && len(rootNode.Content) > 0 {
		docNode = rootNode.Content[0]
	} else if rootNode.Kind == yaml.MappingNode {
		docNode = rootNode
	} else {
		return fmt.Errorf("invalid YAML structure")
	}

	if docNode.Kind != yaml.MappingNode {
		return fmt.Errorf("root node is not a mapping")
	}

	// Look for existing persona_bindings key
	for i := 0; i < len(docNode.Content); i += 2 {
		keyNode := docNode.Content[i]
		if keyNode.Value == "persona_bindings" {
			// Found existing persona_bindings, update it
			valueNode := docNode.Content[i+1]
			if valueNode.Kind != yaml.MappingNode {
				// Convert to mapping node
				valueNode.Kind = yaml.MappingNode
				valueNode.Content = []*yaml.Node{}
			}

			// Look for existing role or add new one
			found := false
			for j := 0; j < len(valueNode.Content); j += 2 {
				roleKeyNode := valueNode.Content[j]
				if roleKeyNode.Value == role {
					// Update existing role
					valueNode.Content[j+1].Value = personaName
					found = true
					break
				}
			}

			if !found {
				// Add new role-persona pair
				roleKeyNode := &yaml.Node{
					Kind:  yaml.ScalarNode,
					Value: role,
				}
				personaValueNode := &yaml.Node{
					Kind:  yaml.ScalarNode,
					Value: personaName,
				}
				valueNode.Content = append(valueNode.Content, roleKeyNode, personaValueNode)
			}
			return nil
		}
	}

	// persona_bindings key not found, add it
	bindingsKeyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: "persona_bindings",
	}

	roleKeyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: role,
	}
	personaValueNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: personaName,
	}

	bindingsValueNode := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: []*yaml.Node{roleKeyNode, personaValueNode},
	}

	docNode.Content = append(docNode.Content, bindingsKeyNode, bindingsValueNode)

	return nil
}

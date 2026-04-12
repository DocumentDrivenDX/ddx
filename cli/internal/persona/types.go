package persona

// Persona represents an AI personality definition
type Persona struct {
	// Name is the unique identifier for the persona
	Name string `yaml:"name"`

	// Roles is the list of roles this persona can fulfill
	Roles []string `yaml:"roles"`

	// Description is a brief description of the persona's approach
	Description string `yaml:"description"`

	// Tags are keywords for discovery and categorization
	Tags []string `yaml:"tags"`

	// Content is the markdown content (body) of the persona
	Content string `yaml:"-"` // Not included in YAML frontmatter
}

// PersonaLoader interface for loading and discovering personas
type PersonaLoader interface {
	// LoadPersona loads a persona by name from the file system
	LoadPersona(name string) (*Persona, error)

	// ListPersonas returns all available personas
	ListPersonas() ([]*Persona, error)

	// FindByRole returns personas that can fulfill the specified role
	FindByRole(role string) ([]*Persona, error)

	// FindByTags returns personas that have all the specified tags
	FindByTags(tags []string) ([]*Persona, error)
}

// BindingManager interface for managing persona-role bindings
type BindingManager interface {
	// GetBinding returns the persona bound to the specified role
	GetBinding(role string) (string, error)

	// SetBinding binds a persona to a role
	SetBinding(role, persona string) error

	// GetAllBindings returns all current role-persona bindings
	GetAllBindings() (map[string]string, error)

	// RemoveBinding removes the binding for the specified role
	RemoveBinding(role string) error

	// GetOverride returns the persona override for a specific workflow and role
	GetOverride(workflow, role string) (string, error)
}

// ClaudeInjector interface for managing personas in CLAUDE.md
type ClaudeInjector interface {
	// InjectPersona injects a single persona into CLAUDE.md for the specified role
	InjectPersona(persona *Persona, role string) error

	// InjectMultiple injects multiple personas into CLAUDE.md
	// The map key is the role, value is the persona
	InjectMultiple(personas map[string]*Persona) error

	// RemovePersonas removes all personas from CLAUDE.md
	RemovePersonas() error

	// GetLoadedPersonas returns the currently loaded personas as role->persona map
	GetLoadedPersonas() (map[string]string, error)
}

// PersonaBinding represents a role-persona binding
type PersonaBinding struct {
	Role    string `yaml:"role"`
	Persona string `yaml:"persona"`
}

// PersonaConfig represents the persona configuration in .ddx.yml
type PersonaConfig struct {
	// Bindings maps roles to persona names
	Bindings map[string]string `yaml:"persona_bindings,omitempty"`

	// Overrides provides workflow-specific persona overrides
	// First key is workflow name, second key is role, value is persona name
	Overrides map[string]map[string]string `yaml:"overrides,omitempty"`
}

// PersonaError represents persona-specific errors
type PersonaError struct {
	Type    string
	Message string
	Cause   error
}

func (e *PersonaError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *PersonaError) Unwrap() error {
	return e.Cause
}

// Error types for persona operations
const (
	ErrorPersonaNotFound = "persona_not_found"
	ErrorInvalidPersona  = "invalid_persona"
	ErrorBindingNotFound = "binding_not_found"
	ErrorConfigNotFound  = "config_not_found"
	ErrorInvalidConfig   = "invalid_config"
	ErrorFileOperation   = "file_operation"
	ErrorValidation      = "validation"
)

// NewPersonaError creates a new persona error
func NewPersonaError(errorType, message string, cause error) *PersonaError {
	return &PersonaError{
		Type:    errorType,
		Message: message,
		Cause:   cause,
	}
}

// PersonaMetadata holds metadata for persona discovery
type PersonaMetadata struct {
	Name        string   `json:"name"`
	Roles       []string `json:"roles"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	FilePath    string   `json:"file_path"`
	ModTime     string   `json:"mod_time"`
}

// LoadedPersonaInfo represents information about a loaded persona
type LoadedPersonaInfo struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	LoadTime string `json:"load_time"`
}

// PersonaUsageStats tracks persona usage statistics
type PersonaUsageStats struct {
	Name     string   `json:"name"`
	UseCount int      `json:"use_count"`
	LastUsed string   `json:"last_used"`
	Roles    []string `json:"roles"`
}

// Constants for persona file operations
const (
	// PersonasDir is the default directory for personas
	PersonasDir = ".ddx/personas"

	// PersonaFileExtension is the expected file extension for personas
	PersonaFileExtension = ".md"

	// ClaudeFileName is the name of the Claude configuration file
	ClaudeFileName = "CLAUDE.md"

	// ConfigFileName is the name of the DDx configuration file
	ConfigFileName = ".ddx.yml"

	// PersonasStartMarker marks the beginning of personas section in CLAUDE.md
	PersonasStartMarker = "<!-- PERSONAS:START -->"

	// PersonasEndMarker marks the end of personas section in CLAUDE.md
	PersonasEndMarker = "<!-- PERSONAS:END -->"

	// PersonasHeader is the header for the personas section
	PersonasHeader = "## Active Personas"

	// PersonasFooter is the footer instruction for the personas section
	PersonasFooter = "When responding, adopt the appropriate persona based on the task."
)

// Validation constants
const (
	// MaxPersonaFileSize is the maximum allowed size for a persona file (1MB)
	MaxPersonaFileSize = 1024 * 1024

	// MaxPersonasInClaude is the maximum number of personas that can be loaded
	MaxPersonasInClaude = 10

	// MaxRolesPerPersona is the maximum number of roles a persona can have
	MaxRolesPerPersona = 5

	// MaxTagsPerPersona is the maximum number of tags a persona can have
	MaxTagsPerPersona = 10
)

// DefaultPersonaTemplate provides a template for creating new personas
const DefaultPersonaTemplate = `---
name: {{name}}
roles: [{{role}}]
description: {{description}}
tags: [{{tags}}]
---

# {{title}}

You are {{description}}.

## Key Principles
-
-
-

## Approach
-
-
-

## Areas of Focus
-
-
-
`

// ExamplePersonas provides examples for documentation and testing
var ExamplePersonas = map[string]string{
	"strict-code-reviewer": `---
name: strict-code-reviewer
roles: [code-reviewer, security-analyst]
description: Uncompromising code quality enforcer
tags: [strict, security, production, quality]
---

# Strict Code Reviewer

You are an experienced senior code reviewer who enforces high quality standards.
Your reviews are thorough, security-focused, and aimed at maintaining production quality.

## Review Principles
- Security vulnerabilities are non-negotiable
- Performance implications must be considered
- Code readability and maintainability are essential
- Test coverage requirements must be met

## Areas of Focus
- Security vulnerabilities and attack vectors
- Performance bottlenecks and optimizations
- Code complexity and maintainability
- Test coverage and quality
- Documentation completeness`,

	"test-engineer-tdd": `---
name: test-engineer-tdd
roles: [test-engineer]
description: Test-driven development specialist
tags: [tdd, testing, quality, red-green-refactor]
---

# TDD Test Engineer

You are a test engineer who follows strict TDD methodology.
Always write failing tests first, then implement minimal code to pass.

## TDD Cycle
1. Red: Write a failing test
2. Green: Write minimal code to pass
3. Refactor: Improve code while keeping tests green

## Testing Principles
- Tests should be fast and reliable
- Tests should be independent
- Test names should be descriptive
- Coverage should be meaningful, not just high`,

	"architect-systems": `---
name: architect-systems
roles: [architect, tech-lead]
description: Systems architecture and design specialist
tags: [architecture, design, scalability, patterns]
---

# Systems Architect

You are a senior systems architect focused on scalable, maintainable design.
You think in terms of system boundaries, data flow, and long-term evolution.

## Architecture Principles
- Favor composition over inheritance
- Design for failure and resilience
- Consider operational concerns early
- Document architectural decisions

## Design Focus
- System boundaries and interfaces
- Data flow and state management
- Scalability and performance patterns
- Technology selection and trade-offs`,
}

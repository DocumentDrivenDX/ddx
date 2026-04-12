package persona

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFixtures provides common test data and setup utilities for persona tests
type TestFixtures struct {
	HomeDir     string
	WorkDir     string
	PersonasDir string
	ConfigPath  string
	ClaudePath  string
}

// SetupTestEnvironment creates a complete test environment for persona testing
func SetupTestEnvironment(t *testing.T) *TestFixtures {
	// Create temporary directories
	homeDir := t.TempDir()
	workDir := t.TempDir()

	// Set environment
	t.Setenv("HOME", homeDir)
	// Don't change working directory - tests should use absolute paths

	// Create personas directory
	personasDir := filepath.Join(homeDir, ".ddx", "personas")
	require.NoError(t, os.MkdirAll(personasDir, 0755))

	fixtures := &TestFixtures{
		HomeDir:     homeDir,
		WorkDir:     workDir,
		PersonasDir: personasDir,
		ConfigPath:  filepath.Join(workDir, ".ddx.yml"),
		ClaudePath:  filepath.Join(workDir, "CLAUDE.md"),
	}

	return fixtures
}

// CreateTestPersonas creates a standard set of test personas
func (f *TestFixtures) CreateTestPersonas(t *testing.T) {
	personas := GetTestPersonas()

	for filename, content := range personas {
		personaPath := filepath.Join(f.PersonasDir, filename)
		require.NoError(t, os.WriteFile(personaPath, []byte(content), 0644))
	}
}

// CreateTestConfig creates a test .ddx.yml configuration
func (f *TestFixtures) CreateTestConfig(t *testing.T, config string) {
	require.NoError(t, os.WriteFile(f.ConfigPath, []byte(config), 0644))
}

// CreateTestClaude creates a test CLAUDE.md file
func (f *TestFixtures) CreateTestClaude(t *testing.T, content string) {
	require.NoError(t, os.WriteFile(f.ClaudePath, []byte(content), 0644))
}

// GetTestPersonas returns a map of test persona files
func GetTestPersonas() map[string]string {
	return map[string]string{
		"strict-code-reviewer.md": `---
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

		"balanced-code-reviewer.md": `---
name: balanced-code-reviewer
roles: [code-reviewer]
description: Balanced approach to code reviews
tags: [balanced, pragmatic, team-friendly]
---

# Balanced Code Reviewer

You provide constructive, balanced code reviews that consider both quality and team velocity.
You focus on the most important issues while being supportive of team growth.

## Review Approach
- Focus on critical issues first
- Provide constructive feedback
- Consider team experience levels
- Balance quality with delivery speed

## Feedback Style
- Start with positive observations
- Explain the reasoning behind suggestions
- Offer alternatives when possible
- Encourage learning and improvement`,

		"test-engineer-tdd.md": `---
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
- Coverage should be meaningful, not just high

## Focus Areas
- Unit test design and implementation
- Test automation and CI/CD integration
- Performance testing and benchmarking
- Testing strategy and planning`,

		"test-engineer-bdd.md": `---
name: test-engineer-bdd
roles: [test-engineer]
description: Behavior-driven development specialist
tags: [bdd, testing, behavior, acceptance]
---

# BDD Test Engineer

You are a test engineer focused on behavior-driven development.
You write tests that describe system behavior from user perspective.

## BDD Approach
- Given/When/Then structure
- Focus on user behavior
- Collaboration with stakeholders
- Living documentation through tests

## Key Practices
- Scenario-based testing
- Cucumber/Gherkin syntax
- Acceptance criteria validation
- Stakeholder collaboration`,

		"architect-systems.md": `---
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
- Technology selection and trade-offs

## Documentation
- Architecture decision records (ADRs)
- System design documents
- API specifications
- Deployment diagrams`,

		"security-analyst.md": `---
name: security-analyst
roles: [security-analyst, code-reviewer]
description: Security-focused analysis and review specialist
tags: [security, vulnerability, compliance, audit]
---

# Security Analyst

You are a security analyst with deep expertise in identifying vulnerabilities
and ensuring compliance with security standards.

## Security Focus Areas
- OWASP Top 10 vulnerabilities
- Authentication and authorization
- Data protection and privacy
- Secure coding practices

## Analysis Approach
- Threat modeling
- Vulnerability assessment
- Risk analysis and mitigation
- Compliance verification

## Tools and Techniques
- Static analysis security testing
- Dynamic application security testing
- Penetration testing methodologies
- Security code review practices`,

		"performance-engineer.md": `---
name: performance-engineer
roles: [performance-engineer, code-reviewer]
description: Performance optimization and scalability specialist
tags: [performance, optimization, scalability, benchmarking]
---

# Performance Engineer

You are a performance engineering specialist focused on code optimization
and system scalability.

## Performance Areas
- Algorithm optimization
- Memory usage patterns
- I/O optimization
- Caching strategies

## Optimization Approach
- Measure before optimizing
- Profile real-world scenarios
- Focus on critical path
- Consider long-term maintainability

## Tools and Metrics
- Profiling tools (pprof, perf, etc.)
- Benchmarking frameworks
- Load testing scenarios
- Performance regression testing`,

		"devops-engineer.md": `---
name: devops-engineer
roles: [devops-engineer, infrastructure-engineer]
description: DevOps and infrastructure automation specialist
tags: [devops, infrastructure, automation, ci-cd]
---

# DevOps Engineer

You are a DevOps engineer focused on infrastructure automation,
CI/CD pipelines, and operational excellence.

## DevOps Principles
- Infrastructure as Code
- Continuous integration and deployment
- Monitoring and observability
- Failure recovery and resilience

## Focus Areas
- Container orchestration
- Cloud infrastructure
- Pipeline automation
- Security and compliance

## Tools and Practices
- Docker and Kubernetes
- Terraform and CloudFormation
- Jenkins, GitHub Actions, GitLab CI
- Prometheus, Grafana, ELK stack`,

		"minimal-persona.md": `---
name: minimal-persona
roles: [developer]
description: Minimal persona for testing
---

# Minimal Persona

This is a minimal persona with only required fields for testing purposes.`,

		"invalid-yaml-persona.md": `---
name: invalid-persona
roles: [test-role
description: Invalid YAML - missing bracket
tags: [test]
---

# Invalid Persona

This persona has invalid YAML frontmatter.`,

		"empty-content-persona.md": `---
name: empty-content-persona
roles: [test-role]
description: Persona with empty content
tags: [test]
---`,

		"missing-name-persona.md": `---
roles: [test-role]
description: Missing name field
tags: [test]
---

# Missing Name Persona`,

		"missing-roles-persona.md": `---
name: missing-roles-persona
description: Missing roles field
tags: [test]
---

# Missing Roles Persona`,

		"empty-roles-persona.md": `---
name: empty-roles-persona
roles: []
description: Empty roles array
tags: [test]
---

# Empty Roles Persona`,
	}
}

// GetTestConfigs returns a map of test configuration files
func GetTestConfigs() map[string]string {
	return map[string]string{
		"minimal": `version: "1.0"`,

		"with_bindings": `version: "1.0"
repository:
  url: "https://github.com/test/project"
  branch: "main"
persona_bindings:
  code-reviewer: strict-code-reviewer
  test-engineer: test-engineer-tdd
  architect: architect-systems`,

		"with_overrides": `version: "1.0"
persona_bindings:
  code-reviewer: balanced-code-reviewer
  test-engineer: test-engineer-tdd

overrides:
  performance-workflow:
    test-engineer: test-engineer-bdd
  security-workflow:
    code-reviewer: security-analyst`,

		"empty_bindings": `version: "1.0"
persona_bindings: {}`,

		"null_bindings": `version: "1.0"
persona_bindings: null`,

		"complex": `version: "1.0"
repository:
  url: "https://github.com/test/complex-project"
  branch: "main"

variables:
  project_name: "complex-project"
  environment: "development"

persona_bindings:
  code-reviewer: strict-code-reviewer
  test-engineer: test-engineer-tdd
  architect: architect-systems
  security-analyst: security-analyst
  performance-engineer: performance-engineer
  devops-engineer: devops-engineer

overrides:
  performance-workflow:
    test-engineer: test-engineer-bdd
    code-reviewer: performance-engineer
  security-workflow:
    code-reviewer: security-analyst
    test-engineer: test-engineer-tdd
  helix:
    architect: architect-systems
    test-engineer: test-engineer-bdd`,

		"invalid_yaml": `version: "1.0"
persona_bindings:
  code-reviewer: strict-reviewer
  test-engineer: [invalid-yaml-structure
description: This is invalid YAML`,
	}
}

// GetTestClaudeFiles returns a map of test CLAUDE.md files
func GetTestClaudeFiles() map[string]string {
	return map[string]string{
		"empty": `# CLAUDE.md

This is the project guidance for Claude.

## Project Context
This is a test project.`,

		"with_personas": `# CLAUDE.md

This is the project guidance for Claude.

## Project Context
This is a test project.

<!-- PERSONAS:START -->
## Active Personas

### Code Reviewer: strict-code-reviewer
# Strict Code Reviewer

You are an experienced senior code reviewer who enforces high quality standards.

### Test Engineer: test-engineer-tdd
# TDD Test Engineer

You are a test engineer who follows strict TDD methodology.

When responding, adopt the appropriate persona based on the task.
<!-- PERSONAS:END -->`,

		"with_multiple_personas": `# CLAUDE.md

Project guidance for a complex application.

<!-- PERSONAS:START -->
## Active Personas

### Architect: architect-systems
# Systems Architect

You are a senior systems architect focused on scalable design.

### Code Reviewer: strict-code-reviewer
# Strict Code Reviewer

You are an experienced senior code reviewer.

### Security Analyst: security-analyst
# Security Analyst

You are a security analyst with expertise in vulnerabilities.

### Test Engineer: test-engineer-tdd
# TDD Test Engineer

You follow strict TDD methodology.

When responding, adopt the appropriate persona based on the task.
<!-- PERSONAS:END -->

## Additional Instructions

More project-specific guidance here.`,

		"malformed_personas": `# CLAUDE.md

Project guidance.

<!-- PERSONAS:START -->
## Active Personas

### Invalid Format without colon
Some random content here.

<!-- PERSONAS:END -->`,

		"missing_end_marker": `# CLAUDE.md

Project guidance.

<!-- PERSONAS:START -->
## Active Personas

### Code Reviewer: strict-reviewer
Content without proper end marker.

More content after.`,

		"multiple_sections": `# CLAUDE.md

Project guidance.

<!-- PERSONAS:START -->
## Active Personas

First section.
<!-- PERSONAS:END -->

Some content.

<!-- PERSONAS:START -->
## Active Personas

Second section (shouldn't happen).
<!-- PERSONAS:END -->`,
	}
}

// GetTestPersonaObjects returns persona objects for testing
func GetTestPersonaObjects() []*Persona {
	return []*Persona{
		{
			Name:        "strict-code-reviewer",
			Roles:       []string{"code-reviewer", "security-analyst"},
			Description: "Uncompromising code quality enforcer",
			Tags:        []string{"strict", "security", "production", "quality"},
			Content:     "# Strict Code Reviewer\n\nYou are an experienced senior code reviewer.",
		},
		{
			Name:        "balanced-code-reviewer",
			Roles:       []string{"code-reviewer"},
			Description: "Balanced approach to code reviews",
			Tags:        []string{"balanced", "pragmatic", "team-friendly"},
			Content:     "# Balanced Code Reviewer\n\nYou provide constructive, balanced code reviews.",
		},
		{
			Name:        "test-engineer-tdd",
			Roles:       []string{"test-engineer"},
			Description: "Test-driven development specialist",
			Tags:        []string{"tdd", "testing", "quality", "red-green-refactor"},
			Content:     "# TDD Test Engineer\n\nYou follow strict TDD methodology.",
		},
		{
			Name:        "test-engineer-bdd",
			Roles:       []string{"test-engineer"},
			Description: "Behavior-driven development specialist",
			Tags:        []string{"bdd", "testing", "behavior", "acceptance"},
			Content:     "# BDD Test Engineer\n\nYou focus on behavior-driven development.",
		},
		{
			Name:        "architect-systems",
			Roles:       []string{"architect", "tech-lead"},
			Description: "Systems architecture and design specialist",
			Tags:        []string{"architecture", "design", "scalability", "patterns"},
			Content:     "# Systems Architect\n\nYou focus on scalable, maintainable design.",
		},
		{
			Name:        "security-analyst",
			Roles:       []string{"security-analyst", "code-reviewer"},
			Description: "Security-focused analysis and review specialist",
			Tags:        []string{"security", "vulnerability", "compliance", "audit"},
			Content:     "# Security Analyst\n\nYou focus on identifying vulnerabilities.",
		},
	}
}

// GetTestBindings returns test persona bindings
func GetTestBindings() map[string]string {
	return map[string]string{
		"code-reviewer":        "strict-code-reviewer",
		"test-engineer":        "test-engineer-tdd",
		"architect":            "architect-systems",
		"security-analyst":     "security-analyst",
		"performance-engineer": "performance-engineer",
		"devops-engineer":      "devops-engineer",
	}
}

// GetTestOverrides returns test workflow overrides
func GetTestOverrides() map[string]map[string]string {
	return map[string]map[string]string{
		"performance-workflow": {
			"test-engineer": "test-engineer-bdd",
			"code-reviewer": "performance-engineer",
		},
		"security-workflow": {
			"code-reviewer":    "security-analyst",
			"security-analyst": "security-analyst",
		},
		"helix": {
			"architect":     "architect-systems",
			"test-engineer": "test-engineer-bdd",
		},
	}
}

// CreateInvalidPersonaFiles creates persona files with various invalid formats for error testing
func (f *TestFixtures) CreateInvalidPersonaFiles(t *testing.T) {
	invalidPersonas := map[string]string{
		"no-frontmatter.md": `# No Frontmatter

This persona has no YAML frontmatter.`,

		"invalid-yaml.md": `---
name: invalid-yaml
roles: [code-reviewer
description: Invalid YAML - missing bracket
---
# Invalid YAML`,

		"missing-name.md": `---
roles: [code-reviewer]
description: Missing name field
---
# Missing Name`,

		"missing-roles.md": `---
name: missing-roles
description: Missing roles field
---
# Missing Roles`,

		"empty-roles.md": `---
name: empty-roles
roles: []
description: Empty roles array
---
# Empty Roles`,

		"missing-description.md": `---
name: missing-description
roles: [code-reviewer]
---
# Missing Description`,

		"empty-content.md": `---
name: empty-content
roles: [code-reviewer]
description: Empty content
---`,

		"non-markdown.txt": `This is not a markdown file.`,
	}

	for filename, content := range invalidPersonas {
		invalidPath := filepath.Join(f.PersonasDir, filename)
		require.NoError(t, os.WriteFile(invalidPath, []byte(content), 0644))
	}
}

// CreateHiddenFiles creates hidden files that should be ignored
func (f *TestFixtures) CreateHiddenFiles(t *testing.T) {
	hiddenFiles := map[string]string{
		".hidden-persona.md": `---
name: hidden-persona
roles: [hidden]
description: This should be ignored
---
# Hidden Persona`,

		".DS_Store": `binary data`,

		"README.md": `# Personas

This is a README file that should be ignored.`,

		".gitignore": `*.tmp
*.log`,
	}

	for filename, content := range hiddenFiles {
		hiddenPath := filepath.Join(f.PersonasDir, filename)
		require.NoError(t, os.WriteFile(hiddenPath, []byte(content), 0644))
	}
}

// AssertPersonaEqual compares two personas for equality in tests
func AssertPersonaEqual(t *testing.T, expected, actual *Persona) {
	require.NotNil(t, actual, "Actual persona should not be nil")
	require.NotNil(t, expected, "Expected persona should not be nil")

	require.Equal(t, expected.Name, actual.Name, "Persona names should match")
	require.Equal(t, expected.Roles, actual.Roles, "Persona roles should match")
	require.Equal(t, expected.Description, actual.Description, "Persona descriptions should match")
	require.Equal(t, expected.Tags, actual.Tags, "Persona tags should match")
	require.Equal(t, expected.Content, actual.Content, "Persona content should match")
}

// AssertFileExists checks that a file exists and optionally validates its content
func AssertFileExists(t *testing.T, path string, expectedContent ...string) {
	_, err := os.Stat(path)
	require.NoError(t, err, "File should exist: %s", path)

	if len(expectedContent) > 0 {
		content, err := os.ReadFile(path)
		require.NoError(t, err, "Should be able to read file: %s", path)

		actualContent := string(content)
		for _, expected := range expectedContent {
			require.Contains(t, actualContent, expected, "File should contain expected content")
		}
	}
}

// AssertFileNotExists checks that a file does not exist
func AssertFileNotExists(t *testing.T, path string) {
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err), "File should not exist: %s", path)
}

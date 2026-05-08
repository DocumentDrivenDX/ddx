package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Acceptance tests validate persona system user stories and business requirements
// These tests follow the Given/When/Then pattern from user stories US-030 through US-035

// setupPersonaTestProject initializes a DDx project for persona testing
func setupPersonaTestProject(t *testing.T) *TestEnvironment {
	env := NewTestEnvironment(t)
	env.InitWithDDx()
	return env
}

// setupMockPersonas creates mock persona files in the test environment's library
func setupMockPersonas(t *testing.T, env *TestEnvironment) {
	t.Helper()

	libPath := filepath.Join(env.Dir, ".ddx", "plugins", "ddx")
	personasDir := filepath.Join(libPath, "personas")
	require.NoError(t, os.MkdirAll(personasDir, 0755))

	// Create code-reviewer persona
	codeReviewerContent := `---
name: code-reviewer
roles: [code-reviewer, security-analyst]
description: Uncompromising code quality enforcer
tags: [strict, security, production, quality]
---

# Code Reviewer

You are an experienced senior code reviewer who enforces high quality standards.
Your reviews are thorough, security-focused, and aimed at maintaining production quality.

## Review Principles
- Security vulnerabilities are non-negotiable
- Performance implications must be considered
- Code readability and maintainability are essential
- Test coverage requirements must be met`

	require.NoError(t, os.WriteFile(
		filepath.Join(personasDir, "code-reviewer.md"),
		[]byte(codeReviewerContent),
		0644,
	))

	// Create test-engineer persona
	testEngineerContent := `---
name: test-engineer
roles: [test-engineer, qa]
description: Test-driven development advocate
tags: [tdd, testing, quality]
---

# Test Engineer

You are a test engineer who champions test-driven development practices.`

	require.NoError(t, os.WriteFile(
		filepath.Join(personasDir, "test-engineer.md"),
		[]byte(testEngineerContent),
		0644,
	))

	// Create architect persona
	architectContent := `---
name: architect
roles: [architect, systems-design]
description: Systems architecture expert
tags: [architecture, design, systems]
---

# Architect

You are a systems architect with deep expertise in distributed systems.`

	require.NoError(t, os.WriteFile(
		filepath.Join(personasDir, "architect.md"),
		[]byte(architectContent),
		0644,
	))
}

// TestAcceptance_US030_LoadPersonasForSession tests US-030: Developer Loading Personas for Session
func TestAcceptance_US030_LoadPersonasForSession(t *testing.T) {
	tests := []struct {
		name     string
		scenario string
		given    func(t *testing.T) (string, string)           // Setup conditions (homeDir, workDir)
		when     func(t *testing.T, workDir string) error      // Execute action
		then     func(t *testing.T, workDir string, err error) // Verify outcome
	}{
		{
			name:     "load_all_bound_personas",
			scenario: "Developer loads all project's bound personas with single command",
			given: func(t *testing.T) (string, string) {
				// Given: I am a developer with a project that has persona bindings configured
				workDir := t.TempDir()

				// Create .ddx/config.yaml with persona bindings
				ddxDir := filepath.Join(workDir, ".ddx")
				require.NoError(t, os.MkdirAll(ddxDir, 0755))
				config := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/test/project
    branch: main
persona_bindings:
  code-reviewer: "code-reviewer"
  test-engineer: "test-engineer"
  architect: "architect"`

				require.NoError(t, os.WriteFile(
					filepath.Join(ddxDir, "config.yaml"),
					[]byte(config),
					0644,
				))

				// Create CLAUDE.md for persona injection
				claudeContent := `# CLAUDE.md

This is my project's guidance for Claude.

## Project Context
This is a test project for validating persona functionality.`

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(workDir, "CLAUDE.md"),
					[]byte(claudeContent),
					0644,
				))

				// Create personas in library directory
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				// Set library path to project-local library
				libraryDir := filepath.Join(workDir, ".ddx", "plugins", "ddx")
				personasDir := filepath.Join(libraryDir, "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				// Create the bound personas
				codeReviewerContent := `---
name: code-reviewer
roles: [code-reviewer, security-analyst]
description: Uncompromising code quality enforcer
tags: [strict, security, production, quality]
---

# Code Reviewer

You are an experienced senior code reviewer who enforces high quality standards.
Your reviews are thorough, security-focused, and aimed at maintaining production quality.

## Review Principles
- Security vulnerabilities are non-negotiable
- Performance implications must be considered
- Code readability and maintainability are essential
- Test coverage requirements must be met`

				testEngineerContent := `---
name: test-engineer
roles: [test-engineer]
description: Test-driven development specialist
tags: [tdd, testing, quality, red-green-refactor]
---

# Test Engineer

You are a test engineer who follows strict TDD methodology.
Always write failing tests first, then implement minimal code to pass.

## TDD Cycle
1. Red: Write a failing test
2. Green: Write minimal code to pass
3. Refactor: Improve code while keeping tests green`

				architectContent := `---
name: architect
roles: [architect, tech-lead]
description: Systems architecture and design specialist
tags: [architecture, design, scalability, patterns]
---

# Architect

You are a senior systems architect focused on scalable, maintainable design.
You think in terms of system boundaries, data flow, and long-term evolution.

## Architecture Principles
- Favor composition over inheritance
- Design for failure and resilience
- Consider operational concerns early
- Document architectural decisions`

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "code-reviewer.md"),
					[]byte(codeReviewerContent),
					0644,
				))

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "test-engineer.md"),
					[]byte(testEngineerContent),
					0644,
				))

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "architect.md"),
					[]byte(architectContent),
					0644,
				))

				return homeDir, workDir
			},
			when: func(t *testing.T, workDir string) error {
				// When: I run `ddx persona load` to load all bound personas
				factory := NewCommandFactory(workDir)
				rootCmd := factory.NewRootCommand()

				// TODO: Add persona command when implemented
				// Commands already registered
				_, err := executeCommand(rootCmd, "persona", "load")
				return err
			},
			then: func(t *testing.T, workDir string, err error) {
				// Then: all bound personas are loaded into my AI assistant context

				// Persona command is now implemented
				assert.NoError(t, err, "Loading personas should succeed")

				// Verify CLAUDE.md has been updated with persona content
				claudePath := filepath.Join(workDir, "CLAUDE.md")
				content, readErr := os.ReadFile(claudePath)
				require.NoError(t, readErr)

				claudeStr := string(content)

				// Should contain persona markers
				assert.Contains(t, claudeStr, "<!-- PERSONAS:START -->")
				assert.Contains(t, claudeStr, "<!-- PERSONAS:END -->")

				// Should contain all three personas
				assert.Contains(t, claudeStr, "Code Reviewer")
				assert.Contains(t, claudeStr, "Test Engineer")
				assert.Contains(t, claudeStr, "Architect")

				// Should preserve original content
				assert.Contains(t, claudeStr, "This is my project's guidance for Claude")
				assert.Contains(t, claudeStr, "Project Context")

				// Should indicate role mapping
				assert.Contains(t, claudeStr, "Code Reviewer: code-reviewer")
				assert.Contains(t, claudeStr, "Test Engineer: test-engineer")
				assert.Contains(t, claudeStr, "Architect: architect")
			},
		},
		{
			name:     "load_specific_persona_by_name",
			scenario: "Developer loads specific persona by name",
			given: func(t *testing.T) (string, string) {
				// Given: I have personas available and want to load a specific one
				workDir := t.TempDir()

				// Create CLAUDE.md
				claudeContent := `# CLAUDE.md

Project guidance for my application.`
				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(workDir, "CLAUDE.md"),
					[]byte(claudeContent),
					0644,
				))

				// Create .ddx/config.yaml to make this a valid DDx project
				config := `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://github.com/DocumentDrivenDX/ddx-library"
    branch: "main"
persona_bindings: {}`

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(workDir, ".ddx", "config.yaml"),
					[]byte(config),
					0644,
				))

				// Create persona in library directory
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				// Set library path to project-local library
				libraryDir := filepath.Join(workDir, ".ddx", "plugins", "ddx")
				personasDir := filepath.Join(libraryDir, "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				personaContent := `---
name: security-analyst
roles: [security-analyst, code-reviewer]
description: Security-focused code analysis
tags: [security, vulnerability, compliance]
---

# Security Analyst

You are a security analyst focused on identifying vulnerabilities and security issues.`

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "security-analyst.md"),
					[]byte(personaContent),
					0644,
				))

				return homeDir, workDir
			},
			when: func(t *testing.T, workDir string) error {
				// When: I run `ddx persona load security-analyst`
				factory := NewCommandFactory(workDir)
				rootCmd := factory.NewRootCommand()

				// TODO: Add persona command when implemented
				// Commands already registered
				_, err := executeCommand(rootCmd, "persona", "load", "security-analyst")
				return err
			},
			then: func(t *testing.T, workDir string, err error) {
				// Then: the specific persona is loaded into my AI assistant context

				// Persona command is now implemented
				assert.NoError(t, err, "Loading specific persona should succeed")

				// Verify CLAUDE.md has been updated
				claudePath := filepath.Join(workDir, "CLAUDE.md")
				content, readErr := os.ReadFile(claudePath)
				require.NoError(t, readErr)

				claudeStr := string(content)
				assert.Contains(t, claudeStr, "Security Analyst")
				assert.Contains(t, claudeStr, "security analyst focused on identifying vulnerabilities")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Commands are isolated via factory, no need to reset

			//	// originalDir, _ := os.Getwd() // REMOVED: Using CommandFactory injection // REMOVED: Using CommandFactory injection

			// Given
			_, workDir := tt.given(t)

			// When
			err := tt.when(t, workDir)

			// Then
			tt.then(t, workDir, err)
		})
	}
}

// TestAcceptance_US031_BindPersonasToRoles tests US-031: Team Lead Binding Personas to Roles
func TestAcceptance_US031_BindPersonasToRoles(t *testing.T) {
	tests := []struct {
		name     string
		scenario string
		given    func(t *testing.T) (string, string)
		when     func(t *testing.T, workDir string) error
		then     func(t *testing.T, workDir string, err error)
	}{
		{
			name:     "bind_persona_to_role",
			scenario: "Team lead binds specific persona to role in project configuration",
			given: func(t *testing.T) (string, string) {
				// Given: I am a team lead with a project and available personas
				workDir := t.TempDir()

				// Create initial .ddx.yml configuration
				config := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/team/project
    branch: main
persona_bindings:
  project_name: "team-project"`

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(workDir, ".ddx", "config.yaml"),
					[]byte(config),
					0644,
				))

				// Create available persona in library directory
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				// Set library path to project-local library
				libraryDir := filepath.Join(workDir, ".ddx", "plugins", "ddx")
				personasDir := filepath.Join(libraryDir, "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				personaContent := `---
name: balanced-code-reviewer
roles: [code-reviewer]
description: Balanced approach to code review
tags: [balanced, pragmatic, team]
---

# Balanced Code Reviewer

You provide constructive, balanced code reviews that consider both quality and team velocity.`

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "balanced-code-reviewer.md"),
					[]byte(personaContent),
					0644,
				))

				return homeDir, workDir
			},
			when: func(t *testing.T, workDir string) error {
				// When: I run `ddx persona bind code-reviewer balanced-code-reviewer`
				factory := NewCommandFactory(workDir)
				rootCmd := factory.NewRootCommand()

				// TODO: Add persona command when implemented
				// Commands already registered
				_, err := executeCommand(rootCmd, "persona", "bind", "code-reviewer", "balanced-code-reviewer")
				return err
			},
			then: func(t *testing.T, workDir string, err error) {
				// Then: the persona is bound to the role in project configuration

				// Persona command is now implemented
				assert.NoError(t, err, "Binding persona should succeed")

				// Verify .ddx.yml has been updated with persona binding
				configPath := filepath.Join(workDir, ".ddx", "config.yaml")
				content, readErr := os.ReadFile(configPath)
				require.NoError(t, readErr)

				var config map[string]interface{}
				require.NoError(t, yaml.Unmarshal(content, &config))

				// Should have persona_bindings section
				personaBindings, exists := config["persona_bindings"]
				assert.True(t, exists, "persona_bindings section should exist")

				bindings := personaBindings.(map[string]interface{})
				assert.Equal(t, "balanced-code-reviewer", bindings["code-reviewer"])

				// Should preserve existing configuration
				assert.Equal(t, "1.0", config["version"])
				library := config["library"].(map[string]interface{})
				repo := library["repository"].(map[string]interface{})
				assert.Equal(t, "https://github.com/team/project", repo["url"])
			},
		},
		{
			name:     "override_existing_binding",
			scenario: "Team lead changes existing persona binding",
			given: func(t *testing.T) (string, string) {
				// Given: I have a project with existing persona bindings
				workDir := t.TempDir()

				config := `version: "1.0"
persona_bindings:
  code-reviewer: "old-reviewer"
  test-engineer: "current-tester"`

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(workDir, ".ddx", "config.yaml"),
					[]byte(config),
					0644,
				))

				// Create new persona to bind in library directory
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				// Set library path to project-local library
				libraryDir := filepath.Join(workDir, ".ddx", "plugins", "ddx")
				personasDir := filepath.Join(libraryDir, "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				personaContent := `---
name: new-reviewer
roles: [code-reviewer]
description: Updated reviewer approach
tags: [modern, efficient]
---

# New Reviewer`

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "new-reviewer.md"),
					[]byte(personaContent),
					0644,
				))

				return homeDir, workDir
			},
			when: func(t *testing.T, workDir string) error {
				// When: I bind a new persona to an existing role
				factory := NewCommandFactory(workDir)
				rootCmd := factory.NewRootCommand()

				// TODO: Add persona command when implemented
				// Commands already registered
				_, err := executeCommand(rootCmd, "persona", "bind", "code-reviewer", "new-reviewer")
				return err
			},
			then: func(t *testing.T, workDir string, err error) {
				// Then: the existing binding is updated

				// Persona command is now implemented
				assert.NoError(t, err, "Updating binding should succeed")

				// Verify binding was updated
				configPath := filepath.Join(workDir, ".ddx", "config.yaml")
				content, readErr := os.ReadFile(configPath)
				require.NoError(t, readErr)

				var config map[string]interface{}
				require.NoError(t, yaml.Unmarshal(content, &config))

				bindings := config["persona_bindings"].(map[string]interface{})
				assert.Equal(t, "new-reviewer", bindings["code-reviewer"])
				assert.Equal(t, "current-tester", bindings["test-engineer"]) // Should preserve other bindings
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Commands are isolated via factory, no need to reset

			//	// originalDir, _ := os.Getwd() // REMOVED: Using CommandFactory injection // REMOVED: Using CommandFactory injection

			// Given
			_, workDir := tt.given(t)

			// When
			err := tt.when(t, workDir)

			// Then
			tt.then(t, workDir, err)
		})
	}
}

// TestAcceptance_US032_WorkflowAuthorRequiringRoles tests US-032: Workflow Author Requiring Roles
func TestAcceptance_US032_WorkflowAuthorRequiringRoles(t *testing.T) {
	tests := []struct {
		name     string
		scenario string
		given    func(t *testing.T) (string, string)
		when     func(t *testing.T, workDir string) error
		then     func(t *testing.T, workDir string, err error)
	}{
		{
			name:     "workflow_specifies_required_roles",
			scenario: "Workflow author specifies required roles for phases and artifacts",
			given: func(t *testing.T) (string, string) {
				// Given: I am a workflow author creating a workflow with role requirements
				workDir := t.TempDir()

				// Create workflow with required roles
				workflowContent := `name: test-workflow
version: 1.0.0
description: Testing workflow with personas

phases:
  - id: design
    name: Design Phase
    description: Design the solution
    required_role: architect

  - id: test
    name: Test Phase
    description: Write tests first
    required_role: test-engineer

artifacts:
  - name: architecture-doc
    description: System architecture document
    required_role: architect
    prompt: "Design the system architecture for {{project_name}}"

  - name: test-plan
    description: Comprehensive test plan
    required_role: test-engineer
    prompt: "Create test plan for {{project_name}}"`

				workflowDir := filepath.Join(workDir, "workflows", "test-workflow")
				require.NoError(t, os.MkdirAll(workflowDir, 0755))
				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(workflowDir, "workflow.yml"),
					[]byte(workflowContent),
					0644,
				))

				// Create .ddx.yml to make this a valid DDx project
				config := `version: "1.0"
persona_bindings:
  architect: "architect"
  test-engineer: "test-engineer"`

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(workDir, ".ddx", "config.yaml"),
					[]byte(config),
					0644,
				))

				// Create required personas
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				personasDir := filepath.Join(homeDir, ".ddx", "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				architectContent := `---
name: architect
roles: [architect]
description: Systems architect
tags: [design]
---
# Architect`

				testEngineerContent := `---
name: test-engineer
roles: [test-engineer]
description: Test engineer
tags: [testing]
---
# Test Engineer`

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "architect.md"),
					[]byte(architectContent),
					0644,
				))

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "test-engineer.md"),
					[]byte(testEngineerContent),
					0644,
				))

				return homeDir, workDir
			},
			when: func(t *testing.T, workDir string) error {
				// When: I execute the workflow (this would be done by DDx workflow engine)
				// For this test, we simulate validating the workflow can find required personas

				// TODO: This would actually invoke the workflow engine
				// For now, we just verify the workflow structure is valid
				workflowPath := filepath.Join(workDir, "workflows", "test-workflow", "workflow.yml")
				_, err := os.Stat(workflowPath)
				return err
			},
			then: func(t *testing.T, workDir string, err error) {
				// Then: appropriate expertise is applied regardless of specific persona used
				assert.NoError(t, err, "Workflow file should exist")

				// TODO: When workflow engine is implemented, verify:
				// 1. Workflow engine reads required_role fields
				// 2. Correct personas are resolved from bindings
				// 3. Persona content is combined with artifact prompts
				// 4. Appropriate expertise is applied to each phase/artifact

				// For now, just verify the workflow structure
				workflowPath := filepath.Join(workDir, "workflows", "test-workflow", "workflow.yml")
				content, readErr := os.ReadFile(workflowPath)
				require.NoError(t, readErr)

				workflowStr := string(content)
				assert.Contains(t, workflowStr, "required_role: architect")
				assert.Contains(t, workflowStr, "required_role: test-engineer")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Commands are isolated via factory, no need to reset

			//	// originalDir, _ := os.Getwd() // REMOVED: Using CommandFactory injection // REMOVED: Using CommandFactory injection

			// Given
			_, workDir := tt.given(t)

			// When
			err := tt.when(t, workDir)

			// Then
			tt.then(t, workDir, err)
		})
	}
}

// TestAcceptance_US033_DeveloperContributingPersonas tests US-033: Developer Contributing Personas
func TestAcceptance_US033_DeveloperContributingPersonas(t *testing.T) {
	tests := []struct {
		name     string
		scenario string
		given    func(t *testing.T) (string, string)
		when     func(t *testing.T, workDir string) error
		then     func(t *testing.T, workDir string, err error)
	}{
		{
			name:     "create_new_persona",
			scenario: "Developer creates new persona for community contribution",
			given: func(t *testing.T) (string, string) {
				// Given: I am a developer with a refined interaction pattern I want to share
				workDir := t.TempDir()

				// Simulate developer working directory with DDx
				config := `version: "1.0"
repository:
  url: "https://github.com/ddx-toolkit/ddx"
  branch: "main"`

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(workDir, ".ddx", "config.yaml"),
					[]byte(config),
					0644,
				))

				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)

				return homeDir, workDir
			},
			when: func(t *testing.T, workDir string) error {
				// When: I create a new persona file in the personas directory
				homeDir := os.Getenv("HOME")
				personasDir := filepath.Join(homeDir, ".ddx", "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				// Developer creates new persona
				newPersonaContent := `---
name: performance-optimizer
roles: [performance-engineer, code-reviewer]
description: Performance-focused code optimization specialist
tags: [performance, optimization, scalability, benchmarking]
---

# Performance Optimizer

You are a performance engineering specialist focused on code optimization and scalability.
Your reviews prioritize performance characteristics and efficient resource utilization.

## Performance Principles
- Measure before optimizing
- Understand algorithmic complexity
- Consider memory allocation patterns
- Profile real-world scenarios

## Key Areas
- CPU-intensive operations
- Memory usage patterns
- I/O optimization
- Caching strategies
- Parallel processing opportunities

## Tools and Techniques
- Profiling tools (pprof, perf, etc.)
- Benchmarking frameworks
- Load testing scenarios
- Performance regression testing`

				personaPath := filepath.Join(personasDir, "performance-optimizer.md")
				return os.WriteFile(personaPath, []byte(newPersonaContent), 0644)
			},
			then: func(t *testing.T, workDir string, err error) {
				// Then: the persona is available for community contribution
				assert.NoError(t, err, "Creating persona should succeed")

				// Verify persona was created with correct format
				homeDir := os.Getenv("HOME")
				personaPath := filepath.Join(homeDir, ".ddx", "personas", "performance-optimizer.md")

				content, readErr := os.ReadFile(personaPath)
				require.NoError(t, readErr)

				personaStr := string(content)
				assert.Contains(t, personaStr, "name: performance-optimizer")
				assert.Contains(t, personaStr, "roles: [performance-engineer, code-reviewer]")
				assert.Contains(t, personaStr, "Performance Optimizer")
				assert.Contains(t, personaStr, "Performance Principles")

				// TODO: When contribution workflow is implemented, verify:
				// - ddx contribute command can package the persona
				// - PR can be created to main repository
				// - Community can discover and use the persona
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Commands are isolated via factory, no need to reset

			//	// originalDir, _ := os.Getwd() // REMOVED: Using CommandFactory injection // REMOVED: Using CommandFactory injection

			// Given
			_, workDir := tt.given(t)

			// When
			err := tt.when(t, workDir)

			// Then
			tt.then(t, workDir, err)
		})
	}
}

// TestAcceptance_US034_DeveloperDiscoveringPersonas tests US-034: Developer Discovering Personas
func TestAcceptance_US034_DeveloperDiscoveringPersonas(t *testing.T) {
	tests := []struct {
		name     string
		scenario string
		given    func(t *testing.T) (string, string)
		when     func(t *testing.T, workDir string) (string, error)
		then     func(t *testing.T, workDir string, output string, err error)
	}{
		{
			name:     "discover_personas_by_role",
			scenario: "Developer discovers personas by role for their needs",
			given: func(t *testing.T) (string, string) {
				// Given: I am a developer looking for personas for a specific role
				workDir := t.TempDir()

				// Create variety of personas to discover
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				// Set library path to project-local library
				libraryDir := filepath.Join(workDir, ".ddx", "plugins", "ddx")
				personasDir := filepath.Join(libraryDir, "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				personas := map[string]string{
					"strict-reviewer.md": `---
name: strict-reviewer
roles: [code-reviewer]
description: Uncompromising quality enforcer
tags: [strict, security]
---
# Strict Reviewer`,

					"balanced-reviewer.md": `---
name: balanced-reviewer
roles: [code-reviewer]
description: Balanced approach to reviews
tags: [balanced, pragmatic]
---
# Balanced Reviewer`,

					"tdd-engineer.md": `---
name: tdd-engineer
roles: [test-engineer]
description: TDD specialist
tags: [tdd, testing]
---
# TDD Engineer`,

					"security-analyst.md": `---
name: security-analyst
roles: [security-analyst, code-reviewer]
description: Security-focused analysis
tags: [security, vulnerability]
---
# Security Analyst`,
				}

				for filename, content := range personas {
					require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
					require.NoError(t, os.WriteFile(
						filepath.Join(personasDir, filename),
						[]byte(content),
						0644,
					))
				}

				return homeDir, workDir
			},
			when: func(t *testing.T, workDir string) (string, error) {
				// When: I search for personas by role
				factory := NewCommandFactory(workDir)
				rootCmd := factory.NewRootCommand()
				// TODO: Add persona command when implemented
				// Commands already registered
				return executeCommand(rootCmd, "persona", "list", "--role", "code-reviewer")
			},
			then: func(t *testing.T, workDir string, output string, err error) {
				// Then: I can find appropriate personalities for my needs

				// Persona command is now implemented
				assert.NoError(t, err, "Listing personas by role should succeed")

				// Should show personas that can fulfill code-reviewer role
				assert.Contains(t, output, "strict-reviewer")
				assert.Contains(t, output, "balanced-reviewer")
				assert.Contains(t, output, "security-analyst") // Has code-reviewer role

				// Should not show personas that don't match the role
				assert.NotContains(t, output, "tdd-engineer")

				// Should display helpful information
				assert.Contains(t, output, "Uncompromising quality enforcer")
				assert.Contains(t, output, "Balanced approach to reviews")
			},
		},
		{
			name:     "discover_personas_by_tags",
			scenario: "Developer discovers personas by capability tags",
			given: func(t *testing.T) (string, string) {
				// Given: I need personas with specific capabilities
				workDir := t.TempDir()

				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				// Set library path to project-local library
				libraryDir := filepath.Join(workDir, ".ddx", "plugins", "ddx")
				personasDir := filepath.Join(libraryDir, "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				personas := map[string]string{
					"security-expert.md": `---
name: security-expert
roles: [security-analyst]
description: Security specialist
tags: [security, vulnerability, compliance]
---
# Security Expert`,

					"performance-specialist.md": `---
name: performance-specialist
roles: [performance-engineer]
description: Performance optimization
tags: [performance, optimization, scalability]
---
# Performance Specialist`,

					"security-reviewer.md": `---
name: security-reviewer
roles: [code-reviewer]
description: Security-focused reviewer
tags: [security, code-review, strict]
---
# Security Reviewer`,
				}

				for filename, content := range personas {
					require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
					require.NoError(t, os.WriteFile(
						filepath.Join(personasDir, filename),
						[]byte(content),
						0644,
					))
				}

				return homeDir, workDir
			},
			when: func(t *testing.T, workDir string) (string, error) {
				// When: I search for personas by tag
				factory := NewCommandFactory(workDir)
				rootCmd := factory.NewRootCommand()
				// TODO: Add persona command when implemented
				// Commands already registered
				return executeCommand(rootCmd, "persona", "list", "--tag", "security")
			},
			then: func(t *testing.T, workDir string, output string, err error) {
				// Then: I find personas with the desired capabilities

				// Persona command is now implemented
				assert.NoError(t, err, "Listing personas by tag should succeed")

				// Should show personas with security tag
				assert.Contains(t, output, "security-reviewer")

				// Should not show personas without the tag
				assert.NotContains(t, output, "performance-specialist")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Commands are isolated via factory, no need to reset

			//	// originalDir, _ := os.Getwd() // REMOVED: Using CommandFactory injection // REMOVED: Using CommandFactory injection

			// Given
			_, workDir := tt.given(t)

			// When
			output, err := tt.when(t, workDir)

			// Then
			tt.then(t, workDir, output, err)
		})
	}
}

// TestAcceptance_US035_DeveloperOverridingWorkflowPersonas tests US-035: Developer Overriding Workflow Personas
func TestAcceptance_US035_DeveloperOverridingWorkflowPersonas(t *testing.T) {
	tests := []struct {
		name     string
		scenario string
		given    func(t *testing.T) (string, string)
		when     func(t *testing.T, workDir string) error
		then     func(t *testing.T, workDir string, err error)
	}{
		{
			name:     "override_workflow_specific_persona",
			scenario: "Developer overrides default persona for specific workflow",
			given: func(t *testing.T) (string, string) {
				// Given: I have default persona bindings but want different approach for specific workflow
				workDir := t.TempDir()

				// Create .ddx.yml with default bindings
				config := `version: "1.0"
persona_bindings:
  test-engineer: "test-engineer"

overrides:
  performance-workflow:
    test-engineer: "test-engineer-bdd"  # Use BDD approach for performance testing`

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(workDir, ".ddx", "config.yaml"),
					[]byte(config),
					0644,
				))

				// Create personas
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				personasDir := filepath.Join(homeDir, ".ddx", "personas")
				require.NoError(t, os.MkdirAll(personasDir, 0755))

				testEngineerContent := `---
name: test-engineer
roles: [test-engineer]
description: Test engineer
tags: [testing]
---
# Test Engineer`

				bddContent := `---
name: test-engineer-bdd
roles: [test-engineer]
description: BDD specialist
tags: [bdd, behavior]
---
# BDD Engineer`

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "test-engineer.md"),
					[]byte(testEngineerContent),
					0644,
				))

				require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(personasDir, "test-engineer-bdd.md"),
					[]byte(bddContent),
					0644,
				))

				return homeDir, workDir
			},
			when: func(t *testing.T, workDir string) error {
				// When: I execute the performance workflow
				// TODO: This would be done by workflow engine
				// For now, just verify the configuration structure is correct
				configPath := filepath.Join(workDir, ".ddx", "config.yaml")
				_, err := os.Stat(configPath)
				return err
			},
			then: func(t *testing.T, workDir string, err error) {
				// Then: the workflow uses the overridden persona instead of default
				assert.NoError(t, err, "Configuration should exist")

				// Verify override configuration structure
				configPath := filepath.Join(workDir, ".ddx", "config.yaml")
				content, readErr := os.ReadFile(configPath)
				require.NoError(t, readErr)

				var config map[string]interface{}
				require.NoError(t, yaml.Unmarshal(content, &config))

				// Verify default bindings
				personaBindings := config["persona_bindings"].(map[string]interface{})
				assert.Equal(t, "test-engineer", personaBindings["test-engineer"])

				// Verify overrides
				overrides := config["overrides"].(map[string]interface{})
				perfWorkflow := overrides["performance-workflow"].(map[string]interface{})
				assert.Equal(t, "test-engineer-bdd", perfWorkflow["test-engineer"])

				// TODO: When workflow engine is implemented, verify:
				// 1. Default workflows use test-engineer
				// 2. performance-workflow uses test-engineer-bdd override
				// 3. Override takes precedence over default binding
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Commands are isolated via factory, no need to reset

			//	// originalDir, _ := os.Getwd() // REMOVED: Using CommandFactory injection // REMOVED: Using CommandFactory injection

			// Given
			_, workDir := tt.given(t)

			// When
			err := tt.when(t, workDir)

			// Then
			tt.then(t, workDir, err)
		})
	}
}

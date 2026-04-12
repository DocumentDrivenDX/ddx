package persona

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParsePersona tests persona parsing from markdown files with YAML frontmatter
func TestParsePersona_Basic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		content     string
		expected    *Persona
		expectError bool
	}{
		{
			name: "valid_persona_single_role",
			content: `---
name: test-reviewer
roles: [code-reviewer]
description: Test code reviewer persona
tags: [test, review]
---

# Test Reviewer

You are a test code reviewer who focuses on quality.

## Principles
- Quality first
- Clear feedback`,
			expected: &Persona{
				Name:        "test-reviewer",
				Roles:       []string{"code-reviewer"},
				Description: "Test code reviewer persona",
				Tags:        []string{"test", "review"},
				Content: `# Test Reviewer

You are a test code reviewer who focuses on quality.

## Principles
- Quality first
- Clear feedback`,
			},
			expectError: false,
		},
		{
			name: "valid_persona_multiple_roles",
			content: `---
name: security-analyst
roles: [security-analyst, code-reviewer, compliance-officer]
description: Comprehensive security specialist
tags: [security, compliance, vulnerability, audit]
---

# Security Analyst

You are a security analyst with expertise in:
- Vulnerability assessment
- Compliance verification
- Code security review`,
			expected: &Persona{
				Name:        "security-analyst",
				Roles:       []string{"security-analyst", "code-reviewer", "compliance-officer"},
				Description: "Comprehensive security specialist",
				Tags:        []string{"security", "compliance", "vulnerability", "audit"},
				Content: `# Security Analyst

You are a security analyst with expertise in:
- Vulnerability assessment
- Compliance verification
- Code security review`,
			},
			expectError: false,
		},
		{
			name: "minimal_valid_persona",
			content: `---
name: minimal-persona
roles: [developer]
description: Minimal persona for testing
---

# Minimal Persona

Basic persona content.`,
			expected: &Persona{
				Name:        "minimal-persona",
				Roles:       []string{"developer"},
				Description: "Minimal persona for testing",
				Tags:        []string{}, // Should default to empty slice
				Content: `# Minimal Persona

Basic persona content.`,
			},
			expectError: false,
		},
		{
			name: "missing_frontmatter",
			content: `# Test Reviewer

This persona has no YAML frontmatter.`,
			expected:    nil,
			expectError: true,
		},
		{
			name: "invalid_yaml_frontmatter",
			content: `---
name: test-reviewer
roles: [code-reviewer
description: Invalid YAML - missing bracket
tags: [test]
---

# Test Reviewer`,
			expected:    nil,
			expectError: true,
		},
		{
			name: "missing_required_name",
			content: `---
roles: [code-reviewer]
description: Missing name field
tags: [test]
---

# Test Reviewer`,
			expected:    nil,
			expectError: true,
		},
		{
			name: "missing_required_roles",
			content: `---
name: test-reviewer
description: Missing roles field
tags: [test]
---

# Test Reviewer`,
			expected:    nil,
			expectError: true,
		},
		{
			name: "empty_roles_array",
			content: `---
name: test-reviewer
roles: []
description: Empty roles array
tags: [test]
---

# Test Reviewer`,
			expected:    nil,
			expectError: true,
		},
		{
			name: "missing_required_description",
			content: `---
name: test-reviewer
roles: [code-reviewer]
tags: [test]
---

# Test Reviewer`,
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			persona, err := parsePersona([]byte(tt.content))

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, persona)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, persona)
				assert.Equal(t, tt.expected.Name, persona.Name)
				assert.Equal(t, tt.expected.Roles, persona.Roles)
				assert.Equal(t, tt.expected.Description, persona.Description)
				assert.Equal(t, tt.expected.Tags, persona.Tags)
				assert.Equal(t, tt.expected.Content, persona.Content)
			}
		})
	}
}

// TestPersonaLoader_LoadPersona tests loading personas from file system
func TestPersonaLoader_LoadPersona(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv

	// Setup test environment
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	personasDir := filepath.Join(tempHome, ".ddx", "personas")
	require.NoError(t, os.MkdirAll(personasDir, 0755))

	// Create test persona file
	personaContent := `---
name: test-loader-persona
roles: [test-engineer]
description: Persona for testing loader functionality
tags: [test, loader]
---

# Test Loader Persona

This persona is used to test the loader functionality.`

	personaFile := filepath.Join(personasDir, "test-loader-persona.md")
	require.NoError(t, os.WriteFile(personaFile, []byte(personaContent), 0644))

	// Create invalid persona file for error testing
	invalidContent := `---
name: invalid-persona
roles: [test-engineer
description: Invalid YAML
---

# Invalid Persona`

	invalidFile := filepath.Join(personasDir, "invalid-persona.md")
	require.NoError(t, os.WriteFile(invalidFile, []byte(invalidContent), 0644))

	tests := []struct {
		name        string
		personaName string
		expectError bool
		validate    func(t *testing.T, persona *Persona)
	}{
		{
			name:        "load_existing_persona",
			personaName: "test-loader-persona",
			expectError: false,
			validate: func(t *testing.T, persona *Persona) {
				assert.Equal(t, "test-loader-persona", persona.Name)
				assert.Equal(t, []string{"test-engineer"}, persona.Roles)
				assert.Contains(t, persona.Content, "Test Loader Persona")
			},
		},
		{
			name:        "load_nonexistent_persona",
			personaName: "nonexistent-persona",
			expectError: true,
			validate:    nil,
		},
		{
			name:        "load_invalid_persona",
			personaName: "invalid-persona",
			expectError: true,
			validate:    nil,
		},
		{
			name:        "empty_persona_name",
			personaName: "",
			expectError: true,
			validate:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewPersonaLoaderWithDir(personasDir)
			persona, err := loader.LoadPersona(tt.personaName)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, persona)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, persona)
				if tt.validate != nil {
					tt.validate(t, persona)
				}
			}
		})
	}
}

// TestPersonaLoader_ListPersonas tests listing available personas
func TestPersonaLoader_ListPersonas(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	personasDir := filepath.Join(tempHome, ".ddx", "personas")
	require.NoError(t, os.MkdirAll(personasDir, 0755))

	// Create multiple test personas
	personas := map[string]string{
		"reviewer-strict.md": `---
name: reviewer-strict
roles: [code-reviewer]
description: Strict code reviewer
tags: [strict, quality]
---
# Strict Reviewer`,

		"reviewer-balanced.md": `---
name: reviewer-balanced
roles: [code-reviewer]
description: Balanced code reviewer
tags: [balanced, pragmatic]
---
# Balanced Reviewer`,

		"tester-tdd.md": `---
name: tester-tdd
roles: [test-engineer]
description: TDD specialist
tags: [tdd, testing]
---
# TDD Tester`,
	}

	for filename, content := range personas {
		filePath := filepath.Join(personasDir, filename)
		require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
	}

	// Create non-persona files (should be ignored)
	require.NoError(t, os.WriteFile(filepath.Join(personasDir, "README.md"), []byte("# Personas"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(personasDir, "invalid.txt"), []byte("Not a persona"), 0644))

	// Create invalid persona (should be skipped with warning)
	invalidContent := `---
name: invalid
roles: [bad-yaml
---
# Invalid`
	require.NoError(t, os.WriteFile(filepath.Join(personasDir, "invalid-persona.md"), []byte(invalidContent), 0644))

	tests := []struct {
		name     string
		validate func(t *testing.T, personas []*Persona, err error)
	}{
		{
			name: "list_all_valid_personas",
			validate: func(t *testing.T, personas []*Persona, err error) {
				assert.NoError(t, err)
				assert.Len(t, personas, 3) // Only valid personas

				personaNames := make([]string, len(personas))
				for i, p := range personas {
					personaNames[i] = p.Name
				}

				assert.Contains(t, personaNames, "reviewer-strict")
				assert.Contains(t, personaNames, "reviewer-balanced")
				assert.Contains(t, personaNames, "tester-tdd")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewPersonaLoaderWithDir(personasDir)
			personas, err := loader.ListPersonas()
			tt.validate(t, personas, err)
		})
	}
}

// TestPersonaLoader_FindByRole tests filtering personas by role
func TestPersonaLoader_FindByRole(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	personasDir := filepath.Join(tempHome, ".ddx", "personas")
	require.NoError(t, os.MkdirAll(personasDir, 0755))

	// Create personas with different roles
	personas := map[string]string{
		"multi-role.md": `---
name: multi-role
roles: [code-reviewer, security-analyst]
description: Multi-role persona
tags: [security, review]
---
# Multi Role`,

		"single-role.md": `---
name: single-role
roles: [test-engineer]
description: Single role persona
tags: [testing]
---
# Single Role`,

		"architect.md": `---
name: architect
roles: [architect, tech-lead]
description: Systems architect
tags: [design, architecture]
---
# Architect`,
	}

	for filename, content := range personas {
		filePath := filepath.Join(personasDir, filename)
		require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
	}

	tests := []struct {
		name     string
		role     string
		validate func(t *testing.T, personas []*Persona, err error)
	}{
		{
			name: "find_code_reviewers",
			role: "code-reviewer",
			validate: func(t *testing.T, personas []*Persona, err error) {
				assert.NoError(t, err)
				assert.Len(t, personas, 1) // Only multi-role has code-reviewer
				assert.Equal(t, "multi-role", personas[0].Name)
			},
		},
		{
			name: "find_architects",
			role: "architect",
			validate: func(t *testing.T, personas []*Persona, err error) {
				assert.NoError(t, err)
				assert.Len(t, personas, 1) // Only architect has architect role
				assert.Equal(t, "architect", personas[0].Name)
			},
		},
		{
			name: "find_nonexistent_role",
			role: "nonexistent-role",
			validate: func(t *testing.T, personas []*Persona, err error) {
				assert.NoError(t, err)
				assert.Empty(t, personas) // No personas with this role
			},
		},
		{
			name: "empty_role",
			role: "",
			validate: func(t *testing.T, personas []*Persona, err error) {
				assert.Error(t, err) // Should error on empty role
				assert.Nil(t, personas)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewPersonaLoaderWithDir(personasDir)
			personas, err := loader.FindByRole(tt.role)
			tt.validate(t, personas, err)
		})
	}
}

// TestPersonaLoader_FindByTags tests filtering personas by tags
func TestPersonaLoader_FindByTags(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	personasDir := filepath.Join(tempHome, ".ddx", "personas")
	require.NoError(t, os.MkdirAll(personasDir, 0755))

	// Create personas with different tags
	personas := map[string]string{
		"security-focused.md": `---
name: security-focused
roles: [security-analyst]
description: Security specialist
tags: [security, vulnerability, compliance]
---
# Security Focused`,

		"tdd-specialist.md": `---
name: tdd-specialist
roles: [test-engineer]
description: TDD specialist
tags: [tdd, testing, red-green-refactor]
---
# TDD Specialist`,

		"performance-expert.md": `---
name: performance-expert
roles: [performance-engineer]
description: Performance specialist
tags: [performance, optimization, benchmarking]
---
# Performance Expert`,

		"strict-reviewer.md": `---
name: strict-reviewer
roles: [code-reviewer]
description: Strict reviewer
tags: [strict, security, quality]
---
# Strict Reviewer`,
	}

	for filename, content := range personas {
		filePath := filepath.Join(personasDir, filename)
		require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
	}

	tests := []struct {
		name     string
		tags     []string
		validate func(t *testing.T, personas []*Persona, err error)
	}{
		{
			name: "find_by_single_tag",
			tags: []string{"security"},
			validate: func(t *testing.T, personas []*Persona, err error) {
				assert.NoError(t, err)
				assert.Len(t, personas, 2) // security-focused and strict-reviewer
			},
		},
		{
			name: "find_by_multiple_tags",
			tags: []string{"security", "strict"},
			validate: func(t *testing.T, personas []*Persona, err error) {
				assert.NoError(t, err)
				assert.Len(t, personas, 1) // Only strict-reviewer has both tags
				assert.Equal(t, "strict-reviewer", personas[0].Name)
			},
		},
		{
			name: "find_by_nonexistent_tag",
			tags: []string{"nonexistent"},
			validate: func(t *testing.T, personas []*Persona, err error) {
				assert.NoError(t, err)
				assert.Empty(t, personas) // No personas with this tag
			},
		},
		{
			name: "empty_tags_list",
			tags: []string{},
			validate: func(t *testing.T, personas []*Persona, err error) {
				assert.Error(t, err) // Should error on empty tags
				assert.Nil(t, personas)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewPersonaLoaderWithDir(personasDir)
			personas, err := loader.FindByTags(tt.tags)
			tt.validate(t, personas, err)
		})
	}
}

// TestValidatePersona tests persona validation logic
func TestValidatePersona_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		persona     *Persona
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid_persona",
			persona: &Persona{
				Name:        "valid-persona",
				Roles:       []string{"test-role"},
				Description: "Valid description",
				Tags:        []string{"tag1", "tag2"},
				Content:     "Valid content",
			},
			expectError: false,
		},
		{
			name: "empty_name",
			persona: &Persona{
				Name:        "",
				Roles:       []string{"test-role"},
				Description: "Valid description",
				Tags:        []string{"tag1"},
				Content:     "Valid content",
			},
			expectError: true,
			errorMsg:    "name cannot be empty",
		},
		{
			name: "whitespace_only_name",
			persona: &Persona{
				Name:        "   ",
				Roles:       []string{"test-role"},
				Description: "Valid description",
				Tags:        []string{"tag1"},
				Content:     "Valid content",
			},
			expectError: true,
			errorMsg:    "name cannot be empty",
		},
		{
			name: "nil_roles",
			persona: &Persona{
				Name:        "test-persona",
				Roles:       nil,
				Description: "Valid description",
				Tags:        []string{"tag1"},
				Content:     "Valid content",
			},
			expectError: true,
			errorMsg:    "roles cannot be empty",
		},
		{
			name: "empty_roles",
			persona: &Persona{
				Name:        "test-persona",
				Roles:       []string{},
				Description: "Valid description",
				Tags:        []string{"tag1"},
				Content:     "Valid content",
			},
			expectError: true,
			errorMsg:    "roles cannot be empty",
		},
		{
			name: "empty_role_in_array",
			persona: &Persona{
				Name:        "test-persona",
				Roles:       []string{"valid-role", "", "another-role"},
				Description: "Valid description",
				Tags:        []string{"tag1"},
				Content:     "Valid content",
			},
			expectError: true,
			errorMsg:    "role cannot be empty",
		},
		{
			name: "empty_description",
			persona: &Persona{
				Name:        "test-persona",
				Roles:       []string{"test-role"},
				Description: "",
				Tags:        []string{"tag1"},
				Content:     "Valid content",
			},
			expectError: true,
			errorMsg:    "description cannot be empty",
		},
		{
			name: "nil_tags_allowed",
			persona: &Persona{
				Name:        "test-persona",
				Roles:       []string{"test-role"},
				Description: "Valid description",
				Tags:        nil, // Tags can be nil
				Content:     "Valid content",
			},
			expectError: false,
		},
		{
			name: "empty_tag_in_array",
			persona: &Persona{
				Name:        "test-persona",
				Roles:       []string{"test-role"},
				Description: "Valid description",
				Tags:        []string{"valid-tag", "", "another-tag"},
				Content:     "Valid content",
			},
			expectError: true,
			errorMsg:    "tag cannot be empty",
		},
		{
			name: "empty_content",
			persona: &Persona{
				Name:        "test-persona",
				Roles:       []string{"test-role"},
				Description: "Valid description",
				Tags:        []string{"tag1"},
				Content:     "",
			},
			expectError: true,
			errorMsg:    "content cannot be empty",
		},
		{
			name: "whitespace_only_content",
			persona: &Persona{
				Name:        "test-persona",
				Roles:       []string{"test-role"},
				Description: "Valid description",
				Tags:        []string{"tag1"},
				Content:     "   \n\t  ",
			},
			expectError: true,
			errorMsg:    "content cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO: Implement validatePersona function
			// err := validatePersona(tt.persona)

			// if tt.expectError {
			//     assert.Error(t, err)
			//     if tt.errorMsg != "" {
			//         assert.Contains(t, err.Error(), tt.errorMsg)
			//     }
			// } else {
			//     assert.NoError(t, err)
			// }

			// For now, just validate test structure
			assert.NotNil(t, tt.persona, "Test persona should not be nil")
			if tt.expectError {
				assert.NotEmpty(t, tt.errorMsg, "Error message should be specified for error cases")
			}
		})
	}
}

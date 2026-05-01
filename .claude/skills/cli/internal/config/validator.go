package config

import (
	_ "embed"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

//go:embed schema/config.schema.json
var schemaJSON []byte

// Validator validates DDx configuration files using two-phase validation
type Validator interface {
	Validate(content []byte) error
	ValidateFile(path string) error
}

// ConfigValidator implements two-phase validation for DDx configuration
type ConfigValidator struct {
	schema *jsonschema.Schema
}

// NewValidator creates a new configuration validator
func NewValidator() (*ConfigValidator, error) {
	compiler := jsonschema.NewCompiler()

	// Load the embedded schema
	if err := compiler.AddResource("config.schema.json", strings.NewReader(string(schemaJSON))); err != nil {
		return nil, fmt.Errorf("failed to add schema resource: %w", err)
	}

	schema, err := compiler.Compile("config.schema.json")
	if err != nil {
		return nil, fmt.Errorf("failed to compile schema: %w", err)
	}

	return &ConfigValidator{schema: schema}, nil
}

// Validate performs two-phase validation on configuration content
func (v *ConfigValidator) Validate(content []byte) error {
	// Phase 1: YAML syntax validation
	var rawConfig interface{}
	if err := yaml.Unmarshal(content, &rawConfig); err != nil {
		return &ConfigValidationError{
			Phase:   "syntax",
			Message: "Invalid YAML syntax",
			Details: err.Error(),
			Line:    extractLineNumber(err),
			Column:  extractColumnNumber(err),
		}
	}

	// Phase 2: Schema validation
	if err := v.schema.Validate(rawConfig); err != nil {
		return &ConfigValidationError{
			Phase:       "schema",
			Message:     "Configuration does not match schema",
			Details:     formatSchemaErrors(err),
			Suggestions: generateSuggestions(err),
		}
	}

	return nil
}

// ValidateFile validates a configuration file by path
func (v *ConfigValidator) ValidateFile(path string) error {
	content, err := readFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	return v.Validate(content)
}

// ConfigValidationError represents a configuration validation error
type ConfigValidationError struct {
	Phase       string   // "syntax" or "schema"
	Message     string   // User-friendly message
	Line        int      // Line number (syntax errors)
	Column      int      // Column number (syntax errors)
	FieldPath   string   // Field path (schema errors)
	Details     string   // Technical details
	Suggestions []string // Helpful suggestions
}

func (e *ConfigValidationError) Error() string {
	var parts []string

	parts = append(parts, fmt.Sprintf("[%s] %s", strings.ToUpper(e.Phase), e.Message))

	if e.Line > 0 {
		parts = append(parts, fmt.Sprintf("at line %d", e.Line))
		if e.Column > 0 {
			parts[len(parts)-1] += fmt.Sprintf(", column %d", e.Column)
		}
	}

	if e.FieldPath != "" {
		parts = append(parts, fmt.Sprintf("field: %s", e.FieldPath))
	}

	result := strings.Join(parts, " ")

	if e.Details != "" {
		result += fmt.Sprintf("\n\nDetails: %s", e.Details)
	}

	if len(e.Suggestions) > 0 {
		result += "\n\nSuggestions:"
		for _, suggestion := range e.Suggestions {
			result += fmt.Sprintf("\n  - %s", suggestion)
		}
	}

	return result
}

// formatSchemaErrors converts JSON schema errors to user-friendly messages
func formatSchemaErrors(err error) string {
	validationErr, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return err.Error()
	}

	var messages []string

	// Process all validation errors
	for _, cause := range flattenErrors(validationErr) {
		message := formatSingleError(cause)
		if message != "" {
			messages = append(messages, message)
		}
	}

	if len(messages) == 0 {
		return validationErr.Message
	}

	return strings.Join(messages, "\n")
}

// flattenErrors recursively flattens nested validation errors
func flattenErrors(err *jsonschema.ValidationError) []*jsonschema.ValidationError {
	var errors []*jsonschema.ValidationError

	if len(err.Causes) == 0 {
		errors = append(errors, err)
	} else {
		for _, cause := range err.Causes {
			errors = append(errors, flattenErrors(cause)...)
		}
	}

	return errors
}

// formatSingleError formats a single JSON schema validation error
func formatSingleError(err *jsonschema.ValidationError) string {
	fieldPath := strings.TrimPrefix(err.InstanceLocation, "/")
	fieldPath = strings.ReplaceAll(fieldPath, "/", ".")

	message := err.Message

	// Try to make messages more user-friendly based on common patterns
	if strings.Contains(message, "missing properties") {
		missing := extractMissingProperty(message)
		if missing != "" {
			if fieldPath == "" {
				return fmt.Sprintf("missing required field: %s", missing)
			}
			return fmt.Sprintf("missing required field: %s.%s", fieldPath, missing)
		}
		return fmt.Sprintf("missing required field in %s", fieldPath)
	}

	if strings.Contains(message, "expected") && strings.Contains(message, "but got") {
		expected := extractExpectedType(message)
		if expected != "" {
			return fmt.Sprintf("%s: must be %s", fieldPath, expected)
		}
		return fmt.Sprintf("%s: invalid type", fieldPath)
	}

	if strings.Contains(message, "does not match pattern") && fieldPath == "version" {
		return "version: must be in format 'X.Y' (e.g., '1.0', '2.1')"
	}

	if strings.Contains(message, "does not match pattern") {
		return fmt.Sprintf("%s: format is invalid", fieldPath)
	}

	if strings.Contains(message, "invalid format") {
		if strings.Contains(message, "uri") {
			return fmt.Sprintf("%s: must be a valid URL (e.g., 'https://github.com/user/repo')", fieldPath)
		}
		if strings.Contains(message, "email") {
			return fmt.Sprintf("%s: must be a valid email address", fieldPath)
		}
		return fmt.Sprintf("%s: invalid format", fieldPath)
	}

	if strings.Contains(message, "additional property") {
		property := extractAdditionalProperty(message)
		if property != "" {
			return fmt.Sprintf("unknown field: %s", property)
		}
		return "contains unknown fields"
	}

	// Default: use original message with field path
	if fieldPath != "" {
		return fmt.Sprintf("%s: %s", fieldPath, message)
	}
	return message
}

// generateSuggestions provides helpful suggestions based on validation errors
func generateSuggestions(err error) []string {
	validationErr, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return nil
	}

	var suggestions []string

	for _, cause := range flattenErrors(validationErr) {
		message := cause.Message

		if strings.Contains(message, "missing properties") && strings.Contains(message, "version") {
			suggestions = append(suggestions, "Add 'version: \"1.0\"' to your configuration")
		}

		if strings.Contains(message, "does not match pattern") {
			fieldPath := strings.TrimPrefix(cause.InstanceLocation, "/")
			if fieldPath == "version" {
				suggestions = append(suggestions, "Use version format like '1.0' or '2.1' (with quotes)")
			}
		}

		if strings.Contains(message, "invalid format") || strings.Contains(message, "format") {
			if strings.Contains(message, "uri") {
				suggestions = append(suggestions, "Ensure URL starts with 'https://' or 'http://'")
			}
			if strings.Contains(message, "email") {
				suggestions = append(suggestions, "Use format like 'user@example.com'")
			}
		}

		if strings.Contains(message, "expected") && strings.Contains(message, "string") {
			fieldPath := strings.TrimPrefix(cause.InstanceLocation, "/")
			if fieldPath != "" {
				suggestions = append(suggestions, fmt.Sprintf("Wrap %s value in quotes", fieldPath))
			}
		}

		if strings.Contains(message, "additional property") {
			suggestions = append(suggestions, "Check for typos in field names")
			suggestions = append(suggestions, "See documentation for allowed configuration fields")
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	var unique []string
	for _, suggestion := range suggestions {
		if !seen[suggestion] {
			seen[suggestion] = true
			unique = append(unique, suggestion)
		}
	}

	return unique
}

// Helper functions for error parsing

var lineRegex = regexp.MustCompile(`line (\d+)`)
var columnRegex = regexp.MustCompile(`column (\d+)`)

func extractLineNumber(err error) int {
	matches := lineRegex.FindStringSubmatch(err.Error())
	if len(matches) > 1 {
		if line, parseErr := strconv.Atoi(matches[1]); parseErr == nil {
			return line
		}
	}
	return 0
}

func extractColumnNumber(err error) int {
	matches := columnRegex.FindStringSubmatch(err.Error())
	if len(matches) > 1 {
		if col, parseErr := strconv.Atoi(matches[1]); parseErr == nil {
			return col
		}
	}
	return 0
}

func extractMissingProperty(message string) string {
	// Extract property name from messages like "missing properties: 'version'"
	re := regexp.MustCompile(`missing properties?: '([^']+)'`)
	matches := re.FindStringSubmatch(message)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func extractExpectedType(message string) string {
	// Extract type from messages like "expected string, but got number"
	re := regexp.MustCompile(`expected (\w+)`)
	matches := re.FindStringSubmatch(message)
	if len(matches) > 1 {
		return "a " + matches[1]
	}
	return ""
}

func extractAdditionalProperty(message string) string {
	// Extract property name from messages about additional properties
	re := regexp.MustCompile(`additional property '([^']+)' is not allowed`)
	matches := re.FindStringSubmatch(message)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// readFile is a helper function that can be mocked for testing
var readFile = os.ReadFile

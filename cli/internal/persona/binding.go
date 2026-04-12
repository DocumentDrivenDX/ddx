package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// BindingManagerImpl implements the BindingManager interface
type BindingManagerImpl struct {
	configPath string
}

// NewBindingManager creates a new binding manager with the default config path
func NewBindingManager() BindingManager {
	return &BindingManagerImpl{
		configPath: ConfigFileName, // ".ddx.yml" in current directory
	}
}

// NewBindingManagerWithPath creates a new binding manager with a specific config path
func NewBindingManagerWithPath(configPath string) BindingManager {
	return &BindingManagerImpl{
		configPath: configPath,
	}
}

// GetBinding returns the persona bound to the specified role
func (b *BindingManagerImpl) GetBinding(role string) (string, error) {
	if strings.TrimSpace(role) == "" {
		return "", NewPersonaError(ErrorValidation, "role cannot be empty", nil)
	}

	config, err := b.loadConfig()
	if err != nil {
		// Return the error so CLI can handle it
		return "", err
	}

	if config.Bindings == nil {
		return "", nil
	}

	persona, exists := config.Bindings[role]
	if !exists {
		return "", nil
	}

	return persona, nil
}

// SetBinding binds a persona to a role
func (b *BindingManagerImpl) SetBinding(role, persona string) error {
	if strings.TrimSpace(role) == "" {
		return NewPersonaError(ErrorValidation, "role cannot be empty", nil)
	}
	if strings.TrimSpace(persona) == "" {
		return NewPersonaError(ErrorValidation, "persona cannot be empty", nil)
	}

	// Load existing config or create new one
	var config map[string]interface{}

	if fileExists(b.configPath) {
		content, err := os.ReadFile(b.configPath)
		if err != nil {
			return NewPersonaError(ErrorFileOperation,
				fmt.Sprintf("failed to read config file %s", b.configPath), err)
		}

		if err := yaml.Unmarshal(content, &config); err != nil {
			return NewPersonaError(ErrorInvalidConfig,
				fmt.Sprintf("failed to parse config file %s", b.configPath), err)
		}
	} else {
		config = make(map[string]interface{})
	}

	// Ensure persona_bindings section exists
	bindings, exists := config["persona_bindings"]
	if !exists {
		bindings = make(map[string]interface{})
		config["persona_bindings"] = bindings
	}

	// Convert to map if needed
	bindingsMap, ok := bindings.(map[string]interface{})
	if !ok {
		bindingsMap = make(map[string]interface{})
		config["persona_bindings"] = bindingsMap
	}

	// Set the binding
	bindingsMap[role] = persona

	// Save config
	return b.saveConfig(config)
}

// GetAllBindings returns all current role-persona bindings
func (b *BindingManagerImpl) GetAllBindings() (map[string]string, error) {
	config, err := b.loadConfig()
	if err != nil {
		// Return the error so CLI can handle it
		return nil, err
	}

	if config.Bindings == nil {
		return make(map[string]string), nil
	}

	// Return a copy to prevent modification
	result := make(map[string]string)
	for role, persona := range config.Bindings {
		result[role] = persona
	}

	return result, nil
}

// RemoveBinding removes the binding for the specified role
func (b *BindingManagerImpl) RemoveBinding(role string) error {
	if strings.TrimSpace(role) == "" {
		return NewPersonaError(ErrorValidation, "role cannot be empty", nil)
	}

	// Load existing config
	if !fileExists(b.configPath) {
		return nil // Nothing to remove
	}

	content, err := os.ReadFile(b.configPath)
	if err != nil {
		return NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to read config file %s", b.configPath), err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(content, &config); err != nil {
		return NewPersonaError(ErrorInvalidConfig,
			fmt.Sprintf("failed to parse config file %s", b.configPath), err)
	}

	// Check if persona_bindings section exists
	bindings, exists := config["persona_bindings"]
	if !exists {
		return nil // Nothing to remove
	}

	bindingsMap, ok := bindings.(map[string]interface{})
	if !ok {
		return nil // Invalid bindings format, nothing to remove
	}

	// Remove the binding
	delete(bindingsMap, role)

	// If bindings is now empty, we can leave it (no need to remove the section)

	// Save config
	return b.saveConfig(config)
}

// GetOverride returns the persona override for a specific workflow and role
func (b *BindingManagerImpl) GetOverride(workflow, role string) (string, error) {
	if strings.TrimSpace(workflow) == "" {
		return "", NewPersonaError(ErrorValidation, "workflow cannot be empty", nil)
	}
	if strings.TrimSpace(role) == "" {
		return "", NewPersonaError(ErrorValidation, "role cannot be empty", nil)
	}

	config, err := b.loadConfig()
	if err != nil {
		// If config doesn't exist, return empty string (no override)
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	if config.Overrides == nil {
		return "", nil
	}

	workflowOverrides, exists := config.Overrides[workflow]
	if !exists {
		return "", nil
	}

	persona, exists := workflowOverrides[role]
	if !exists {
		return "", nil
	}

	return persona, nil
}

// loadConfig loads the persona configuration from the config file
func (b *BindingManagerImpl) loadConfig() (*PersonaConfig, error) {
	if !fileExists(b.configPath) {
		return nil, os.ErrNotExist
	}

	content, err := os.ReadFile(b.configPath)
	if err != nil {
		return nil, NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to read config file %s", b.configPath), err)
	}

	// Parse as generic map first to handle the full config structure
	var fullConfig map[string]interface{}
	if err := yaml.Unmarshal(content, &fullConfig); err != nil {
		return nil, NewPersonaError(ErrorInvalidConfig,
			fmt.Sprintf("failed to parse config file %s", b.configPath), err)
	}

	// Extract persona configuration
	var personaConfig PersonaConfig

	// Extract persona_bindings
	if bindings, exists := fullConfig["persona_bindings"]; exists {
		if bindingsMap, ok := bindings.(map[string]interface{}); ok {
			personaConfig.Bindings = make(map[string]string)
			for role, persona := range bindingsMap {
				if personaStr, ok := persona.(string); ok {
					personaConfig.Bindings[role] = personaStr
				}
			}
		}
	}

	// Extract overrides
	if overrides, exists := fullConfig["overrides"]; exists {
		if overridesMap, ok := overrides.(map[string]interface{}); ok {
			personaConfig.Overrides = make(map[string]map[string]string)
			for workflow, workflowOverrides := range overridesMap {
				if workflowMap, ok := workflowOverrides.(map[string]interface{}); ok {
					personaConfig.Overrides[workflow] = make(map[string]string)
					for role, persona := range workflowMap {
						if personaStr, ok := persona.(string); ok {
							personaConfig.Overrides[workflow][role] = personaStr
						}
					}
				}
			}
		}
	}

	return &personaConfig, nil
}

// saveConfig saves the configuration to the config file
func (b *BindingManagerImpl) saveConfig(config map[string]interface{}) error {
	// Ensure the directory exists
	dir := filepath.Dir(b.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to create config directory %s", dir), err)
	}

	// Marshal to YAML
	content, err := yaml.Marshal(config)
	if err != nil {
		return NewPersonaError(ErrorInvalidConfig,
			"failed to marshal config to YAML", err)
	}

	// Write file
	if err := os.WriteFile(b.configPath, content, 0644); err != nil {
		return NewPersonaError(ErrorFileOperation,
			fmt.Sprintf("failed to write config file %s", b.configPath), err)
	}

	return nil
}

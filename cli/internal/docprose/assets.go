package docprose

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

const assetRootDir = "library/checks/prose-quality"

type defaultConfig struct {
	Mode       string     `yaml:"mode"`
	Severity   string     `yaml:"severity"`
	Policy     string     `yaml:"policy"`
	Runner     string     `yaml:"runner"`
	Vale       ValeConfig `yaml:"vale"`
	StylePack  StylePack  `yaml:"rule_pack"`
	Includes   []string   `yaml:"includes"`
	Excludes   []string   `yaml:"excludes"`
	Vocabulary Vocabulary `yaml:"vocabulary"`
}

type ValeConfig struct {
	Version    string `yaml:"version"`
	StylesPath string `yaml:"styles_path"`
	Style      string `yaml:"style"`
}

type StylePack struct {
	Name    string      `yaml:"name"`
	Version string      `yaml:"version"`
	Path    string      `yaml:"path"`
	Rules   []StyleRule `yaml:"rules"`
}

type StyleRule struct {
	File   string `yaml:"file"`
	RuleID string `yaml:"rule_id"`
}

type rulePack struct {
	Mode  string     `yaml:"mode"`
	Rules []ruleSpec `yaml:"rules"`
}

type ruleSpec struct {
	ID            string   `yaml:"id"`
	Severity      string   `yaml:"severity"`
	Rationale     string   `yaml:"rationale"`
	SuggestedEdit string   `yaml:"suggested_edit"`
	ContainsAny   []string `yaml:"contains_any,omitempty"`
	Kind          string   `yaml:"kind,omitempty"`
}

type Vocabulary struct {
	DefaultPath string   `yaml:"default_path,omitempty"`
	ValeStyle   string   `yaml:"vale_style,omitempty"`
	Accept      []string `yaml:"accept"`
	Reject      []string `yaml:"reject"`
}

func defaultAssetRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("locate docprose assets: runtime caller unavailable")
	}
	dir := filepath.Dir(file)
	root := filepath.Clean(filepath.Join(dir, "..", "..", ".."))
	return filepath.Join(root, assetRootDir), nil
}

func loadDefaultConfig() (defaultConfig, error) {
	root, err := defaultAssetRoot()
	if err != nil {
		return defaultConfig{}, err
	}
	data, err := os.ReadFile(filepath.Join(root, "check.yaml"))
	if err != nil {
		return defaultConfig{}, fmt.Errorf("read default config: %w", err)
	}
	var cfg defaultConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return defaultConfig{}, fmt.Errorf("parse default config: %w", err)
	}
	return cfg, nil
}

func loadRulePack(mode string) ([]ruleSpec, error) {
	root, err := defaultAssetRoot()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(root, "rules", mode+".yaml"))
	if err != nil {
		return nil, fmt.Errorf("read %s rules: %w", mode, err)
	}
	var pack rulePack
	if err := yaml.Unmarshal(data, &pack); err != nil {
		return nil, fmt.Errorf("parse %s rules: %w", mode, err)
	}
	if pack.Mode != mode {
		return nil, fmt.Errorf("rules file mode %q does not match requested mode %q", pack.Mode, mode)
	}
	return pack.Rules, nil
}

func loadDefaultVocabulary() (Vocabulary, error) {
	root, err := defaultAssetRoot()
	if err != nil {
		return Vocabulary{}, err
	}
	data, err := os.ReadFile(filepath.Join(root, "vocabulary", "default.yaml"))
	if err != nil {
		return Vocabulary{}, fmt.Errorf("read default vocabulary: %w", err)
	}
	var vocab Vocabulary
	if err := yaml.Unmarshal(data, &vocab); err != nil {
		return Vocabulary{}, fmt.Errorf("parse default vocabulary: %w", err)
	}
	return vocab, nil
}

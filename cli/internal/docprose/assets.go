package docprose

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const assetRootDir = "assets/prose-quality"

//go:embed assets/prose-quality
var defaultAssets embed.FS

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

func loadDefaultConfig() (defaultConfig, error) {
	data, err := readDefaultAsset("check.yaml")
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
	data, err := readDefaultAsset(filepath.Join("rules", mode+".yaml"))
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
	data, err := readDefaultAsset(filepath.Join("vocabulary", "default.yaml"))
	if err != nil {
		return Vocabulary{}, fmt.Errorf("read default vocabulary: %w", err)
	}
	var vocab Vocabulary
	if err := yaml.Unmarshal(data, &vocab); err != nil {
		return Vocabulary{}, fmt.Errorf("parse default vocabulary: %w", err)
	}
	return vocab, nil
}

func readDefaultAsset(rel string) ([]byte, error) {
	return defaultAssets.ReadFile(filepath.ToSlash(filepath.Join(assetRootDir, rel)))
}

func materializeDefaultAssetDir(rel, dst string) error {
	srcRoot := filepath.ToSlash(filepath.Join(assetRootDir, rel))
	return fs.WalkDir(defaultAssets, srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		sub, err := filepath.Rel(filepath.FromSlash(srcRoot), filepath.FromSlash(path))
		if err != nil {
			return err
		}
		target := filepath.Join(dst, sub)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := defaultAssets.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

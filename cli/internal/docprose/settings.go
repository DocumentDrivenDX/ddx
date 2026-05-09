package docprose

import ddxconfig "github.com/DocumentDrivenDX/ddx/internal/config"

// Settings captures the prose-quality defaults packaged with DDx.
type Settings struct {
	Mode       Mode
	Severity   string
	Policy     string
	Runner     string
	Vale       ValeConfig
	StylePack  StylePack
	Includes   []string
	Excludes   []string
	Vocabulary Vocabulary
}

// DefaultSettings returns the packaged prose-quality defaults from the
// embedded check asset tree.
func DefaultSettings() (Settings, error) {
	cfg, err := loadDefaultConfig()
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		Mode:     Mode(cfg.Mode),
		Severity: cfg.Severity,
		Policy:   cfg.Policy,
		Runner:   cfg.Runner,
		Vale: ValeConfig{
			Version:    cfg.Vale.Version,
			StylesPath: cfg.Vale.StylesPath,
			Style:      cfg.Vale.Style,
		},
		StylePack: StylePack{
			Name:    cfg.StylePack.Name,
			Version: cfg.StylePack.Version,
			Path:    cfg.StylePack.Path,
			Rules:   append([]StyleRule(nil), cfg.StylePack.Rules...),
		},
		Includes: append([]string(nil), cfg.Includes...),
		Excludes: append([]string(nil), cfg.Excludes...),
		Vocabulary: Vocabulary{
			DefaultPath: cfg.Vocabulary.DefaultPath,
			ValeStyle:   cfg.Vocabulary.ValeStyle,
			Accept:      append([]string(nil), cfg.Vocabulary.Accept...),
			Reject:      append([]string(nil), cfg.Vocabulary.Reject...),
		},
	}, nil
}

// ResolveSettings returns the effective prose-quality settings after layering
// any project configuration onto the packaged defaults.
func ResolveSettings(cfg *ddxconfig.Config) (Settings, error) {
	settings, err := DefaultSettings()
	if err != nil {
		return Settings{}, err
	}
	if cfg == nil || cfg.Prose == nil {
		return settings, nil
	}

	if cfg.Prose.Mode != "" {
		settings.Mode = Mode(cfg.Prose.Mode)
	}
	if cfg.Prose.Severity != "" {
		settings.Severity = cfg.Prose.Severity
	}
	if cfg.Prose.Policy != "" {
		settings.Policy = cfg.Prose.Policy
	}
	if cfg.Prose.Runner != "" {
		settings.Runner = cfg.Prose.Runner
	}
	if len(cfg.Prose.Includes) > 0 {
		settings.Includes = append([]string(nil), cfg.Prose.Includes...)
	}
	if len(cfg.Prose.Excludes) > 0 {
		settings.Excludes = append([]string(nil), cfg.Prose.Excludes...)
	}
	if cfg.Prose.Vocabulary != nil {
		if len(cfg.Prose.Vocabulary.Accept) > 0 {
			settings.Vocabulary.Accept = append([]string(nil), cfg.Prose.Vocabulary.Accept...)
		}
		if len(cfg.Prose.Vocabulary.Reject) > 0 {
			settings.Vocabulary.Reject = append([]string(nil), cfg.Prose.Vocabulary.Reject...)
		}
	}

	return settings, nil
}

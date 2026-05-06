package docprose

// Settings captures the prose-quality defaults packaged with DDx.
type Settings struct {
	Mode       Mode
	Severity   string
	Policy     string
	Runner     string
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
		Includes: append([]string(nil), cfg.Includes...),
		Excludes: append([]string(nil), cfg.Excludes...),
		Vocabulary: Vocabulary{
			Accept: append([]string(nil), cfg.Vocabulary.Accept...),
			Reject: append([]string(nil), cfg.Vocabulary.Reject...),
		},
	}, nil
}

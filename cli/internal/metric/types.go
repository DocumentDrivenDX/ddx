package metric

import "time"

const (
	ComparisonLowerIsBetter  = "lower-is-better"
	ComparisonHigherIsBetter = "higher-is-better"
	StatusPass               = "pass"
	StatusFail               = "fail"
)

// Definition describes an executable metric runtime record.
type Definition struct {
	DefinitionID string            `json:"definition_id"`
	MetricID     string            `json:"metric_id"`
	Command      []string          `json:"command"`
	Cwd          string            `json:"cwd,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	Thresholds   Thresholds        `json:"thresholds,omitempty"`
	Comparison   string            `json:"comparison,omitempty"`
	Active       bool              `json:"active"`
	CreatedAt    time.Time         `json:"created_at"`
}

// Thresholds describe the target values used for pass/fail evaluation.
type Thresholds struct {
	Warn    float64 `json:"warn,omitempty"`
	Ratchet float64 `json:"ratchet,omitempty"`
	Unit    string  `json:"unit,omitempty"`
}

// HistoryRecord stores one metric observation.
type HistoryRecord struct {
	RunID        string           `json:"run_id"`
	MetricID     string           `json:"metric_id"`
	DefinitionID string           `json:"definition_id"`
	ObservedAt   time.Time        `json:"observed_at"`
	Status       string           `json:"status"`
	Value        float64          `json:"value"`
	Unit         string           `json:"unit,omitempty"`
	Comparison   ComparisonResult `json:"comparison"`
	ExitCode     int              `json:"exit_code"`
	DurationMS   int64            `json:"duration_ms,omitempty"`
	Stdout       string           `json:"stdout,omitempty"`
	Stderr       string           `json:"stderr,omitempty"`
	ArtifactID   string           `json:"artifact_id,omitempty"`
}

// ComparisonResult records the comparison against a baseline or target.
type ComparisonResult struct {
	Baseline  float64 `json:"baseline"`
	Delta     float64 `json:"delta"`
	Direction string  `json:"direction"`
}

// TrendSummary aggregates a sequence of history records.
type TrendSummary struct {
	MetricID  string    `json:"metric_id"`
	Count     int       `json:"count"`
	Min       float64   `json:"min"`
	Max       float64   `json:"max"`
	Average   float64   `json:"average"`
	Latest    float64   `json:"latest"`
	Unit      string    `json:"unit,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

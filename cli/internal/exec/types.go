package exec

import (
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// AgentRunner is the interface exec needs from agent.
type AgentRunner interface {
	Run(opts agent.RunOptions) (*agent.Result, error)
}

const (
	ExecutorKindCommand = "command"
	ExecutorKindAgent   = "agent"

	StatusSuccess  = "success"
	StatusFailed   = "failed"
	StatusTimedOut = "timed_out"
	StatusErrored  = "errored"
)

// Definition describes a machine-readable execution contract.
type Definition struct {
	ID          string       `json:"id"`
	ArtifactIDs []string     `json:"artifact_ids"`
	Executor    ExecutorSpec `json:"executor"`
	Result      ResultSpec   `json:"result,omitempty"`
	Evaluation  Evaluation   `json:"evaluation,omitempty"`
	Active      bool         `json:"active"`
	CreatedAt   time.Time    `json:"created_at"`
}

// ExecutorSpec captures how a definition should be invoked.
type ExecutorSpec struct {
	Kind      string            `json:"kind"`
	Command   []string          `json:"command,omitempty"`
	Cwd       string            `json:"cwd,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	TimeoutMS int               `json:"timeout_ms,omitempty"`
}

// ResultSpec describes the structured result payload.
type ResultSpec struct {
	Metric *MetricResultSpec `json:"metric,omitempty"`
}

// MetricResultSpec defines the metric-shaped projection fields.
type MetricResultSpec struct {
	Unit        string `json:"unit,omitempty"`
	ValuePath   string `json:"value_path,omitempty"`
	SamplesPath string `json:"samples_path,omitempty"`
}

// Evaluation holds optional interpretation rules for a definition.
type Evaluation struct {
	Comparison string     `json:"comparison,omitempty"`
	Thresholds Thresholds `json:"thresholds,omitempty"`
}

// Thresholds captures the ratchet policy for metric-shaped results.
type Thresholds struct {
	WarnMS    float64 `json:"warn_ms,omitempty"`
	RatchetMS float64 `json:"ratchet_ms,omitempty"`
}

// RunManifest is the authoritative metadata record for one execution.
type RunManifest struct {
	RunID          string            `json:"run_id"`
	DefinitionID   string            `json:"definition_id"`
	ArtifactIDs    []string          `json:"artifact_ids"`
	StartedAt      time.Time         `json:"started_at"`
	FinishedAt     time.Time         `json:"finished_at"`
	Status         string            `json:"status"`
	ExitCode       int               `json:"exit_code"`
	AgentSessionID string            `json:"agent_session_id,omitempty"`
	Attachments    map[string]string `json:"attachments,omitempty"`
	Provenance     Provenance        `json:"provenance,omitempty"`
}

// Provenance captures host and version metadata for a run.
type Provenance struct {
	Actor      string `json:"actor,omitempty"`
	Host       string `json:"host,omitempty"`
	GitRev     string `json:"git_rev,omitempty"`
	DDXVersion string `json:"ddx_version,omitempty"`
}

// MetricObservation is the structured metric-shaped projection for a run.
type MetricObservation struct {
	ArtifactID   string           `json:"artifact_id"`
	DefinitionID string           `json:"definition_id"`
	ObservedAt   time.Time        `json:"observed_at"`
	Status       string           `json:"status"`
	Value        float64          `json:"value"`
	Unit         string           `json:"unit,omitempty"`
	Samples      []float64        `json:"samples,omitempty"`
	Comparison   ComparisonResult `json:"comparison,omitempty"`
}

// ComparisonResult records a comparison against a baseline or target.
type ComparisonResult struct {
	Baseline  float64 `json:"baseline"`
	Delta     float64 `json:"delta"`
	Direction string  `json:"direction"`
}

// RunResult stores the structured output for one execution run.
type RunResult struct {
	Metric *MetricObservation `json:"metric,omitempty"`
	Stdout string             `json:"stdout,omitempty"`
	Stderr string             `json:"stderr,omitempty"`
	Value  float64            `json:"value,omitempty"`
	Unit   string             `json:"unit,omitempty"`
	Parsed bool               `json:"parsed,omitempty"`
}

// RunRecord combines the manifest and structured result for CLI use.
type RunRecord struct {
	RunManifest
	Result RunResult `json:"result"`
}

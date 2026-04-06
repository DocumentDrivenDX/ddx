package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// VirtualDictionaryDir is the default directory for recorded prompt→response pairs.
const VirtualDictionaryDir = ".ddx/agent-dictionary"

// VirtualEntry represents a recorded prompt→response pair stored on disk.
type VirtualEntry struct {
	PromptHash   string  `json:"prompt_hash"`
	PromptLen    int     `json:"prompt_len"`
	Prompt       string  `json:"prompt"`
	Response     string  `json:"response"`
	Harness      string  `json:"harness"`
	Model        string  `json:"model,omitempty"`
	DelayMS      int     `json:"delay_ms"`
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
	RecordedAt   string  `json:"recorded_at"`
}

// NormalizePrompt applies configured regex→replacement patterns to a prompt
// before hashing. This allows prompts with dynamic content (temp paths,
// timestamps, bead IDs) to produce stable hashes across runs.
func NormalizePrompt(prompt string, patterns []config.NormalizePattern) string {
	for _, p := range patterns {
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			continue // skip invalid patterns
		}
		prompt = re.ReplaceAllString(prompt, p.Replace)
	}
	return prompt
}

// PromptHash computes a truncated SHA-256 hash of a prompt for use as a
// dictionary filename. The hash is 16 hex characters (64 bits), which is
// sufficient to avoid collisions in practice while keeping filenames readable.
func PromptHash(prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(h[:8])
}

// RecordEntry saves a prompt→response pair to the dictionary directory.
// If normalize patterns are provided, they are applied before hashing.
func RecordEntry(dictDir string, entry *VirtualEntry, patterns ...config.NormalizePattern) error {
	if err := os.MkdirAll(dictDir, 0755); err != nil {
		return fmt.Errorf("creating dictionary dir: %w", err)
	}

	normalized := NormalizePrompt(entry.Prompt, patterns)
	entry.PromptHash = PromptHash(normalized)
	entry.PromptLen = len(entry.Prompt)
	if entry.RecordedAt == "" {
		entry.RecordedAt = time.Now().UTC().Format(time.RFC3339)
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling entry: %w", err)
	}

	path := filepath.Join(dictDir, entry.PromptHash+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing dictionary entry: %w", err)
	}
	return nil
}

// LookupEntry finds a recorded response for a prompt by its hash.
// If normalize patterns are provided, they are applied before hashing.
func LookupEntry(dictDir, prompt string, patterns ...config.NormalizePattern) (*VirtualEntry, error) {
	normalized := NormalizePrompt(prompt, patterns)
	hash := PromptHash(normalized)
	path := filepath.Join(dictDir, hash+".json")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("no recorded response for prompt (hash %s). Record one with: ddx agent run --harness <name> --record --prompt <file>", hash)
	}
	if err != nil {
		return nil, fmt.Errorf("reading dictionary entry: %w", err)
	}

	var entry VirtualEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("parsing dictionary entry %s: %w", path, err)
	}
	return &entry, nil
}

// RunVirtual replays a recorded response from the dictionary.
func (r *Runner) RunVirtual(opts RunOptions) (*Result, error) {
	prompt, err := r.resolvePrompt(opts)
	if err != nil {
		return nil, err
	}

	dictDir := filepath.Join(r.Config.SessionLogDir, "..", "agent-dictionary")
	// Prefer project-local dictionary
	if _, err := os.Stat(VirtualDictionaryDir); err == nil {
		dictDir = VirtualDictionaryDir
	}

	// Load normalization patterns from config.
	var patterns []config.NormalizePattern
	if cfg, err := config.Load(); err == nil && cfg.Agent != nil && cfg.Agent.Virtual != nil {
		patterns = cfg.Agent.Virtual.Normalize
	}

	entry, err := LookupEntry(dictDir, prompt, patterns...)
	if err != nil {
		return nil, err
	}

	// Simulate delay for realistic demo recordings.
	if entry.DelayMS > 0 {
		time.Sleep(time.Duration(entry.DelayMS) * time.Millisecond)
	}

	result := &Result{
		Harness:      "virtual",
		Model:        entry.Model,
		Output:       entry.Response,
		ExitCode:     0,
		DurationMS:   entry.DelayMS,
		InputTokens:  entry.InputTokens,
		OutputTokens: entry.OutputTokens,
		Tokens:       entry.InputTokens + entry.OutputTokens,
		CostUSD:      entry.CostUSD,
	}

	promptSource := opts.PromptSource
	if promptSource == "" {
		if opts.PromptFile != "" {
			promptSource = opts.PromptFile
		} else {
			promptSource = "inline"
		}
	}
	r.logSession(result, len(prompt), prompt, promptSource, opts.Correlation)
	return result, nil
}

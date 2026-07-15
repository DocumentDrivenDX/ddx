package escalation

import (
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

const BeadEstimatedDifficultyKey = "triage.estimated_difficulty"

type EstimatedDifficulty string

const (
	DifficultyEasy   EstimatedDifficulty = "easy"
	DifficultyMedium EstimatedDifficulty = "medium"
	DifficultyHard   EstimatedDifficulty = "hard"
)

// InferInitialMinPower maps the bead's authored difficulty estimate to the
// abstract numeric floor DDx sends to Fizeau. It deliberately ignores every
// other bead field: Fizeau, not DDx, owns concrete route selection.
func InferInitialMinPower(b *bead.Bead) int {
	if difficulty, ok := BeadEstimatedDifficulty(b); ok {
		return MinPowerForEstimatedDifficulty(difficulty)
	}
	return 7
}

// BeadEstimatedDifficulty reads the single durable bead-level difficulty hint.
// Other bead metadata is intentionally ignored; readiness output is transient
// and must not write this key during ordinary dispatch.
func BeadEstimatedDifficulty(b *bead.Bead) (EstimatedDifficulty, bool) {
	if b == nil || b.Extra == nil {
		return "", false
	}
	raw, ok := b.Extra[BeadEstimatedDifficultyKey]
	if !ok {
		return "", false
	}
	return parseEstimatedDifficulty(fmt.Sprint(raw))
}

func parseEstimatedDifficulty(raw string) (EstimatedDifficulty, bool) {
	l := strings.ToLower(strings.TrimSpace(raw))
	if l == "" {
		return "", false
	}
	switch l {
	case string(DifficultyEasy):
		return DifficultyEasy, true
	case string(DifficultyMedium):
		return DifficultyMedium, true
	case string(DifficultyHard):
		return DifficultyHard, true
	default:
		return "", false
	}
}

func MinPowerForEstimatedDifficulty(difficulty EstimatedDifficulty) int {
	switch difficulty {
	case DifficultyEasy:
		return 0
	case DifficultyHard:
		return 9
	case DifficultyMedium:
		return 7
	default:
		return 7
	}
}

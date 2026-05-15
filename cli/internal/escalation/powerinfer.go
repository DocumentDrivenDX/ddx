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

// InferPowerClass maps the bead's authored difficulty estimate to an abstract
// execution power class. Beads without a valid difficulty estimate use the
// ordinary standard route.
func InferPowerClass(b *bead.Bead) PowerClass {
	if difficulty, ok := BeadEstimatedDifficulty(b); ok {
		return PowerClassForEstimatedDifficulty(difficulty)
	}
	return PowerStandard
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

func PowerClassForEstimatedDifficulty(difficulty EstimatedDifficulty) PowerClass {
	switch difficulty {
	case DifficultyEasy:
		return PowerCheap
	case DifficultyHard:
		return PowerSmart
	case DifficultyMedium:
		return PowerStandard
	default:
		return PowerStandard
	}
}

package agent

import (
	"context"
	"time"

	agenttry "github.com/DocumentDrivenDX/ddx/internal/agent/try"
	"github.com/DocumentDrivenDX/ddx/internal/config"
)

type NoChangesRationaleKind = agenttry.NoChangesRationaleKind
type SatisfactionChecker = agenttry.SatisfactionChecker

const (
	NoChangesKindVerified             = agenttry.NoChangesKindVerified
	NoChangesKindLifecycleStatus      = agenttry.NoChangesKindLifecycleStatus
	NoChangesKindRejectedLegacyStatus = agenttry.NoChangesKindRejectedLegacyStatus
	NoChangesKindUnjustified          = agenttry.NoChangesKindUnjustified
)

const (
	NoChangesEventVerified             = agenttry.NoChangesEventVerified
	NoChangesEventUnverified           = agenttry.NoChangesEventUnverified
	NoChangesEventUnjustified          = agenttry.NoChangesEventUnjustified
	NoChangesEventAutonomousRetry      = agenttry.NoChangesEventAutonomousRetry
	NoChangesEventOperatorRequired     = agenttry.NoChangesEventOperatorRequired
	NoChangesEventBlocked              = agenttry.NoChangesEventBlocked
	NoChangesEventLegacyStatusRejected = agenttry.NoChangesEventLegacyStatusRejected
)

const (
	NoChangesLabelUnverified  = agenttry.NoChangesLabelUnverified
	NoChangesLabelUnjustified = agenttry.NoChangesLabelUnjustified
)

type ParsedNoChangesRationale = agenttry.ParsedNoChangesRationale

func ParseNoChangesRationale(text string) ParsedNoChangesRationale {
	return agenttry.ParseNoChangesRationale(text)
}

type VerificationCommandRunner = agenttry.VerificationCommandRunner

const DefaultVerificationCommandTimeout = agenttry.DefaultVerificationCommandTimeout

func DefaultVerificationCommandRunner(ctx context.Context, projectRoot, command string) (int, string, error) {
	return agenttry.DefaultVerificationCommandRunner(ctx, projectRoot, command)
}

func DefaultVerificationCommandRunnerWithTimeout(timeout time.Duration) VerificationCommandRunner {
	return agenttry.DefaultVerificationCommandRunnerWithTimeout(timeout)
}

func defaultVerificationCommandRunnerForConfig(rcfg config.ResolvedConfig) VerificationCommandRunner {
	timeout := rcfg.NoChangesVerificationTimeout()
	if timeout <= 0 {
		return DefaultVerificationCommandRunner
	}
	return DefaultVerificationCommandRunnerWithTimeout(timeout)
}

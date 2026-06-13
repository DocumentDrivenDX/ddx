package cmd

import (
	"context"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	agentlib "github.com/easel/fizeau"
)

type implementationProfileSelector struct {
	projectRoot string
	harness     string
	once        sync.Once
	snap        agent.ProfileSnapshot
	err         error
}

func newImplementationProfileSelector(projectRoot, harness string) *implementationProfileSelector {
	return &implementationProfileSelector{projectRoot: projectRoot, harness: harness}
}

func (s *implementationProfileSelector) Select(ctx context.Context, powerClass escalation.PowerClass, floor int) (agent.ImplementationProfileSelection, error) {
	if s == nil {
		return agent.ImplementationProfileSelection{}, nil
	}
	s.once.Do(func() {
		svc, err := agent.ResolvePreflightServiceFromWorkDir(s.projectRoot)
		if err != nil {
			s.err = err
			return
		}
		snapCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		filter := agentlib.ModelFilter{}
		if s.harness != "" {
			filter.Harness = s.harness
		}
		s.snap, s.err = agent.LoadProfileSnapshotWithFilter(snapCtx, svc, filter)
	})
	if s.err != nil {
		return agent.ImplementationProfileSelection{}, s.err
	}
	if powerClass == "" {
		powerClass = escalation.PowerStandard
	}
	if floor > 0 {
		return agent.SelectImplementationProfileForMinPower(s.snap, powerClass, floor), nil
	}
	return agent.SelectImplementationProfile(s.snap, powerClass), nil
}

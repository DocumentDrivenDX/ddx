package cmd

import (
	"context"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
)

type implementationProfileSelector struct {
	projectRoot string
	once        sync.Once
	snap        agent.ProfileSnapshot
	err         error
}

func newImplementationProfileSelector(projectRoot string) *implementationProfileSelector {
	return &implementationProfileSelector{projectRoot: projectRoot}
}

func (s *implementationProfileSelector) Select(ctx context.Context, powerClass escalation.PowerClass, floor int) (agent.ImplementationProfileSelection, error) {
	if s == nil {
		return agent.ImplementationProfileSelection{}, nil
	}
	s.once.Do(func() {
		svc, err := agent.ResolveServiceFromWorkDir(s.projectRoot)
		if err != nil {
			s.err = err
			return
		}
		snapCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		s.snap, s.err = agent.LoadProfileSnapshot(snapCtx, svc)
	})
	if s.err != nil {
		return agent.ImplementationProfileSelection{}, s.err
	}
	if powerClass == "" {
		powerClass = escalation.PowerCheap
	}
	if floor > 0 {
		return agent.SelectImplementationProfileForMinPower(s.snap, powerClass, floor), nil
	}
	return agent.SelectImplementationProfile(s.snap, powerClass), nil
}

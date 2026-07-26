package cmd

import (
	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// cooldownLoopStore is the narrowest store surface the ignore-cooldown and
// single-bead wrappers need. ReadyExecutionIgnoringCooldown is still concrete
// on *bead.Store and is not yet part of bead.Backend, so it is expressed here
// as a local extension of ExecuteBeadLoopStore rather than embedding *bead.Store.
type cooldownLoopStore interface {
	agent.ExecuteBeadLoopStore
	ReadyExecutionIgnoringCooldown() ([]bead.Bead, error)
}

type ignoreCooldownStore struct {
	cooldownLoopStore
	overrideRetryAfter map[string]string
}

func newIgnoreCooldownStore(store cooldownLoopStore) *ignoreCooldownStore {
	return &ignoreCooldownStore{cooldownLoopStore: store}
}

func (s *ignoreCooldownStore) ReadyExecution() ([]bead.Bead, error) {
	standard, err := s.cooldownLoopStore.ReadyExecution()
	if err != nil {
		return nil, err
	}
	withCooldown, err := s.cooldownLoopStore.ReadyExecutionIgnoringCooldown()
	if err != nil {
		return nil, err
	}
	standardIDs := make(map[string]struct{}, len(standard))
	for _, b := range standard {
		standardIDs[b.ID] = struct{}{}
	}
	s.overrideRetryAfter = make(map[string]string)
	for _, b := range withCooldown {
		if _, ok := standardIDs[b.ID]; ok {
			continue
		}
		s.overrideRetryAfter[b.ID] = retryAfterString(b)
	}
	return withCooldown, nil
}

func (s *ignoreCooldownStore) CooldownOverrideInfo(beadID string) (string, bool) {
	if s.overrideRetryAfter == nil {
		return "", false
	}
	retryAfter, ok := s.overrideRetryAfter[beadID]
	return retryAfter, ok
}

// singleBeadStore narrows the execution-ready queue to one specific target.
// It respects the underlying ready queue by default and only surfaces a bead
// in retry cooldown when forceCooldown is enabled.
type singleBeadStore struct {
	cooldownLoopStore
	targetID           string
	forceCooldown      bool
	overrideRetryAfter map[string]string
}

func (s *singleBeadStore) ReadyExecution() ([]bead.Bead, error) {
	standard, err := s.cooldownLoopStore.ReadyExecution()
	if err != nil {
		return nil, err
	}
	for _, b := range standard {
		if b.ID == s.targetID {
			s.overrideRetryAfter = nil
			return []bead.Bead{b}, nil
		}
	}
	if !s.forceCooldown {
		s.overrideRetryAfter = nil
		return nil, nil
	}
	withCooldown, err := s.cooldownLoopStore.ReadyExecutionIgnoringCooldown()
	if err != nil {
		return nil, err
	}
	for _, b := range withCooldown {
		if b.ID != s.targetID {
			continue
		}
		s.overrideRetryAfter = map[string]string{s.targetID: retryAfterString(b)}
		return []bead.Bead{b}, nil
	}
	s.overrideRetryAfter = nil
	return nil, nil
}

func (s *singleBeadStore) CooldownOverrideInfo(beadID string) (string, bool) {
	if s.overrideRetryAfter == nil {
		return "", false
	}
	retryAfter, ok := s.overrideRetryAfter[beadID]
	return retryAfter, ok
}

func retryAfterString(b bead.Bead) string {
	if b.Extra == nil {
		return ""
	}
	retryAfter, _ := b.Extra[bead.ExtraRetryAfter].(string)
	return retryAfter
}

func containsBeadID(beads []bead.Bead, beadID string) bool {
	for _, b := range beads {
		if b.ID == beadID {
			return true
		}
	}
	return false
}

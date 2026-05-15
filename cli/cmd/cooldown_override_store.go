package cmd

import (
	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

type ignoreCooldownStore struct {
	*bead.Store
	overrideRetryAfter map[string]string
}

func newIgnoreCooldownStore(store *bead.Store) *ignoreCooldownStore {
	return &ignoreCooldownStore{Store: store}
}

func (s *ignoreCooldownStore) ReadyExecution() ([]bead.Bead, error) {
	standard, err := s.Store.ReadyExecution()
	if err != nil {
		return nil, err
	}
	withCooldown, err := s.ReadyExecutionIgnoringCooldown()
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
	*bead.Store
	targetID           string
	forceCooldown      bool
	overrideRetryAfter map[string]string
}

func (s *singleBeadStore) ReadyExecution() ([]bead.Bead, error) {
	standard, err := s.Store.ReadyExecution()
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
	withCooldown, err := s.ReadyExecutionIgnoringCooldown()
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

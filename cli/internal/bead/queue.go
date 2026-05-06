package bead

import (
	"fmt"
	"time"
)

const queueRankKey = "queue-rank"
const queueRankStep = 10

type queueOp int

const (
	queueOpTop queueOp = iota
	queueOpMoveBefore
	queueOpMoveAfter
)

// QueueTop moves a bead to the front of its priority bucket.
func (s *Store) QueueTop(id string) error {
	return s.queueReorder(id, "", queueOpTop)
}

// QueueMove reorders a bead relative to another bead in the same priority bucket.
func (s *Store) QueueMove(id, otherID string, before bool) error {
	if before {
		return s.queueReorder(id, otherID, queueOpMoveBefore)
	}
	return s.queueReorder(id, otherID, queueOpMoveAfter)
}

// QueueClear removes the explicit queue-rank from a bead.
func (s *Store) QueueClear(id string) error {
	return s.WithLock(func() error {
		beads, _, err := s.readAllLatestRaw()
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		found := false
		for i := range beads {
			if beads[i].ID != id {
				continue
			}
			found = true
			if beads[i].Extra != nil {
				delete(beads[i].Extra, queueRankKey)
				if len(beads[i].Extra) == 0 {
					beads[i].Extra = nil
				}
			}
			beads[i].UpdatedAt = now
			break
		}
		if !found {
			return fmt.Errorf("bead: not found: %s", id)
		}
		return s.WriteAll(beads)
	})
}

func (s *Store) queueReorder(id, otherID string, op queueOp) error {
	return s.WithLock(func() error {
		beads, _, err := s.readAllLatestRaw()
		if err != nil {
			return err
		}

		indexByID := make(map[string]int, len(beads))
		for i := range beads {
			indexByID[beads[i].ID] = i
		}

		targetIdx, ok := indexByID[id]
		if !ok {
			return fmt.Errorf("bead: not found: %s", id)
		}

		targetPriority := beads[targetIdx].Priority
		if op != queueOpTop {
			otherIdx, ok := indexByID[otherID]
			if !ok {
				return fmt.Errorf("bead: not found: %s", otherID)
			}
			if beads[otherIdx].Priority != targetPriority {
				return fmt.Errorf("bead: queue move limited to one priority bucket: %s (P%d) and %s (P%d)", id, targetPriority, otherID, beads[otherIdx].Priority)
			}
		}

		bucket := make([]Bead, 0, len(beads))
		for _, bead := range beads {
			if bead.Priority == targetPriority {
				bucket = append(bucket, bead)
			}
		}
		if len(bucket) == 0 {
			return fmt.Errorf("bead: not found: %s", id)
		}

		sortBeadsForQueue(bucket)
		desired := reorderQueueBucket(bucket, id, otherID, op)

		if rank, ok := queueTargetRank(desired, id); ok {
			now := time.Now().UTC()
			idx := indexByID[id]
			if beads[idx].Extra == nil {
				beads[idx].Extra = make(map[string]any)
			}
			beads[idx].Extra[queueRankKey] = rank
			beads[idx].UpdatedAt = now
			return s.WriteAll(beads)
		}

		now := time.Now().UTC()
		for i, bead := range desired {
			idx := indexByID[bead.ID]
			if beads[idx].Extra == nil {
				beads[idx].Extra = make(map[string]any)
			}
			beads[idx].Extra[queueRankKey] = i * queueRankStep
			beads[idx].UpdatedAt = now
		}

		return s.WriteAll(beads)
	})
}

func queueTargetRank(desired []Bead, targetID string) (int, bool) {
	targetIdx := -1
	for i, bead := range desired {
		if bead.ID == targetID {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return 0, false
	}

	for i := 0; i < targetIdx; i++ {
		if _, ok := parseQueueRank(desired[i].Extra[queueRankKey]); !ok {
			return 0, false
		}
	}

	var prevRank *int
	for i := targetIdx - 1; i >= 0; i-- {
		if rank, ok := parseQueueRank(desired[i].Extra[queueRankKey]); ok {
			prevRank = &rank
			break
		}
	}

	var nextRank *int
	for i := targetIdx + 1; i < len(desired); i++ {
		if rank, ok := parseQueueRank(desired[i].Extra[queueRankKey]); ok {
			nextRank = &rank
			break
		}
	}

	switch {
	case prevRank != nil && nextRank != nil:
		if *nextRank-*prevRank <= 1 {
			return 0, false
		}
		return (*prevRank + *nextRank) / 2, true
	case prevRank != nil:
		return *prevRank + 1, true
	case nextRank != nil:
		return *nextRank - 1, true
	default:
		return 0, true
	}
}

func reorderQueueBucket(bucket []Bead, targetID, otherID string, op queueOp) []Bead {
	if len(bucket) <= 1 {
		return bucket
	}

	targetIdx := -1
	otherIdx := -1
	for i, bead := range bucket {
		switch bead.ID {
		case targetID:
			targetIdx = i
		case otherID:
			otherIdx = i
		}
	}
	if targetIdx < 0 {
		return bucket
	}

	out := append([]Bead(nil), bucket...)
	target := out[targetIdx]
	out = append(out[:targetIdx], out[targetIdx+1:]...)

	switch op {
	case queueOpTop:
		return append([]Bead{target}, out...)
	case queueOpMoveBefore, queueOpMoveAfter:
		if otherIdx < 0 || targetID == otherID {
			return bucket
		}
		if targetIdx < otherIdx {
			otherIdx--
		}
		insertAt := otherIdx
		if op == queueOpMoveAfter {
			insertAt++
		}
		if insertAt < 0 {
			insertAt = 0
		}
		if insertAt > len(out) {
			insertAt = len(out)
		}
		out = append(out, Bead{})
		copy(out[insertAt+1:], out[insertAt:])
		out[insertAt] = target
		return out
	default:
		return bucket
	}
}

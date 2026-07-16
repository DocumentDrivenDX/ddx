// Package queue provides operator-facing queue-ordering helpers for the bead
// tracker. These helpers own the workflow concern of maintaining sparse rank
// ordering within priority buckets; the core bead package owns persistence.
package queue

import (
	"context"
	"fmt"
	"sort"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

const rankStep = 10

// Loader is the persistence interface required by queue helpers.
// *bead.Store satisfies this interface.
type Loader interface {
	ReadAll(ctx context.Context) ([]bead.Bead, error)
	Apply(id string, op bead.Operation) error
}

// Top moves a bead strictly before every other bead in its priority bucket.
// It shares Move's sparse-rank and bucket-local renormalization contract so an
// existing rank-zero bead cannot tie with (and sort before) the new target.
func Top(ctx context.Context, l Loader, id string) error {
	return Move(ctx, l, id, 0)
}

// Move inserts a bead at the given position (0-based index) within its priority
// bucket. When sparse rank gaps allow it, only the target bead is written;
// otherwise all beads in the bucket are renormalized with step-10 ranks.
func Move(ctx context.Context, l Loader, id string, position int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	beads, err := l.ReadAll(ctx)
	if err != nil {
		return err
	}

	var target *bead.Bead
	for i := range beads {
		if beads[i].ID == id {
			target = &beads[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("bead: not found: %s", id)
	}

	bucket := sortedBucket(beads, target.Priority)
	reduced := make([]bead.Bead, 0, len(bucket))
	for _, b := range bucket {
		if b.ID != id {
			reduced = append(reduced, b)
		}
	}

	if position < 0 {
		position = 0
	}
	if position > len(reduced) {
		position = len(reduced)
	}

	desired := make([]bead.Bead, 0, len(bucket))
	desired = append(desired, reduced[:position]...)
	desired = append(desired, *target)
	desired = append(desired, reduced[position:]...)

	if rank, ok := sparseRank(desired, id); ok {
		return l.Apply(id, bead.QueueSetPosition{Position: rank})
	}

	// Dense: renormalize all beads in the bucket.
	for i, b := range desired {
		if err := l.Apply(b.ID, bead.QueueSetPosition{Position: i * rankStep}); err != nil {
			return err
		}
	}
	return nil
}

// Clear removes the explicit queue rank from a bead, restoring natural ordering.
func Clear(ctx context.Context, l Loader, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return l.Apply(id, bead.QueueClearOp{})
}

// BucketPosition returns the 0-based index of refID in the sorted priority
// bucket for targetID, after removing targetID from the bucket. Returns -1
// when refID is not found or belongs to a different priority bucket.
// The CLI uses this to convert --before/--after flags to a position index.
func BucketPosition(beads []bead.Bead, targetID, refID string) int {
	var targetPriority, refPriority int
	targetFound, refFound := false, false
	for _, b := range beads {
		if b.ID == targetID {
			targetPriority = b.Priority
			targetFound = true
		}
		if b.ID == refID {
			refPriority = b.Priority
			refFound = true
		}
	}
	if !targetFound || !refFound || targetPriority != refPriority {
		return -1
	}
	bucket := sortedBucket(beads, targetPriority)
	reduced := make([]bead.Bead, 0, len(bucket))
	for _, b := range bucket {
		if b.ID != targetID {
			reduced = append(reduced, b)
		}
	}
	for i, b := range reduced {
		if b.ID == refID {
			return i
		}
	}
	return -1
}

// sortedBucket filters beads by priority and sorts them by queue order:
// explicit rank (ascending), then created_at, then ID.
func sortedBucket(beads []bead.Bead, priority int) []bead.Bead {
	out := make([]bead.Bead, 0, len(beads))
	for _, b := range beads {
		if b.Priority == priority {
			out = append(out, b)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ir, iok := bead.QueueRank(out[i].Extra)
		jr, jok := bead.QueueRank(out[j].Extra)
		if iok != jok {
			return iok
		}
		if iok && jok && ir != jr {
			return ir < jr
		}
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// sparseRank computes a rank for targetID that fits between its neighbors in
// the desired slice, preserving sparse ordering without renormalizing others.
// Returns (rank, true) when a sparse slot exists; (0, false) when the bucket
// must be renormalized.
func sparseRank(desired []bead.Bead, targetID string) (int, bool) {
	targetIdx := -1
	for i, b := range desired {
		if b.ID == targetID {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return 0, false
	}

	// All beads before target must have explicit ranks for sparse-rank to work.
	for i := 0; i < targetIdx; i++ {
		if _, ok := bead.QueueRank(desired[i].Extra); !ok {
			return 0, false
		}
	}

	var prevRank *int
	for i := targetIdx - 1; i >= 0; i-- {
		if r, ok := bead.QueueRank(desired[i].Extra); ok {
			prevRank = &r
			break
		}
	}

	var nextRank *int
	for i := targetIdx + 1; i < len(desired); i++ {
		if r, ok := bead.QueueRank(desired[i].Extra); ok {
			nextRank = &r
			break
		}
	}

	switch {
	case prevRank != nil && nextRank != nil:
		// Incrementing the lower bound avoids overflow in both the old
		// next-prev gap calculation and the old prev+next midpoint. A value
		// fits only when the increment remains strictly below the upper bound.
		if *prevRank == maxInt() {
			return 0, false
		}
		rank := *prevRank + 1
		if rank >= *nextRank {
			return 0, false
		}
		return rank, true
	case prevRank != nil:
		if *prevRank == maxInt() {
			return 0, false
		}
		return *prevRank + 1, true
	case nextRank != nil:
		if *nextRank == minInt() {
			return 0, false
		}
		return *nextRank - 1, true
	default:
		return 0, true
	}
}

func maxInt() int { return int(^uint(0) >> 1) }

func minInt() int { return -maxInt() - 1 }

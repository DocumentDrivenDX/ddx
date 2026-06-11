package bead

// Operation is a typed mutation applied to a bead snapshot.
type Operation interface {
	Apply(b *Bead) error
}

// QueueSetTop moves a bead to the front of its priority bucket by assigning
// the top rank value (0). Other beads without an explicit rank sort after it.
type QueueSetTop struct{}

func (QueueSetTop) Apply(b *Bead) error {
	if b.Extra == nil {
		b.Extra = make(map[string]any)
	}
	b.Extra[queueRankKey] = 0
	return nil
}

// QueueSetPosition sets an explicit queue rank on a bead. Lower rank values
// sort earlier within the same priority bucket.
type QueueSetPosition struct {
	Position int
}

func (op QueueSetPosition) Apply(b *Bead) error {
	if b.Extra == nil {
		b.Extra = make(map[string]any)
	}
	b.Extra[queueRankKey] = op.Position
	return nil
}

// QueueClearOp removes the explicit queue rank from a bead, restoring natural
// ordering (created_at, then ID) within the priority bucket.
type QueueClearOp struct{}

func (QueueClearOp) Apply(b *Bead) error {
	if b.Extra != nil {
		delete(b.Extra, queueRankKey)
		if len(b.Extra) == 0 {
			b.Extra = nil
		}
	}
	return nil
}

package bead

import "strconv"

// queueRankKey is the canonical Extra field used to store operator-assigned
// queue rank. Lower values sort earlier within the same priority bucket.
// Beads without this key sort after ranked beads, then by created_at, then by
// ID.
const queueRankKey = "queue-rank"

// queueRankAliasKey is accepted for CLI/API ergonomics and for compatibility
// with rows written before queue-rank was consistently canonicalized.
const queueRankAliasKey = "queue_rank"

// queueRankStep is the default gap between ranks when renormalizing a bucket.
const queueRankStep = 10

// QueueRank returns the operator-assigned rank from an Extra map. The
// canonical hyphenated key wins when both keys are present.
func QueueRank(extra map[string]any) (int, bool) {
	if extra == nil {
		return 0, false
	}
	if rank, ok := parseQueueRank(extra[queueRankKey]); ok {
		return rank, true
	}
	return parseQueueRank(extra[queueRankAliasKey])
}

// CanonicalizeQueueRankKey maps accepted queue rank spellings onto the
// canonical persisted field name.
func CanonicalizeQueueRankKey(key string) string {
	if key == queueRankAliasKey {
		return queueRankKey
	}
	return key
}

// ParseQueueRankValue parses a CLI-provided queue-rank value. It is exported
// so command code can reject inert non-numeric rank fields at write time.
func ParseQueueRankValue(value string) (int, bool) {
	return parseQueueRank(value)
}

// SetExtraValue writes a CLI-style key/value pair into Extra while applying
// DDx's field-specific typing rules. Queue rank is always canonicalized and
// stored as a JSON number.
func SetExtraValue(extra map[string]any, key, value string) error {
	if extra == nil {
		return nil
	}
	key = CanonicalizeQueueRankKey(key)
	if key == queueRankKey {
		rank, ok := ParseQueueRankValue(value)
		if !ok {
			return &QueueRankParseError{Value: value}
		}
		extra[queueRankKey] = rank
		delete(extra, queueRankAliasKey)
		return nil
	}
	switch value {
	case "true":
		extra[key] = true
	case "false":
		extra[key] = false
	default:
		extra[key] = value
	}
	return nil
}

// UnsetExtraKey removes the canonical field and accepted alias for queue rank.
func UnsetExtraKey(extra map[string]any, key string) {
	if extra == nil {
		return
	}
	key = CanonicalizeQueueRankKey(key)
	delete(extra, key)
	if key == queueRankKey {
		delete(extra, queueRankAliasKey)
	}
}

// QueueRankParseError reports an invalid operator-supplied queue rank.
type QueueRankParseError struct {
	Value string
}

func (e *QueueRankParseError) Error() string {
	return "queue-rank must be an integer, got: " + strconv.Quote(e.Value)
}

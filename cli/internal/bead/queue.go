package bead

// queueRankKey is the Extra field used to store operator-assigned queue rank.
// Lower values sort earlier within the same priority bucket. Beads without
// this key sort after ranked beads, then by created_at, then by ID.
const queueRankKey = "queue-rank"

// queueRankStep is the default gap between ranks when renormalizing a bucket.
const queueRankStep = 10

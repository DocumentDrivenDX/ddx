// Package blob defines the BlobStore abstraction for byte-blob storage
// (execution evidence, bead attachments, future library content).
//
// FEAT-028 §BlobStore Interface (v1): five storage abstractions for DDx;
// this package covers abstraction #2 (BlobStore).
package blob

import (
	"context"
	"errors"
	"io"
	"time"
)

// Key is a hierarchical, slash-separated identifier for a blob.
// Keys are caller-supplied (not content-addressed) and stable across
// reads and writes. Examples:
//
//	"attachments/ddx-a827d146/events.jsonl"
//	"executions/20260511T030206-bca671a4/result.json"
//
// Key naming conventions (FEAT-028 §Key naming conventions, normative):
//   - Slash-separated, ASCII, no leading slash, no trailing slash, no ".." segments.
//   - First segment is the collection: "attachments", "executions".
//   - Second segment is the owner ID (bead ID, run ID).
//   - Subsequent segments are resource paths within the owner.
type Key string

// Store is the abstraction every persistent byte-blob in DDx flows through.
// Implementations must be safe for concurrent use.
//
// Multi-blob write discipline (FEAT-028, normative for callers): when writing
// multiple blobs that together form a logical unit, always write the manifest
// or index blob LAST, after all referenced blobs have returned nil from Put.
// This ensures any reader that sees the manifest will also see its referenced
// blobs.
type Store interface {
	// Put writes the entire blob at key, overwriting any previous value.
	// Put MUST be both atomic-on-success and durable-on-return:
	//   - Atomic: a concurrent Get either sees the full prior value or
	//     the full new value, never a partial write.
	//   - Durable: when Put returns nil, the blob has survived a process
	//     or host crash. For LocalFSBlob this means fsync of the data
	//     file AND fsync of its parent directory before returning.
	//
	// Crash-safety in callers (e.g. bead attachment externalization in
	// attachments.go) depends on this guarantee — a Put that returns
	// before durability is established can permanently lose data when
	// the caller proceeds to clear the in-memory copy.
	Put(ctx context.Context, key Key, r io.Reader) error

	// Get returns a reader for the blob at key. The caller must Close the
	// returned ReadCloser when done. Returns ErrNotFound (errors.Is) when
	// the key does not exist.
	Get(ctx context.Context, key Key) (io.ReadCloser, error)

	// Stat returns metadata for the blob at key without fetching its body.
	// Returns ErrNotFound when the key does not exist.
	Stat(ctx context.Context, key Key) (Info, error)

	// List enumerates keys with the given prefix. Order is unspecified.
	// Implementations should stream rather than buffer for large prefixes.
	List(ctx context.Context, prefix Key) ([]Key, error)

	// Delete removes the blob at key. Deleting a missing key is not an error.
	Delete(ctx context.Context, key Key) error
}

// Info is the metadata Stat returns. Size and ModTime are required; ETag
// is optional and may be empty for backends that do not surface one.
type Info struct {
	Key     Key
	Size    int64
	ModTime time.Time
	ETag    string
}

// ErrNotFound is returned (or wrapped) by Get and Stat when a key does not exist.
var ErrNotFound = errors.New("blob: not found")

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/blob"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// evidenceKey returns the blob.Key for a named evidence file within an attempt.
// Convention (FEAT-028 §Key naming conventions):
//
//	collection: "executions"
//	owner: <attempt-id>
//	resource: <filename>
func evidenceKey(attemptID, filename string) blob.Key {
	return blob.Key("executions/" + attemptID + "/" + filename)
}

// evidenceBlobStore returns a LocalFSBlob rooted at the .ddx directory within
// dir. It is the default BlobStore for evidence writes when no store is injected
// via ExecuteBeadRuntime.Blobs.
func evidenceBlobStore(dir string) *blob.LocalFSBlob {
	return blob.NewLocalFS(filepath.Join(dir, ddxroot.DirName))
}

// blobPutBytes writes data to key in the given store.
func blobPutBytes(ctx context.Context, s blob.Store, key blob.Key, data []byte) error {
	return s.Put(ctx, key, bytes.NewReader(data))
}

// blobPutJSON marshals payload to indented JSON and writes it to key.
func blobPutJSON(ctx context.Context, s blob.Store, key blob.Key, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json for %q: %w", key, err)
	}
	return s.Put(ctx, key, bytes.NewReader(data))
}

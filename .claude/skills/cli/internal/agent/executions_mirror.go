package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// ExecutionsMirrorIndexFile is the local pointer file that lists every
// successfully mirrored bundle. One JSON object per line.
const ExecutionsMirrorIndexFile = ".ddx/executions/mirror-index.jsonl"

// ExecutionsMirrorLogFile records mirror upload attempts and failures so
// operators can diagnose mirror-side issues without re-running execute-bead.
const ExecutionsMirrorLogFile = ".ddx/agent-logs/mirror.log"

// MirrorIndexEntry is one row of the local mirror-index.jsonl pointer file.
type MirrorIndexEntry struct {
	AttemptID  string    `json:"attempt_id"`
	BeadID     string    `json:"bead_id,omitempty"`
	MirrorURI  string    `json:"mirror_uri"`
	UploadedAt time.Time `json:"uploaded_at"`
	ByteSize   int64     `json:"byte_size"`
	Kind       string    `json:"kind"`
}

// MirrorBackend uploads and retrieves bundle directories.
type MirrorBackend interface {
	// Kind returns the backend identifier (matches the configured kind, e.g. "local").
	Kind() string
	// Upload copies the resolved set of files in srcDir to a destination derived
	// from the rendered destination URI/path. Returns the canonical mirror URI
	// for the bundle along with the total bytes uploaded.
	Upload(destURI, srcDir string, includeFilter func(relPath string) bool) (string, int64, error)
	// Fetch downloads a previously mirrored bundle to destDir.
	Fetch(mirrorURI, destDir string) error
}

// MirrorRequest carries everything needed to mirror one execute-bead bundle.
type MirrorRequest struct {
	ProjectRoot string
	AttemptID   string
	BeadID      string
	BundleDir   string // absolute path to .ddx/executions/<attempt-id>/
	Cfg         *config.ExecutionsMirrorConfig
}

// allMirrorParts is the canonical default include list — every part of the
// bundle. Used when ExecutionsMirrorConfig.Include is empty.
var allMirrorParts = []string{"manifest", "prompt", "result", "usage", "checks", "embedded"}

// fileForPart returns the relative bundle path (or directory prefix) for a
// part name. The "embedded" part is a directory; the rest are single files.
func fileForPart(part string) (string, bool) {
	switch part {
	case "manifest":
		return "manifest.json", false
	case "prompt":
		return "prompt.md", false
	case "result":
		return "result.json", false
	case "usage":
		return "usage.json", false
	case "checks":
		return "checks.json", false
	case "embedded":
		return "embedded", true
	}
	return "", false
}

// IncludeFilter returns a filter callable matching the include list. An empty
// list means everything is included.
func IncludeFilter(parts []string) func(string) bool {
	if len(parts) == 0 {
		parts = allMirrorParts
	}
	files := make(map[string]struct{})
	dirs := make([]string, 0)
	for _, p := range parts {
		rel, isDir := fileForPart(strings.ToLower(strings.TrimSpace(p)))
		if rel == "" {
			continue
		}
		if isDir {
			dirs = append(dirs, rel+string(filepath.Separator))
		} else {
			files[rel] = struct{}{}
		}
	}
	return func(rel string) bool {
		rel = filepath.ToSlash(rel)
		// Map back to OS separators for the file lookup.
		osRel := filepath.FromSlash(rel)
		if _, ok := files[osRel]; ok {
			return true
		}
		for _, prefix := range dirs {
			pfx := filepath.ToSlash(prefix)
			if strings.HasPrefix(rel, pfx) {
				return true
			}
		}
		return false
	}
}

// RenderMirrorPath substitutes the {project}, {attempt_id}, {date}, {bead_id}
// placeholders in template. Unknown placeholders are left in place. The
// "{date}" expansion uses YYYY-MM-DD derived from the attempt id when it
// matches the standard "YYYYMMDDTHHMMSS-..." form, falling back to the
// current UTC date.
func RenderMirrorPath(template, projectName, attemptID, beadID string) string {
	date := time.Now().UTC().Format("2006-01-02")
	if len(attemptID) >= 8 && isAllDigits(attemptID[:8]) {
		date = fmt.Sprintf("%s-%s-%s", attemptID[0:4], attemptID[4:6], attemptID[6:8])
	}
	r := strings.NewReplacer(
		"{project}", projectName,
		"{attempt_id}", attemptID,
		"{date}", date,
		"{bead_id}", beadID,
	)
	return r.Replace(template)
}

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// projectName derives a short identifier from the project root path.
func projectName(projectRoot string) string {
	clean := filepath.Clean(projectRoot)
	return filepath.Base(clean)
}

// MirrorBundle uploads one bundle synchronously via the configured backend
// and appends a row to the local mirror-index.jsonl pointer file. Returns
// the index entry on success.
func MirrorBundle(req MirrorRequest) (*MirrorIndexEntry, error) {
	if req.Cfg == nil || strings.TrimSpace(req.Cfg.Kind) == "" || strings.TrimSpace(req.Cfg.Path) == "" {
		return nil, fmt.Errorf("mirror not configured")
	}
	backend, err := NewMirrorBackend(req.Cfg.Kind)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(req.BundleDir); err != nil {
		return nil, fmt.Errorf("bundle dir %s: %w", req.BundleDir, err)
	}

	dest := RenderMirrorPath(req.Cfg.Path, projectName(req.ProjectRoot), req.AttemptID, req.BeadID)
	filter := IncludeFilter(req.Cfg.Include)

	mirrorURI, bytes, err := backend.Upload(dest, req.BundleDir, filter)
	if err != nil {
		return nil, fmt.Errorf("mirror upload: %w", err)
	}

	entry := &MirrorIndexEntry{
		AttemptID:  req.AttemptID,
		BeadID:     req.BeadID,
		MirrorURI:  mirrorURI,
		UploadedAt: time.Now().UTC(),
		ByteSize:   bytes,
		Kind:       backend.Kind(),
	}
	if err := AppendMirrorIndex(req.ProjectRoot, entry); err != nil {
		return entry, fmt.Errorf("appending mirror-index.jsonl: %w", err)
	}
	return entry, nil
}

// MirrorBundleAsync runs MirrorBundle in a background goroutine and never
// returns an error to the caller. Failures are written to mirror.log so the
// bead's primary outcome is never affected by mirror-side problems.
func MirrorBundleAsync(req MirrorRequest) {
	go func() {
		if entry, err := MirrorBundle(req); err != nil {
			LogMirrorFailure(req.ProjectRoot, req.AttemptID, req.BeadID, err)
		} else if entry != nil {
			LogMirrorSuccess(req.ProjectRoot, entry)
		}
	}()
}

// MirrorOrLog dispatches according to the configured async setting. When
// the bead description says default async=true, callers should treat a nil
// or true Async as background.
func MirrorOrLog(req MirrorRequest) {
	if req.Cfg == nil || strings.TrimSpace(req.Cfg.Kind) == "" || strings.TrimSpace(req.Cfg.Path) == "" {
		return
	}
	async := true
	if req.Cfg.Async != nil {
		async = *req.Cfg.Async
	}
	if async {
		MirrorBundleAsync(req)
		return
	}
	if entry, err := MirrorBundle(req); err != nil {
		LogMirrorFailure(req.ProjectRoot, req.AttemptID, req.BeadID, err)
	} else if entry != nil {
		LogMirrorSuccess(req.ProjectRoot, entry)
	}
}

// mirrorIndexMu serializes appends to the local pointer file across
// goroutines launched by MirrorBundleAsync.
var mirrorIndexMu sync.Mutex

// AppendMirrorIndex appends one entry to .ddx/executions/mirror-index.jsonl.
func AppendMirrorIndex(projectRoot string, entry *MirrorIndexEntry) error {
	if entry == nil {
		return nil
	}
	mirrorIndexMu.Lock()
	defer mirrorIndexMu.Unlock()

	path := filepath.Join(projectRoot, ExecutionsMirrorIndexFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

// ReadMirrorIndex returns every row in mirror-index.jsonl. Missing file is
// returned as an empty slice with no error.
func ReadMirrorIndex(projectRoot string) ([]MirrorIndexEntry, error) {
	path := filepath.Join(projectRoot, ExecutionsMirrorIndexFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []MirrorIndexEntry
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e MirrorIndexEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// LookupMirrorEntry returns the most recent index entry whose AttemptID
// matches the given id, or nil when no such entry exists.
func LookupMirrorEntry(projectRoot, attemptID string) (*MirrorIndexEntry, error) {
	entries, err := ReadMirrorIndex(projectRoot)
	if err != nil {
		return nil, err
	}
	var match *MirrorIndexEntry
	for i := range entries {
		if entries[i].AttemptID == attemptID {
			match = &entries[i]
		}
	}
	return match, nil
}

// LogMirrorSuccess records a one-line success entry to mirror.log.
func LogMirrorSuccess(projectRoot string, entry *MirrorIndexEntry) {
	line := fmt.Sprintf("%s OK attempt=%s bead=%s bytes=%d uri=%s\n",
		time.Now().UTC().Format(time.RFC3339), entry.AttemptID, entry.BeadID, entry.ByteSize, entry.MirrorURI)
	appendMirrorLog(projectRoot, line)
}

// LogMirrorFailure records a one-line failure entry to mirror.log.
func LogMirrorFailure(projectRoot, attemptID, beadID string, err error) {
	line := fmt.Sprintf("%s ERR attempt=%s bead=%s err=%q\n",
		time.Now().UTC().Format(time.RFC3339), attemptID, beadID, err.Error())
	appendMirrorLog(projectRoot, line)
}

func appendMirrorLog(projectRoot, line string) {
	path := filepath.Join(projectRoot, ExecutionsMirrorLogFile)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}

// NewMirrorBackend returns the backend for a given kind. Currently only
// "local" is supported; other kinds return a clear error.
func NewMirrorBackend(kind string) (MirrorBackend, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "local":
		return &LocalDirMirror{}, nil
	default:
		return nil, fmt.Errorf("unsupported mirror kind %q (only 'local' is implemented)", kind)
	}
}

// LocalDirMirror copies the bundle into a directory on the local filesystem.
// The destination URI is the absolute path that the bundle was written to.
type LocalDirMirror struct{}

func (LocalDirMirror) Kind() string { return "local" }

func (LocalDirMirror) Upload(destURI, srcDir string, includeFilter func(relPath string) bool) (string, int64, error) {
	if includeFilter == nil {
		includeFilter = func(string) bool { return true }
	}
	if err := os.MkdirAll(destURI, 0o755); err != nil {
		return "", 0, fmt.Errorf("creating mirror dest %s: %w", destURI, err)
	}
	var total int64
	walkErr := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return relErr
		}
		if !includeFilter(rel) {
			return nil
		}
		dest := filepath.Join(destURI, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		n, copyErr := copyFile(path, dest)
		if copyErr != nil {
			return copyErr
		}
		total += n
		return nil
	})
	if walkErr != nil {
		return destURI, total, walkErr
	}
	return destURI, total, nil
}

func (LocalDirMirror) Fetch(mirrorURI, destDir string) error {
	info, err := os.Stat(mirrorURI)
	if err != nil {
		return fmt.Errorf("mirror source %s: %w", mirrorURI, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("mirror source %s is not a directory", mirrorURI)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	return filepath.Walk(mirrorURI, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(mirrorURI, path)
		if relErr != nil {
			return relErr
		}
		dest := filepath.Join(destDir, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		_, copyErr := copyFile(path, dest)
		return copyErr
	})
}

func copyFile(src, dst string) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	return io.Copy(out, in)
}

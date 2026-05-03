package agent

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	SessionIndexDirName      = "sessions"
	LegacySessionsFileName   = "sessions.jsonl"
	LegacySessionsRenameName = "sessions.jsonl.legacy"
)

// SessionIndexEntry is the pointer-only session index row. Heavy prompt,
// response, and stderr bodies stay in execution bundles or native logs.
type SessionIndexEntry struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"projectID,omitempty"`
	BeadID          string    `json:"beadID,omitempty"`
	WorkerID        string    `json:"workerID,omitempty"`
	Harness         string    `json:"harness"`
	Provider        string    `json:"provider,omitempty"`
	Surface         string    `json:"surface,omitempty"`
	BaseURL         string    `json:"baseURL,omitempty"`
	BillingMode     string    `json:"billingMode"`
	Model           string    `json:"model,omitempty"`
	StartedAt       time.Time `json:"startedAt"`
	EndedAt         time.Time `json:"endedAt,omitempty"`
	DurationMS      int       `json:"durationMs,omitempty"`
	CostUSD         float64   `json:"cost,omitempty"`
	CostPresent     bool      `json:"costPresent,omitempty"`
	Tokens          int       `json:"tokens,omitempty"`
	InputTokens     int       `json:"inputTokens,omitempty"`
	CachedTokens    int       `json:"cachedTokens,omitempty"`
	OutputTokens    int       `json:"outputTokens,omitempty"`
	Outcome         string    `json:"outcome,omitempty"`
	ExitCode        int       `json:"exitCode"`
	NativeSessionID string    `json:"nativeSessionID,omitempty"`
	TraceID         string    `json:"traceID,omitempty"`
	SpanID          string    `json:"spanID,omitempty"`
	BundlePath      string    `json:"bundlePath,omitempty"`
	NativeLogRef    string    `json:"nativeLogRef,omitempty"`
	Effort          string    `json:"effort,omitempty"`
	Detail          string    `json:"detail,omitempty"`
	BaseRev         string    `json:"baseRev,omitempty"`
	ResultRev       string    `json:"resultRev,omitempty"`
	// ToolCalls is the normalized stream of tool_call/tool_result pairs
	// captured at drain time (Story 16). Persisting here means resolver-time
	// reads do not have to parse raw native logs.
	ToolCalls []ToolCallEntry `json:"toolCalls,omitempty"`
}

type SessionIndexQuery struct {
	StartedAfter  *time.Time
	StartedBefore *time.Time
	DefaultRecent bool
}

type SessionBodies struct {
	Prompt   string
	Response string
	Stderr   string
}

func SessionIndexDir(logDir string) string {
	return filepath.Join(logDir, SessionIndexDirName)
}

func SessionIndexShardName(t time.Time) string {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return fmt.Sprintf("sessions-%04d-%02d.jsonl", t.UTC().Year(), t.UTC().Month())
}

func SessionIndexShardPath(logDir string, t time.Time) string {
	return filepath.Join(SessionIndexDir(logDir), SessionIndexShardName(t))
}

func ProjectIDForPath(path string) string {
	canonical, err := filepath.Abs(path)
	if err == nil {
		path = canonical
	}
	h := sha256.Sum256([]byte(path))
	return fmt.Sprintf("proj-%x", h[:4])
}

func AppendSessionIndex(logDir string, entry SessionIndexEntry, now time.Time) error {
	if logDir == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if entry.ID == "" {
		entry.ID = genSessionID()
	}
	if entry.StartedAt.IsZero() {
		entry.StartedAt = now.UTC()
	}
	if entry.EndedAt.IsZero() && entry.DurationMS > 0 {
		entry.EndedAt = entry.StartedAt.Add(time.Duration(entry.DurationMS) * time.Millisecond).UTC()
	}
	if entry.Outcome == "" {
		if entry.ExitCode == 0 && entry.Detail == "" {
			entry.Outcome = "success"
		} else {
			entry.Outcome = "failure"
		}
	}
	if entry.BillingMode == "" {
		entry.BillingMode = billingModeFor(entry.Harness, entry.Surface, entry.BaseURL)
	}
	if !ValidateBillingMode(entry.BillingMode) {
		return fmt.Errorf("invalid billingMode %q", entry.BillingMode)
	}
	if err := os.MkdirAll(SessionIndexDir(logDir), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	path := SessionIndexShardPath(logDir, now.UTC())
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

// SessionIndexInputs carries the small subset of RunArgs fields that
// SessionIndexEntryFromResult needs, decoupling it from the full RunArgs
// surface.
type SessionIndexInputs struct {
	Harness     string
	Model       string
	Provider    string
	Effort      string
	Correlation map[string]string
}

func SessionIndexEntryFromResult(projectRoot string, inputs SessionIndexInputs, result *Result, startedAt, endedAt time.Time) SessionIndexEntry {
	if result == nil {
		result = &Result{}
	}
	corr := inputs.Correlation
	id := ""
	beadID := ""
	workerID := ""
	bundlePath := ""
	baseRev := ""
	nativeLogRef := ""
	nativeSessionID := ""
	traceID := ""
	spanID := ""
	if corr != nil {
		id = corr["session_id"]
		beadID = corr["bead_id"]
		workerID = corr["worker_id"]
		bundlePath = corr["bundle_path"]
		baseRev = corr["base_rev"]
		nativeLogRef = corr["native_log_ref"]
		nativeSessionID = corr["native_session_id"]
		traceID = corr["trace_id"]
		spanID = corr["span_id"]
	}
	if id == "" {
		id = genSessionID()
	}
	if bundlePath == "" && corr != nil && corr["attempt_id"] != "" {
		bundlePath = filepath.Join(ExecuteBeadArtifactDir, corr["attempt_id"])
	}
	if nativeLogRef == "" && result.AgentSessionID != "" {
		nativeLogRef = relativizePath(projectRoot, result.AgentSessionID)
	}
	if nativeSessionID == "" {
		nativeSessionID = result.AgentSessionID
	}
	harness := result.Harness
	if harness == "" {
		harness = inputs.Harness
	}
	model := result.Model
	if model == "" {
		model = inputs.Model
	}
	outcome := "success"
	if result.ExitCode != 0 || result.Error != "" {
		outcome = "failure"
	}
	return SessionIndexEntry{
		ID:              id,
		ProjectID:       ProjectIDForPath(projectRoot),
		BeadID:          beadID,
		WorkerID:        workerID,
		Harness:         harness,
		Provider:        firstNonEmpty(result.Provider, inputs.Provider),
		BaseURL:         result.ResolvedBaseURL,
		BillingMode:     billingModeFor(harness, "", result.ResolvedBaseURL),
		Model:           model,
		StartedAt:       startedAt.UTC(),
		EndedAt:         endedAt.UTC(),
		DurationMS:      int(endedAt.Sub(startedAt).Milliseconds()),
		CostUSD:         result.CostUSD,
		CostPresent:     result.CostUSD != 0,
		Tokens:          result.Tokens,
		InputTokens:     result.InputTokens,
		CachedTokens:    result.CachedTokens,
		OutputTokens:    result.OutputTokens,
		Outcome:         outcome,
		ExitCode:        result.ExitCode,
		NativeSessionID: nativeSessionID,
		TraceID:         traceID,
		SpanID:          spanID,
		BundlePath:      filepath.ToSlash(bundlePath),
		NativeLogRef:    filepath.ToSlash(nativeLogRef),
		Effort:          inputs.Effort,
		Detail:          result.Error,
		BaseRev:         baseRev,
		ToolCalls:       append([]ToolCallEntry(nil), result.ToolCalls...),
	}
}

func SessionIndexEntryFromLegacy(projectRoot string, e SessionEntry) SessionIndexEntry {
	beadID := ""
	workerID := ""
	provider := e.Provider
	effort := ""
	bundlePath := ""
	baseRev := e.BaseRev
	baseURL := e.BaseURL
	if e.Correlation != nil {
		beadID = e.Correlation["bead_id"]
		workerID = e.Correlation["worker_id"]
		provider = firstNonEmpty(provider, e.Correlation["provider"], e.Correlation["resolved_provider"])
		effort = e.Correlation["effort"]
		baseURL = firstNonEmpty(baseURL, e.Correlation["base_url"], e.Correlation["resolved_base_url"])
		if baseRev == "" {
			baseRev = e.Correlation["base_rev"]
		}
		if attemptID := e.Correlation["attempt_id"]; attemptID != "" {
			bundlePath = filepath.Join(ExecuteBeadArtifactDir, attemptID)
		}
	}
	outcome := "success"
	if e.ExitCode != 0 || e.Error != "" {
		outcome = "failure"
	}
	endedAt := time.Time{}
	if e.Duration > 0 {
		endedAt = e.Timestamp.Add(time.Duration(e.Duration) * time.Millisecond).UTC()
	}
	totalTokens := e.TotalTokens
	if totalTokens == 0 {
		totalTokens = e.Tokens
	}
	billingMode := e.BillingMode
	if billingMode == "" {
		billingMode = billingModeFor(e.Harness, e.Surface, baseURL)
	}
	return SessionIndexEntry{
		ID:              e.ID,
		ProjectID:       ProjectIDForPath(projectRoot),
		BeadID:          beadID,
		WorkerID:        workerID,
		Harness:         e.Harness,
		Provider:        provider,
		Surface:         e.Surface,
		BaseURL:         baseURL,
		BillingMode:     billingMode,
		Model:           e.Model,
		StartedAt:       e.Timestamp.UTC(),
		EndedAt:         endedAt,
		DurationMS:      e.Duration,
		CostUSD:         e.CostUSD,
		CostPresent:     e.CostUSD != 0,
		Tokens:          totalTokens,
		InputTokens:     e.InputTokens,
		CachedTokens:    e.CachedTokens,
		OutputTokens:    e.OutputTokens,
		Outcome:         outcome,
		ExitCode:        e.ExitCode,
		NativeSessionID: e.NativeSessionID,
		TraceID:         e.TraceID,
		SpanID:          e.SpanID,
		BundlePath:      filepath.ToSlash(bundlePath),
		NativeLogRef:    filepath.ToSlash(e.NativeLogRef),
		Effort:          effort,
		Detail:          e.Error,
		BaseRev:         baseRev,
		ResultRev:       e.ResultRev,
		ToolCalls:       append([]ToolCallEntry(nil), e.ToolCalls...),
	}
}

func ReadSessionIndex(logDir string, q SessionIndexQuery) ([]SessionIndexEntry, error) {
	files, err := SessionIndexShardFiles(logDir, q)
	if err != nil {
		return nil, err
	}
	var out []SessionIndexEntry
	if q.DefaultRecent && q.StartedAfter == nil && q.StartedBefore == nil {
		out, err = readRecentSessionIndexFiles(files, 1000)
	} else {
		out, err = readSessionIndexFiles(files, q)
	}
	if err != nil {
		return nil, err
	}
	if len(out) == 0 && q.DefaultRecent && q.StartedAfter == nil && q.StartedBefore == nil {
		files, err = SessionIndexShardFiles(logDir, SessionIndexQuery{})
		if err != nil {
			return nil, err
		}
		out, err = readSessionIndexFiles(files, q)
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out, nil
}

func readRecentSessionIndexFiles(files []string, perFile int) ([]SessionIndexEntry, error) {
	var out []SessionIndexEntry
	for _, file := range files {
		data, err := os.ReadFile(file)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
		start := 0
		if perFile > 0 && len(lines) > perFile {
			start = len(lines) - perFile
		}
		for _, line := range lines[start:] {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var entry SessionIndexEntry
			if err := json.Unmarshal(line, &entry); err == nil {
				out = append(out, entry)
			}
		}
	}
	return out, nil
}

func readSessionIndexFiles(files []string, q SessionIndexQuery) ([]SessionIndexEntry, error) {
	var out []SessionIndexEntry
	for _, file := range files {
		err := ForEachJSONL[SessionIndexEntry](file, func(entry SessionIndexEntry) error {
			if !sessionIndexEntryInRange(entry, q) {
				return nil
			}
			out = append(out, entry)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func FindSessionIndex(logDir, id string) (SessionIndexEntry, bool, error) {
	files, err := SessionIndexShardFiles(logDir, SessionIndexQuery{})
	if err != nil {
		return SessionIndexEntry{}, false, err
	}
	for _, file := range files {
		var found SessionIndexEntry
		ok := false
		err := ForEachJSONL[SessionIndexEntry](file, func(entry SessionIndexEntry) error {
			if entry.ID == id {
				found = entry
				ok = true
				return fmt.Errorf("found")
			}
			return nil
		})
		if ok {
			return found, true, nil
		}
		if err != nil && err.Error() != "found" {
			return SessionIndexEntry{}, false, err
		}
	}
	return SessionIndexEntry{}, false, nil
}

func SessionIndexShardFiles(logDir string, q SessionIndexQuery) ([]string, error) {
	dir := SessionIndexDir(logDir)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasPrefix(name, "sessions-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		if !sessionShardIntersects(name, q) {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files, nil
}

func ReindexLegacySessions(projectRoot, logDir string) (int, error) {
	legacyPath := filepath.Join(logDir, LegacySessionsFileName)
	readPath := legacyPath
	if _, err := os.Stat(readPath); os.IsNotExist(err) {
		readPath = filepath.Join(logDir, LegacySessionsRenameName)
		if _, legacyErr := os.Stat(readPath); os.IsNotExist(legacyErr) {
			return 0, nil
		} else if legacyErr != nil {
			return 0, legacyErr
		}
	} else if err != nil {
		return 0, err
	}

	existing, err := existingSessionIndexIDs(logDir)
	if err != nil {
		return 0, err
	}
	count := 0
	err = ForEachJSONL[SessionEntry](readPath, func(entry SessionEntry) error {
		if entry.ID == "" {
			return nil
		}
		if _, ok := existing[entry.ID]; ok {
			return nil
		}
		idx := SessionIndexEntryFromLegacy(projectRoot, entry)
		if err := AppendSessionIndex(logDir, idx, entry.Timestamp.UTC()); err != nil {
			return err
		}
		existing[entry.ID] = struct{}{}
		count++
		return nil
	})
	if err != nil {
		return count, err
	}
	if readPath == legacyPath {
		renamed := filepath.Join(logDir, LegacySessionsRenameName)
		if _, err := os.Stat(renamed); os.IsNotExist(err) {
			if err := os.Rename(legacyPath, renamed); err != nil {
				return count, err
			}
		}
	}
	return count, nil
}

func LoadSessionBodies(projectRoot string, entry SessionIndexEntry) SessionBodies {
	var bodies SessionBodies
	if entry.BundlePath != "" {
		bundleDir := resolveProjectPath(projectRoot, entry.BundlePath)
		if data, err := os.ReadFile(filepath.Join(bundleDir, "prompt.md")); err == nil {
			bodies.Prompt = string(data)
		}
		if data, err := os.ReadFile(filepath.Join(bundleDir, "result.json")); err == nil {
			bodies.Response, bodies.Stderr = bodiesFromResultJSON(data)
		}
	}
	if entry.NativeLogRef != "" {
		nativePath := resolveProjectPath(projectRoot, entry.NativeLogRef)
		resp, stderr := bodiesFromNativeLog(nativePath)
		if bodies.Response == "" {
			bodies.Response = resp
		}
		if bodies.Stderr == "" {
			bodies.Stderr = stderr
		}
	}
	if bodies.Stderr == "" && entry.Detail != "" && entry.Outcome == "failure" {
		bodies.Stderr = entry.Detail
	}
	return bodies
}

func SessionIndexEntryToLegacy(e SessionIndexEntry) SessionEntry {
	corr := map[string]string{}
	if e.BeadID != "" {
		corr["bead_id"] = e.BeadID
	}
	if e.WorkerID != "" {
		corr["worker_id"] = e.WorkerID
	}
	if e.Effort != "" {
		corr["effort"] = e.Effort
	}
	if e.BundlePath != "" {
		corr["bundle_path"] = e.BundlePath
		if attemptID := filepath.Base(filepath.FromSlash(e.BundlePath)); attemptID != "." && attemptID != string(filepath.Separator) {
			corr["attempt_id"] = attemptID
		}
	}
	return SessionEntry{
		ID:              e.ID,
		Timestamp:       e.StartedAt,
		Harness:         e.Harness,
		Provider:        e.Provider,
		Surface:         e.Surface,
		BaseURL:         e.BaseURL,
		BillingMode:     e.BillingMode,
		Model:           e.Model,
		Correlation:     corr,
		NativeSessionID: e.NativeSessionID,
		NativeLogRef:    e.NativeLogRef,
		TraceID:         e.TraceID,
		SpanID:          e.SpanID,
		Tokens:          e.Tokens,
		InputTokens:     e.InputTokens,
		CachedTokens:    e.CachedTokens,
		OutputTokens:    e.OutputTokens,
		TotalTokens:     e.Tokens,
		CostUSD:         e.CostUSD,
		Duration:        e.DurationMS,
		ExitCode:        e.ExitCode,
		Error:           e.Detail,
		BaseRev:         e.BaseRev,
		ResultRev:       e.ResultRev,
		ToolCalls:       append([]ToolCallEntry(nil), e.ToolCalls...),
	}
}

func sessionIndexEntryInRange(entry SessionIndexEntry, q SessionIndexQuery) bool {
	if q.StartedAfter != nil && entry.StartedAt.Before(*q.StartedAfter) {
		return false
	}
	if q.StartedBefore != nil && !entry.StartedAt.Before(*q.StartedBefore) {
		return false
	}
	return true
}

func sessionShardIntersects(name string, q SessionIndexQuery) bool {
	month, ok := sessionShardMonth(name)
	if !ok {
		return false
	}
	next := month.AddDate(0, 1, 0)
	if q.DefaultRecent && q.StartedAfter == nil && q.StartedBefore == nil {
		now := time.Now().UTC()
		current := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		previous := current.AddDate(0, -1, 0)
		return month.Equal(current) || month.Equal(previous)
	}
	if q.StartedAfter != nil && !next.After(*q.StartedAfter) {
		return false
	}
	if q.StartedBefore != nil && !month.Before(*q.StartedBefore) {
		return false
	}
	return true
}

func sessionShardMonth(name string) (time.Time, bool) {
	if len(name) != len("sessions-2006-01.jsonl") {
		return time.Time{}, false
	}
	month, err := time.Parse("2006-01", strings.TrimSuffix(strings.TrimPrefix(name, "sessions-"), ".jsonl"))
	if err != nil {
		return time.Time{}, false
	}
	return month.UTC(), true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func existingSessionIndexIDs(logDir string) (map[string]struct{}, error) {
	ids := map[string]struct{}{}
	files, err := SessionIndexShardFiles(logDir, SessionIndexQuery{})
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		err := ForEachJSONL[SessionIndexEntry](file, func(entry SessionIndexEntry) error {
			if entry.ID != "" {
				ids[entry.ID] = struct{}{}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func relativizePath(root, path string) string {
	if path == "" {
		return ""
	}
	if root != "" && filepath.IsAbs(path) {
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

func resolveProjectPath(projectRoot, path string) string {
	if path == "" || filepath.IsAbs(path) || projectRoot == "" {
		return path
	}
	return filepath.Join(projectRoot, filepath.FromSlash(path))
}

func bodiesFromResultJSON(data []byte) (string, string) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", ""
	}
	response := firstString(raw, "response", "output", "final_text", "finalText", "detail")
	stderr := firstString(raw, "stderr", "error")
	return response, stderr
}

func bodiesFromNativeLog(path string) (string, string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	var response, stderr string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var raw map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			continue
		}
		if s := firstNestedString(raw, []string{"data", "final_text"}, []string{"data", "finalText"}, []string{"final_text"}, []string{"finalText"}); s != "" {
			response = s
		}
		if s := firstNestedString(raw, []string{"data", "error"}, []string{"error"}, []string{"stderr"}); s != "" {
			stderr = s
		}
	}
	return response, stderr
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := raw[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func firstNestedString(raw map[string]any, paths ...[]string) string {
	for _, path := range paths {
		cur := any(raw)
		for _, key := range path {
			m, ok := cur.(map[string]any)
			if !ok {
				cur = nil
				break
			}
			cur = m[key]
		}
		if s, ok := cur.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

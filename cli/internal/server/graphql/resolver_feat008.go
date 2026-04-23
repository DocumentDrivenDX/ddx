package graphql

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	"github.com/DocumentDrivenDX/ddx/internal/persona"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const (
	paletteResultCap       = 50
	efficacyWarningFloor   = 0.70
	queuedPlaceholderState = "queued"
)

// BeadClose is the resolver for the beadClose field.
func (r *mutationResolver) BeadClose(ctx context.Context, id string, reason *string) (*Bead, error) {
	if r.WorkingDir == "" {
		return nil, fmt.Errorf("working directory not configured")
	}
	store := r.beadStore()
	if err := store.Close(id); err != nil {
		return nil, err
	}
	if reason != nil && strings.TrimSpace(*reason) != "" {
		_ = store.AppendEvent(id, bead.BeadEvent{
			Kind:    "summary",
			Summary: "closed: " + strings.TrimSpace(*reason),
		})
	}
	b, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	return beadModelFromBead(b), nil
}

// WorkerDispatch is the resolver for the workerDispatch field.
func (r *mutationResolver) WorkerDispatch(ctx context.Context, kind string, projectID string, args *string) (*WorkerDispatchResult, error) {
	switch kind {
	case "execute-loop":
		if r.Actions == nil {
			return nil, fmt.Errorf("execute-loop worker dispatcher is not configured")
		}
		return r.Actions.DispatchWorker(ctx, kind, r.projectRoot(projectID), args)
	case "realign-specs", "run-checks":
		return &WorkerDispatchResult{
			ID:    "queued-worker-" + slug(kind),
			State: queuedPlaceholderState,
			Kind:  kind,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported worker kind %q", kind)
	}
}

// StartWorker is the resolver for the startWorker field.
func (r *mutationResolver) StartWorker(ctx context.Context, input StartWorkerInput) (*WorkerDispatchResult, error) {
	if r.Actions == nil {
		return nil, fmt.Errorf("execute-loop worker dispatcher is not configured")
	}
	args := map[string]string{}
	if input.Harness != nil && strings.TrimSpace(*input.Harness) != "" {
		args["harness"] = strings.TrimSpace(*input.Harness)
	}
	profile := "smart"
	if input.Profile != nil && strings.TrimSpace(*input.Profile) != "" {
		profile = strings.TrimSpace(*input.Profile)
	}
	args["profile"] = profile
	effort := "medium"
	if input.Effort != nil && strings.TrimSpace(*input.Effort) != "" {
		effort = strings.TrimSpace(*input.Effort)
	}
	args["effort"] = effort
	if input.LabelFilter != nil && strings.TrimSpace(*input.LabelFilter) != "" {
		args["label_filter"] = strings.TrimSpace(*input.LabelFilter)
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	rawArgs := string(raw)
	return r.Actions.DispatchWorker(ctx, "execute-loop", r.projectRoot(input.ProjectID), &rawArgs)
}

// StopWorker is the resolver for the stopWorker field.
func (r *mutationResolver) StopWorker(ctx context.Context, id string) (*WorkerLifecycleResult, error) {
	if r.Actions == nil {
		return nil, fmt.Errorf("execute-loop worker dispatcher is not configured")
	}
	return r.Actions.StopWorker(ctx, id)
}

// PluginDispatch is the resolver for the pluginDispatch field.
func (r *mutationResolver) PluginDispatch(ctx context.Context, name string, action string, scope string) (*PluginDispatchResult, error) {
	name = strings.TrimSpace(name)
	action = strings.ToLower(strings.TrimSpace(action))
	scope = strings.ToLower(strings.TrimSpace(scope))
	if name == "" {
		return nil, fmt.Errorf("plugin name is required")
	}
	if scope != "project" {
		return nil, fmt.Errorf("unsupported plugin scope %q", scope)
	}

	state, err := dispatchPluginAction(r.WorkingDir, name, action)
	if err != nil {
		return nil, err
	}
	id := newDispatchID("plugin", action, name)
	if err := writeJSONRecord(r.WorkingDir, "plugin-dispatches", id, pluginDispatchRecord{
		ID:        id,
		Name:      name,
		Action:    action,
		Scope:     scope,
		State:     state,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		return nil, err
	}
	return &PluginDispatchResult{
		ID:     id,
		State:  state,
		Action: action,
	}, nil
}

// ComparisonDispatch is the resolver for the comparisonDispatch field.
func (r *mutationResolver) ComparisonDispatch(ctx context.Context, arms []*ComparisonArmInput) (*ComparisonDispatchResult, error) {
	if len(arms) == 0 {
		return nil, fmt.Errorf("comparison requires at least one arm")
	}
	for i, arm := range arms {
		if arm == nil {
			return nil, fmt.Errorf("comparison arm %d is required", i)
		}
		arm.Model = strings.TrimSpace(arm.Model)
		arm.Prompt = strings.TrimSpace(arm.Prompt)
		if arm.Model == "" {
			return nil, fmt.Errorf("comparison arm %d model is required", i)
		}
		if arm.Prompt == "" {
			return nil, fmt.Errorf("comparison arm %d prompt is required", i)
		}
		if arm.Harness != nil {
			trimmed := strings.TrimSpace(*arm.Harness)
			arm.Harness = &trimmed
		}
		if arm.Provider != nil {
			trimmed := strings.TrimSpace(*arm.Provider)
			arm.Provider = &trimmed
		}
	}

	id := newDispatchID("comparison")
	record := comparisonDispatchRecord{
		ID:        id,
		State:     queuedPlaceholderState,
		ArmCount:  len(arms),
		Arms:      arms,
		CreatedAt: time.Now().UTC(),
	}
	if err := writeJSONRecord(r.WorkingDir, "comparisons", id, record); err != nil {
		return nil, err
	}
	return &ComparisonDispatchResult{
		ID:       id,
		State:    queuedPlaceholderState,
		ArmCount: len(arms),
	}, nil
}

// PersonaBind is the resolver for the personaBind field.
func (r *mutationResolver) PersonaBind(ctx context.Context, role string, personaName string, projectID string) (*PersonaBindResult, error) {
	root := r.projectRoot(projectID)
	configPath := filepath.Join(root, ".ddx", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return nil, err
	}
	manager := persona.NewBindingManagerWithPath(configPath)
	if err := manager.SetBinding(role, personaName); err != nil {
		return nil, err
	}
	return &PersonaBindResult{Ok: true, Role: role, Persona: personaName}, nil
}

// PersonaCreate is the resolver for the personaCreate field.
func (r *mutationResolver) PersonaCreate(ctx context.Context, name string, body string, projectID string) (*Persona, error) {
	writer := persona.NewProjectPersonaWriter(r.projectRoot(projectID))
	p, err := writer.Create(name, body)
	if err != nil {
		return nil, personaGraphQLError(err)
	}
	return personaToGQL(p), nil
}

// PersonaUpdate is the resolver for the personaUpdate field.
func (r *mutationResolver) PersonaUpdate(ctx context.Context, name string, body string, projectID string) (*Persona, error) {
	writer := persona.NewProjectPersonaWriter(r.projectRoot(projectID))
	p, err := writer.Update(name, body)
	if err != nil {
		return nil, personaGraphQLError(err)
	}
	return personaToGQL(p), nil
}

// PersonaDelete is the resolver for the personaDelete field.
func (r *mutationResolver) PersonaDelete(ctx context.Context, name string, projectID string) (*PersonaDeleteResult, error) {
	writer := persona.NewProjectPersonaWriter(r.projectRoot(projectID))
	if err := writer.Delete(name); err != nil {
		return nil, personaGraphQLError(err)
	}
	return &PersonaDeleteResult{Ok: true, Name: name}, nil
}

// PersonaFork is the resolver for the personaFork field.
func (r *mutationResolver) PersonaFork(ctx context.Context, libraryName string, newName *string, projectID string) (*Persona, error) {
	writer := persona.NewProjectPersonaWriter(r.projectRoot(projectID))
	target := ""
	if newName != nil {
		target = *newName
	}
	p, err := writer.Fork(libraryName, target)
	if err != nil {
		return nil, personaGraphQLError(err)
	}
	return personaToGQL(p), nil
}

func personaGraphQLError(err error) error {
	var personaErr *persona.PersonaError
	if !errors.As(err, &personaErr) {
		return err
	}
	return &gqlerror.Error{
		Message: personaErr.Message,
		Err:     err,
		Extensions: map[string]any{
			"code": personaErr.Type,
		},
	}
}

// QueueSummary is the resolver for the queueSummary field.
func (r *queryResolver) QueueSummary(ctx context.Context, projectID string) (*QueueSummary, error) {
	store := bead.NewStore(filepath.Join(r.projectRoot(projectID), ".ddx"))
	ready, err := store.Ready()
	if err != nil {
		return nil, err
	}
	blocked, err := store.BlockedAll()
	if err != nil {
		return nil, err
	}
	all, err := store.ReadAll()
	if err != nil {
		return nil, err
	}
	var inProgress int
	for _, b := range all {
		if b.Status == bead.StatusInProgress || b.Status == "in-progress" {
			inProgress++
		}
	}
	return &QueueSummary{
		Ready:      len(ready),
		Blocked:    len(blocked),
		InProgress: inProgress,
	}, nil
}

// EfficacyRows is the resolver for the efficacyRows field.
func (r *queryResolver) EfficacyRows(ctx context.Context, since *string, until *string, projectID *string) ([]*EfficacyRow, error) {
	snap, err := r.efficacySnapshot(since, until, projectID)
	if err != nil {
		return nil, err
	}
	return append([]*EfficacyRow(nil), snap.rows...), nil
}

// EfficacyAttempts is the resolver for the efficacyAttempts field.
func (r *queryResolver) EfficacyAttempts(ctx context.Context, rowKey string, since *string, until *string, projectID *string) (*EfficacyAttempts, error) {
	snap, err := r.efficacySnapshot(since, until, projectID)
	if err != nil {
		return nil, err
	}
	attempts := append([]*EfficacyAttempt(nil), snap.attempts[rowKey]...)
	return &EfficacyAttempts{RowKey: rowKey, Attempts: attempts}, nil
}

// Comparisons is the resolver for the comparisons field.
func (r *queryResolver) Comparisons(ctx context.Context) ([]*ComparisonRecord, error) {
	records, err := readComparisonRecords(r.WorkingDir)
	if err != nil {
		return nil, err
	}
	out := make([]*ComparisonRecord, 0, len(records))
	for _, record := range records {
		out = append(out, &ComparisonRecord{
			ID:       record.ID,
			State:    record.State,
			ArmCount: record.ArmCount,
		})
	}
	return out, nil
}

// PluginsList is the resolver for the pluginsList field.
func (r *queryResolver) PluginsList(ctx context.Context) ([]*PluginInfo, error) {
	return pluginCatalog(r.WorkingDir)
}

// PluginDetail is the resolver for the pluginDetail field.
func (r *queryResolver) PluginDetail(ctx context.Context, name string) (*PluginInfo, error) {
	plugins, err := pluginCatalog(r.WorkingDir)
	if err != nil {
		return nil, err
	}
	for _, plugin := range plugins {
		if plugin.Name == name {
			return plugin, nil
		}
	}
	return nil, nil
}

// ProjectBindings is the resolver for the projectBindings field.
func (r *queryResolver) ProjectBindings(ctx context.Context, projectID string) (string, error) {
	cfg, err := config.LoadWithWorkingDir(r.projectRoot(projectID))
	if err != nil {
		return "{}", nil
	}
	raw, err := json.Marshal(cfg.PersonaBindings)
	if err != nil {
		return "{}", nil
	}
	return string(raw), nil
}

// PaletteSearch is the resolver for the paletteSearch field.
func (r *queryResolver) PaletteSearch(ctx context.Context, query string) (*PaletteSearchResults, error) {
	q := strings.TrimSpace(query)
	out := &PaletteSearchResults{
		Documents:  []*PaletteDocumentResult{},
		Beads:      []*PaletteBeadResult{},
		Actions:    []*PaletteActionResult{},
		Navigation: []*PaletteNavigationResult{},
	}
	if q == "" {
		return out, nil
	}

	matches := collectPaletteMatches(q, r.WorkingDir)
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score < matches[j].score
		}
		return matches[i].sortKey < matches[j].sortKey
	})
	if len(matches) > paletteResultCap {
		matches = matches[:paletteResultCap]
	}
	for _, match := range matches {
		switch match.kind {
		case "document":
			out.Documents = append(out.Documents, &PaletteDocumentResult{Kind: "document", Path: match.id, Title: match.title})
		case "bead":
			out.Beads = append(out.Beads, &PaletteBeadResult{Kind: "bead", ID: match.id, Title: match.title})
		case "action":
			out.Actions = append(out.Actions, &PaletteActionResult{Kind: "action", ID: match.id, Label: match.title})
		case "nav":
			out.Navigation = append(out.Navigation, &PaletteNavigationResult{Kind: "nav", Route: match.id, Title: match.title})
		}
	}
	return out, nil
}

func (r *Resolver) projectRoot(projectID string) string {
	if r.State != nil {
		if proj, ok := r.State.GetProjectSnapshotByID(projectID); ok && proj.Path != "" {
			return proj.Path
		}
	}
	return r.WorkingDir
}

type pluginDispatchRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Action    string    `json:"action"`
	Scope     string    `json:"scope"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}

type comparisonDispatchRecord struct {
	ID        string                `json:"id"`
	State     string                `json:"state"`
	ArmCount  int                   `json:"arm_count"`
	Arms      []*ComparisonArmInput `json:"arms"`
	CreatedAt time.Time             `json:"created_at"`
}

func dispatchPluginAction(workingDir string, name string, action string) (string, error) {
	state, err := registry.LoadState()
	if err != nil {
		return "", fmt.Errorf("loading plugin state: %w", err)
	}

	switch action {
	case "install":
		if entry := state.FindInstalled(name); entry != nil && entry.VerifyFiles() {
			return "installed", nil
		}
		entry, err := installRegistryPlugin(workingDir, state, name)
		if err != nil {
			return "", err
		}
		if entry != nil && entry.VerifyFiles() {
			return "installed", nil
		}
		return "", fmt.Errorf("plugin %q install completed but recorded files are not present", name)
	case "update":
		installed := state.FindInstalled(name)
		if installed == nil {
			return "", fmt.Errorf("plugin %q is not installed", name)
		}
		pkg, err := registry.BuiltinRegistry().Find(name)
		if err != nil {
			return "", fmt.Errorf("plugin %q is not updateable from the built-in registry", name)
		}
		if installed.Version == pkg.Version && installed.VerifyFiles() {
			return "installed", nil
		}
		entry, err := installRegistryPlugin(workingDir, state, name)
		if err != nil {
			return "", err
		}
		if entry != nil && entry.VerifyFiles() {
			return "installed", nil
		}
		return "", fmt.Errorf("plugin %q update completed but recorded files are not present", name)
	case "uninstall":
		entry := state.FindInstalled(name)
		if entry == nil {
			return "", fmt.Errorf("plugin %q is not installed", name)
		}
		if err := registry.UninstallPackage(entry); err != nil {
			return "", err
		}
		state.Remove(name)
		if err := registry.SaveState(state); err != nil {
			return "", fmt.Errorf("saving plugin state: %w", err)
		}
		return "uninstalled", nil
	default:
		return "", fmt.Errorf("unsupported plugin action %q", action)
	}
}

func installRegistryPlugin(workingDir string, state *registry.InstalledState, name string) (*registry.InstalledEntry, error) {
	pkg, err := registry.BuiltinRegistry().Find(name)
	if err != nil {
		return nil, err
	}

	origDir, _ := os.Getwd()
	if workingDir != "" && origDir != workingDir {
		if err := os.Chdir(workingDir); err != nil {
			return nil, fmt.Errorf("entering project root: %w", err)
		}
		defer func() { _ = os.Chdir(origDir) }()
	}

	entry, err := registry.InstallPackage(pkg)
	if err != nil {
		return nil, fmt.Errorf("installing plugin %q: %w", name, err)
	}
	state.AddOrUpdate(entry)
	if err := registry.SaveState(state); err != nil {
		return nil, fmt.Errorf("saving plugin state: %w", err)
	}
	return state.FindInstalled(name), nil
}

func readComparisonRecords(workingDir string) ([]comparisonDispatchRecord, error) {
	dir := filepath.Join(workingDir, ".ddx", "comparisons")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []comparisonDispatchRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	records := make([]comparisonDispatchRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var record comparisonDispatchRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("parsing comparison record %s: %w", entry.Name(), err)
		}
		if record.ID == "" {
			continue
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if !records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].CreatedAt.After(records[j].CreatedAt)
		}
		return records[i].ID < records[j].ID
	})
	return records, nil
}

func writeJSONRecord(workingDir string, kind string, id string, record any) error {
	dir := filepath.Join(workingDir, ".ddx", kind)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, id+".json"), data, 0o644)
}

func newDispatchID(prefix string, parts ...string) string {
	segments := []string{slug(prefix)}
	for _, part := range parts {
		if s := slug(part); s != "" {
			segments = append(segments, s)
		}
	}
	segments = append(segments, randomHex(4))
	return strings.Join(segments, "-")
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(b)
}

func pluginCatalog(workingDir string) ([]*PluginInfo, error) {
	reg := registry.BuiltinRegistry()
	state, err := registry.LoadState()
	if err != nil {
		return nil, err
	}

	byName := map[string]*PluginInfo{}
	installedByName := map[string]registry.InstalledEntry{}
	for _, entry := range state.Installed {
		installedByName[entry.Name] = entry
	}

	for _, pkg := range reg.Packages {
		info := pluginInfoFromPackage(pkg, workingDir, installedByName[pkg.Name])
		byName[info.Name] = info
	}
	for _, entry := range state.Installed {
		if _, ok := byName[entry.Name]; ok {
			continue
		}
		info := pluginInfoFromInstalled(entry, workingDir)
		byName[info.Name] = info
	}

	out := make([]*PluginInfo, 0, len(byName))
	for _, info := range byName {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func pluginInfoFromPackage(pkg registry.Package, workingDir string, installed registry.InstalledEntry) *PluginInfo {
	info := &PluginInfo{
		Name:           pkg.Name,
		Version:        pkg.Version,
		Type:           string(pkg.Type),
		Description:    pkg.Description,
		Keywords:       append([]string(nil), pkg.Keywords...),
		Status:         "available",
		RegistrySource: pkg.Source,
		Skills:         []string{},
		Prompts:        []string{},
		Templates:      []string{},
	}
	if installed.Name != "" {
		info.InstalledVersion = strPtr(installed.Version)
		info.Status = "installed"
		if installed.Version != "" && installed.Version != pkg.Version {
			info.Status = "update-available"
		}
	}
	if manifest, err := json.Marshal(pkg); err == nil {
		info.Manifest = strPtr(string(manifest))
	}
	enrichPluginInfoFromDisk(info, workingDir, pkg.Name, installed)
	return info
}

func pluginInfoFromInstalled(entry registry.InstalledEntry, workingDir string) *PluginInfo {
	info := &PluginInfo{
		Name:             entry.Name,
		Version:          entry.Version,
		InstalledVersion: strPtr(entry.Version),
		Type:             string(entry.Type),
		Description:      entry.Name,
		Status:           "installed",
		RegistrySource:   entry.Source,
		Skills:           []string{},
		Prompts:          []string{},
		Templates:        []string{},
	}
	enrichPluginInfoFromDisk(info, workingDir, entry.Name, entry)
	return info
}

func enrichPluginInfoFromDisk(info *PluginInfo, workingDir string, name string, entry registry.InstalledEntry) {
	for _, root := range pluginRootCandidates(workingDir, name, entry) {
		if root == "" {
			continue
		}
		if stat, err := os.Stat(root); err != nil || !stat.IsDir() {
			continue
		}
		if info.DiskBytes == 0 {
			info.DiskBytes = dirSize(root)
		}
		if pkg, _, err := registry.LoadPackageManifest(root); err == nil && pkg != nil {
			if pkg.Version != "" {
				info.Version = pkg.Version
			}
			if pkg.Description != "" {
				info.Description = pkg.Description
			}
			if pkg.Type != "" {
				info.Type = string(pkg.Type)
			}
			if pkg.Source != "" {
				info.RegistrySource = pkg.Source
			}
			if len(pkg.Keywords) > 0 {
				info.Keywords = append([]string(nil), pkg.Keywords...)
			}
			if manifest, err := json.Marshal(pkg); err == nil {
				info.Manifest = strPtr(string(manifest))
			}
		}
		info.Skills = uniqueStrings(append(info.Skills, scanNamedChildren(root, []string{"skills", ".agents/skills", ".claude/skills"}, "SKILL.md")...))
		info.Prompts = uniqueStrings(append(info.Prompts, scanNamedChildren(root, []string{"prompts", "library/prompts"}, "")...))
		info.Templates = uniqueStrings(append(info.Templates, scanNamedChildren(root, []string{"templates", "library/templates"}, "")...))
	}
}

func pluginRootCandidates(workingDir string, name string, entry registry.InstalledEntry) []string {
	var roots []string
	if workingDir != "" {
		roots = append(roots, filepath.Join(workingDir, ".ddx", "plugins", name))
	}
	roots = append(roots, registry.ExpandHome(filepath.Join("~", ".ddx", "plugins", name)))
	for _, f := range entry.Files {
		expanded := registry.ExpandHome(f)
		if expanded == "" {
			continue
		}
		roots = append(roots, pluginRootFromRecordedFile(expanded, name))
	}
	return uniqueStrings(roots)
}

func pluginRootFromRecordedFile(path string, name string) string {
	cleaned := filepath.Clean(path)
	if filepath.Base(cleaned) == name {
		return cleaned
	}
	for dir := cleaned; dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		if filepath.Base(dir) == name && filepath.Base(filepath.Dir(dir)) == "plugins" {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
	}
	return cleaned
}

func dirSize(root string) int {
	var total int64
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	const maxInt32 = int64(1<<31 - 1)
	if total > maxInt32 {
		return int(maxInt32)
	}
	return int(total)
}

func scanNamedChildren(root string, relDirs []string, marker string) []string {
	var out []string
	for _, rel := range relDirs {
		base := filepath.Join(root, filepath.FromSlash(rel))
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if marker != "" {
				if !entry.IsDir() {
					continue
				}
				if _, err := os.Stat(filepath.Join(base, entry.Name(), marker)); err != nil {
					continue
				}
			}
			out = append(out, entry.Name())
		}
	}
	return out
}

type efficacySnapshot struct {
	rows     []*EfficacyRow
	attempts map[string][]*EfficacyAttempt
}

type sessionEfficacyCacheEntry struct {
	fingerprint string
	snapshot    efficacySnapshot
}

var sessionEfficacyCache struct {
	sync.Mutex
	byQuery map[string]sessionEfficacyCacheEntry
}

func (r *queryResolver) efficacySnapshot(since, until, projectID *string) (efficacySnapshot, error) {
	projectRoot := r.WorkingDir
	if projectID != nil && strings.TrimSpace(*projectID) != "" {
		projectRoot = r.projectRoot(strings.TrimSpace(*projectID))
	}
	startedAfter, err := parseOptionalEfficacyTime("since", since)
	if err != nil {
		return efficacySnapshot{}, err
	}
	startedBefore, err := parseOptionalEfficacyTime("until", until)
	if err != nil {
		return efficacySnapshot{}, err
	}
	logDir := agent.SessionLogDirForWorkDir(projectRoot)
	query := agent.SessionIndexQuery{
		StartedAfter:  startedAfter,
		StartedBefore: startedBefore,
	}
	cacheKey := sessionEfficacyCacheKey(logDir, startedAfter, startedBefore)
	fingerprint, err := sessionEfficacyIndexFingerprint(logDir, query)
	if err != nil {
		return efficacySnapshot{}, err
	}
	sessionEfficacyCache.Lock()
	if sessionEfficacyCache.byQuery == nil {
		sessionEfficacyCache.byQuery = map[string]sessionEfficacyCacheEntry{}
	}
	if cached, ok := sessionEfficacyCache.byQuery[cacheKey]; ok && cached.fingerprint == fingerprint {
		sessionEfficacyCache.Unlock()
		return cached.snapshot, nil
	}
	sessionEfficacyCache.Unlock()

	entries, err := agent.ReadSessionIndex(logDir, query)
	if err != nil {
		return efficacySnapshot{}, err
	}
	snap := buildSessionIndexEfficacySnapshot(entries)
	sessionEfficacyCache.Lock()
	sessionEfficacyCache.byQuery[cacheKey] = sessionEfficacyCacheEntry{fingerprint: fingerprint, snapshot: snap}
	sessionEfficacyCache.Unlock()
	return snap, nil
}

func sessionEfficacyCacheKey(logDir string, startedAfter, startedBefore *time.Time) string {
	return strings.Join([]string{logDir, timeKey(startedAfter), timeKey(startedBefore)}, "|")
}

func timeKey(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func sessionEfficacyIndexFingerprint(logDir string, query agent.SessionIndexQuery) (string, error) {
	files, err := agent.SessionIndexShardFiles(logDir, query)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(files))
	for _, file := range files {
		info, err := os.Stat(file)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		parts = append(parts, fmt.Sprintf("%s:%d:%d", file, info.Size(), info.ModTime().UnixNano()))
	}
	return strings.Join(parts, "|"), nil
}

func parseOptionalEfficacyTime(name string, raw *string) (*time.Time, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	value := strings.TrimSpace(*raw)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			utc := parsed.UTC()
			return &utc, nil
		}
	}
	return nil, fmt.Errorf("%s must be RFC3339 or YYYY-MM-DD, got %q", name, value)
}

type sessionEfficacyAttempt struct {
	RowKey       string
	BeadID       string
	Outcome      string
	Harness      string
	Provider     string
	Model        string
	InputTokens  int
	OutputTokens int
	DurationMs   int
	CostUsd      *float64
	EvidencePath string
	CreatedAt    time.Time
}

func buildSessionIndexEfficacySnapshot(entries []agent.SessionIndexEntry) efficacySnapshot {
	grouped := map[string][]sessionEfficacyAttempt{}
	for _, entry := range entries {
		attempt := efficacyAttemptFromSession(entry)
		if attempt.RowKey == "" {
			continue
		}
		grouped[attempt.RowKey] = append(grouped[attempt.RowKey], attempt)
	}
	rows := make([]*EfficacyRow, 0, len(grouped))
	attemptDetails := map[string][]*EfficacyAttempt{}
	for rowKey, group := range grouped {
		sort.Slice(group, func(i, j int) bool { return group[i].CreatedAt.After(group[j].CreatedAt) })
		attemptDetails[rowKey] = make([]*EfficacyAttempt, 0, len(group))
		var inputTokens, outputTokens, durations []int
		var costs []float64
		var successes int
		for _, attempt := range group {
			if attempt.Outcome == "succeeded" {
				successes++
			}
			inputTokens = append(inputTokens, attempt.InputTokens)
			outputTokens = append(outputTokens, attempt.OutputTokens)
			durations = append(durations, attempt.DurationMs)
			if attempt.CostUsd != nil {
				costs = append(costs, *attempt.CostUsd)
			}
			attemptDetails[rowKey] = append(attemptDetails[rowKey], &EfficacyAttempt{
				BeadID:            attempt.BeadID,
				Outcome:           attempt.Outcome,
				DurationMs:        attempt.DurationMs,
				CostUsd:           attempt.CostUsd,
				EvidenceBundleURL: attempt.EvidencePath,
			})
		}
		parts := strings.Split(rowKey, "|")
		row := &EfficacyRow{
			RowKey:             rowKey,
			Harness:            parts[0],
			Provider:           parts[1],
			Model:              parts[2],
			Attempts:           len(group),
			Successes:          successes,
			SuccessRate:        float64(successes) / float64(len(group)),
			MedianInputTokens:  medianInt(inputTokens),
			MedianOutputTokens: medianInt(outputTokens),
			MedianDurationMs:   medianInt(durations),
			MedianCostUsd:      medianFloatPtr(costs),
		}
		if row.SuccessRate < efficacyWarningFloor {
			row.Warning = &EfficacyWarning{Kind: "below-adaptive-floor", Threshold: floatPtr(efficacyWarningFloor)}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].SuccessRate != rows[j].SuccessRate {
			return rows[i].SuccessRate > rows[j].SuccessRate
		}
		return rows[i].RowKey < rows[j].RowKey
	})

	return efficacySnapshot{rows: rows, attempts: attemptDetails}
}

func efficacyAttemptFromSession(entry agent.SessionIndexEntry) sessionEfficacyAttempt {
	model := firstNonEmpty(entry.Model, "unknown")
	provider := firstNonEmpty(entry.Provider, providerFromModel(model), entry.Harness, "unknown")
	harness := firstNonEmpty(entry.Harness, provider, "unknown")
	cost := efficacyCost(entry)
	outcome := efficacyOutcome(entry)
	return sessionEfficacyAttempt{
		RowKey:       strings.Join([]string{harness, provider, model}, "|"),
		BeadID:       entry.BeadID,
		Outcome:      outcome,
		Harness:      harness,
		Provider:     provider,
		Model:        model,
		InputTokens:  entry.InputTokens,
		OutputTokens: entry.OutputTokens,
		DurationMs:   entry.DurationMS,
		CostUsd:      cost,
		EvidencePath: efficacyEvidencePath(entry),
		CreatedAt:    entry.StartedAt,
	}
}

func providerFromModel(model string) string {
	if before, _, ok := strings.Cut(model, "/"); ok && strings.TrimSpace(before) != "" {
		return before
	}
	return ""
}

func efficacyCost(entry agent.SessionIndexEntry) *float64 {
	if !entry.CostPresent && entry.CostUSD == 0 {
		return nil
	}
	cost := entry.CostUSD
	return &cost
}

func efficacyOutcome(entry agent.SessionIndexEntry) string {
	switch strings.ToLower(strings.TrimSpace(entry.Outcome)) {
	case "success", "succeeded", "ok", "passed":
		return "succeeded"
	case "failure", "failed", "error", "timed_out", "timeout", "cancelled", "stalled":
		return "failed"
	}
	if entry.ExitCode == 0 && entry.Detail == "" {
		return "succeeded"
	}
	return "failed"
}

func efficacyEvidencePath(entry agent.SessionIndexEntry) string {
	if entry.BundlePath != "" {
		return filepath.ToSlash(entry.BundlePath)
	}
	if entry.NativeLogRef != "" {
		return filepath.ToSlash(entry.NativeLogRef)
	}
	if entry.ID != "" {
		return "#session:" + entry.ID
	}
	return ""
}

type paletteMatch struct {
	kind    string
	id      string
	title   string
	score   int
	sortKey string
}

func collectPaletteMatches(query string, workingDir string) []paletteMatch {
	var out []paletteMatch
	if graph, err := docgraph.BuildGraphWithConfig(workingDir); err == nil {
		for _, doc := range graph.AllNodesForOutput() {
			if score, ok := paletteScore(query, doc.Path, doc.Title); ok {
				out = append(out, paletteMatch{
					kind:    "document",
					id:      filepath.ToSlash(doc.Path),
					title:   doc.Title,
					score:   score,
					sortKey: "document:" + doc.Path,
				})
			}
		}
	}

	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	if beads, err := store.ReadAll(); err == nil {
		for _, b := range beads {
			if score, ok := paletteScore(query, b.ID, b.Title); ok {
				out = append(out, paletteMatch{kind: "bead", id: b.ID, title: b.Title, score: score, sortKey: "bead:" + b.ID})
			}
		}
	}

	for _, action := range []struct{ id, label string }{
		{"drain-queue", "Drain queue"},
		{"realign-specs", "Re-align specs"},
		{"run-checks", "Run checks"},
	} {
		if score, ok := paletteScore(query, action.id, action.label); ok {
			out = append(out, paletteMatch{kind: "action", id: action.id, title: action.label, score: score, sortKey: "action:" + action.id})
		}
	}
	for _, nav := range []struct{ route, title string }{
		{"/beads", "Beads"},
		{"/documents", "Documents"},
		{"/graph", "Graph"},
		{"/workers", "Workers"},
		{"/personas", "Personas"},
		{"/plugins", "Plugins"},
		{"/efficacy", "Efficacy"},
	} {
		if score, ok := paletteScore(query, nav.route, nav.title); ok {
			out = append(out, paletteMatch{kind: "nav", id: nav.route, title: nav.title, score: score, sortKey: "nav:" + nav.route})
		}
	}
	return out
}

func paletteScore(query string, fields ...string) (int, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return 0, false
	}
	terms := strings.Fields(q)
	best := 1000
	for _, field := range fields {
		f := strings.ToLower(field)
		if strings.HasPrefix(f, q) {
			best = min(best, 0)
			continue
		}
		for _, part := range strings.FieldsFunc(f, func(r rune) bool {
			return r == '/' || r == '-' || r == '_' || r == '.' || r == ' ' || r == ':'
		}) {
			if strings.HasPrefix(part, q) {
				best = min(best, 1)
			}
		}
		allTerms := true
		for _, term := range terms {
			if !strings.Contains(f, term) {
				allTerms = false
				break
			}
		}
		if allTerms {
			best = min(best, 2)
		}
	}
	if best == 1000 {
		return 0, false
	}
	return best, true
}

func medianInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	sort.Ints(values)
	return values[len(values)/2]
}

func medianFloatPtr(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sort.Float64s(values)
	v := values[len(values)/2]
	return &v
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func strPtr(v string) *string {
	return &v
}

func floatPtr(v float64) *float64 {
	return &v
}

func slug(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, "_", "-")
	v = strings.ReplaceAll(v, " ", "-")
	return v
}

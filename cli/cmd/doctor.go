package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/metaprompt"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/spf13/cobra"
)

// DiagnosticIssue represents a detected problem and its remediation
type DiagnosticIssue struct {
	Type        string
	Description string
	Remediation []string
	SystemInfo  map[string]string
}

// runDoctor implements the doctor command logic
func (f *CommandFactory) runDoctor(cmd *cobra.Command, args []string) error {
	verbose, _ := cmd.Flags().GetBool("verbose")
	fix, _ := cmd.Flags().GetBool("fix")

	fmt.Println("🩺 DDx Installation Diagnostics")
	fmt.Println("=====================================")
	fmt.Println()

	var issues []DiagnosticIssue
	allGood := true
	auditPlugins, _ := cmd.Flags().GetBool("plugins")

	// Check 1: DDX Binary Executable and Install Location
	fmt.Print("✓ Checking DDX Binary... ")
	executable, err := os.Executable()
	if err != nil {
		fmt.Println("❌ Cannot determine executable location")
		allGood = false
	} else {
		fmt.Printf("✅ DDX Binary Executable (%s)\n", executable)
		if locationIssues := checkBinaryInstallLocation(executable); len(locationIssues) > 0 {
			for _, issue := range locationIssues {
				fmt.Printf("   ⚠️  %s\n", issue.Description)
				for _, r := range issue.Remediation {
					fmt.Printf("   💡 %s\n", r)
				}
			}
			issues = append(issues, locationIssues...)
		}
	}

	// Check 2: PATH Configuration
	fmt.Print("✓ Checking PATH Configuration... ")
	if isInPath() {
		fmt.Println("✅ PATH Configuration")
	} else {
		fmt.Println("⚠️  DDX not found in PATH")

		// Check for problem simulation
		problemState := os.Getenv("DDX_PROBLEM_STATE")
		if problemState == "path_issue" || verbose {
			issues = append(issues, DiagnosticIssue{
				Type:        "path_configuration",
				Description: "DDX binary not accessible from PATH",
				Remediation: []string{
					"Run 'ddx setup path'",
					"Restart shell session",
					"Manually add to PATH",
				},
				SystemInfo: map[string]string{
					"shell": os.Getenv("SHELL"),
					"path":  os.Getenv("PATH"),
				},
			})
		}

		if !verbose {
			suggestPathFix()
		}
		// Not marking as failure since DDx is running
	}

	// Check 3: Configuration File
	fmt.Print("✓ Checking Configuration... ")
	if checkConfiguration() {
		fmt.Println("✅ Configuration Valid")
	} else {
		fmt.Println("⚠️  Configuration Issues (non-critical)")
	}

	// Check 4: Git Installation
	fmt.Print("✓ Checking Git... ")
	if checkGit() {
		fmt.Println("✅ Git Available")
	} else {
		fmt.Println("❌ Git Not Found")
		fmt.Println("   Git is required for DDX synchronization features")
		allGood = false
	}

	// Check 4b: Git Repo Health (core.bare / core.worktree corruption)
	fmt.Print("✓ Checking Git Repo Health... ")
	repoIssues := checkGitRepoHealth(f.WorkingDir, fix)
	if len(repoIssues) == 0 {
		fmt.Println("✅ Git Repo Clean")
	} else {
		if fix {
			fmt.Printf("🔧 Git Repo Issues Fixed (%d)\n", len(repoIssues))
		} else {
			fmt.Printf("⚠️  Git Repo Issues (%d) — re-run with --fix to remediate\n", len(repoIssues))
		}
		for _, issue := range repoIssues {
			fmt.Printf("   ⚠️  %s\n", issue.Description)
			for _, r := range issue.Remediation {
				fmt.Printf("   💡 %s\n", r)
			}
		}
		issues = append(issues, repoIssues...)
	}

	// Check 5: Network Connectivity
	fmt.Print("✓ Checking Network... ")
	if checkNetwork() {
		fmt.Println("✅ Network Connectivity")
	} else {
		fmt.Println("⚠️  Network Issues (optional)")

		// Check for problem simulation
		problemState := os.Getenv("DDX_PROBLEM_STATE")
		if problemState == "network_issue" || verbose {
			issues = append(issues, DiagnosticIssue{
				Type:        "network_connectivity",
				Description: "Unable to reach external repositories",
				Remediation: []string{
					"Check internet connection",
					"Verify proxy settings",
					"Try offline installation",
				},
				SystemInfo: map[string]string{
					"dns_server": "Check /etc/resolv.conf or network settings",
					"proxy":      os.Getenv("HTTP_PROXY"),
				},
			})
		}
	}

	// Check 7: Permissions
	fmt.Print("✓ Checking Permissions... ")
	problemState := os.Getenv("DDX_PROBLEM_STATE")
	if checkPermissions() && problemState != "permission_issue" {
		fmt.Println("✅ File Permissions")
	} else {
		fmt.Println("❌ Permission Issues")
		allGood = false

		// Add permission issue details for critical failures or verbose mode
		if problemState == "permission_issue" || verbose || !checkPermissions() {
			issues = append(issues, DiagnosticIssue{
				Type:        "file_permissions",
				Description: "Cannot create files in current directory",
				Remediation: []string{
					"Check directory permissions",
					"Try installing to different location",
					"Verify user has write access",
				},
				SystemInfo: map[string]string{
					"user":        os.Getenv("USER"),
					"working_dir": f.WorkingDir,
					"permissions": getDirectoryPermissions(f.WorkingDir),
				},
			})
		}
	}

	// Check 8: Library Path
	fmt.Print("✓ Checking Library Path... ")
	if checkLibraryPathFromWorkingDir(f.WorkingDir) {
		fmt.Println("✅ Library Path Accessible")
	} else {
		fmt.Println("⚠️  Library Path Issues (optional)")

		// Check for problem simulation
		problemState := os.Getenv("DDX_PROBLEM_STATE")
		if problemState == "library_path_issue" || verbose {
			issues = append(issues, DiagnosticIssue{
				Type:        "library_path_configuration",
				Description: "DDX library path not accessible or not configured",
				Remediation: []string{
					"Initialize DDX in your project with 'ddx init'",
					"Check .ddx.yml configuration file",
					"Verify library path exists and is readable",
					"Try setting DDX_LIBRARY_BASE_PATH environment variable",
					"Re-clone or update DDX library repository",
				},
				SystemInfo: map[string]string{
					"library_path": getLibraryPathInfo(f.WorkingDir),
					"config_file":  getConfigFileInfo(),
					"env_override": os.Getenv("DDX_LIBRARY_BASE_PATH"),
				},
			})
		}
	}

	// Check 9: Meta-Prompt Sync Status
	fmt.Print("✓ Checking Meta-Prompt Sync... ")
	if metaPromptCheck := checkMetaPromptSync(f.WorkingDir); metaPromptCheck == nil {
		fmt.Println("✅ Meta-Prompt In Sync")
	} else {
		fmt.Println("⚠️  Meta-Prompt Out of Sync")
		if verbose {
			issues = append(issues, DiagnosticIssue{
				Type:        "meta_prompt_sync",
				Description: metaPromptCheck.Error(),
				Remediation: []string{
					"Run 'ddx update' to sync meta-prompt",
				},
				SystemInfo: map[string]string{
					"claude_file": filepath.Join(f.WorkingDir, "CLAUDE.md"),
				},
			})
		}
	}

	// Check 10: Installed Package Launchers
	checkInstalledLaunchers(verbose)

	if auditPlugins {
		pluginIssues := checkInstalledPlugins(verbose)
		if len(pluginIssues) > 0 {
			allGood = false
			issues = append(issues, pluginIssues...)
		}
	}

	// Check 11: package.json locations — detect missing/stale node_modules.
	fmt.Print("✓ Checking package.json locations... ")
	pkgIssues := checkPackageJSONLocations(f.WorkingDir)
	if len(pkgIssues) == 0 {
		fmt.Println("✅ Node dependencies up to date")
	} else {
		fmt.Printf("⚠️  %d location(s) need 'bun install'\n", len(pkgIssues))
		for _, issue := range pkgIssues {
			fmt.Printf("   ⚠️  %s\n", issue.Description)
			for _, r := range issue.Remediation {
				fmt.Printf("   💡 %s\n", r)
			}
		}
		issues = append(issues, pkgIssues...)
	}

	// Check 13: Legacy symlinks under project skill directories (FEAT-015).
	// In the project-local model all skills are real files. Symlinks indicate
	// a pre-migration install; ddx update --force replaces them.
	if legacyDirs := legacySkillSymlinkDirs(f.WorkingDir); len(legacyDirs) > 0 {
		errOut := cmd.ErrOrStderr()
		for _, dir := range legacyDirs {
			fmt.Fprintf(errOut, "symlink detected under %s; pre-migration install detected\n", dir)
		}
		fmt.Fprintln(errOut, "run: ddx update --force")
		return fmt.Errorf("legacy skill symlinks detected")
	}

	fmt.Println()
	if allGood && len(issues) == 0 {
		fmt.Println("🎉 All critical checks passed! DDX is ready to use.")
	} else if allGood && len(issues) > 0 {
		fmt.Println("⚠️  Some non-critical issues detected. DDX is functional but may have limitations.")
		fmt.Println("💡 Run 'ddx doctor --help' for troubleshooting tips.")
	} else {
		fmt.Println("⚠️  Some issues detected. DDX may have limited functionality.")
		fmt.Println("💡 Run 'ddx doctor --help' for troubleshooting tips.")
	}

	// Generate detailed diagnostic report if verbose or issues detected
	if verbose || len(issues) > 0 {
		generateDiagnosticReport(issues, verbose, f.WorkingDir)
	}

	return nil
}

// legacySkillSymlinkDirs returns the relative names of skill directories
// (e.g. ".agents/skills", ".claude/skills") under workingDir that contain
// at least one symlink. A symlink indicates a pre-FEAT-015 install.
func legacySkillSymlinkDirs(workingDir string) []string {
	if workingDir == "" {
		return nil
	}
	var found []string
	for _, rel := range []string{".agents/skills", ".claude/skills"} {
		dir := filepath.Join(workingDir, rel)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 {
				found = append(found, rel)
				break
			}
		}
	}
	return found
}

// checkBinaryInstallLocation verifies the running binary is at the canonical install
// location ($HOME/.local/bin/ddx) and scans PATH for other ddx copies whose
// SHA-256 differs from the running binary.
func checkBinaryInstallLocation(executable string) []DiagnosticIssue {
	var issues []DiagnosticIssue

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return issues
	}

	canonicalPath := filepath.Join(homeDir, ".local", "bin", "ddx")

	// (2) Check whether running binary matches the canonical install location.
	resolvedExec, _ := filepath.EvalSymlinks(executable)
	if resolvedExec == "" {
		resolvedExec = executable
	}
	resolvedCanonical, _ := filepath.EvalSymlinks(canonicalPath)
	execMatchesCanonical := resolvedExec == canonicalPath ||
		(resolvedCanonical != "" && resolvedExec == resolvedCanonical)
	if !execMatchesCanonical {
		issues = append(issues, DiagnosticIssue{
			Type:        "binary_not_canonical",
			Description: fmt.Sprintf("Running binary (%s) is not at canonical install location (%s)", executable, canonicalPath),
			Remediation: []string{
				"Re-run install.sh to reinstall to the canonical location",
				fmt.Sprintf("Or ensure %s is earlier in your PATH", filepath.Dir(canonicalPath)),
			},
		})
	}

	// (3) Walk PATH and flag any ddx copy whose SHA-256 differs from the running binary.
	runningSHA, err := computeFileSHA256(executable)
	if err != nil {
		return issues
	}

	seen := make(map[string]bool)
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		candidate := filepath.Join(dir, "ddx")
		if seen[candidate] {
			continue
		}
		seen[candidate] = true

		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
			continue
		}

		candidateSHA, err := computeFileSHA256(candidate)
		if err != nil {
			continue
		}

		if candidateSHA == runningSHA {
			continue // same binary, not stale
		}

		issues = append(issues, DiagnosticIssue{
			Type:        "binary_sha_mismatch",
			Description: fmt.Sprintf("Stale ddx copy on PATH: %s (SHA-256 differs from running binary)", candidate),
			Remediation: []string{
				fmt.Sprintf("rm %s && cp %s %s", candidate, executable, candidate),
				"Or re-run install.sh to update all copies",
			},
		})
	}

	return issues
}

// computeFileSHA256 returns the hex-encoded SHA-256 digest of the named file.
func computeFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// isInPath checks if DDX is accessible from PATH
func isInPath() bool {
	_, err := exec.LookPath("ddx")
	return err == nil
}

// checkConfiguration validates the DDX configuration
func checkConfiguration() bool {
	_, err := config.Load()
	return err == nil
}

// checkGit verifies git is available
func checkGit() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// checkNetwork tests basic network connectivity
func checkNetwork() bool {
	// Simple check - try to resolve a hostname
	_, err := exec.Command("ping", "-c", "1", "github.com").Output()
	return err == nil
}

// checkPermissions verifies file system permissions
func checkPermissions() bool {
	// Check if we can create files in the current directory
	tempFile := "ddx-test-permissions"
	file, err := os.Create(tempFile)
	if err != nil {
		return false
	}
	_ = file.Close()
	_ = os.Remove(tempFile)
	return true
}

// checkLibraryPath verifies library path is accessible
func checkLibraryPathFromWorkingDir(workingDir string) bool {
	cfg, err := config.LoadWithWorkingDir(workingDir)
	if err != nil {
		return false
	}

	if cfg.Library == nil || cfg.Library.Path == "" {
		return false
	}

	// Resolve library path relative to working directory
	libPath := cfg.Library.Path
	if !filepath.IsAbs(libPath) {
		libPath = filepath.Join(workingDir, libPath)
	}

	_, err = os.Stat(libPath)
	return err == nil
}

// suggestPathFix provides suggestions for PATH configuration
func suggestPathFix() {
	fmt.Println("   💡 To add DDX to your PATH:")

	homeDir, _ := os.UserHomeDir()

	switch runtime.GOOS {
	case "windows":
		binPath := filepath.Join(homeDir, "bin")
		fmt.Printf("   Add %s to your PATH environment variable\n", binPath)
	default:
		binPath := filepath.Join(homeDir, ".local", "bin")
		fmt.Printf("   Add 'export PATH=\"%s:$PATH\"' to your shell profile\n", binPath)
	}
}

// generateDiagnosticReport creates a detailed diagnostic report
func generateDiagnosticReport(issues []DiagnosticIssue, verbose bool, workingDir string) {
	if len(issues) == 0 && !verbose {
		return
	}

	fmt.Println()
	fmt.Println("📊 DETAILED DIAGNOSTIC REPORT")
	fmt.Println("========================================")

	if verbose {
		fmt.Println()
		fmt.Println("🔍 System Information:")
		fmt.Printf("  OS: %s\n", runtime.GOOS)
		fmt.Printf("  Architecture: %s\n", runtime.GOARCH)
		fmt.Printf("  Go Runtime: %s\n", runtime.Version())
		fmt.Printf("  Working Directory: %s\n", workingDir)
		if executable, err := os.Executable(); err == nil {
			fmt.Printf("  DDX Binary: %s\n", executable)
		}
	}

	if len(issues) > 0 {
		fmt.Printf("\n🛠️  DETECTED ISSUES (%d):\n", len(issues))
		fmt.Println()

		for i, issue := range issues {
			fmt.Printf("Issue #%d: %s\n", i+1, issue.Type)
			fmt.Printf("  Description: %s\n", issue.Description)
			fmt.Println("  Remediation Steps:")
			for j, step := range issue.Remediation {
				fmt.Printf("    %d. %s\n", j+1, step)
			}

			if verbose && len(issue.SystemInfo) > 0 {
				fmt.Println("  System Information:")
				for key, value := range issue.SystemInfo {
					if value != "" {
						fmt.Printf("    %s: %s\n", key, value)
					}
				}
			}
			fmt.Println()
		}
	}

	if verbose {
		fmt.Println("💡 Additional Troubleshooting Tips:")
		fmt.Println("  • Run 'ddx doctor' periodically to check system health")
		fmt.Println("  • Use 'ddx doctor --verbose' for detailed diagnostics")
		fmt.Println("  • Check DDX documentation at https://github.com/DocumentDrivenDX/ddx")
		fmt.Println("  • Report issues at https://github.com/DocumentDrivenDX/ddx/issues")
	}
}

// getDirectoryPermissions returns permission information for the given directory
func getDirectoryPermissions(workingDir string) string {
	if info, err := os.Stat(workingDir); err == nil {
		return info.Mode().String()
	}
	return "unknown"
}

// getLibraryPathInfo returns information about the DDX library path
func getLibraryPathInfo(workingDir string) string {
	if cfg, err := config.LoadWithWorkingDir(workingDir); err == nil && cfg.Library != nil && cfg.Library.Path != "" {
		libPath := cfg.Library.Path
		if !filepath.IsAbs(libPath) {
			libPath = filepath.Join(workingDir, libPath)
		}
		return libPath
	}
	return "not configured"
}

// getConfigFileInfo returns information about the DDX configuration file
func getConfigFileInfo() string {
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, ".ddx.yml")
	if _, err := os.Stat(configPath); err == nil {
		return configPath
	}

	// Check current directory
	if _, err := os.Stat(".ddx.yml"); err == nil {
		return "./.ddx.yml"
	}

	return "not found"
}

// checkInstalledLaunchers checks whether installed packages with Scripts mappings
// have a working launcher in the target path (e.g. ~/.local/bin/helix).
func checkInstalledLaunchers(verbose bool) {
	state, err := registry.LoadState()
	if err != nil || len(state.Installed) == 0 {
		return
	}

	reg := registry.BuiltinRegistry()
	for _, entry := range state.Installed {
		pkg, err := reg.Find(entry.Name)
		if err != nil || pkg.Install.Scripts == nil {
			continue
		}

		dst := registry.ExpandHome(pkg.Install.Scripts.Target)
		name := filepath.Base(dst)
		fmt.Printf("✓ Checking %s launcher... ", name)

		li, err := os.Lstat(dst)
		if os.IsNotExist(err) {
			fmt.Printf("❌ MISSING (%s not found, run: ddx install %s)\n", dst, pkg.Name)
			continue
		}
		if err != nil {
			fmt.Printf("❌ error: %v\n", err)
			continue
		}

		if li.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(dst)
			if verbose {
				fmt.Printf("✅ OK (developer symlink → %s)\n", target)
			} else {
				fmt.Println("✅ OK (developer symlink)")
			}
			continue
		}

		// Regular file — check it's executable.
		if li.Mode()&0111 != 0 {
			fmt.Printf("✅ OK (%s)\n", dst)
		} else {
			fmt.Printf("⚠️  not executable (run: chmod +x %s)\n", dst)
		}
	}
}

// checkInstalledPlugins audits installed plugin roots and manifests.
func checkInstalledPlugins(verbose bool) []DiagnosticIssue {
	state, err := registry.LoadState()
	if err != nil || len(state.Installed) == 0 {
		return nil
	}

	reg := registry.BuiltinRegistry()
	var issues []DiagnosticIssue

	for _, entry := range state.Installed {
		fallback, _ := reg.Find(entry.Name)
		entryType := entry.Type
		if entryType == "" && fallback != nil {
			entryType = fallback.Type
		}
		if entryType == "" && !looksLikePluginInstall(entry) {
			continue
		}
		switch entryType {
		case registry.PackageTypePlugin, registry.PackageTypeWorkflow:
		case "":
		default:
			continue
		}
		for _, issue := range registry.AuditInstalledEntry(entry, fallback) {
			diag := DiagnosticIssue{
				Type:        "plugin_validation",
				Description: issue.Error(),
				Remediation: []string{
					"Check package.yaml, skill directories, and installed symlinks for the path shown above",
					"Reinstall the package after fixing the source tree or manifest",
				},
				SystemInfo: map[string]string{
					"plugin": entry.Name,
				},
			}
			if verbose {
				diag.SystemInfo["source"] = entry.Source
			}
			issues = append(issues, diag)
		}
	}

	return issues
}

func looksLikePluginInstall(entry registry.InstalledEntry) bool {
	candidates := make([]string, 0, 2+len(entry.Files))
	if root := installedEntryRootCandidate(entry); root != "" {
		candidates = append(candidates, root)
	}
	candidates = append(candidates, entry.Files...)

	for _, candidate := range candidates {
		path := registry.ExpandHome(strings.TrimSpace(candidate))
		if path == "" {
			continue
		}

		info, err := os.Lstat(path)
		if err != nil {
			continue
		}

		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}

	return false
}

func installedEntryRootCandidate(entry registry.InstalledEntry) string {
	if len(entry.Files) > 0 && strings.TrimSpace(entry.Files[0]) != "" {
		return entry.Files[0]
	}
	return strings.TrimSpace(entry.Source)
}

// checkGitRepoHealth detects git-repo corruption from prior ddx incidents:
//  1. core.bare=true on a repo that has a working tree (not actually bare)
//  2. Stray core.worktree that does not match the actual toplevel
//  3. extensions.worktreeConfig not enabled (warning only)
//
// If fix is true, conditions (1) and (2) are remediated with `git config --unset`.
// (3) is always a warning — never auto-fixed, since enabling per-worktree config
// is a user-facing choice.
func checkGitRepoHealth(workingDir string, fix bool) []DiagnosticIssue {
	var issues []DiagnosticIssue
	ctx := context.Background()

	// Bail out quietly if this directory isn't a git repo at all. We can't rely
	// on `--is-inside-work-tree` here because the corruption we're looking for
	// (core.bare=true on a repo that has a working tree) causes git to report
	// "false" for that query. Instead, probe `--git-dir`, which succeeds on both
	// bare and non-bare repos.
	gitDirProbe := gitpkg.Command(ctx, workingDir, "rev-parse", "--git-dir")
	gdOut, err := gitDirProbe.Output()
	if err != nil {
		return nil
	}
	gitDirRaw := strings.TrimSpace(string(gdOut))
	if gitDirRaw == "" {
		return nil
	}

	// A "real" working tree on disk: either .git is a directory/file at
	// workingDir, or git reports a separate work-tree via --show-toplevel.
	// If neither holds, this is a genuinely bare repo → skip the checks.
	hasWorkTreeOnDisk := false
	if _, statErr := os.Stat(filepath.Join(workingDir, ".git")); statErr == nil {
		hasWorkTreeOnDisk = true
	}
	if !hasWorkTreeOnDisk {
		// Maybe we're inside a linked worktree where git treats us normally.
		inWT := gitpkg.Command(ctx, workingDir, "rev-parse", "--is-inside-work-tree")
		if b, err := inWT.Output(); err == nil && strings.TrimSpace(string(b)) == "true" {
			hasWorkTreeOnDisk = true
		}
	}
	if !hasWorkTreeOnDisk {
		return nil
	}

	gitConfigValue := func(key string) (string, bool) {
		c := gitpkg.Command(ctx, workingDir, "config", "--local", "--get", key)
		b, err := c.Output()
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(string(b)), true
	}
	gitConfigUnset := func(key string) error {
		c := gitpkg.Command(ctx, workingDir, "config", "--local", "--unset", key)
		return c.Run()
	}

	// (1) core.bare=true on a repo with a working tree → corruption.
	if val, ok := gitConfigValue("core.bare"); ok && val == "true" {
		desc := "core.bare=true is set on a repository that has a working tree (not actually bare)"
		rem := []string{"git config --unset core.bare"}
		if fix {
			if err := gitConfigUnset("core.bare"); err == nil {
				desc += " — removed"
			} else {
				desc += fmt.Sprintf(" — unset failed: %v", err)
			}
		}
		issues = append(issues, DiagnosticIssue{
			Type:        "git_core_bare_corruption",
			Description: desc,
			Remediation: rem,
		})
	}

	// (2) Stray core.worktree that doesn't match the directory we're running
	// against. Compare to workingDir directly — asking git for --show-toplevel
	// would just echo back core.worktree.
	if worktreeVal, ok := gitConfigValue("core.worktree"); ok && worktreeVal != "" {
		actual := workingDir

		cmpVal := worktreeVal
		if !filepath.IsAbs(cmpVal) {
			cmpVal = filepath.Clean(filepath.Join(gitDirRaw, worktreeVal))
		}
		resolvedCmp, _ := filepath.EvalSymlinks(cmpVal)
		if resolvedCmp == "" {
			resolvedCmp = cmpVal
		}
		resolvedActual, _ := filepath.EvalSymlinks(actual)
		if resolvedActual == "" {
			resolvedActual = actual
		}

		if actual != "" && resolvedCmp != resolvedActual {
			desc := fmt.Sprintf("core.worktree=%q does not match actual worktree %q", worktreeVal, actual)
			rem := []string{"git config --unset core.worktree"}
			if fix {
				if err := gitConfigUnset("core.worktree"); err == nil {
					desc += " — removed"
				} else {
					desc += fmt.Sprintf(" — unset failed: %v", err)
				}
			}
			issues = append(issues, DiagnosticIssue{
				Type:        "git_stray_core_worktree",
				Description: desc,
				Remediation: rem,
			})
		}
	}

	// (3) extensions.worktreeConfig not enabled → warning only.
	val, ok := gitConfigValue("extensions.worktreeConfig")
	if !ok || val != "true" {
		issues = append(issues, DiagnosticIssue{
			Type:        "git_worktree_config_disabled",
			Description: "extensions.worktreeConfig is not enabled; per-worktree config changes can corrupt the shared .git/config",
			Remediation: []string{
				"git config extensions.worktreeConfig true",
			},
		})
	}

	return issues
}

// packageJSONCheck holds the result of scanning one package.json location.
type packageJSONCheck struct {
	Dir      string // relative path from project root (e.g. "website")
	HasPkg   bool   // package.json exists
	NeedsBun bool   // node_modules missing or stale vs bun.lock
	Reason   string // "missing" | "stale"
}

// knownPackageJSONDirs are the subdirectories (relative to workingDir) that
// may contain a package.json. "" means the repo root itself.
var knownPackageJSONDirs = []string{"", "website", "cli/internal/server/frontend"}

// checkPackageJSONLocations scans the known package.json locations under
// workingDir and returns a DiagnosticIssue for each location where
// node_modules/ is absent or older than bun.lock.
func checkPackageJSONLocations(workingDir string) []DiagnosticIssue {
	var issues []DiagnosticIssue

	for _, rel := range knownPackageJSONDirs {
		var dir string
		if rel == "" {
			dir = workingDir
		} else {
			dir = filepath.Join(workingDir, filepath.FromSlash(rel))
		}

		pkgPath := filepath.Join(dir, "package.json")
		if _, err := os.Stat(pkgPath); err != nil {
			continue // no package.json here
		}

		label := rel
		if label == "" {
			label = "(repo root)"
		}

		nmPath := filepath.Join(dir, "node_modules")
		nmInfo, nmErr := os.Stat(nmPath)
		if nmErr != nil {
			// node_modules missing entirely
			issues = append(issues, DiagnosticIssue{
				Type:        "node_modules_missing",
				Description: fmt.Sprintf("node_modules/ missing in %s", label),
				Remediation: []string{
					fmt.Sprintf("cd %s && bun install", dir),
				},
				SystemInfo: map[string]string{"package_json": pkgPath},
			})
			continue
		}

		// Check if bun.lock is newer than node_modules
		lockPath := filepath.Join(dir, "bun.lock")
		lockInfo, lockErr := os.Stat(lockPath)
		if lockErr == nil && lockInfo.ModTime().After(nmInfo.ModTime()) {
			issues = append(issues, DiagnosticIssue{
				Type:        "node_modules_stale",
				Description: fmt.Sprintf("node_modules/ in %s is older than bun.lock (run bun install)", label),
				Remediation: []string{
					fmt.Sprintf("cd %s && bun install", dir),
				},
				SystemInfo: map[string]string{"package_json": pkgPath},
			})
		}
	}

	return issues
}

// checkMetaPromptSync checks if the meta-prompt in CLAUDE.md is in sync with library
func checkMetaPromptSync(workingDir string) error {
	cfg, err := config.LoadWithWorkingDir(workingDir)
	if err != nil {
		// Can't load config - skip check
		return nil
	}

	promptPath := cfg.GetMetaPrompt()
	if promptPath == "" {
		// Meta-prompt disabled - not an issue
		return nil
	}

	injector := metaprompt.NewMetaPromptInjectorWithPaths(
		"CLAUDE.md",
		cfg.Library.Path,
		workingDir,
	)

	inSync, err := injector.IsInSync()
	if err != nil {
		// Could not check (file missing, etc) - not a critical issue
		return nil
	}

	if !inSync {
		return fmt.Errorf("meta-prompt is out of sync with library")
	}

	return nil
}

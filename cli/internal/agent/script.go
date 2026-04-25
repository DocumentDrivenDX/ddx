package agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

// runScriptFn executes a directive file against opts.WorkDir using real filesystem
// and git operations. The directive file path is taken from opts.Model (if it is
// a readable file) or from opts.PromptFile as a fallback.
func runScriptFn(r *Runner, opts RunOptions) (*Result, error) {
	start := time.Now()

	// Resolve directive file: opts.Model first, then opts.PromptFile.
	directivePath := opts.Model
	if directivePath == "" || !isReadableFile(directivePath) {
		if opts.PromptFile != "" {
			directivePath = opts.PromptFile
		}
	}
	if directivePath == "" {
		return nil, fmt.Errorf("script harness: no directive file (set --model or --prompt-file)")
	}

	data, err := os.ReadFile(directivePath)
	if err != nil {
		return nil, fmt.Errorf("script harness: read directive file %s: %w", directivePath, err)
	}

	workDir := opts.WorkDir
	if workDir == "" {
		workDir = "."
	}

	var outputLines []string
	exitCode := 0
	var execErr error

	directives := parseDirectives(string(data))

	// Build env interpolation map from Correlation.
	envMap := map[string]string{
		"DDX_BEAD_ID":         "",
		"DDX_ATTEMPT_ID":      "",
		"DDX_WORKER_ID":       "",
		"DDX_SESSION_LOG_DIR": opts.SessionLogDir,
	}
	if opts.Correlation != nil {
		if v, ok := opts.Correlation["bead_id"]; ok {
			envMap["DDX_BEAD_ID"] = v
		}
		if v, ok := opts.Correlation["attempt_id"]; ok {
			envMap["DDX_ATTEMPT_ID"] = v
		}
		if v, ok := opts.Correlation["worker_id"]; ok {
			envMap["DDX_WORKER_ID"] = v
		}
	}
	expand := func(s string) string {
		return os.Expand(s, func(key string) string {
			return envMap[key]
		})
	}

	for i, d := range directives {
		verb := d[0]
		args := d[1:]

		switch verb {
		case "no-op":
			outputLines = append(outputLines, "no-op")

		case "set-exit":
			if len(args) < 1 {
				execErr = fmt.Errorf("script harness: set-exit requires an argument")
				goto done
			}
			n, err := strconv.Atoi(args[0])
			if err != nil {
				execErr = fmt.Errorf("script harness: set-exit: invalid code %q", args[0])
				goto done
			}
			exitCode = n

		case "sleep-ms":
			if len(args) < 1 {
				execErr = fmt.Errorf("script harness: sleep-ms requires an argument")
				goto done
			}
			n, err := strconv.Atoi(args[0])
			if err != nil {
				execErr = fmt.Errorf("script harness: sleep-ms: invalid duration %q", args[0])
				goto done
			}
			time.Sleep(time.Duration(n) * time.Millisecond)

		case "fail-during":
			if len(args) < 1 {
				execErr = fmt.Errorf("script harness: fail-during requires an argument")
				goto done
			}
			n, err := strconv.Atoi(args[0])
			if err != nil {
				execErr = fmt.Errorf("script harness: fail-during: invalid index %q", args[0])
				goto done
			}
			if i == n {
				execErr = fmt.Errorf("script harness: synthetic failure at directive %d", i)
				goto done
			}

		case "append-line":
			if len(args) < 2 {
				execErr = fmt.Errorf("script harness: append-line requires path and text")
				goto done
			}
			path, err := resolvePath(workDir, expand(args[0]))
			if err != nil {
				execErr = err
				goto done
			}
			text := expand(strings.Join(args[1:], " "))
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				execErr = fmt.Errorf("script harness: append-line mkdir: %w", err)
				goto done
			}
			f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				execErr = fmt.Errorf("script harness: append-line open: %w", err)
				goto done
			}
			_, werr := fmt.Fprintln(f, text)
			f.Close()
			if werr != nil {
				execErr = fmt.Errorf("script harness: append-line write: %w", werr)
				goto done
			}
			outputLines = append(outputLines, fmt.Sprintf("append-line %s", args[0]))

		case "create-file":
			if len(args) < 2 {
				execErr = fmt.Errorf("script harness: create-file requires path and content")
				goto done
			}
			path, err := resolvePath(workDir, expand(args[0]))
			if err != nil {
				execErr = err
				goto done
			}
			content := expand(strings.Join(args[1:], " "))
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				execErr = fmt.Errorf("script harness: create-file mkdir: %w", err)
				goto done
			}
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				execErr = fmt.Errorf("script harness: create-file write: %w", err)
				goto done
			}
			outputLines = append(outputLines, fmt.Sprintf("create-file %s", args[0]))

		case "delete-file":
			if len(args) < 1 {
				execErr = fmt.Errorf("script harness: delete-file requires a path")
				goto done
			}
			path, err := resolvePath(workDir, expand(args[0]))
			if err != nil {
				execErr = err
				goto done
			}
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				execErr = fmt.Errorf("script harness: delete-file: %w", err)
				goto done
			}
			outputLines = append(outputLines, fmt.Sprintf("delete-file %s", args[0]))

		case "modify-line":
			if len(args) < 3 {
				execErr = fmt.Errorf("script harness: modify-line requires path, regex, and replacement")
				goto done
			}
			path, err := resolvePath(workDir, expand(args[0]))
			if err != nil {
				execErr = err
				goto done
			}
			pattern := expand(args[1])
			repl := expand(strings.Join(args[2:], " "))
			re, err := regexp.Compile(pattern)
			if err != nil {
				execErr = fmt.Errorf("script harness: modify-line: bad regex %q: %w", pattern, err)
				goto done
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				execErr = fmt.Errorf("script harness: modify-line read: %w", err)
				goto done
			}
			lines := strings.Split(string(raw), "\n")
			for li, line := range lines {
				if re.MatchString(line) {
					lines[li] = re.ReplaceAllString(line, repl)
					break
				}
			}
			if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
				execErr = fmt.Errorf("script harness: modify-line write: %w", err)
				goto done
			}
			outputLines = append(outputLines, fmt.Sprintf("modify-line %s", args[0]))

		case "run":
			if len(args) < 1 {
				execErr = fmt.Errorf("script harness: run requires a shell command")
				goto done
			}
			shell := expand(strings.Join(args, " "))
			cmd := exec.Command("sh", "-c", shell)
			cmd.Dir = workDir
			cmd.Env = scrubbedGitEnvScript()
			out, err := cmd.CombinedOutput()
			outputLines = append(outputLines, strings.TrimRight(string(out), "\n"))
			if err != nil {
				execErr = fmt.Errorf("script harness: run %q: %w", shell, err)
				goto done
			}

		case "commit":
			if len(args) < 1 {
				execErr = fmt.Errorf("script harness: commit requires a message")
				goto done
			}
			msg := expand(strings.Join(args, " "))
			if err := gitCommitAll(workDir, msg); err != nil {
				execErr = fmt.Errorf("script harness: commit: %w", err)
				goto done
			}
			outputLines = append(outputLines, fmt.Sprintf("commit %q", msg))

		default:
			execErr = fmt.Errorf("script harness: unknown directive %q at index %d", verb, i)
			goto done
		}
	}

done:
	elapsed := time.Since(start)
	sessionID := fmt.Sprintf("script-%d", start.UnixNano())

	result := &Result{
		Harness:        "script",
		Provider:       "script",
		RouteReason:    "direct-override",
		Model:          directivePath,
		Output:         strings.Join(outputLines, "\n"),
		ExitCode:       exitCode,
		DurationMS:     int(elapsed.Milliseconds()),
		AgentSessionID: sessionID,
	}
	if execErr != nil {
		result.Error = execErr.Error()
		if result.ExitCode == 0 {
			result.ExitCode = 1
		}
	}
	return result, execErr
}

// parseDirectives parses a directive file into a slice of token slices.
// Lines starting with # are comments. Empty lines are skipped.
// Tokens are whitespace-separated; a quoted string in position 2+ is kept together.
func parseDirectives(content string) [][]string {
	var out [][]string
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		tokens := splitDirective(line)
		if len(tokens) == 0 {
			continue
		}
		out = append(out, tokens)
	}
	return out
}

// splitDirective splits a directive line into tokens.
// Returns [verb, arg1, arg2, ...] where the final argument absorbs any
// remaining whitespace-separated words as a single string. Surrounding
// double-quotes on the final argument are stripped.
//
// For directives that need 3 distinct args (modify-line: path regex repl)
// we return verb + 3 separate fields: parts[0..2], with parts[3:] joined
// onto the last field.
//
// The heuristic: each case in RunScript knows how many discrete args it
// needs and accesses them by index; the split just returns all fields, and
// the caller joins args[N:] for the "rest" argument.
func splitDirective(line string) []string {
	// We need to preserve the raw fields so callers can pick the right number.
	// Return all whitespace-split tokens; the final quoted token has quotes stripped.
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}
	// Strip quotes from the last token if the entire last token is quoted.
	last := parts[len(parts)-1]
	if len(last) >= 2 && last[0] == '"' && last[len(last)-1] == '"' {
		parts[len(parts)-1] = last[1 : len(last)-1]
	}
	return parts
}

// resolvePath resolves a relative path against workDir and rejects absolute paths.
func resolvePath(workDir, path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("script harness: absolute path rejected: %s", path)
	}
	return filepath.Join(workDir, path), nil
}

// gitCommitAll runs git add -A && git commit -m msg in dir with scrubbed GIT_* env.
func gitCommitAll(dir, msg string) error {
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-m", msg},
	} {
		cmd := internalgit.Command(context.Background(), dir, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %w\n%s", args, err, out)
		}
	}
	return nil
}

// scrubbedGitEnvScript returns the environment with all GIT_* variables removed.
func scrubbedGitEnvScript() []string {
	parent := os.Environ()
	env := make([]string, 0, len(parent))
	for _, kv := range parent {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// isReadableFile returns true if path points to a regular, readable file.
func isReadableFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

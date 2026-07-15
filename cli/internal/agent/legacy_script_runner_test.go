package agent

// The directive interpreter is a test fixture, not a production harness.
// Production script/virtual passthrough values remain opaque and execute only
// through Fizeau.

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

func runScriptFn(opts RunArgs) (*Result, error) {
	start := time.Now()
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

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
	envMap := map[string]string{
		"DDX_BEAD_ID":         os.Getenv("DDX_BEAD_ID"),
		"DDX_ATTEMPT_ID":      os.Getenv("DDX_ATTEMPT_ID"),
		"DDX_WORKER_ID":       os.Getenv("DDX_WORKER_ID"),
		"DDX_SESSION_LOG_DIR": os.Getenv("DDX_SESSION_LOG_DIR"),
	}
	for key, value := range opts.Env {
		if strings.HasPrefix(key, "DDX_") {
			envMap[key] = value
		}
	}
	if opts.SessionLogDir != "" {
		envMap["DDX_SESSION_LOG_DIR"] = opts.SessionLogDir
	}
	if opts.Correlation != nil {
		if value, ok := opts.Correlation["bead_id"]; ok {
			envMap["DDX_BEAD_ID"] = value
		}
		if value, ok := opts.Correlation["attempt_id"]; ok {
			envMap["DDX_ATTEMPT_ID"] = value
		}
		if value, ok := opts.Correlation["worker_id"]; ok {
			envMap["DDX_WORKER_ID"] = value
		}
	}
	expand := func(value string) string {
		return os.Expand(value, func(key string) string { return envMap[key] })
	}

	for index, directive := range directives {
		if err := ctx.Err(); err != nil {
			execErr = err
			goto done
		}
		verb := directive[0]
		args := directive[1:]
		switch verb {
		case "no-op":
			outputLines = append(outputLines, "no-op")
		case "set-exit":
			if len(args) < 1 {
				execErr = fmt.Errorf("script harness: set-exit requires an argument")
				goto done
			}
			code, err := strconv.Atoi(args[0])
			if err != nil {
				execErr = fmt.Errorf("script harness: set-exit: invalid code %q", args[0])
				goto done
			}
			exitCode = code
		case "sleep-ms":
			if len(args) < 1 {
				execErr = fmt.Errorf("script harness: sleep-ms requires an argument")
				goto done
			}
			delay, err := strconv.Atoi(args[0])
			if err != nil {
				execErr = fmt.Errorf("script harness: sleep-ms: invalid duration %q", args[0])
				goto done
			}
			timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
			select {
			case <-timer.C:
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				execErr = ctx.Err()
				goto done
			}
		case "fail-during":
			if len(args) < 1 {
				execErr = fmt.Errorf("script harness: fail-during requires an argument")
				goto done
			}
			failureIndex, err := strconv.Atoi(args[0])
			if err != nil {
				execErr = fmt.Errorf("script harness: fail-during: invalid index %q", args[0])
				goto done
			}
			if index == failureIndex {
				execErr = fmt.Errorf("script harness: synthetic failure at directive %d", index)
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
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				execErr = fmt.Errorf("script harness: append-line mkdir: %w", err)
				goto done
			}
			file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				execErr = fmt.Errorf("script harness: append-line open: %w", err)
				goto done
			}
			_, writeErr := fmt.Fprintln(file, text)
			_ = file.Close()
			if writeErr != nil {
				execErr = fmt.Errorf("script harness: append-line write: %w", writeErr)
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
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				execErr = fmt.Errorf("script harness: create-file mkdir: %w", err)
				goto done
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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
			replacement := expand(strings.Join(args[2:], " "))
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
			for lineIndex, line := range lines {
				if re.MatchString(line) {
					lines[lineIndex] = re.ReplaceAllString(line, replacement)
					break
				}
			}
			if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
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
			cmd := exec.CommandContext(ctx, "sh", "-c", shell)
			cmd.Dir = workDir
			cmd.Env = envWithOverrides(scrubbedGitEnvScript(), opts.Env)
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
			message := expand(strings.Join(args, " "))
			if err := gitCommitAll(workDir, message, opts.Env); err != nil {
				execErr = fmt.Errorf("script harness: commit: %w", err)
				goto done
			}
			outputLines = append(outputLines, fmt.Sprintf("commit %q", message))
		default:
			execErr = fmt.Errorf("script harness: unknown directive %q at index %d", verb, index)
			goto done
		}
	}

done:
	elapsed := time.Since(start)
	result := &Result{
		Harness: "script", Provider: "script", RouteReason: "direct-override",
		Model: directivePath, Output: strings.Join(outputLines, "\n"), ExitCode: exitCode,
		DurationMS: int(elapsed.Milliseconds()), AgentSessionID: fmt.Sprintf("script-%d", start.UnixNano()),
	}
	if execErr != nil {
		result.Error = execErr.Error()
		if result.ExitCode == 0 {
			result.ExitCode = 1
		}
	}
	return result, execErr
}

func parseDirectives(content string) [][]string {
	var directives [][]string
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if tokens := splitDirective(line); len(tokens) > 0 {
			directives = append(directives, tokens)
		}
	}
	return directives
}

func splitDirective(line string) []string {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}
	last := parts[len(parts)-1]
	if len(last) >= 2 && last[0] == '"' && last[len(last)-1] == '"' {
		parts[len(parts)-1] = last[1 : len(last)-1]
	}
	return parts
}

func resolvePath(workDir, path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("script harness: absolute path rejected: %s", path)
	}
	return filepath.Join(workDir, path), nil
}

func gitCommitAll(dir, message string, extraEnv map[string]string) error {
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", message}} {
		cmd := internalgit.Command(context.Background(), dir, args...)
		cmd.Env = envWithOverrides(cmd.Env, extraEnv)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %w\n%s", args, err, out)
		}
	}
	return nil
}

func scrubbedGitEnvScript() []string {
	return internalgit.CleanEnv()
}

func isReadableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

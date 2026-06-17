package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	urlpkg "net/url"
	"strings"
	"syscall"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/spf13/cobra"
)

var (
	workerServerReadinessRetryBudget = 30 * time.Second
	workerServerReadinessRetryDelay  = 200 * time.Millisecond
)

func (f *CommandFactory) newWorkerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Manage server-supervised workers",
		Long:  "Commands for managing server-supervised workers and their desired state.",
	}
	cmd.AddCommand(f.newWorkerStatusCommand())
	cmd.AddCommand(f.newWorkerSetCommand())
	cmd.AddCommand(f.newWorkerStartCommand())
	cmd.AddCommand(f.newWorkerStopCommand())
	cmd.AddCommand(f.newWorkerRestartCommand())
	cmd.AddCommand(f.newWorkerReconcileCommand())
	cmd.AddCommand(f.newWorkerCleanupCommand())
	return cmd
}

func (f *CommandFactory) newWorkerStatusCommand() *cobra.Command {
	var asJSON bool
	var project string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show worker status",
		RunE: func(cmd *cobra.Command, args []string) error {
			base := resolveServerURL(f.WorkingDir)
			req, err := http.NewRequest(http.MethodGet, base+"/api/agent/workers", nil)
			if err != nil {
				return err
			}
			resp, err := doWorkerServerRequest(req)
			if err != nil {
				return fmt.Errorf("server request: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("reading response: %w", err)
			}
			var workers []workerRecord
			if err := json.Unmarshal(body, &workers); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			filtered := workers
			if project != "" {
				filtered = filtered[:0]
				for _, w := range workers {
					if workerstatus.SamePath(w.ProjectRoot, project) {
						filtered = append(filtered, w)
					}
				}
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(filtered)
			}
			if len(filtered) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No workers.")
				return nil
			}
			for _, w := range filtered {
				age := formatDuration(time.Since(w.StartedAt))
				fmt.Fprintf(cmd.OutOrStdout(), "%-36s %-6s %-12s %s\n", w.ID, age, w.State, w.ProjectRoot)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().StringVar(&project, "project", "", "Filter by project path")
	return cmd
}

func (f *CommandFactory) newWorkerSetCommand() *cobra.Command {
	var project string
	var count int
	var restartEnabled bool
	var noRestart bool
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set desired worker count and restart policy",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot := project
			if projectRoot == "" {
				projectRoot = f.WorkingDir
			}
			restart := restartEnabled && !noRestart
			base := resolveServerURL(f.WorkingDir)
			body, err := json.Marshal(map[string]interface{}{
				"project_root":    projectRoot,
				"desired_count":   count,
				"restart_enabled": restart,
			})
			if err != nil {
				return err
			}
			req, err := http.NewRequest(http.MethodPut, base+"/api/agent/workers/desired", bytes.NewReader(body))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := doWorkerServerRequest(req)
			if err != nil {
				return fmt.Errorf("server request: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			out, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server error %d: %s", resp.StatusCode, string(out))
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			reconcileOut, err := requestWorkerReconcile(base, projectRoot)
			if err != nil {
				return err
			}
			if !emptyWorkerReconcileOutput(reconcileOut) {
				fmt.Fprintln(cmd.OutOrStdout(), string(reconcileOut))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project path (defaults to current working dir)")
	cmd.Flags().IntVar(&count, "count", 1, "Desired worker count")
	cmd.Flags().BoolVar(&restartEnabled, "restart", true, "Enable automatic restart on exit")
	cmd.Flags().BoolVar(&noRestart, "no-restart", false, "Disable automatic restart on exit")
	return cmd
}

func (f *CommandFactory) newWorkerStartCommand() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a worker (sets desired count to 1)",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot := project
			if projectRoot == "" {
				projectRoot = f.WorkingDir
			}
			base := resolveServerURL(f.WorkingDir)
			body, err := json.Marshal(map[string]interface{}{
				"project_root":    projectRoot,
				"desired_count":   1,
				"restart_enabled": true,
			})
			if err != nil {
				return err
			}
			req, err := http.NewRequest(http.MethodPut, base+"/api/agent/workers/desired", bytes.NewReader(body))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := doWorkerServerRequest(req)
			if err != nil {
				return fmt.Errorf("server request: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			out, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server error %d: %s", resp.StatusCode, string(out))
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			reconcileOut, err := requestWorkerReconcile(base, projectRoot)
			if err != nil {
				return err
			}
			if !emptyWorkerReconcileOutput(reconcileOut) {
				fmt.Fprintln(cmd.OutOrStdout(), string(reconcileOut))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project path (defaults to current working dir)")
	return cmd
}

func (f *CommandFactory) newWorkerStopCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <worker-id>",
		Short: "Stop a worker",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			base := resolveServerURL(f.WorkingDir)
			req, err := http.NewRequest(http.MethodPost, base+"/api/agent/workers/"+args[0]+"/stop", nil)
			if err != nil {
				return err
			}
			resp, err := doWorkerServerRequest(req)
			if err != nil {
				return fmt.Errorf("server request: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			out, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
				return fmt.Errorf("server error %d: %s", resp.StatusCode, string(out))
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
	return cmd
}

func (f *CommandFactory) newWorkerRestartCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart <worker-id>",
		Short: "Restart a worker",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			base := resolveServerURL(f.WorkingDir)
			req, err := http.NewRequest(http.MethodPost, base+"/api/agent/workers/"+args[0]+"/restart", nil)
			if err != nil {
				return err
			}
			resp, err := doWorkerServerRequest(req)
			if err != nil {
				return fmt.Errorf("server request: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			out, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server error %d: %s", resp.StatusCode, string(out))
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
	return cmd
}

func requestWorkerReconcile(base, projectRoot string) ([]byte, error) {
	reconcileURL := base + "/api/agent/workers/reconcile"
	if projectRoot != "" {
		reconcileURL += "?project=" + urlpkg.QueryEscape(projectRoot)
	}
	req, err := http.NewRequest(http.MethodPost, reconcileURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := doWorkerServerRequest(req)
	if err != nil {
		return nil, fmt.Errorf("server request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server error %d: %s", resp.StatusCode, string(out))
	}
	return out, nil
}

func emptyWorkerReconcileOutput(out []byte) bool {
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("{}")) {
		return true
	}
	var object map[string]any
	if err := json.Unmarshal(trimmed, &object); err == nil && len(object) == 0 {
		return true
	}
	return false
}

func (f *CommandFactory) newWorkerReconcileCommand() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Reconcile workers to desired state",
		RunE: func(cmd *cobra.Command, args []string) error {
			base := resolveServerURL(f.WorkingDir)
			url := base + "/api/agent/workers/reconcile"
			if project != "" {
				url += "?project=" + urlpkg.QueryEscape(project)
			}
			req, err := http.NewRequest(http.MethodPost, url, nil)
			if err != nil {
				return err
			}
			resp, err := doWorkerServerRequest(req)
			if err != nil {
				return fmt.Errorf("server request: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			out, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server error %d: %s", resp.StatusCode, string(out))
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project path")
	return cmd
}

func (f *CommandFactory) newWorkerCleanupCommand() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Clean up stale workers",
		RunE: func(cmd *cobra.Command, args []string) error {
			base := resolveServerURL(f.WorkingDir)
			url := base + "/api/agent/workers/cleanup"
			if project != "" {
				url += "?project=" + project
			}
			req, err := http.NewRequest(http.MethodPost, url, nil)
			if err != nil {
				return err
			}
			resp, err := doWorkerServerRequest(req)
			if err != nil {
				return fmt.Errorf("server request: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()
			out, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server error %d: %s", resp.StatusCode, string(out))
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project path")
	return cmd
}

func doWorkerServerRequest(req *http.Request) (*http.Response, error) {
	client := newLocalServerClient()
	resp, err := client.Do(req)
	if err == nil || !shouldRetryWorkerServerReadiness(req, err) {
		return resp, err
	}
	if workerServerReadinessRetryBudget <= 0 {
		return nil, err
	}

	deadline := time.Now().Add(workerServerReadinessRetryBudget)
	lastErr := err
	for time.Now().Before(deadline) {
		time.Sleep(workerServerReadinessRetryDelay)
		retryReq, cloneErr := cloneRequestForRetry(req)
		if cloneErr != nil {
			return nil, cloneErr
		}
		resp, err = client.Do(retryReq)
		if err == nil {
			return resp, nil
		}
		if !shouldRetryWorkerServerReadiness(retryReq, err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("local server readiness timeout after %s: %w", workerServerReadinessRetryBudget, lastErr)
}

func cloneRequestForRetry(req *http.Request) (*http.Request, error) {
	retryReq := req.Clone(req.Context())
	if req.Body != nil {
		if req.GetBody == nil {
			return nil, fmt.Errorf("cannot retry %s %s: request body is not replayable", req.Method, req.URL.Redacted())
		}
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("replay request body: %w", err)
		}
		retryReq.Body = body
	}
	return retryReq, nil
}

func shouldRetryWorkerServerReadiness(req *http.Request, err error) bool {
	return req != nil && req.URL != nil && isLocalWorkerServerURL(req.URL) && isConnectionRefused(err)
}

func isLocalWorkerServerURL(u *urlpkg.URL) bool {
	host := u.Hostname()
	if host == "" {
		host = u.Host
		if splitHost, _, err := net.SplitHostPort(host); err == nil {
			host = splitHost
		}
	}
	host = strings.Trim(host, "[]")
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func isConnectionRefused(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	if strings.Contains(strings.ToLower(err.Error()), "connection refused") {
		return true
	}
	var urlErr *urlpkg.Error
	if errors.As(err, &urlErr) {
		return isConnectionRefused(urlErr.Err)
	}
	return false
}

//go:build windows

package cmd

import (
	"os"
	"os/exec"
)

// providerLaunchPrepare is a no-op on Windows. PR_SET_PDEATHSIG has no
// equivalent and Windows job objects are not exercised by this shim;
// see executor_windows.go for the cmdSetProcessGroup fallback used by
// the direct executor path.
func providerLaunchPrepare() error { return nil }

// providerLaunchExec runs the resolved provider binary as a normal
// child and proxies its exit code. Windows lacks an execve equivalent
// so we wait on the child and exit with its status.
func providerLaunchExec(binary string, args []string) error {
	cmd := exec.Command(binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}

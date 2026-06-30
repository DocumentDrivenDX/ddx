//go:build !windows

package server

import "syscall"

func newManagedWorkerSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

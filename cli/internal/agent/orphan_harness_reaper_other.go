//go:build !linux

package agent

import "context"

type procOrphanHarnessScanner struct{}

func newOrphanHarnessProcessScanner() orphanHarnessProcessScanner {
	return procOrphanHarnessScanner{}
}

func (procOrphanHarnessScanner) Scan(context.Context) ([]orphanHarnessProcess, error) {
	return nil, nil
}

func killProcessGroup(int) error { return nil }

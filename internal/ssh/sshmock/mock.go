// Package sshmock provides an in-memory ssh.Executor for tests. It is
// intentionally in its own package so the production binary does not
// link the mock.
package sshmock

import (
	"context"
	"io/fs"
)

// Executor is a deterministic in-memory ssh.Executor.
type Executor struct {
	// Files is the fake remote filesystem (absolute path → content).
	Files map[string][]byte
	// Modes tracks chmod calls (absolute path → mode).
	Modes map[string]fs.FileMode
	// RunFunc handles Run(). If nil all commands succeed with empty
	// output. Test cases override this to simulate xray -test etc.
	RunFunc func(ctx context.Context, cmd string) (stdout, stderr []byte, err error)
	// Commands records every Run invocation in order.
	Commands []string
}

// New returns a fresh mock Executor.
func New() *Executor {
	return &Executor{Files: map[string][]byte{}, Modes: map[string]fs.FileMode{}}
}

// Run implements ssh.Executor.
func (m *Executor) Run(ctx context.Context, cmd string) ([]byte, []byte, error) {
	m.Commands = append(m.Commands, cmd)
	if m.RunFunc != nil {
		return m.RunFunc(ctx, cmd)
	}
	return nil, nil, nil
}

// Upload implements ssh.Executor.
func (m *Executor) Upload(_ context.Context, path string, data []byte, mode fs.FileMode) error {
	m.Files[path] = append([]byte(nil), data...)
	m.Modes[path] = mode
	return nil
}

// Download implements ssh.Executor.
func (m *Executor) Download(_ context.Context, path string) ([]byte, error) {
	data, ok := m.Files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

// MkdirAll implements ssh.Executor (no-op).
func (m *Executor) MkdirAll(_ context.Context, _ string, _ fs.FileMode) error { return nil }

// Close implements ssh.Executor.
func (m *Executor) Close() error { return nil }

// Package ssh provides a minimal SSH/SFTP client for wrtbox.
//
// The Executor interface is the seam used by internal/apply,
// internal/diff and internal/rollback. Tests substitute a fake;
// production wiring uses the Client implementation in client.go.
package ssh

import (
	"context"
	"io/fs"
)

// Executor is the narrow surface the higher-level apply/diff/rollback
// pipelines need. Any implementation must be safe for sequential use
// by one goroutine (the pipelines never go parallel over a single
// connection).
type Executor interface {
	// Run executes a shell command on the remote and returns its
	// combined stdout and stderr along with the exit error (if any).
	// Implementations MUST respect ctx cancellation.
	Run(ctx context.Context, cmd string) (stdout, stderr []byte, err error)

	// Upload writes data to path, creating parent directories as
	// needed and setting the POSIX mode. Atomic write (tmp + rename)
	// is the caller's concern — the pipeline stages uploads into a
	// separate directory and swaps at the end.
	Upload(ctx context.Context, path string, data []byte, mode fs.FileMode) error

	// Download reads and returns the contents of path.
	Download(ctx context.Context, path string) ([]byte, error)

	// MkdirAll behaves like os.MkdirAll but on the remote.
	MkdirAll(ctx context.Context, path string, mode fs.FileMode) error

	// Close releases all resources (session, sftp subsystem,
	// underlying TCP). Safe to call multiple times.
	Close() error
}

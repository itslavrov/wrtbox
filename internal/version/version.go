// Package version exposes build metadata stamped in by ldflags.
package version

// These variables are populated at build time via -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

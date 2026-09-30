// Package version holds build-time version variables injected via -ldflags.
package version

// Variables are overwritten at build time by the Makefile via:
//
//	-X github.com/hasangenc0/bordo/control-plane/internal/version.Version=$(VERSION)
//	-X github.com/hasangenc0/bordo/control-plane/internal/version.Commit=$(COMMIT)
//	-X github.com/hasangenc0/bordo/control-plane/internal/version.BuildTime=$(BTIME)
var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

// String returns a human-readable version string.
func String() string {
	return "bordod version " + Version + " (commit " + Commit + ", built " + BuildTime + ")"
}
